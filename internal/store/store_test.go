package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/llm"
)

// 第一次启动时什么文件都没有 —— 这不是错误，得拿到一份空配置。
func TestOpenOnEmptyDir(t *testing.T) {
	s := Open(t.TempDir())
	if got := s.Config(); len(got.Providers) != 0 || got.Active != 0 {
		t.Errorf("空目录应该给出零值配置，got %+v", got)
	}
	if s.Provider() != nil {
		t.Error("没配置供应商时应该返回 nil，好走环境变量的默认凭证链")
	}
	if st := s.State(); len(st.Viewed) != 0 || st.LastRepo != "" {
		t.Errorf("空目录应该给出空状态，got %+v", st)
	}
}

// 存进去的东西要能读回来。
func TestSaveAndReopen(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	s.SetConfig(Config{
		Providers: []llm.Provider{{Name: "本地", Type: "openai", BaseURL: "http://x", Model: "m"}},
		Active:    0,
		CodeSize:  14,
		Split:     true,
		// 拓扑图配色与终端字体：三份切片/字符串都得原样回来，
		// 空位（"" 那一格）尤其不能丢 —— 丢了就变成「第 5 号」跑到第 0 号去。
		GraphPreset:     "vivid",
		GraphLanesDark:  []string{"", "#ff00ff", "", "", "", "", "", "#00ff00"},
		GraphLanesLight: []string{"#0000ff"},
		TermFontFamily:  "JetBrains Mono, Menlo",
		TermFontSize:    16,
	})
	s.SetViewed(map[string]string{"a.go": "fp1"})
	s.SetLastRepo("/tmp/repo")
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	again := Open(dir)
	if p := again.Provider(); p == nil || p.Name != "本地" || p.Model != "m" {
		t.Errorf("供应商没读回来：%+v", p)
	}
	if got := again.Config(); got.CodeSize != 14 || !got.Split {
		t.Errorf("设置没读回来：%+v", got)
	}
	cfg := again.Config()
	if cfg.GraphPreset != "vivid" || cfg.TermFontFamily != "JetBrains Mono, Menlo" || cfg.TermFontSize != 16 {
		t.Errorf("配色方案与终端字体没读回来：%+v", cfg)
	}
	if len(cfg.GraphLanesDark) != 8 || cfg.GraphLanesDark[1] != "#ff00ff" || cfg.GraphLanesDark[7] != "#00ff00" {
		t.Errorf("深色覆盖没读回来：%q", cfg.GraphLanesDark)
	}
	if len(cfg.GraphLanesLight) != 1 || cfg.GraphLanesLight[0] != "#0000ff" {
		t.Errorf("浅色覆盖没读回来：%q", cfg.GraphLanesLight)
	}
	if got := again.State(); got.Viewed["a.go"] != "fp1" || got.LastRepo != "/tmp/repo" {
		t.Errorf("状态没读回来：%+v", got)
	}
}

// config.json 里有 API key，权限必须是 0600。
func TestConfigFileIsPrivate(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	s.SetConfig(Config{Providers: []llm.Provider{{Name: "x", APIKey: "sk-secret"}}})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	fi, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("config.json 权限 = %o, want 600（里面有 API key）", perm)
	}
}

// 配置坏了不能让应用起不来 —— 顶多是这次设置没了。
func TestCorruptFilesAreIgnored(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"config.json", "state.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{ 不是 JSON"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	s := Open(dir)
	if s.Provider() != nil || s.State().LastRepo != "" {
		t.Error("坏文件应该当作空")
	}
	// 而且还能正常写回去
	s.SetLastRepo("/x")
	if err := s.Save(); err != nil {
		t.Fatalf("坏文件之后 Save 失败：%v", err)
	}
	if got := Open(dir).State().LastRepo; got != "/x" {
		t.Errorf("重写后读回 LastRepo = %q, want /x", got)
	}
}

// 存的文件得是能读的 JSON，而不是 Go 的默认编码。
func TestSavedFilesAreValidJSON(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)
	s.SetViewed(map[string]string{"带中文的.go": "fp"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	for _, name := range []string{"config.json", "state.json"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("读 %s: %v", name, err)
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			t.Errorf("%s 不是合法 JSON: %v", name, err)
		}
		if !strings.HasSuffix(string(b), "\n") {
			t.Errorf("%s 结尾少了换行", name)
		}
	}
}

// Active 越界时退回第一个，不能让界面拿到一个不存在的供应商。
func TestActiveOutOfRangeFallsBack(t *testing.T) {
	c := Config{Providers: []llm.Provider{{Name: "first"}, {Name: "second"}}, Active: 9}
	if p := c.Provider(); p == nil || p.Name != "first" {
		t.Errorf("Active 越界应退回第一个，got %+v", p)
	}
}

// State() 返回的 map 要是副本：调用方改它不该影响 store。
func TestStateReturnsCopy(t *testing.T) {
	s := Open(t.TempDir())
	s.SetViewed(map[string]string{"a.go": "fp"})
	st := s.State()
	st.Viewed["a.go"] = "改了"
	st.Viewed["b.go"] = "新加的"

	again := s.State()
	if again.Viewed["a.go"] != "fp" || len(again.Viewed) != 1 {
		t.Errorf("State() 返回的不是副本：%+v", again.Viewed)
	}
}

func TestRecentReposPersistence(t *testing.T) {
	dir := t.TempDir()
	s := Open(dir)

	s.AddRecentRepo("/repo/a")
	s.AddRecentRepo("/repo/b")
	s.AddRecentRepo("/repo/a") // 重复打开，应移到最前

	st := s.State()
	if st.LastRepo != "/repo/a" {
		t.Errorf("LastRepo = %q, want /repo/a", st.LastRepo)
	}
	if len(st.RecentRepos) != 2 || st.RecentRepos[0] != "/repo/a" || st.RecentRepos[1] != "/repo/b" {
		t.Errorf("RecentRepos = %v, want [/repo/a, /repo/b]", st.RecentRepos)
	}

	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reopened := Open(dir)
	rSt := reopened.State()
	if rSt.LastRepo != "/repo/a" {
		t.Errorf("重载后 LastRepo = %q, want /repo/a", rSt.LastRepo)
	}
	if len(rSt.RecentRepos) != 2 || rSt.RecentRepos[0] != "/repo/a" || rSt.RecentRepos[1] != "/repo/b" {
		t.Errorf("重载后 RecentRepos = %v, want [/repo/a, /repo/b]", rSt.RecentRepos)
	}

	reopened.ClearRecentRepos()
	if len(reopened.State().RecentRepos) != 0 {
		t.Errorf("清空后 RecentRepos 该为空")
	}
}

// Package store 把用户设置和界面状态落到磁盘。
//
// 目录由调用方给（桌面包传 mygo 的用户数据目录），所以这个包不依赖 mygo，
// 测试里给个 t.TempDir() 就能完整跑。
//
// 三条约定：
//   - 读文件失败（第一次启动没有文件、JSON 损坏）一律当作空，绝不因此
//     让应用起不来 —— 配置文件坏了可以重建，应用打不开就全完了。
//   - 写入先落临时文件再 rename：中途崩掉也不会留下半个 JSON。
//   - 有 API key，config.json 用 0600。
package store

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/CarlOpenLab/Gloss/internal/llm"
)

// Config 是用户设置。
type Config struct {
	Providers []llm.Provider `json:"providers,omitempty"`
	// Active 是当前使用的供应商下标。
	Active int `json:"active,omitempty"`
	// CodeSize 是代码字号，0 表示用默认值。
	CodeSize float32 `json:"codeSize,omitempty"`
	// Split 为 true 时用左右对照视图。
	Split bool `json:"split,omitempty"`
	// Unified 为 true 时用单栏统一视图（默认左右对照）。
	Unified bool `json:"unified,omitempty"`
	// SidebarOff 而不是 SidebarOn：侧栏默认是开的，零值正好表示这个默认。
	SidebarOff bool `json:"sidebarOff,omitempty"`
	// SidebarWidth 是侧栏宽度，0 表示默认值。
	SidebarWidth float32 `json:"sidebarWidth,omitempty"`
	// FilePaneWidth 是主区文件列表栏宽度，0 表示默认值。
	FilePaneWidth float32 `json:"filePaneWidth,omitempty"`
	// ThemeMode 是外观模式偏好："" 或 "system"（跟随系统）、"light"（浅色模式）、"dark"（暗黑模式）。
	ThemeMode string `json:"themeMode,omitempty"`
	// Locale 是界面语言设定："" 或 "zh"（简体中文）、"en"（English）。
	Locale string `json:"locale,omitempty"`
	// AppIcon 是应用图标风格："black"（纯黑曜石）或 "white"（纯白极简）。
	AppIcon string `json:"appIcon,omitempty"`
	// CommitPrompt 是自定义 Git 提交信息提示词，留空使用内置预设。
	CommitPrompt string `json:"commitPrompt,omitempty"`
	// BranchPrompt 是自定义 Git 分支名生成提示词，留空使用内置预设。
	BranchPrompt string `json:"branchPrompt,omitempty"`
}

// EffectiveCommitPrompt 返回生效的 Git 提交信息生成系统提示词。
func (c Config) EffectiveCommitPrompt(preset string) string {
	if s := strings.TrimSpace(c.CommitPrompt); s != "" {
		return s
	}
	return preset
}

// EffectiveBranchPrompt 返回生效的 Git 分支名生成系统提示词。
func (c Config) EffectiveBranchPrompt(preset string) string {
	if s := strings.TrimSpace(c.BranchPrompt); s != "" {
		return s
	}
	return preset
}

// Provider 是当前选中的供应商配置。没有配置任何供应商时返回 nil，
// 此时 StreamExplanation 会走 Anthropic 的环境变量凭证链。
func (c Config) Provider() *llm.Provider {
	// Active < 0 表示用户把当前生效的供应商关掉了（设置里那个开关可开可
	// 关）：和没有任何供应商一样，都算「没有生效的供应商」，走环境变量
	// 凭证链。大于等于列表长度的旧配置钳回第 0 个。
	if len(c.Providers) == 0 || c.Active < 0 {
		return nil
	}
	i := c.Active
	if i >= len(c.Providers) {
		i = 0
	}
	p := c.Providers[i]
	return &p
}

// State 是跟着仓库走的界面状态。
type State struct {
	LastRepo    string   `json:"lastRepo,omitempty"`
	RecentRepos []string `json:"recentRepos,omitempty"`
	OpenRepos   []string `json:"openRepos,omitempty"`
	// Viewed 是「已 review」标记：文件路径 -> 当时的内容指纹。
	Viewed map[string]string `json:"viewed,omitempty"`
	// Drafts 是各仓库的提交信息草稿：仓库根路径 -> 还没提交出去的消息。
	// 消息写到一半切去看别的仓库、或者关掉窗口，回来还应该在。
	Drafts map[string]string `json:"drafts,omitempty"`
}

// Store 是两份文件（设置、状态）的门面，可被多个 goroutine 同时使用。
type Store struct {
	dir string

	mu  sync.Mutex
	cfg Config
	st  State
}

// Open 从 dir 读取已有的设置和状态。文件不存在或读不动都当作空，
// 返回值不会是 nil。dir 为空表示「没有数据目录」：读写都变成空操作，
// 应用照常能跑，只是这次的改动不会被保存。
func Open(dir string) *Store {
	s := &Store{dir: dir}
	if dir == "" {
		return s
	}
	readJSON(filepath.Join(dir, "config.json"), &s.cfg)
	readJSON(filepath.Join(dir, "state.json"), &s.st)
	return s
}

// Dir 是这个 store 的数据目录。
func (s *Store) Dir() string { return s.dir }

// Config 返回设置的副本。
func (s *Store) Config() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

// SetConfig 替换设置。
func (s *Store) SetConfig(c Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg = c
}

// Provider 是当前选中的供应商，没配置就是 nil（走环境变量凭证链）。
func (s *Store) Provider() *llm.Provider {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Provider()
}

// State 返回状态，Viewed 和 Drafts 都是新的一份 map，改它们不会影响 store。
func (s *Store) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := s.st
	cp.Viewed = make(map[string]string, len(s.st.Viewed))
	for k, v := range s.st.Viewed {
		cp.Viewed[k] = v
	}
	cp.Drafts = make(map[string]string, len(s.st.Drafts))
	for k, v := range s.st.Drafts {
		cp.Drafts[k] = v
	}
	return cp
}

// SetViewed 替换「已 review」标记。
func (s *Store) SetViewed(m map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.Viewed = make(map[string]string, len(m))
	for k, v := range m {
		s.st.Viewed[k] = v
	}
}

// SetDraft 记下某个仓库的提交信息草稿。msg 为空就把这条删掉 ——
// 空草稿留在文件里没有任何用处。
func (s *Store) SetDraft(repo, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st.Drafts == nil {
		s.st.Drafts = map[string]string{}
	}
	if msg == "" {
		delete(s.st.Drafts, repo)
		return
	}
	s.st.Drafts[repo] = msg
}

const maxRecentRepos = 10

// AddRecentRepo 将 path 记为最近打开的仓库（最新的在最前），并更新 LastRepo。
func (s *Store) AddRecentRepo(path string) {
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.LastRepo = path
	var updated []string
	updated = append(updated, path)
	for _, p := range s.st.RecentRepos {
		if p != path && p != "" {
			updated = append(updated, p)
		}
		if len(updated) >= maxRecentRepos {
			break
		}
	}
	s.st.RecentRepos = updated
}

// ClearRecentRepos 清空最近打开的仓库列表。
func (s *Store) ClearRecentRepos() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.RecentRepos = nil
}

// SetLastRepo 记住最后打开的仓库，下次启动直接打开。
func (s *Store) SetLastRepo(path string) {
	if path == "" {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.st.LastRepo = ""
		return
	}
	s.AddRecentRepo(path)
}

// SetOpenRepos 记住当前打开的多个项目仓库路径列表。
func (s *Store) SetOpenRepos(paths []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.st.OpenRepos = slicesClone(paths)
}

func slicesClone(s []string) []string {
	if s == nil {
		return nil
	}
	c := make([]string, len(s))
	copy(c, s)
	return c
}

// Save 把两份文件写回磁盘。
//
// 用法是「改完就存」：文件都很小，写一次在毫秒级，不值得为它做脏标记或者
// 防抖。整段加锁，所以并发调用是安全的（后一次写覆盖前一次，结果一致）。
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dir == "" {
		return nil // 没有数据目录，没什么可存的
	}

	st := s.st
	if st.Viewed == nil {
		st.Viewed = map[string]string{}
	}

	if err := writeJSON(filepath.Join(s.dir, "config.json"), s.cfg, 0o600); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.dir, "state.json"), st, 0o644)
}

// readJSON 读一个 JSON 文件。任何失败都只是「没有这份配置」，不是错误。
func readJSON(path string, v any) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(b, v)
}

// writeJSON 原子地写一个 JSON 文件：先写同目录下的临时文件，再 rename 过去。
// 直接截断原文件的话，写到一半断电就只剩半截 JSON 了。
func writeJSON(path string, v any, perm fs.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // rename 成功后这里会失败，无害

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, perm); err != nil {
		return err
	}
	return os.Rename(name, path)
}

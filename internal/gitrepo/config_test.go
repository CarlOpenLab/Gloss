package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// configTestRepo 造一个仓库，并把三层配置指到临时文件上。
//
// 必须这么做：这组用例会**写全局配置**，而包里的 git 调用走 os.Environ
// （见 gitEnv），不指走的话就在改跑测试这台机器的 ~/.gitconfig。
// 系统级同理 —— macOS 上 /Applications/Xcode.app/…/gitconfig 也在里面。
func configTestRepo(t *testing.T) (dir, globalPath string) {
	t.Helper()
	dir = t.TempDir()
	globalPath = filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", globalPath)
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(t.TempDir(), "gitconfig"))
	runGitT(t, dir, "init", "-q")
	return dir, globalPath
}

func entryOf(entries []ConfigEntry, scope, key string) (ConfigEntry, bool) {
	for _, e := range entries {
		if e.Scope == scope && e.Key == key {
			return e, true
		}
	}
	return ConfigEntry{}, false
}

// 一次读回三层：层级和来源文件都得对上，界面靠这两样分栏和标「文件」。
func TestReadConfigTagsScopeAndFile(t *testing.T) {
	dir, globalPath := configTestRepo(t)
	ctx := context.Background()

	if err := WriteConfig(ctx, dir, ScopeGlobal, "user.name", "Glob Al"); err != nil {
		t.Fatalf("写全局配置: %v", err)
	}
	if err := WriteConfig(ctx, dir, ScopeLocal, "user.email", "local@example.com"); err != nil {
		t.Fatalf("写项目配置: %v", err)
	}

	entries, err := ReadConfig(ctx, dir)
	if err != nil {
		t.Fatalf("读配置: %v", err)
	}

	global, ok := entryOf(entries, ScopeGlobal, "user.name")
	if !ok {
		t.Fatalf("没读回全局的 user.name，实际条目：%+v", entries)
	}
	if global.Value != "Glob Al" {
		t.Errorf("全局值 = %q，想要 %q", global.Value, "Glob Al")
	}
	if global.File != globalPath {
		t.Errorf("全局来源 = %q，想要 %q", global.File, globalPath)
	}

	local, ok := entryOf(entries, ScopeLocal, "user.email")
	if !ok {
		t.Fatalf("没读回项目级的 user.email，实际条目：%+v", entries)
	}
	if local.Value != "local@example.com" {
		t.Errorf("项目值 = %q，想要 %q", local.Value, "local@example.com")
	}
	want := filepath.Join(dir, ".git", "config")
	if local.File != want {
		t.Errorf("项目来源 = %q，想要 %q", local.File, want)
	}

	// git init 自己写的 core.* 也得在，不然界面上项目级会是一片空白。
	if _, ok := entryOf(entries, ScopeLocal, "core.repositoryformatversion"); !ok {
		t.Errorf("项目级缺少 git init 写入的 core.*，实际条目：%+v", entries)
	}
}

// 写入是「同名压成一条」，不是追一条。
func TestWriteConfigReplacesAllValues(t *testing.T) {
	dir, _ := configTestRepo(t)
	ctx := context.Background()

	runGitT(t, dir, "config", "--local", "--add", "url.git@github.com:.insteadof", "ssh://git@github.com/")
	runGitT(t, dir, "config", "--local", "--add", "url.git@github.com:.insteadof", "git@github.com:")

	if err := WriteConfig(ctx, dir, ScopeLocal, "url.git@github.com:.insteadof", "https://github.com/"); err != nil {
		t.Fatalf("写配置: %v", err)
	}

	entries, err := ReadConfig(ctx, dir)
	if err != nil {
		t.Fatalf("读配置: %v", err)
	}
	var got []string
	for _, e := range entries {
		if e.Scope == ScopeLocal && e.Key == "url.git@github.com:.insteadof" {
			got = append(got, e.Value)
		}
	}
	if len(got) != 1 || got[0] != "https://github.com/" {
		t.Errorf("替换后应只剩一条 https://github.com/，实际 %q", got)
	}
}

// 以「-」开头的值不会被当成选项吃掉。
func TestWriteConfigKeepsDashValue(t *testing.T) {
	dir, _ := configTestRepo(t)
	ctx := context.Background()

	if err := WriteConfig(ctx, dir, ScopeLocal, "core.pager", "-R"); err != nil {
		t.Fatalf("写含 - 的值: %v", err)
	}
	out := strings.TrimSpace(gitOut(t, dir, "config", "--local", "core.pager"))
	if out != "-R" {
		t.Errorf("core.pager = %q，想要 %q", out, "-R")
	}
}

// 删除是幂等的：键不在（或已经被别处删掉）不该报错。
func TestUnsetConfigIsIdempotent(t *testing.T) {
	dir, _ := configTestRepo(t)
	ctx := context.Background()

	if err := WriteConfig(ctx, dir, ScopeGlobal, "pull.rebase", "false"); err != nil {
		t.Fatalf("写配置: %v", err)
	}
	if err := UnsetConfig(ctx, dir, ScopeGlobal, "pull.rebase"); err != nil {
		t.Fatalf("删配置: %v", err)
	}
	if err := UnsetConfig(ctx, dir, ScopeGlobal, "pull.rebase"); err != nil {
		t.Fatalf("删已经不在的键应无声通过，实际: %v", err)
	}

	entries, err := ReadConfig(ctx, dir)
	if err != nil {
		t.Fatalf("读配置: %v", err)
	}
	if _, ok := entryOf(entries, ScopeGlobal, "pull.rebase"); ok {
		t.Error("删过的键还在")
	}
}

// 项目级只有一个仓库看得见：另一个仓库读不到它。
func TestLocalConfigStaysInItsRepo(t *testing.T) {
	dir, _ := configTestRepo(t)
	other := t.TempDir()
	runGitT(t, other, "init", "-q")
	ctx := context.Background()

	if err := WriteConfig(ctx, dir, ScopeLocal, "user.name", "Only Here"); err != nil {
		t.Fatalf("写项目配置: %v", err)
	}

	entries, err := ReadConfig(ctx, other)
	if err != nil {
		t.Fatalf("读另一个仓库: %v", err)
	}
	if _, ok := entryOf(entries, ScopeLocal, "user.name"); ok {
		t.Error("项目级配置串到别的仓库了")
	}
}

// 仓库之外照样能读全局与系统级（项目级那一栏由界面说「没开仓库」）。
func TestReadConfigOutsideRepo(t *testing.T) {
	_, globalPath := configTestRepo(t)
	ctx := context.Background()

	if err := WriteConfig(ctx, "", ScopeGlobal, "init.defaultbranch", "main"); err != nil {
		t.Fatalf("写全局配置: %v", err)
	}
	entries, err := ReadConfig(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("仓库外读配置: %v", err)
	}
	if e, ok := entryOf(entries, ScopeGlobal, "init.defaultbranch"); !ok || e.Value != "main" {
		t.Errorf("仓库外没读到全局配置：%+v（全局文件 %s）", entries, globalPath)
	}
}

// 值里可以带换行（多行值就是这么存的），解析不能把它切断。
func TestParseConfigListKeepsMultilineValues(t *testing.T) {
	raw := strings.Join([]string{
		ScopeSystem, "file:/etc/gitconfig", "core.editor\nvim",
		ScopeGlobal, "file:/home/u/.gitconfig", "alias.co\ncheckout",
		ScopeGlobal, "file:/home/u/.gitconfig", "gpg.program\ngpg\n--homedir /k",
	}, "\x00") + "\x00"

	entries := parseConfigList([]byte(raw))
	if len(entries) != 3 {
		t.Fatalf("解析出 %d 条，想要 3 条：%+v", len(entries), entries)
	}
	if entries[2].Key != "gpg.program" || entries[2].Value != "gpg\n--homedir /k" {
		t.Errorf("多行值被切坏了：%+v", entries[2])
	}
	if entries[0].Scope != ScopeSystem || entries[0].File != "/etc/gitconfig" {
		t.Errorf("层级/来源没解析对：%+v", entries[0])
	}
	// 空值也要留住：`key =` 是合法配置，丢掉它等于把这条吃没了。
	empty := parseConfigList([]byte(ScopeGlobal + "\x00file:/g\x00core.bare\n\x00"))
	if len(empty) != 1 || empty[0].Value != "" {
		t.Errorf("空值条目解析错：%+v", empty)
	}
}

func TestCheckConfigKey(t *testing.T) {
	ok := []string{"core.autocrlf", "url.git@github.com:.insteadof", "user.name", "credential.https://x.y.helper"}
	for _, k := range ok {
		if err := checkConfigKey(k); err != nil {
			t.Errorf("%q 应当合法，却报 %v", k, err)
		}
	}
	bad := []string{"", "-R", "--file", "core.editor x", "a=b", "a\tb"}
	for _, k := range bad {
		if err := checkConfigKey(k); err == nil {
			t.Errorf("%q 应当被挡下", k)
		}
	}
}

// 写坏键名时不落盘：错误要看得见，配置文件不能被写脏。
func TestWriteConfigRejectsBadKey(t *testing.T) {
	dir, globalPath := configTestRepo(t)
	if err := WriteConfig(context.Background(), dir, ScopeGlobal, "--file", "x"); err == nil {
		t.Fatal("坏键名应当报错")
	}
	if _, err := os.Stat(globalPath); err == nil {
		t.Error("坏键名不该写出全局配置文件")
	}
	if err := UnsetConfig(context.Background(), dir, ScopeGlobal, "--file"); err == nil {
		t.Error("坏键名应当报错")
	}
}

package gitrepo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runGitT 在 dir 里跑一条 git 命令，失败就炸测试。
func runGitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// fps 打开 dir 这个仓库算一次当前指纹。
func fps(t *testing.T, dir string) string {
	t.Helper()
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return r.StateFingerprint(context.Background())
}

// TestStateFingerprintTracksChanges 验证指纹在仓库实际上没变时保持不变，
// 工作区、提交、引用、储藏每动一处都会被它捕捉到。
func TestStateFingerprintTracksChanges(t *testing.T) {
	dir := t.TempDir()
	runGitT(t, dir, "init", "-b", "main")
	runGitT(t, dir, "config", "user.email", "test@example.com")
	runGitT(t, dir, "config", "user.name", "Test")
	runGitT(t, dir, "config", "commit.gpgsign", "false")

	// 空仓库也得出一个指纹，不能挂在 Open 上
	base := fps(t, dir)
	write := func(name, body string) {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString(body)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}

	// 没有任何变化时，连续两次指纹必须一致
	if got := fps(t, dir); got != base {
		t.Fatalf("空仓库指纹应稳定，变了：\n%q\n%q", base, got)
	}

	// 未跟踪的新文件：改动列表要跟上
	write("a.go", "package main\n")
	afterNewFile := fps(t, dir)
	if afterNewFile == base {
		t.Fatal("新增未跟踪文件后指纹应变化")
	}
	if got := fps(t, dir); got != afterNewFile {
		t.Fatal("文件没再动时指纹应保持稳定")
	}

	// 暂存（index 改动）
	runGitT(t, dir, "add", "a.go")
	afterStage := fps(t, dir)
	if afterStage == afterNewFile {
		t.Fatal("暂存后指纹应变化")
	}

	// 提交（HEAD 前进）
	runGitT(t, dir, "commit", "-m", "init")
	afterCommit := fps(t, dir)
	if afterCommit == afterStage {
		t.Fatal("提交后指纹应变化")
	}

	// 修改已跟踪文件（工作区本地改动）
	write("a.go", "package main\n\nfunc main() {}\n")
	afterEdit := fps(t, dir)
	if afterEdit == afterCommit {
		t.Fatal("改动已跟踪文件后指纹应变化")
	}

	// 新分支（引用变化，侧栏要跟上）
	runGitT(t, dir, "checkout", "-b", "topic")
	afterBranch := fps(t, dir)
	if afterBranch == afterEdit {
		t.Fatal("新建分支后指纹应变化")
	}

	// 打标签（引用变化）
	runGitT(t, dir, "tag", "v1.0.0")
	afterTag := fps(t, dir)
	if afterTag == afterBranch {
		t.Fatal("打标签后指纹应变化")
	}

	// 储藏（refs/stash 出现）
	write("b.go", "package b\n")
	runGitT(t, dir, "stash", "push", "-m", "wip")
	afterStash := fps(t, dir)
	if afterStash == afterTag {
		t.Fatal("储藏后指纹应变化")
	}
}

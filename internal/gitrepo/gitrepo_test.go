package gitrepo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// tempRepo 建一个带一次提交的临时仓库，仓库里有一个已跟踪文件。
func tempRepo(t *testing.T) *Repo {
	t.Helper()
	dir := t.TempDir()
	// macOS 的 t.TempDir() 在 /var/folders 下，而 /var 是指向 /private/var 的
	// 符号链接。不解析掉的话，git rev-parse --show-toplevel 返回的路径
	// 和 t.TempDir() 给的路径字符串不一致，后面拼绝对路径会错。
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}

	git(t, dir, "init", "-b", "main")
	git(t, dir, "config", "user.email", "test@example.com")
	git(t, dir, "config", "user.name", "Test")
	git(t, dir, "config", "commit.gpgsign", "false")

	write(t, filepath.Join(dir, "a.txt"), "one\ntwo\nthree\n")
	git(t, dir, "add", "a.txt")
	git(t, dir, "commit", "-m", "init")

	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return r
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenRejectsNonRepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(context.Background(), dir); err == nil {
		t.Fatal("Open 在非 git 目录下应该报错")
	}
}

func TestBranch(t *testing.T) {
	r := tempRepo(t)
	branch, err := r.Branch(context.Background())
	if err != nil {
		t.Fatalf("Branch: %v", err)
	}
	if branch != "main" {
		t.Errorf("branch = %q, want main", branch)
	}
}

func TestLoadWorkingTree(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	// 一个已跟踪文件被修改
	write(t, filepath.Join(r.Root, "a.txt"), "one\ntwo changed\nthree\nfour\n")
	// 一个未跟踪文件
	write(t, filepath.Join(r.Root, "new.txt"), "hello\nworld\n")

	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Branch != "main" {
		t.Errorf("Branch = %q, want main", m.Branch)
	}
	if len(m.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2（a.txt 和 new.txt）: %+v", len(m.Files), m.Files)
	}

	byPath := map[string]File{}
	for _, f := range m.Files {
		byPath[f.NewPath] = f
	}

	a, ok := byPath["a.txt"]
	if !ok {
		t.Fatal("缺少 a.txt")
	}
	if a.Status != StatusModified {
		t.Errorf("a.txt status = %q, want modified", a.Status)
	}
	if !a.Unstaged || a.Staged {
		t.Errorf("a.txt Staged=%v Unstaged=%v, want false/true", a.Staged, a.Unstaged)
	}
	// 原内容是 one/two/three，改成 one/two changed/three/four：
	// 净效果是删 1 行（two）加 2 行（two changed、four）
	if a.AddCount != 2 || a.DelCount != 1 {
		t.Errorf("a.txt +%d -%d, want +2 -1", a.AddCount, a.DelCount)
	}
	if len(a.Hunks) == 0 || a.Hunks[0].ID == "" {
		t.Error("a.txt 的 hunk 应该有内容且 ID 非空")
	}

	n, ok := byPath["new.txt"]
	if !ok {
		t.Fatal("缺少 new.txt")
	}
	if n.Status != StatusUntracked {
		t.Errorf("new.txt status = %q, want untracked", n.Status)
	}
	// 未跟踪文件 git diff 不输出，应该由 Load 合成出整文件新增的 hunk
	if n.AddCount != 2 {
		t.Errorf("new.txt +%d, want +2", n.AddCount)
	}
	if len(n.Hunks) != 1 || len(n.Hunks[0].Lines) != 2 {
		t.Fatalf("new.txt 应该合成出 1 个含 2 行的 hunk，实际 %+v", n.Hunks)
	}
	if got := n.Hunks[0].Lines[0]; got.Type != "add" || got.Content != "hello" || got.NewNum != 1 {
		t.Errorf("new.txt 第一行 = %+v, want add/hello/1", got)
	}

	if m.TotalAdd != 4 || m.TotalDel != 1 {
		t.Errorf("Total +%d -%d, want +4 -1", m.TotalAdd, m.TotalDel)
	}
}

// 已暂存和未暂存必须能分开 —— 下一步做暂存/提交功能全靠这个。
func TestLoadDistinguishesStagedFromUnstaged(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	write(t, filepath.Join(r.Root, "a.txt"), "one\nstaged\nthree\n")
	git(t, r.Root, "add", "a.txt") // 暂存这一版
	// 暂存之后又改了一次，于是工作区比 index 更新
	write(t, filepath.Join(r.Root, "a.txt"), "one\nstaged\nthree\nunstaged\n")

	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(m.Files))
	}
	f := m.Files[0]
	if !f.Staged || !f.Unstaged {
		t.Errorf("Staged=%v Unstaged=%v, 两者都应该是 true", f.Staged, f.Unstaged)
	}
}

func TestLoadDetectsRename(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	git(t, r.Root, "mv", "a.txt", "b.txt")

	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(m.Files))
	}
	f := m.Files[0]
	if f.Status != StatusRenamed {
		t.Errorf("status = %q, want renamed", f.Status)
	}
	if f.OldPath != "a.txt" || f.NewPath != "b.txt" {
		t.Errorf("OldPath=%q NewPath=%q, want a.txt / b.txt", f.OldPath, f.NewPath)
	}
}

// 内容变了指纹就要变，否则「已 review」标记会在文件改动后错误地保留。
func TestFingerprintChangesWithContent(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	write(t, filepath.Join(r.Root, "a.txt"), "one\ntwo\nTHREE\n")
	m1, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	write(t, filepath.Join(r.Root, "a.txt"), "one\ntwo\nFOUR\n")
	m2, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if m1.Files[0].Fingerprint == m2.Files[0].Fingerprint {
		t.Error("内容变了，指纹却没变")
	}
}

// 空仓库（还没有任何提交）不应该报错，改动要能全部显示出来。
func TestLoadEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	git(t, dir, "init", "-b", "main")
	write(t, filepath.Join(dir, "first.txt"), "hello\n")

	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	m, err := r.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 空仓库: %v", err)
	}
	if len(m.Files) != 1 || m.Files[0].NewPath != "first.txt" {
		t.Fatalf("空仓库应该把未跟踪文件列出来，实际 %+v", m.Files)
	}
	if m.TotalAdd != 1 {
		t.Errorf("TotalAdd = %d, want 1", m.TotalAdd)
	}
}

// 未跟踪的空文件不该让整个加载失败。
func TestLoadEmptyUntrackedFile(t *testing.T) {
	r := tempRepo(t)
	write(t, filepath.Join(r.Root, "empty.txt"), "")

	m, err := r.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Files) != 1 {
		t.Fatalf("len(Files) = %d, want 1", len(m.Files))
	}
	if m.Files[0].NewPath != "empty.txt" {
		t.Errorf("NewPath = %q, want empty.txt", m.Files[0].NewPath)
	}
}

func TestParseStatusSynthetic(t *testing.T) {
	// 造一段 porcelain=v2 -z 的原始输出：已暂存重命名（带原路径）、
	// 已暂存+工作区都改过、未跟踪
	raw := "# branch.oid abc\x00" +
		"1 M. N... 100644 100644 100644 aaa bbb staged.txt\x00" +
		"1 .M N... 100644 100644 100644 aaa bbb worktree.txt\x00" +
		"1 MM N... 100644 100644 100644 aaa bbb both.txt\x00" +
		"2 R. N... 100644 100644 100644 aaa bbb R100 new.txt\x00old.txt\x00" +
		"? untracked.txt\x00"

	changes := parseStatus([]byte(raw))
	if len(changes) != 5 {
		t.Fatalf("解析出 %d 条，want 5: %+v", len(changes), changes)
	}

	if c := changes[0]; c.Path != "staged.txt" || !c.Staged || c.Unstaged {
		t.Errorf("staged.txt = %+v, want Staged only", c)
	}
	if c := changes[1]; c.Path != "worktree.txt" || c.Staged || !c.Unstaged {
		t.Errorf("worktree.txt = %+v, want Unstaged only", c)
	}
	if c := changes[2]; c.Path != "both.txt" || !c.Staged || !c.Unstaged {
		t.Errorf("both.txt = %+v, want both", c)
	}
	if c := changes[3]; c.Path != "new.txt" || c.OldPath != "old.txt" || c.Status != StatusRenamed {
		t.Errorf("rename = %+v, want new.txt <- old.txt renamed", c)
	}
	if c := changes[4]; c.Path != "untracked.txt" || c.Status != StatusUntracked {
		t.Errorf("untracked = %+v", c)
	}
}

func TestStatusLetterAndLabel(t *testing.T) {
	cases := []struct {
		s      Status
		letter byte
	}{
		{StatusAdded, 'A'},
		{StatusUntracked, 'A'},
		{StatusModified, 'M'},
		{StatusDeleted, 'D'},
		{StatusRenamed, 'R'},
		{StatusCopied, 'C'},
		{StatusConflict, '!'},
	}
	for _, c := range cases {
		if got := c.s.Letter(); got != c.letter {
			t.Errorf("%s.Letter() = %q, want %q", c.s, got, c.letter)
		}
		if c.s.Label() == "" {
			t.Errorf("%s.Label() 不应该为空", c.s)
		}
	}
}

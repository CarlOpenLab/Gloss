package gitrepo

import (
	"bytes"
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

// 分离头指针要能认出来 —— 界面靠它把分支芯片换成警示样式，
// 不这么说用户会以为自己在分支上正常干活。
func TestLoadDetachedHead(t *testing.T) {
	r := tempRepo(t)
	git(t, r.Root, "checkout", "--detach", "HEAD")

	m, err := r.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !m.Detached {
		t.Error("检出到提交之后 Detached 该是 true")
	}
	if m.Branch == "" || m.Branch == "main" {
		t.Errorf("分离头指针的分支名该是短哈希，实际 %q", m.Branch)
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

// 已暂存和未暂存拆成两段：两条 File、各自的 Section、各自的 diff 基线 ——
// hunk 级暂存全靠「界面展示的 hunk 和重新跑出来的 diff 同基线」。
func TestLoadSplitsSections(t *testing.T) {
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
	if len(m.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2（已暂存 + 未暂存各一条）: %+v", len(m.Files), m.Files)
	}

	staged, unstaged := m.Files[0], m.Files[1]
	if staged.Section != SectionStaged || !staged.Staged || staged.Unstaged {
		t.Errorf("第一条该是已暂存段: Section=%v Staged=%v Unstaged=%v", staged.Section, staged.Staged, staged.Unstaged)
	}
	if unstaged.Section != SectionUnstaged || unstaged.Staged || !unstaged.Unstaged {
		t.Errorf("第二条该是未暂存段: Section=%v Staged=%v Unstaged=%v", unstaged.Section, unstaged.Staged, unstaged.Unstaged)
	}

	// 已暂存段看 index vs HEAD：two -> staged；未暂存段看 worktree vs index：
	// 只多了最后一行。两段的增删行必须各归各。
	if got := addedLines(staged); len(got) != 1 || got[0] != "staged" {
		t.Errorf("已暂存段的 + 行 = %q, want [staged]", got)
	}
	if got := deletedLines(staged); len(got) != 1 || got[0] != "two" {
		t.Errorf("已暂存段的 - 行 = %q, want [two]", got)
	}
	if got := addedLines(unstaged); len(got) != 1 || got[0] != "unstaged" {
		t.Errorf("未暂存段的 + 行 = %q, want [unstaged]", got)
	}
	if got := deletedLines(unstaged); len(got) != 0 {
		t.Errorf("未暂存段的 - 行 = %q, want 空", got)
	}

	if staged.Fingerprint == unstaged.Fingerprint {
		t.Error("两段内容不同，指纹不该一样")
	}
	if m.TotalAdd != 2 || m.TotalDel != 1 {
		t.Errorf("Total +%d -%d, want +2 -1", m.TotalAdd, m.TotalDel)
	}
}

// addedLines / deletedLines 收集一段里所有增/删行的内容。
func addedLines(f File) []string {
	var out []string
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			if l.Type == "add" {
				out = append(out, l.Content)
			}
		}
	}
	return out
}

func deletedLines(f File) []string {
	var out []string
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			if l.Type == "del" {
				out = append(out, l.Content)
			}
		}
	}
	return out
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

// 二进制文件没有 hunk，指纹得靠对象哈希 —— 否则换一张图，界面还会
// 认为「内容没变」，图片预览和「已 review」标记都停在上一个版本。
func TestBinaryFingerprintChangesWithContent(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	p := filepath.Join(r.Root, "logo.png")
	png := func(tag string) []byte {
		return append([]byte("\x89PNG\r\n\x1a\n\x00"), []byte(tag)...)
	}

	if err := os.WriteFile(p, png("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r.Root, "add", "logo.png")
	git(t, r.Root, "commit", "-m", "add logo")

	if err := os.WriteFile(p, png("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	m1, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f1 := fileByPath(t, m1, "logo.png")
	if !f1.Binary {
		t.Fatalf("带 NUL 的文件该认成二进制")
	}

	if err := os.WriteFile(p, png("v3"), 0o644); err != nil {
		t.Fatal(err)
	}
	m2, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f2 := fileByPath(t, m2, "logo.png")
	if f1.Fingerprint == f2.Fingerprint {
		t.Errorf("二进制内容变了，指纹却没变（%q）", f1.Fingerprint)
	}

	// 暂存之后又改工作区：两段都在，指纹要各不相同（index 里是 v3、
	// 工作区是 v4），否则改一个会带着另一个一起重画/失效。
	git(t, r.Root, "add", "logo.png")
	if err := os.WriteFile(p, png("v4"), 0o644); err != nil {
		t.Fatal(err)
	}
	m3, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	staged, unstaged := fileByPath(t, m3, "logo.png"), lastFileByPath(t, m3, "logo.png")
	if staged.Fingerprint == unstaged.Fingerprint {
		t.Errorf("暂存区和工作区是两个版本，指纹不该一样")
	}
}

// fileByPath 取该路径的第一条（已暂存段优先），lastFileByPath 取最后一条。
func fileByPath(t *testing.T, m *Model, path string) File {
	t.Helper()
	for _, f := range m.Files {
		if f.Path() == path {
			return f
		}
	}
	t.Fatalf("文件列表里没有 %s", path)
	return File{}
}

func lastFileByPath(t *testing.T, m *Model, path string) File {
	t.Helper()
	var out File
	found := false
	for _, f := range m.Files {
		if f.Path() == path {
			out, found = f, true
		}
	}
	if !found {
		t.Fatalf("文件列表里没有 %s", path)
	}
	return out
}

// 未跟踪的图片超过文本读取上限也要完整收进来：常见的截图就 1~3 MB，
// 按 1 MB 截断的话指纹只算半截，解码也只剩前半张。
func TestLoadUntrackedImageBeyondTextCap(t *testing.T) {
	r := tempRepo(t)
	data := append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0x00}, 2<<20)...)
	if err := os.WriteFile(filepath.Join(r.Root, "big.png"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := r.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f := fileByPath(t, m, "big.png")
	if !f.Binary {
		t.Errorf("PNG 该标成二进制")
	}
	if f.TooLarge {
		t.Errorf("图片不是「文本太大读不下」，不该标 TooLarge")
	}
	if f.AddCount != 0 || len(f.Hunks) != 0 {
		t.Errorf("二进制文件不该有 hunk/增删行（hunks=%d add=%d）", len(f.Hunks), f.AddCount)
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

func TestParseStatusBranch(t *testing.T) {
	raw := "# branch.oid abc\x00" +
		"# branch.head feat\x00" +
		"# branch.upstream origin/feat\x00" +
		"# branch.ab +5 -2\x00" +
		"1 M. N... 100644 100644 100644 aaa bbb staged.txt\x00"

	ahead, behind, upstream := parseStatusBranch([]byte(raw))
	if ahead != 5 {
		t.Errorf("ahead = %d, want 5", ahead)
	}
	if behind != 2 {
		t.Errorf("behind = %d, want 2", behind)
	}
	if upstream != "origin/feat" {
		t.Errorf("upstream = %q, want origin/feat", upstream)
	}

	// 测试没有上游的普通分支
	rawNoUpstream := "# branch.oid abc\x00# branch.head feat\x00"
	ahead, behind, upstream = parseStatusBranch([]byte(rawNoUpstream))
	if ahead != 0 || behind != 0 || upstream != "" {
		t.Errorf("no upstream parsed = (+%d, -%d, %q), want (+0, -0, \"\")", ahead, behind, upstream)
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

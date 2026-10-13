package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitOut 跑一条 git 命令并返回输出。
//
// 写操作的验收一律走它（而不是再 Load 一遍）：用自己的读路径验证自己的
// 写路径，Load 里的 bug 会把写操作的 bug 藏起来。
func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// longFile 是一份 20 行的内容，l1..l20。
func longFile() string {
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		fmt.Fprintf(&b, "l%d\n", i)
	}
	return b.String()
}

// 保存一个可执行脚本时不该把权限位抹掉：那是没人要求的一次改动，而且
// git status 也不会把它报出来 —— 用户要过很久才会发现脚本跑不起来了。
func TestWriteContentsKeepsExistingMode(t *testing.T) {
	r := tempRepo(t)
	script := filepath.Join(r.Root, "run.sh")
	write(t, script, "#!/bin/sh\necho old\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.WriteContents("run.sh", []byte("#!/bin/sh\necho new\n")); err != nil {
		t.Fatalf("WriteContents: %v", err)
	}
	info, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("权限位 = %o，want 755", got)
	}

	// 新文件才按 0644 建。
	if err := r.WriteContents("fresh.txt", []byte("x\n")); err != nil {
		t.Fatalf("WriteContents: %v", err)
	}
	info, err = os.Stat(filepath.Join(r.Root, "fresh.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("新文件权限位 = %o，want 644", got)
	}
}

// 只暂存点中的那一个 hunk：另一个必须原样留在未暂存段里。
func TestStageHunkOnlyThatHunk(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	write(t, filepath.Join(r.Root, "a.txt"), longFile())
	git(t, r.Root, "add", "a.txt")
	git(t, r.Root, "commit", "-m", "base")

	// 两处改动隔得够远（>3 行上下文），会分成两个 hunk
	edited := strings.Replace(longFile(), "l2\n", "TWO\n", 1)
	edited = strings.Replace(edited, "l18\n", "EIGHTEEN\n", 1)
	write(t, filepath.Join(r.Root, "a.txt"), edited)

	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Files) != 1 || len(m.Files[0].Hunks) != 2 {
		t.Fatalf("测试夹具该是 1 个文件 2 个 hunk，实际 %d 个文件", len(m.Files))
	}

	if err := r.StageHunk(ctx, "a.txt", m.Files[0].Hunks[0].ID); err != nil {
		t.Fatalf("StageHunk: %v", err)
	}

	cached := gitOut(t, r.Root, "diff", "--cached")
	if !strings.Contains(cached, "+TWO") || strings.Contains(cached, "+EIGHTEEN") {
		t.Errorf("index 里应该只有第一处改动:\n%s", cached)
	}
	rest := gitOut(t, r.Root, "diff")
	if strings.Contains(rest, "+TWO") || !strings.Contains(rest, "+EIGHTEEN") {
		t.Errorf("工作区里应该只剩第二处改动:\n%s", rest)
	}

	// 重新 Load：拆成两段，各带一个 hunk
	m2, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m2.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2", len(m2.Files))
	}
	if m2.Files[0].Section != SectionStaged || m2.Files[1].Section != SectionUnstaged {
		t.Errorf("分段顺序错了: %v / %v", m2.Files[0].Section, m2.Files[1].Section)
	}
}

// 缺尾换行的文件：暂存之后 index 里的内容必须逐字节正确（\ No newline
// 标记要是丢了，git apply 会拒收或者留下带换行的错误内容）。
func TestStageHunkNoTrailingNewline(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	write(t, filepath.Join(r.Root, "a.txt"), "a\nb") // 不带尾换行
	git(t, r.Root, "add", "a.txt")
	git(t, r.Root, "commit", "-m", "base")

	write(t, filepath.Join(r.Root, "a.txt"), "a\nc") // 同样不带

	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Files) != 1 || len(m.Files[0].Hunks) != 1 {
		t.Fatalf("夹具该是 1 个 hunk: %+v", m.Files)
	}
	if err := r.StageHunk(ctx, "a.txt", m.Files[0].Hunks[0].ID); err != nil {
		t.Fatalf("StageHunk: %v", err)
	}

	if got := gitOut(t, r.Root, "show", ":0:a.txt"); got != "a\nc" {
		t.Errorf("index 内容 = %q, want %q", got, "a\nc")
	}
}

// 取消暂存一个 hunk：index 回到 HEAD 的对应状态。
func TestUnstageHunkRestoresIndex(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	write(t, filepath.Join(r.Root, "a.txt"), longFile())
	git(t, r.Root, "add", "a.txt")
	git(t, r.Root, "commit", "-m", "base")

	edited := strings.Replace(longFile(), "l2\n", "TWO\n", 1)
	edited = strings.Replace(edited, "l18\n", "EIGHTEEN\n", 1)
	write(t, filepath.Join(r.Root, "a.txt"), edited)
	git(t, r.Root, "add", "a.txt") // 全部暂存

	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(m.Files) != 1 || m.Files[0].Section != SectionStaged || len(m.Files[0].Hunks) != 2 {
		t.Fatalf("夹具该是已暂存段 2 个 hunk: %+v", m.Files)
	}
	if err := r.UnstageHunk(ctx, "a.txt", m.Files[0].Hunks[1].ID); err != nil {
		t.Fatalf("UnstageHunk: %v", err)
	}

	cached := gitOut(t, r.Root, "diff", "--cached")
	if !strings.Contains(cached, "+TWO") || strings.Contains(cached, "+EIGHTEEN") {
		t.Errorf("index 里该只剩第一处改动:\n%s", cached)
	}
	if rest := gitOut(t, r.Root, "diff"); !strings.Contains(rest, "+EIGHTEEN") {
		t.Errorf("工作区该出现第二处改动:\n%s", rest)
	}
}

// 拿到 hunk ID 之后文件又被改过：ErrHunkGone，而不是把别的改动塞进去。
func TestStageHunkGoneAfterEdit(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	write(t, filepath.Join(r.Root, "a.txt"), "one\nTWO\nthree\n")
	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	id := m.Files[0].Hunks[0].ID

	// 改掉同一处，@@ 头对不上，旧 ID 就该失效
	write(t, filepath.Join(r.Root, "a.txt"), "zero\none\nTWO\nthree\n")

	err = r.StageHunk(ctx, "a.txt", id)
	if !errors.Is(err, ErrHunkGone) {
		t.Fatalf("err = %v, want ErrHunkGone", err)
	}
	if cached := gitOut(t, r.Root, "diff", "--cached"); cached != "" {
		t.Errorf("失败时不该动 index:\n%s", cached)
	}
}

func TestStageFileUntracked(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	write(t, filepath.Join(r.Root, "new.txt"), "hello\n")

	if err := r.StageFile(ctx, "new.txt"); err != nil {
		t.Fatalf("StageFile: %v", err)
	}
	if got := gitOut(t, r.Root, "diff", "--cached", "--name-only"); strings.TrimSpace(got) != "new.txt" {
		t.Errorf("index 里该有 new.txt，实际 %q", got)
	}
}

func TestUnstageFileAdded(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	write(t, filepath.Join(r.Root, "new.txt"), "hello\n")
	git(t, r.Root, "add", "new.txt")

	if err := r.UnstageFile(ctx, "new.txt"); err != nil {
		t.Fatalf("UnstageFile: %v", err)
	}
	if got := gitOut(t, r.Root, "diff", "--cached", "--name-only"); strings.TrimSpace(got) != "" {
		t.Errorf("撤销暂存后 index 该是空的，实际 %q", got)
	}
	// 撤销「已暂存的新增」要回到未跟踪，而不是 index 里留一个空文件
	if got := gitOut(t, r.Root, "status", "--porcelain"); !strings.Contains(got, "?? new.txt") {
		t.Errorf("new.txt 该回到未跟踪，实际 %q", got)
	}
}

func TestStageFileDeleted(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	write(t, filepath.Join(r.Root, "del.txt"), "hello\n")
	git(t, r.Root, "add", "del.txt")
	git(t, r.Root, "commit", "-m", "init del.txt")

	if err := os.Remove(filepath.Join(r.Root, "del.txt")); err != nil {
		t.Fatal(err)
	}

	if err := r.StageFile(ctx, "del.txt"); err != nil {
		t.Fatalf("StageFile deleted: %v", err)
	}
	if got := gitOut(t, r.Root, "diff", "--cached", "--name-only"); strings.TrimSpace(got) != "del.txt" {
		t.Errorf("index 里该有 del.txt，实际 %q", got)
	}

	if err := r.UnstageFile(ctx, "del.txt"); err != nil {
		t.Fatalf("UnstageFile deleted: %v", err)
	}
	if got := gitOut(t, r.Root, "diff", "--cached", "--name-only"); strings.TrimSpace(got) != "" {
		t.Errorf("取消暂存后 index 该是空的，实际 %q", got)
	}
}

func TestCommit(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	write(t, filepath.Join(r.Root, "a.txt"), "one\ncommitted\nthree\n")
	git(t, r.Root, "add", "a.txt")

	if err := r.Commit(ctx, "改一下 a.txt"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got := strings.TrimSpace(gitOut(t, r.Root, "log", "-1", "--format=%s")); got != "改一下 a.txt" {
		t.Errorf("提交信息 = %q", got)
	}
	if got := gitOut(t, r.Root, "status", "--porcelain"); strings.TrimSpace(got) != "" {
		t.Errorf("提交后工作区该是干净的，实际 %q", got)
	}
}

// 提交信息原样走管道：换行、引号、以 - 开头的行都不能被命令行解析咬到。
func TestCommitMultilineMessage(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	write(t, filepath.Join(r.Root, "a.txt"), "one\nx\nthree\n")
	git(t, r.Root, "add", "a.txt")

	msg := "标题行\n\n- 以减号开头的行\nbody 里带 \"引号\" 和 '单引号'"
	if err := r.Commit(ctx, msg); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	got := strings.TrimSpace(gitOut(t, r.Root, "log", "-1", "--format=%B"))
	if got != msg {
		t.Errorf("提交信息 = %q, want %q", got, msg)
	}
}

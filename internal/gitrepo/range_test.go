package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// rangeRepo 建一个仓库：f.txt 在 HEAD 里是 old，工作区已经改成 new，返回
// 仓库和两份原文 —— 界面展示「未暂存」时拿到的就是同两份文本（旧侧 = index，
// 新侧 = 工作区）。
func rangeRepo(t *testing.T, old, newText string) *Repo {
	t.Helper()
	r := tempRepo(t)
	p := filepath.Join(r.Root, "f.txt")
	write(t, p, old)
	git(t, r.Root, "add", "f.txt")
	git(t, r.Root, "commit", "-m", "base")
	write(t, p, newText)
	return r
}

// replaceLine 把一份「每行都有末尾换行」的文本里第 i 行（0 基）换成 s。
func replaceLine(text string, i int, s string) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	lines[i] = s
	return strings.Join(lines, "\n") + "\n"
}

// gitApply 把补丁从管道喂给 git apply（参数由调用方给，比如 --check / --cached）。
func gitApply(t *testing.T, dir, patch string, args ...string) error {
	t.Helper()
	cmd := exec.Command("git", append([]string{"apply"}, args...)...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	cmd.Stdin = strings.NewReader(patch)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git apply %v: %v\n%s", args, err, out)
	}
	return nil
}

// 同一文件里两处相距很远的改动，只 StageRange 第二处：暂存段里只有第二处，
// 第一处原样留在未暂存段；再 UnstageRange 把它退回去。
func TestStageRangeOnlySecondOfTwo(t *testing.T) {
	ctx := context.Background()
	old := longFile() // l1..l20
	newText := replaceLine(replaceLine(old, 2, "X3"), 14, "X15")
	r := rangeRepo(t, old, newText)

	hr := diff.HunkRange{OldStart: 14, OldEnd: 15, NewStart: 14, NewEnd: 15}
	if err := r.StageRange(ctx, "f.txt", old, newText, hr); err != nil {
		t.Fatalf("StageRange: %v", err)
	}

	staged := gitOut(t, r.Root, "diff", "--cached", "--", "f.txt")
	if !strings.Contains(staged, "-l15") || !strings.Contains(staged, "+X15") {
		t.Errorf("暂存段里没有第二处改动：\n%s", staged)
	}
	if strings.Contains(staged, "X3") {
		t.Errorf("暂存段里混进了第一处改动：\n%s", staged)
	}
	if n := strings.Count(staged, "@@ -"); n != 1 {
		t.Errorf("暂存段有 %d 个 hunk，want 1：\n%s", n, staged)
	}

	// index 里第二处已经改了、第一处还没改。
	wantIndex := replaceLine(old, 14, "X15")
	if got := gitOut(t, r.Root, "show", ":f.txt"); got != wantIndex {
		t.Errorf("index 内容 = %q，want %q", got, wantIndex)
	}
	// 工作区那份一个字节没动。
	if got := gitOut(t, r.Root, "show", "HEAD:f.txt"); got != old {
		t.Errorf("HEAD 内容 = %q，want %q", got, old)
	}

	unstaged := gitOut(t, r.Root, "diff", "--", "f.txt")
	if !strings.Contains(unstaged, "-l3") || !strings.Contains(unstaged, "+X3") {
		t.Errorf("未暂存段里第一处改动丢了：\n%s", unstaged)
	}
	if strings.Contains(unstaged, "X15") {
		t.Errorf("第二处已经进 index 了，不该还在未暂存段里：\n%s", unstaged)
	}

	// 退回去：已暂存段的旧侧是 HEAD、新侧是 index，正是上面这两份。
	if err := r.UnstageRange(ctx, "f.txt", old, wantIndex, hr); err != nil {
		t.Fatalf("UnstageRange: %v", err)
	}
	if got := gitOut(t, r.Root, "diff", "--cached", "--", "f.txt"); got != "" {
		t.Errorf("取消暂存后暂存段应该空了：\n%s", got)
	}
	if got := gitOut(t, r.Root, "show", ":f.txt"); got != old {
		t.Errorf("取消暂存后 index 内容 = %q，want %q", got, old)
	}
	unstaged = gitOut(t, r.Root, "diff", "--", "f.txt")
	if !strings.Contains(unstaged, "+X3") || !strings.Contains(unstaged, "+X15") {
		t.Errorf("取消暂存后两处改动都该回到未暂存段：\n%s", unstaged)
	}
}

// 末尾没有换行符的文件：改的是最后一行，apply 要成功，内容要一个字节都不差
// （少了 "\ No newline at end of file"，git 会补一个换行出来）。
func TestStageRangeNoFinalNewline(t *testing.T) {
	ctx := context.Background()
	old := "a\nb\nc" // 末尾没有换行
	newText := "a\nb\nC"
	r := rangeRepo(t, old, newText)

	hr := diff.HunkRange{OldStart: 2, OldEnd: 3, NewStart: 2, NewEnd: 3}
	if err := r.StageRange(ctx, "f.txt", old, newText, hr); err != nil {
		t.Fatalf("StageRange: %v", err)
	}
	if got := gitOut(t, r.Root, "show", ":f.txt"); got != newText {
		t.Errorf("index 内容 = %q，want %q（末尾换行被动了）", got, newText)
	}
	staged := gitOut(t, r.Root, "diff", "--cached", "--", "f.txt")
	if !strings.Contains(staged, "\\ No newline at end of file") {
		t.Errorf("暂存段里没有缺换行的标记：\n%s", staged)
	}

	if err := r.UnstageRange(ctx, "f.txt", old, newText, hr); err != nil {
		t.Fatalf("UnstageRange: %v", err)
	}
	if got := gitOut(t, r.Root, "show", ":f.txt"); got != old {
		t.Errorf("取消暂存后 index 内容 = %q，want %q", got, old)
	}
}

// 新文件整段纯新增（旧侧是空区间）：index 里出现这条记录，内容对得上；
// 再整段退回去时 index 里那条记录要消失，而不是留一个空文件。
func TestStageRangeNewFile(t *testing.T) {
	ctx := context.Background()
	r := tempRepo(t)
	content := "n1\nn2\nn3\n"
	write(t, filepath.Join(r.Root, "new.txt"), content)

	hr := diff.HunkRange{OldStart: 0, OldEnd: 0, NewStart: 0, NewEnd: 3}
	if err := r.StageRange(ctx, "new.txt", "", content, hr); err != nil {
		t.Fatalf("StageRange: %v", err)
	}
	if got := gitOut(t, r.Root, "diff", "--cached", "--name-status"); !strings.Contains(got, "A\tnew.txt") {
		t.Errorf("暂存段里没有新增的 new.txt：\n%s", got)
	}
	if got := gitOut(t, r.Root, "show", ":new.txt"); got != content {
		t.Errorf("index 内容 = %q，want %q", got, content)
	}

	if err := r.UnstageRange(ctx, "new.txt", "", content, hr); err != nil {
		t.Fatalf("UnstageRange: %v", err)
	}
	if got := gitOut(t, r.Root, "diff", "--cached", "--name-status"); got != "" {
		t.Errorf("取消暂存后暂存段应该空了：\n%s", got)
	}
	if got := gitOut(t, r.Root, "status", "--porcelain"); !strings.Contains(got, "?? new.txt") {
		t.Errorf("取消暂存后 new.txt 应该回到未跟踪：\n%s", got)
	}
}

// CRLF 文件（Windows 上的常态）：行里的 \r 必须原样进补丁，否则 apply 出来的
// 是一份 LF 文件，用户下次打开会看到整行都「变了」。
func TestStageRangeCRLF(t *testing.T) {
	ctx := context.Background()
	r := tempRepo(t)
	// 固定换行约定：用户级 core.autocrlf 会让 git 在 add 时把 CRLF 转成 LF，
	// 那样测的就不是这条路径了。
	git(t, r.Root, "config", "core.autocrlf", "false")
	p := filepath.Join(r.Root, "f.txt")
	old := "a\r\nb\r\nc\r\nd\r\n"
	newText := "a\r\nb\r\nC\r\nd\r\n"
	write(t, p, old)
	git(t, r.Root, "add", "f.txt")
	git(t, r.Root, "commit", "-m", "base")
	write(t, p, newText)

	hr := diff.HunkRange{OldStart: 2, OldEnd: 3, NewStart: 2, NewEnd: 3}
	if err := r.StageRange(ctx, "f.txt", old, newText, hr); err != nil {
		t.Fatalf("StageRange: %v", err)
	}
	if got := gitOut(t, r.Root, "show", ":f.txt"); got != newText {
		t.Errorf("index 内容 = %q，want %q", got, newText)
	}
	staged := gitOut(t, r.Root, "diff", "--cached", "--", "f.txt")
	if !strings.Contains(staged, "+C\r") {
		t.Errorf("暂存段里的 C 行丢了 \\r：\n%q", staged)
	}
}

// 纯删除（新侧是空区间）：删掉的两行在暂存段里、index 里也不剩。
func TestStageRangePureDeletion(t *testing.T) {
	ctx := context.Background()
	old := "a\nb\nc\nd\n"
	newText := "a\nc\nd\n" // 删掉 b
	r := rangeRepo(t, old, newText)

	hr := diff.HunkRange{OldStart: 1, OldEnd: 2, NewStart: 1, NewEnd: 1}
	if err := r.StageRange(ctx, "f.txt", old, newText, hr); err != nil {
		t.Fatalf("StageRange: %v", err)
	}
	if got := gitOut(t, r.Root, "show", ":f.txt"); got != newText {
		t.Errorf("index 内容 = %q，want %q", got, newText)
	}
	staged := gitOut(t, r.Root, "diff", "--cached", "--", "f.txt")
	if !strings.Contains(staged, "-b") || strings.Contains(staged, "+b") {
		t.Errorf("暂存段里 b 应该是被删掉的一行：\n%s", staged)
	}

	if err := r.UnstageRange(ctx, "f.txt", old, newText, hr); err != nil {
		t.Fatalf("UnstageRange: %v", err)
	}
	if got := gitOut(t, r.Root, "show", ":f.txt"); got != old {
		t.Errorf("取消暂存后 index 内容 = %q，want %q", got, old)
	}
	if got := gitOut(t, r.Root, "diff", "--", "f.txt"); !strings.Contains(got, "-b") {
		t.Errorf("取消暂存后 b 该回到未暂存段：\n%s", got)
	}
}

// 区间越界（用户点下去和真正执行之间文件变了）报 ErrHunkGone，不是一句
// 没人看得懂的 git 报错。
func TestStageRangeOutOfRange(t *testing.T) {
	ctx := context.Background()
	old := "a\nb\nc\nd\n"
	newText := "a\nc\nd\n"
	r := rangeRepo(t, old, newText)

	hr := diff.HunkRange{OldStart: 1, OldEnd: 9, NewStart: 1, NewEnd: 8}
	err := r.StageRange(ctx, "f.txt", old, newText, hr)
	if !errors.Is(err, ErrHunkGone) {
		t.Fatalf("StageRange 越界 err = %v，want ErrHunkGone", err)
	}
	if got := gitOut(t, r.Root, "diff", "--cached", "--", "f.txt"); got != "" {
		t.Errorf("越界不该动 index：\n%s", got)
	}
}

// 反向验收：RangePatch 生成的补丁直接喂给 `git apply --check` 必须真能打上
// （工作区里那份就是旧侧原文）—— 光断言字符串里有 @@ 是不够的。
func TestRangePatchAppliesWithGitCheck(t *testing.T) {
	r := tempRepo(t)
	old := longFile()
	newText := replaceLine(replaceLine(old, 2, "X3"), 14, "X15")
	write(t, filepath.Join(r.Root, "f.txt"), old)

	for _, hr := range []diff.HunkRange{
		{OldStart: 2, OldEnd: 3, NewStart: 2, NewEnd: 3},     // 第一处
		{OldStart: 14, OldEnd: 15, NewStart: 14, NewEnd: 15}, // 第二处
	} {
		patch := diff.RangePatch("f.txt", old, newText, hr)
		if patch == "" {
			t.Fatalf("RangePatch(%+v) 生成了空补丁", hr)
		}
		if err := gitApply(t, r.Root, patch, "--check"); err != nil {
			t.Errorf("git apply --check 拒收：%v\npatch:\n%s", err, patch)
		}
	}

	// 已经变成新侧之后再打第一处的补丁就该被拒 —— 说明 git 真的在按上下文
	// 核对，而不是照单全收。
	write(t, filepath.Join(r.Root, "f.txt"), newText)
	patch := diff.RangePatch("f.txt", old, newText, diff.HunkRange{OldStart: 14, OldEnd: 15, NewStart: 14, NewEnd: 15})
	if err := gitApply(t, r.Root, patch, "--check"); err == nil {
		t.Error("内容已经是新侧了，这段补丁不该还能打上")
	}
}

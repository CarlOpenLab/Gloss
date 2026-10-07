package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 提交上的几件事（commitops.go / branch.go）在 git 这一层的验收：
// 每条断言都回到 git 自己身上（log、rev-parse、工作区的字节）。

// revOut 是「问一句」的 gitOut，去掉首尾空白 —— 包里的 gitOut 保留原样
// （那边断的是整段输出），这里比的是单个值，带着换行比不相等。
func revOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return strings.TrimSpace(gitOut(t, dir, args...))
}

// twoCommitRepo 是「init + 第二次提交改了 a.txt」的仓库，停在 main 上。
func twoCommitRepo(t *testing.T) *Repo {
	t.Helper()
	r := tempRepo(t)
	write(t, filepath.Join(r.Root, "a.txt"), "changed\n")
	git(t, r.Root, "commit", "-qam", "second")
	return r
}

// 拣选把那条提交的内容重放到当前分支上（HEAD 动、原提交还在）。
func TestCherryPickReplaysCommit(t *testing.T) {
	r := twoCommitRepo(t)
	ctx := context.Background()

	// 从 init 之后建一条分支，上面加一条提交 —— 它就是被拣的那条
	git(t, r.Root, "checkout", "-qb", "side")
	write(t, filepath.Join(r.Root, "b.txt"), "side 加的\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-qm", "side 的提交")
	pick := revOut(t, r.Root, "rev-parse", "HEAD")

	git(t, r.Root, "checkout", "-q", "main")
	if err := r.CherryPick(ctx, pick); err != nil {
		t.Fatalf("CherryPick: %v", err)
	}

	if got := readText(t, filepath.Join(r.Root, "b.txt")); got != "side 加的\n" {
		t.Errorf("b.txt = %q，want side 那条提交的内容", got)
	}
	if got := revOut(t, r.Root, "log", "-1", "--format=%s"); got != "side 的提交" {
		t.Errorf("顶端提交 = %q", got)
	}
	if got := revOut(t, r.Root, "rev-parse", "--symbolic-full-name", "HEAD"); got != "refs/heads/main" {
		t.Errorf("HEAD = %q，want 还附在 main 上", got)
	}
}

// 回退生成一条反向提交：内容回到那条提交之前，原提交仍然在历史里。
func TestRevertIsInverse(t *testing.T) {
	r := twoCommitRepo(t)
	ctx := context.Background()
	second := revOut(t, r.Root, "rev-parse", "HEAD")

	if err := r.Revert(ctx, second); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "one\ntwo\nthree\n" {
		t.Errorf("a.txt = %q，want 回到第二次提交之前", got)
	}
	if got := revOut(t, r.Root, "log", "-1", "--format=%s"); !strings.HasPrefix(got, "Revert") {
		t.Errorf("顶端提交 = %q，want Revert 开头", got)
	}
	// 被回退的那条还在（回退是加一条，不是删一条）
	if got := revOut(t, r.Root, "cat-file", "-t", second); got != "commit" {
		t.Errorf("被回退的提交 = %q，want 还在", got)
	}
}

// 重置三档的区别：分支都挪回去，丢到哪一层不一样。
func TestResetModes(t *testing.T) {
	ctx := context.Background()

	t.Run("soft 把改动留在暂存区", func(t *testing.T) {
		r := twoCommitRepo(t)
		first := revOut(t, r.Root, "rev-parse", "HEAD~1")
		write(t, filepath.Join(r.Root, "a.txt"), "手头的改动\n") // 工作区再脏一层
		if err := r.ResetTo(ctx, first, ResetSoft); err != nil {
			t.Fatalf("ResetTo: %v", err)
		}
		if got := revOut(t, r.Root, "rev-parse", "HEAD"); got != first {
			t.Errorf("HEAD = %s，want %s", got, first)
		}
		// index 没动：第二次提交的改动现在是「已暂存」
		if got := revOut(t, r.Root, "diff", "--cached", "--name-only"); !strings.Contains(got, "a.txt") {
			t.Errorf("soft 之后改动该在暂存区：%q", got)
		}
		// 工作区也没动
		if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "手头的改动\n" {
			t.Errorf("soft 不该碰工作区：a.txt = %q", got)
		}
	})

	t.Run("mixed 清暂存区留工作区", func(t *testing.T) {
		r := twoCommitRepo(t)
		first := revOut(t, r.Root, "rev-parse", "HEAD~1")
		if err := r.ResetTo(ctx, first, ResetMixed); err != nil {
			t.Fatalf("ResetTo: %v", err)
		}
		if got := revOut(t, r.Root, "diff", "--cached", "--name-only"); got != "" {
			t.Errorf("mixed 之后暂存区该是空的：%q", got)
		}
		if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "changed\n" {
			t.Errorf("mixed 不该碰工作区：a.txt = %q", got)
		}
	})

	t.Run("hard 连工作区一起回到那条提交", func(t *testing.T) {
		r := twoCommitRepo(t)
		first := revOut(t, r.Root, "rev-parse", "HEAD~1")
		write(t, filepath.Join(r.Root, "a.txt"), "会丢的改动\n")
		write(t, filepath.Join(r.Root, "u.txt"), "未跟踪\n")
		if err := r.ResetTo(ctx, first, ResetHard); err != nil {
			t.Fatalf("ResetTo: %v", err)
		}
		if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "one\ntwo\nthree\n" {
			t.Errorf("hard 之后 a.txt = %q", got)
		}
		if got := revOut(t, r.Root, "status", "--porcelain"); got != "?? u.txt" {
			t.Errorf("hard 之后该只剩未跟踪文件：%q", got)
		}
	})
}

// 这条钉的是界面那边一个「不做」的决定：reset --hard 会**静默地**把没收尾
// 的合并连同冲突一起抹掉（MERGE_HEAD 消失、工作区回到 HEAD）。所以提交
// 右键菜单在 Pending 里把重置整条灰掉 —— 哪天想放开，得先看这条。
func TestResetHardSilentlyClearsMerge(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	gitExpectFail(t, r.Root, "merge", "topic")
	if _, err := os.Stat(filepath.Join(r.GitDir, "MERGE_HEAD")); err != nil {
		t.Fatalf("造出来的合并该有 MERGE_HEAD：%v", err)
	}

	if err := r.ResetTo(ctx, "HEAD", ResetHard); err != nil {
		t.Fatalf("ResetTo: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.GitDir, "MERGE_HEAD")); !os.IsNotExist(err) {
		t.Errorf("reset --hard 之后 MERGE_HEAD 该没了（这就是界面禁它的原因）：%v", err)
	}
	if got := revOut(t, r.Root, "status", "--porcelain"); got != "" {
		t.Errorf("reset --hard 之后工作区该是干净的：%q", got)
	}
}

// 检出某个提交：HEAD 分离，内容就是那个提交的样子。
func TestCheckoutDetach(t *testing.T) {
	r := twoCommitRepo(t)
	ctx := context.Background()
	first := revOut(t, r.Root, "rev-parse", "HEAD~1")

	if err := r.CheckoutDetach(ctx, first); err != nil {
		t.Fatalf("CheckoutDetach: %v", err)
	}
	if got := revOut(t, r.Root, "rev-parse", "--symbolic-full-name", "HEAD"); got != "HEAD" {
		t.Errorf("rev-parse --symbolic-full-name HEAD = %q，want HEAD（分离）", got)
	}
	if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "one\ntwo\nthree\n" {
		t.Errorf("a.txt = %q，want 第一条提交的内容", got)
	}
}

// 建分支：不切的话 HEAD 不动；切的话用 checkout -b 一步到位。
func TestCreateBranch(t *testing.T) {
	ctx := context.Background()

	t.Run("不切换", func(t *testing.T) {
		r := twoCommitRepo(t)
		if err := r.CreateBranch(ctx, "feature/x", "", false); err != nil {
			t.Fatalf("CreateBranch: %v", err)
		}
		if got := revOut(t, r.Root, "rev-parse", "--symbolic-full-name", "HEAD"); got != "refs/heads/main" {
			t.Errorf("HEAD = %q，want 还在 main 上", got)
		}
		if got := revOut(t, r.Root, "rev-parse", "refs/heads/feature/x"); got != revOut(t, r.Root, "rev-parse", "HEAD") {
			t.Errorf("新分支该指向 HEAD")
		}
	})

	t.Run("从指定提交切换过去", func(t *testing.T) {
		r := twoCommitRepo(t)
		first := revOut(t, r.Root, "rev-parse", "HEAD~1")
		if err := r.CreateBranch(ctx, "from-first", first, true); err != nil {
			t.Fatalf("CreateBranch: %v", err)
		}
		if got := revOut(t, r.Root, "rev-parse", "--symbolic-full-name", "HEAD"); got != "refs/heads/from-first" {
			t.Errorf("HEAD = %q，want 附在新分支上", got)
		}
		if got := revOut(t, r.Root, "rev-parse", "HEAD"); got != first {
			t.Errorf("HEAD 在 %s，want %s", got, first)
		}
	})

	t.Run("重名被 git 拒", func(t *testing.T) {
		r := twoCommitRepo(t)
		if err := r.CreateBranch(ctx, "main", "", false); err == nil ||
			!strings.Contains(err.Error(), "already exists") {
			t.Errorf("重名该被拒：%v", err)
		}
	})
}

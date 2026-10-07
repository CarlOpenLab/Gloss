package gitrepo

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// conflictRepo 造一个 main 与 topic 在 a.txt 上冲突的仓库，当前在 main。
func conflictRepo(t *testing.T) *Repo {
	t.Helper()
	r := tempRepo(t)
	git(t, r.Root, "checkout", "-qb", "topic")
	write(t, filepath.Join(r.Root, "a.txt"), "topic\n")
	git(t, r.Root, "commit", "-qam", "topic")
	git(t, r.Root, "checkout", "-q", "main")
	write(t, filepath.Join(r.Root, "a.txt"), "main\n")
	git(t, r.Root, "commit", "-qam", "main")
	return r
}

// gitExpectFail 跑一条本来就该失败的命令：合并/变基撞冲突是它工作的样子，
// 不是命令坏了。用 git() 会把它 t.Fatalf 掉。
func gitExpectFail(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("git %v 本该失败（用来造冲突），却成功了：\n%s", args, out)
	}
}

// 干净的仓库不该凭空「卡住」。
func TestPendingNoneWhenClean(t *testing.T) {
	r := tempRepo(t)
	m, err := r.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Active() {
		t.Errorf("干净仓库的 Pending 该是零值，实际 %+v", m.Pending)
	}
}

func TestPendingMergeConflict(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	topicSha := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "topic"))

	gitExpectFail(t, r.Root, "merge", "topic")
	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Kind != PendingMerge {
		t.Fatalf("Kind = %v，want PendingMerge", m.Pending.Kind)
	}
	if m.Pending.Other != topicSha {
		t.Errorf("Other = %q，want 被并入的 %q", m.Pending.Other, topicSha)
	}
	if m.Detached || m.Branch != "main" {
		t.Errorf("合并中 HEAD 还该在 main 上：branch=%q detached=%v", m.Branch, m.Detached)
	}
}

// 变基撞冲突是全篇最关键的一次探测：现代 git 只留 rebase-merge/，但老
// 版本会同时留下 CHERRY_PICK_HEAD —— 判错一个，用户点的「继续」发出的
// 就是 git cherry-pick --continue。
func TestPendingRebaseConflict(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	onto := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "main"))

	git(t, r.Root, "checkout", "-q", "topic")
	gitExpectFail(t, r.Root, "rebase", "main")

	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Kind != PendingRebase {
		t.Fatalf("Kind = %v，want PendingRebase（不是 CherryPick）", m.Pending.Kind)
	}
	if m.Pending.Step != 1 || m.Pending.Total != 1 {
		t.Errorf("进度 = %d/%d，want 1/1", m.Pending.Step, m.Pending.Total)
	}
	// 变基期间 git 的 HEAD 是分离的；界面要把话说成「正在变基 topic」，
	// 靠的就是 Pending 里的这枚分支名（m.Branch 此时是短哈希）。
	if !m.Detached {
		t.Error("变基中途 Detached 该是 true —— 界面正是靠它改口")
	}
	if m.Pending.Branch != "topic" {
		t.Errorf("Pending.Branch = %q，want topic", m.Pending.Branch)
	}
	if m.Pending.Onto != onto {
		t.Errorf("Onto = %q，want %q", m.Pending.Onto, onto)
	}
}

// 老版本 git 的变基会同时留下 rebase-merge/ 和 CHERRY_PICK_HEAD；这里
// 手工补一个 CHERRY_PICK_HEAD，探测必须仍然先认 rebase。
func TestPendingRebaseBeatsCherryPickHead(t *testing.T) {
	r := conflictRepo(t)
	git(t, r.Root, "checkout", "-q", "topic")
	gitExpectFail(t, r.Root, "rebase", "main")
	write(t, filepath.Join(r.GitDir, "CHERRY_PICK_HEAD"), head(t, r.Root)+"\n")

	if p := r.pending(); p.Kind != PendingRebase {
		t.Errorf("Kind = %v，同时存在 CHERRY_PICK_HEAD 时也该先认 Rebase", p.Kind)
	}
}

func TestPendingCherryPickConflict(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	topicSha := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "topic"))

	gitExpectFail(t, r.Root, "cherry-pick", "topic")
	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Kind != PendingCherryPick {
		t.Fatalf("Kind = %v，want PendingCherryPick", m.Pending.Kind)
	}
	if m.Pending.Other != topicSha {
		t.Errorf("Other = %q，want 被拣的 %q", m.Pending.Other, topicSha)
	}
}

func TestPendingRevertConflict(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	write(t, filepath.Join(r.Root, "a.txt"), "one\nA\ntwo\nthree\n")
	git(t, r.Root, "commit", "-qam", "A")
	aSha := head(t, r.Root)
	// B 改的正是 A 插进去的那一行：回退 A 时必撞
	write(t, filepath.Join(r.Root, "a.txt"), "one\nB\ntwo\nthree\n")
	git(t, r.Root, "commit", "-qam", "B")

	gitExpectFail(t, r.Root, "revert", "--no-edit", aSha)
	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Kind != PendingRevert {
		t.Fatalf("Kind = %v，want PendingRevert", m.Pending.Kind)
	}
}

// git am 也住在 rebase-apply/ 里，靠目录里有没有 applying 文件区别于
// 变基的 apply 后端（实测）。用户在终端里 am 到一半，界面也得认出来。
func TestPendingAm(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	git(t, r.Root, "checkout", "-qb", "side")
	write(t, filepath.Join(r.Root, "b.txt"), "side\n")
	git(t, r.Root, "add", "b.txt")
	git(t, r.Root, "commit", "-qm", "side")
	patch := gitOut(t, r.Root, "format-patch", "-1", "--stdout")
	patchPath := filepath.Join(t.TempDir(), "p.diff")
	write(t, patchPath, patch)

	git(t, r.Root, "checkout", "-q", "main")
	write(t, filepath.Join(r.Root, "b.txt"), "other\n")
	git(t, r.Root, "add", "b.txt")
	git(t, r.Root, "commit", "-qm", "other")

	gitExpectFail(t, r.Root, "am", patchPath)
	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Kind != PendingAm {
		t.Fatalf("Kind = %v，want PendingAm", m.Pending.Kind)
	}

	if err := r.Abort(ctx, PendingAm); err != nil {
		t.Fatalf("Abort(am): %v", err)
	}
	if p := r.pending(); p.Active() {
		t.Errorf("中止后不该还卡着：%+v", p)
	}
}

// Continue 撞冲突解完的文件：合并没有挂编辑器（GIT_EDITOR=true 兜底），
// 直接用 MERGE_MSG 完成 —— 这条测试要是挂住，就说明那个兜底没生效。
func TestContinueMergeAfterResolve(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	gitExpectFail(t, r.Root, "merge", "topic")
	write(t, filepath.Join(r.Root, "a.txt"), "resolved\n")
	git(t, r.Root, "add", "a.txt")

	if err := r.Continue(ctx, PendingMerge); err != nil {
		t.Fatalf("Continue(merge): %v", err)
	}
	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Active() {
		t.Errorf("收尾后不该还卡着：%+v", m.Pending)
	}
	if m.Branch != "main" || m.Detached {
		t.Errorf("合并完还该在 main 上：branch=%q detached=%v", m.Branch, m.Detached)
	}
	if got := strings.TrimSpace(gitOut(t, r.Root, "log", "--merges", "--format=%s")); got != "Merge branch 'topic'" {
		t.Errorf("该恰好一条合并提交，实际 %q", got)
	}
	if out := strings.TrimSpace(gitOut(t, r.Root, "status", "--porcelain")); out != "" {
		t.Errorf("合并完工作区该干净：%q", out)
	}
}

func TestContinueRebaseAfterResolve(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	git(t, r.Root, "checkout", "-q", "topic")
	gitExpectFail(t, r.Root, "rebase", "main")
	write(t, filepath.Join(r.Root, "a.txt"), "resolved\n")
	git(t, r.Root, "add", "a.txt")

	if err := r.Continue(ctx, PendingRebase); err != nil {
		t.Fatalf("Continue(rebase): %v", err)
	}
	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Active() {
		t.Errorf("收尾后不该还卡着：%+v", m.Pending)
	}
	// 变基收尾要回到原分支上（HEAD 重新附回 refs/heads/topic）
	if m.Branch != "topic" || m.Detached {
		t.Errorf("变基完该回到 topic 上：branch=%q detached=%v", m.Branch, m.Detached)
	}
	if got := gitOut(t, r.Root, "log", "--format=%s", "-2"); got != "topic\nmain\n" {
		t.Errorf("重放没落地：log = %q", got)
	}
}

func TestContinueCherryPickAfterResolve(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	gitExpectFail(t, r.Root, "cherry-pick", "topic")
	write(t, filepath.Join(r.Root, "a.txt"), "resolved\n")
	git(t, r.Root, "add", "a.txt")

	if err := r.Continue(ctx, PendingCherryPick); err != nil {
		t.Fatalf("Continue(cherry-pick): %v", err)
	}
	if p := r.pending(); p.Active() {
		t.Errorf("收尾后不该还卡着：%+v", p)
	}
	if got := gitOut(t, r.Root, "log", "-1", "--format=%s"); strings.TrimSpace(got) != "topic" {
		t.Errorf("拣选该沿用原提交信息：%q", got)
	}
}

func TestAbortMergeRestores(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	before := head(t, r.Root)
	gitExpectFail(t, r.Root, "merge", "topic")

	if err := r.Abort(ctx, PendingMerge); err != nil {
		t.Fatalf("Abort(merge): %v", err)
	}
	if got := head(t, r.Root); got != before {
		t.Errorf("中止后 HEAD 该回到 %s，实际 %s", before, got)
	}
	if out := strings.TrimSpace(gitOut(t, r.Root, "status", "--porcelain")); out != "" {
		t.Errorf("中止后工作区该干净：%q", out)
	}
}

// 变基中止不只是「回退」，还得把 HEAD 从分离态接回原来的分支。
func TestAbortRebaseReturnsToBranch(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	git(t, r.Root, "checkout", "-q", "topic")
	before := head(t, r.Root)
	gitExpectFail(t, r.Root, "rebase", "main")

	if err := r.Abort(ctx, PendingRebase); err != nil {
		t.Fatalf("Abort(rebase): %v", err)
	}
	if got := headRef(t, r.Root); got != "refs/heads/topic" {
		t.Errorf("HEAD = %q，want refs/heads/topic", got)
	}
	if got := head(t, r.Root); got != before {
		t.Errorf("中止后 HEAD 该回到 %s，实际 %s", before, got)
	}
}

// 跳过 = 丢掉这一步：topic 唯一的提交撞了冲突被跳过，变基完成时
// topic 落在 main 上，那条提交不进历史。
func TestSkipDropsReplayedCommit(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	mainSha := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "main"))
	git(t, r.Root, "checkout", "-q", "topic")
	gitExpectFail(t, r.Root, "rebase", "main")

	if err := r.Skip(ctx, PendingRebase); err != nil {
		t.Fatalf("Skip(rebase): %v", err)
	}
	if got := headRef(t, r.Root); got != "refs/heads/topic" {
		t.Errorf("HEAD = %q，want refs/heads/topic", got)
	}
	if got := head(t, r.Root); got != mainSha {
		t.Errorf("跳过之后 topic 该落在 main（%s）上，实际 %s", mainSha, got)
	}
	if got := strings.TrimSpace(gitOut(t, r.Root, "log", "-1", "--format=%s")); got != "main" {
		t.Errorf("被跳过的提交不该在历史里：log -1 = %q", got)
	}
}

func TestSkipMergeRefused(t *testing.T) {
	r := tempRepo(t)
	if err := r.Skip(context.Background(), PendingMerge); err == nil {
		t.Fatal("git merge 没有 --skip，这扇门该被关上")
	}
}

// linked worktree 的 .git 是一个文件 —— GitDir 必须来自 --absolute-git-dir，
// 卡住的探测（纯 stat 标记文件）才不会在每个 worktree 里失灵；而且探测
// 按 gitdir 各算各的：worktree 里合并中，主工作区不该跟着「卡住」。
func TestGitDirAndPendingInLinkedWorktree(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()

	wt := filepath.Join(t.TempDir(), "wt")
	git(t, r.Root, "worktree", "add", "-q", "-b", "wtbranch", wt)

	wr, err := Open(ctx, wt)
	if err != nil {
		t.Fatalf("Open(worktree): %v", err)
	}
	if wr.GitDir == filepath.Join(wr.Root, ".git") {
		t.Fatal("worktree 的 GitDir 不该是 <root>/.git —— 那里是个文件")
	}
	if dir := filepath.Dir(wr.GitDir); filepath.Base(dir) != "worktrees" || filepath.Dir(dir) != r.GitDir {
		t.Errorf("GitDir = %q，want 主仓库 .git/worktrees/<名字>", wr.GitDir)
	}

	gitExpectFail(t, wt, "merge", "topic")
	wm, err := wr.Load(ctx)
	if err != nil {
		t.Fatalf("Load(worktree): %v", err)
	}
	if wm.Pending.Kind != PendingMerge {
		t.Errorf("worktree 里的合并冲突该被认出来，实际 %+v", wm.Pending)
	}

	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Active() {
		t.Errorf("主工作区没在合并，不该跟着卡住：%+v", m.Pending)
	}
}

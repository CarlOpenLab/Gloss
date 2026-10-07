package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 远程这三个动作用**真的远程**验：一个本地裸仓库当 origin。不这样的话
// 「推上去了吗」只能问我们自己 —— 而推送的全部意义就是仓库之外的那一份。
func bareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	git(t, dir, "init", "--bare", "-b", "main")
	return dir
}

// linkedRepo 是 tempRepo 加上一个真的远程。
func linkedRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	r := tempRepo(t)
	bare := bareRemote(t)
	git(t, r.Root, "remote", "add", "origin", bare)
	return r, bare
}

// remoteHead 是远程那条分支现在指向哪儿。
func remoteHead(t *testing.T, bare, branch string) string {
	t.Helper()
	return strings.TrimSpace(gitOut(t, bare, "rev-parse", "refs/heads/"+branch))
}

// cloneOf 是「另一个人」的工作副本：同一份远程的另一个克隆。远程相关的
// 测试都要一个能独立推拉的对方，否则「抓下来的确实是别人的东西」证明不了。
func cloneOf(t *testing.T, bare string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "clone", "-q", bare, ".")
	git(t, dir, "config", "user.email", "other@example.com")
	git(t, dir, "config", "user.name", "Other")
	git(t, dir, "config", "commit.gpgsign", "false") // 别顺走机器上的全局签名设置
	return dir
}

// commitIn 在 dir 里写一个文件并提交。
func commitIn(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	write(t, filepath.Join(dir, name), content)
	git(t, dir, "add", name)
	git(t, dir, "commit", "-m", msg)
}

// 抓取：远程上有了新提交，原样抓下来。
func TestFetchUpdatesRemoteRefs(t *testing.T) {
	r, bare := linkedRepo(t)
	ctx := context.Background()
	if err := r.Push(ctx, "main"); err != nil {
		t.Fatalf("先推一次: %v", err)
	}

	// 别人（另一份克隆）往远程推了一个提交
	other := cloneOf(t, bare)
	commitIn(t, other, "b.txt", "别人写的\n", "别人的提交")
	git(t, other, "push", "-q", "origin", "main")

	// 抓之前本地还看不见它
	before := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "refs/remotes/origin/main"))
	if err := r.Fetch(ctx); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	after := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "refs/remotes/origin/main"))
	if before == after {
		t.Fatal("抓取之后 origin/main 该动了")
	}
	if after != remoteHead(t, bare, "main") {
		t.Errorf("origin/main = %s，远程上是 %s", after, remoteHead(t, bare, "main"))
	}
	// 抓取不动工作区：本地 main 还在原地
	if got := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "HEAD")); got == after {
		t.Error("抓取不该动本地分支")
	}
}

// 抓取要 prune：远端删掉的分支，本地那条远程分支也得消失。
func TestFetchPrunesDeletedBranches(t *testing.T) {
	r, bare := linkedRepo(t)
	ctx := context.Background()
	if err := r.Push(ctx, "main"); err != nil {
		t.Fatalf("先推一次: %v", err)
	}
	git(t, r.Root, "branch", "gone")
	git(t, r.Root, "push", "-q", "-u", "origin", "gone")
	if err := r.Fetch(ctx); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gitOut(t, r.Root, "for-each-ref", "--format=%(refname)", "refs/remotes/origin/gone") == "" {
		t.Fatal("刚推上去的 origin/gone 该在")
	}

	git(t, bare, "branch", "-D", "gone")
	if err := r.Fetch(ctx); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got := gitOut(t, r.Root, "for-each-ref", "--format=%(refname)", "refs/remotes/origin/gone"); got != "" {
		t.Errorf("远端删了之后本地这条也该没：%q", got)
	}
}

// 拉取：把远程的新提交合进当前分支（这里是快进）。
func TestPullFastForwards(t *testing.T) {
	r, bare := linkedRepo(t)
	ctx := context.Background()
	if err := r.Push(ctx, "main"); err != nil {
		t.Fatalf("先推一次: %v", err)
	}

	other := cloneOf(t, bare)
	commitIn(t, other, "c.txt", "别人的一行\n", "远程上的提交")
	git(t, other, "push", "-q", "origin", "main")

	if err := r.Pull(ctx); err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "HEAD")); got != remoteHead(t, bare, "main") {
		t.Errorf("拉取之后本地 HEAD = %s，远程是 %s", got, remoteHead(t, bare, "main"))
	}
	if _, err := os.ReadFile(filepath.Join(r.Root, "c.txt")); err != nil {
		t.Errorf("拉下来的文件该在工作区里：%v", err)
	}
}

// 推送：第一次推一条没有上游的分支，要顺手把上游设好 —— 不然下一次还得再
// 折腾一遍。
func TestPushSetsUpstreamOnFirstPush(t *testing.T) {
	r, bare := linkedRepo(t)
	ctx := context.Background()
	git(t, r.Root, "checkout", "-q", "-b", "feature")

	if err := r.Push(ctx, "feature"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if got := remoteHead(t, bare, "feature"); got == "" {
		t.Fatal("远程上没有这条分支")
	}
	up := strings.TrimSpace(gitOut(t, r.Root, "for-each-ref", "--format=%(upstream)", "refs/heads/feature"))
	if up != "refs/remotes/origin/feature" {
		t.Errorf("推完的上游是 %q，want refs/remotes/origin/feature", up)
	}
}

// 已经有上游时直接推，不用再给参数。
func TestPushUsesExistingUpstream(t *testing.T) {
	r, bare := linkedRepo(t)
	ctx := context.Background()
	if err := r.Push(ctx, "main"); err != nil {
		t.Fatalf("第一次推: %v", err)
	}
	write(t, filepath.Join(r.Root, "a.txt"), "one\ntwo\nthree\nfour\n")
	git(t, r.Root, "add", "a.txt")
	git(t, r.Root, "commit", "-m", "第二个提交")

	if err := r.Push(ctx, "main"); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if got := remoteHead(t, bare, "main"); got != strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "HEAD")) {
		t.Errorf("远程 main = %s，本地 HEAD = %s", got, strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "HEAD")))
	}
}

// 推不上去（远程有你没有的提交）：错误里是 git 的原话，界面翻成「先拉取」。
func TestPushRejectedWhenRemoteIsAhead(t *testing.T) {
	r, bare := linkedRepo(t)
	ctx := context.Background()
	if err := r.Push(ctx, "main"); err != nil {
		t.Fatalf("第一次推: %v", err)
	}

	other := cloneOf(t, bare)
	commitIn(t, other, "d.txt", "别人先推的\n", "别人先推的提交")
	git(t, other, "push", "-q", "origin", "main")

	commitIn(t, r.Root, "e.txt", "我这边也提交了\n", "我这边的提交")

	err := r.Push(ctx, "main")
	if err == nil {
		t.Fatal("远程在前面，这次推送该被拒")
	}
	if f := Describe(OpPush, err); f.Title != "远程有你本地没有的提交" {
		t.Errorf("标题 = %q，want「远程有你本地没有的提交」（原话：%v）", f.Title, err)
	}
}

// 一条远程都没有：明说，不去猜一个。
func TestPushWithoutRemote(t *testing.T) {
	r := tempRepo(t)

	err := r.Push(context.Background(), "main")
	if !errors.Is(err, ErrNoRemote) {
		t.Fatalf("err = %v，want ErrNoRemote", err)
	}
	if f := Describe(OpPush, err); f.Title != "这个仓库没有远程" {
		t.Errorf("标题 = %q", f.Title)
	}
}

// 有远程但没有 origin：也不猜 —— 多远程时哪个才是「那个远程」只有用户知道。
func TestPushWithoutOriginDoesNotGuess(t *testing.T) {
	r := tempRepo(t)
	bare := bareRemote(t)
	git(t, r.Root, "remote", "add", "upstream", bare)

	if err := r.Push(context.Background(), "main"); !errors.Is(err, ErrNoRemote) {
		t.Fatalf("err = %v，want ErrNoRemote（不该挑一个远程来推）", err)
	}
	// 而且真的一个字节都没推出去
	if got := strings.TrimSpace(gitOut(t, bare, "for-each-ref", "--format=%(refname)")); got != "" {
		t.Errorf("远程上不该有任何分支：%q", got)
	}
}

// 没有远程时抓取不该报错（git 自己就是静默退 0）—— 界面那边靠 Remotes
// 判按钮，不靠这条命令失败。
func TestFetchWithoutRemoteIsQuiet(t *testing.T) {
	r := tempRepo(t)
	if err := r.Fetch(context.Background()); err != nil {
		t.Errorf("没有远程时 fetch --all 本来就什么都不做：%v", err)
	}
}

// 分离 HEAD 上推送：这里 branch 是短哈希，绝不能把它当成一条分支名推出去
// （git 会报「src refspec 不匹配」），要说「先有分支」。
func TestPushOnDetachedHead(t *testing.T) {
	r, bare := linkedRepo(t)
	ctx := context.Background()
	if err := r.Push(ctx, "main"); err != nil {
		t.Fatalf("第一次推: %v", err)
	}
	git(t, r.Root, "checkout", "-q", "--detach", "HEAD")
	hash := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "--short", "HEAD"))

	err := r.Push(ctx, hash) // 界面在分离 HEAD 上就是这个值
	if !errors.Is(err, ErrNoBranch) {
		t.Fatalf("err = %v，want ErrNoBranch", err)
	}
	f := Describe(OpPush, err)
	if f.Title != "现在没有分支在手上" {
		t.Errorf("标题 = %q", f.Title)
	}
	if !strings.Contains(f.Hint, "分支") {
		t.Errorf("该指出去哪儿建分支：%q", f.Hint)
	}
	// 而且远程上不能凭这一下多出任何一条分支
	if got := strings.TrimSpace(gitOut(t, bare, "for-each-ref", "--format=%(refname)")); got != "refs/heads/main" {
		t.Errorf("远程上只该有 main：%q", got)
	}
}

// 凭证：我们没有输入口令的地方，所以出路只有「在终端里登录一次」。
func TestRemoteNeedsCredentials(t *testing.T) {
	f := Describe(OpFetch, errors.New(
		"fatal: could not read Username for 'https://github.com': terminal prompts disabled"))
	if f.Title != "远程要你登录" {
		t.Errorf("标题 = %q", f.Title)
	}
	if !strings.Contains(f.Hint, "终端") {
		t.Errorf("该指向终端：%q", f.Hint)
	}
}

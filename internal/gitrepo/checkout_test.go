package gitrepo

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// 检出本地分支要把 HEAD **附**上去，不是分离。
//
// 这条专门钉住「传短名不传全名」：`git checkout refs/heads/topic` 会成功、
// 会切到对的提交，但 HEAD 是分离的 —— 界面看上去一切正常，用户在上面提交
// 之后就丢了（没有分支接着）。只有问 git 本人才能发现。
func TestCheckoutLocalAttachesHead(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "branch", "topic")

	if err := r.CheckoutLocal(ctx, "topic"); err != nil {
		t.Fatalf("CheckoutLocal: %v", err)
	}

	if got := headRef(t, r.Root); got != "refs/heads/topic" {
		t.Errorf("HEAD 指向 %q，want refs/heads/topic（分离了吗？）", got)
	}
}

// 分支名和标签重名时，checkout 必须落在分支上 —— 短名消歧的顺序在
// checkout 这里是 heads 优先（和 rev-parse 相反），这条把它钉住。
func TestCheckoutLocalPrefersBranchOverTag(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "branch", "v1")
	git(t, r.Root, "checkout", "-q", "-b", "other")
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "other 上的提交")
	git(t, r.Root, "tag", "v1") // v1 标签指向 other，v1 分支还在 init
	git(t, r.Root, "checkout", "-q", "main")

	if err := r.CheckoutLocal(ctx, "v1"); err != nil {
		t.Fatalf("CheckoutLocal: %v", err)
	}
	if got := headRef(t, r.Root); got != "refs/heads/v1" {
		t.Errorf("HEAD 指向 %q，want refs/heads/v1", got)
	}
	// 落在分支上 = 拿到的是分支那个提交（init），不是标签那个（other 上的）
	subject := strings.TrimSpace(gitOut(t, r.Root, "log", "-1", "--format=%s"))
	if subject != "init" {
		t.Errorf("检出的是 %q，want init（标签指向的是 other 上的提交）", subject)
	}
}

// 从远程分支检出：建一条本地分支并设好上游。
func TestCheckoutRemoteTracksUpstream(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	// 只有远程上那条，本地还没有 —— 这正是「把远程分支拉下来」的场景
	git(t, r.Root, "update-ref", "refs/remotes/origin/feature/x", "HEAD")
	git(t, r.Root, "remote", "add", "origin", r.Root)

	if err := r.CheckoutRemote(ctx, "feature/x", "refs/remotes/origin/feature/x"); err != nil {
		t.Fatalf("CheckoutRemote: %v", err)
	}

	if got := headRef(t, r.Root); got != "refs/heads/feature/x" {
		t.Errorf("HEAD 指向 %q，want refs/heads/feature/x", got)
	}
	up := gitOut(t, r.Root, "for-each-ref", "--format=%(upstream)", "refs/heads/feature/x")
	if strings.TrimSpace(up) != "refs/remotes/origin/feature/x" {
		t.Errorf("上游是 %q，want refs/remotes/origin/feature/x", strings.TrimSpace(up))
	}
}

// 本地重名时 git 自己会拒 —— 上层靠这个错误降级成「检出本地那条」。
func TestCheckoutRemoteRejectsExistingLocalBranch(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "branch", "feature/x")
	git(t, r.Root, "update-ref", "refs/remotes/origin/feature/x", "HEAD")

	err := r.CheckoutRemote(ctx, "feature/x", "refs/remotes/origin/feature/x")
	if err == nil {
		t.Fatal("本地已有同名分支，这次检出该失败")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("错误该是「已经有了」：%v", err)
	}
}

// 检出标签：切过去了，而且是分离 HEAD（标签不是分支，没有「附」这一说）。
func TestCheckoutTagDetaches(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "tag", "v1")
	git(t, r.Root, "checkout", "-q", "-b", "other")
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "后来的提交")
	git(t, r.Root, "checkout", "-q", "main")

	if err := r.CheckoutTag(ctx, "v1"); err != nil {
		t.Fatalf("CheckoutTag: %v", err)
	}

	if got := headRef(t, r.Root); got != "HEAD" {
		t.Errorf("检出标签之后 HEAD 该是分离的，实际指向 %q", got)
	}
	if subject := strings.TrimSpace(gitOut(t, r.Root, "log", "-1", "--format=%s")); subject != "init" {
		t.Errorf("检出的提交是 %q，want init", subject)
	}
}

// 工作区有挡道的改动时，checkout 失败，而且那句话里带着文件名 ——
// 界面把 Detail 摆出来，用户才知道是哪个文件。
func TestCheckoutBlockedByLocalChanges(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "checkout", "-q", "-b", "other")
	write(t, filepath.Join(r.Root, "b.txt"), "只在这条分支上\n")
	git(t, r.Root, "add", "b.txt")
	git(t, r.Root, "commit", "-m", "加 b.txt")
	git(t, r.Root, "checkout", "-q", "main")
	// main 上没有 b.txt，工作区里却有一个同名的未跟踪文件
	write(t, filepath.Join(r.Root, "b.txt"), "未跟踪的同名文件\n")

	err := r.CheckoutLocal(ctx, "other")
	if err == nil {
		t.Fatal("未跟踪文件挡着，这次检出该失败")
	}
	if !strings.Contains(err.Error(), "b.txt") {
		t.Errorf("git 的原话里该有那个文件名：%v", err)
	}
}

// headRef 是 HEAD 指向哪儿：refs/heads/x 表示附在那条分支上，HEAD 表示分离。
//
// 用 --symbolic-full-name 而不是一直习惯用的 symbolic-ref --short：分支名
// 和标签重名时，--short 会打印 "heads/v1"（为了避免歧义它只去掉 refs/），
// 看着像检出错地方了，其实 HEAD 好好地附在 refs/heads/v1 上。
func headRef(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(gitOut(t, dir, "rev-parse", "--symbolic-full-name", "HEAD"))
}

// 轻量标签：不 -a，指到提交上。
func TestCreateTagLightweight(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	if err := r.CreateTag(ctx, "v0.1", "", ""); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if got := strings.TrimSpace(gitOut(t, r.Root, "cat-file", "-t", "v0.1")); got != "commit" {
		t.Errorf("轻量标签该直接指向 commit，实际 %q", got)
	}
}

// 附注标签：-a -m，是一个 tag 对象，带着说明。
func TestCreateTagAnnotated(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	if err := r.CreateTag(ctx, "v1.0", "第一个版本\n第二行说明", ""); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	if got := strings.TrimSpace(gitOut(t, r.Root, "cat-file", "-t", "v1.0")); got != "tag" {
		t.Fatalf("附注标签该是个 tag 对象，实际 %q", got)
	}
	body := gitOut(t, r.Root, "tag", "-n99", "-l", "v1.0")
	if !strings.Contains(body, "第一个版本") || !strings.Contains(body, "第二行说明") {
		t.Errorf("说明没进去：%q", body)
	}
	// 标签和提交的信息是两回事，提交信息不该被顶掉
	if subject := strings.TrimSpace(gitOut(t, r.Root, "log", "-1", "--format=%s")); subject != "init" {
		t.Errorf("打标签不该动提交：%q", subject)
	}
}

// 打在指定的提交上，不是 HEAD。
func TestCreateTagAtRevision(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	first := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "HEAD"))
	git(t, r.Root, "commit", "-q", "--allow-empty", "-m", "第二个提交")

	if err := r.CreateTag(ctx, "old", "", first); err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	got := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "old^{commit}"))
	if got != first {
		t.Errorf("标签落在 %s，want %s", got, first)
	}
}

// 重名和非法名：都靠 git 自己拒，错误原话留给界面翻。
func TestCreateTagFailures(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "tag", "v1")

	if err := r.CreateTag(ctx, "v1", "", ""); err == nil {
		t.Error("重名该失败")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("重名的错误不对：%v", err)
	}

	if err := r.CreateTag(ctx, "带 空格", "", ""); err == nil {
		t.Error("带空格的标签名该失败")
	} else if !strings.Contains(err.Error(), "not a valid tag name") {
		t.Errorf("非法名的错误不对：%v", err)
	}
}

// 上面几条里出错分支要能分辨：errors.Is 不上就靠字符串，这里只是把
// 「错误确实来自 git」这件事记一笔 —— runIn 在 stderr 为空时会退回
// exit status，那种情况不该出现在这几条里。
func TestGitErrorsCarryStderr(t *testing.T) {
	r := tempRepo(t)
	err := r.CheckoutLocal(context.Background(), "不存在的分支")
	if err == nil {
		t.Fatal("检出一条不存在的分支该失败")
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("不该是 context 错误：%v", err)
	}
	if !strings.Contains(err.Error(), "did not match") && !strings.Contains(err.Error(), "pathspec") {
		t.Errorf("该是 git 自己的话：%v", err)
	}
}

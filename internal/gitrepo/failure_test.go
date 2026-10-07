package gitrepo

import (
	"errors"
	"strings"
	"testing"
)

// 常见的 git 报错都要有中文标题和「怎么办」，原话留在 Detail 里。
func TestDescribeKnownFailures(t *testing.T) {
	cases := []struct {
		op    Op
		msg   string
		title string
	}{
		{OpFetch, "fatal: could not read Username for 'https://github.com': terminal prompts disabled", "远程要你登录"},
		{OpPush, "remote: Invalid username or password.\nfatal: Authentication failed for 'https://github.com/x/y.git/'", "远程没认这个账号"},
		{OpPush, "git@github.com: Permission denied (publickey).\nfatal: Could not read from remote repository.", "远程不认这把 SSH key"},
		{OpFetch, "fatal: unable to access 'https://example.com/x/': Could not resolve host: example.com", "连不上远程"},
		{OpPush, "! [rejected]        main -> main (fetch first)\nerror: failed to push some refs to 'origin'", "远程有你本地没有的提交"},
		{OpPush, "remote: error: GH006: Protected branch update failed for refs/heads/main.", "远程拒绝了这个推送"},
		// 真实的那段（git 2.27+ 在没配 pull.rebase / pull.ff 时的原话）：
		// 八行 hint 之后再给一句 fatal。认的是最后那句。
		{OpPull, "hint: You have divergent branches and need to specify how to reconcile them.\n" +
			"hint: You can do so by running one of the following commands sometime before\n" +
			"hint: your next pull:\n" +
			"hint:\n" +
			"hint:   git config pull.rebase false  # merge\n" +
			"hint:   git config pull.rebase true   # rebase\n" +
			"hint:   git config pull.ff only       # fast-forward only\n" +
			"fatal: Need to specify how to reconcile divergent branches.", "本地和远程分叉了"},
		{OpPull, "CONFLICT (content): Merge conflict in a.txt\nAutomatic merge failed; fix conflicts and then commit the result.", "合并有冲突"},
		{OpPush, "fatal: The current branch feature has no upstream branch.", "这条分支还没有上游"},
		{OpPush, "fatal: The upstream branch of your current branch does not match\nthe name of your current branch.", "上游和本地分支不同名"},
		{OpPush, "fatal: You didn't specify any refspecs to push, and push.default is \"nothing\".", "git 配置成不推任何东西"},
		{OpPush, "fatal: You are not currently on a branch.", "现在没有分支在手上"},
		{OpPull, "fatal: You are not currently on a branch.", "现在没有分支在手上"},
		{OpCheckout, "error: Your local changes to the following files would be overwritten by checkout:\n\ta.txt", "有未提交的改动挡着"},
		{OpStage, "fatal: Unable to create '/tmp/x/.git/index.lock': File exists.", "另一个 git 进程正占着仓库"},
		{OpCommit, "*** Please tell me who you are.\nfatal: unable to auto-detect email address", "还没配 git 用户名和邮箱"},
		{OpCommit, "error: gpg failed to sign the data", "GPG 签名失败"},
		{OpCommit, "Aborting commit due to empty commit message.", "提交信息是空的"},
		{OpCommit, "nothing to commit, working tree clean", "没有可提交的东西"},
		{OpCommit, "husky - pre-commit hook exited with code 1 (error)", "提交被仓库的钩子拦下了"},
		{OpStage, "error: patch does not apply", "这段改动落不进 index"},
		{OpTag, "fatal: tag 'v1.0' already exists", "已经有同名标签"},
		{OpTag, "fatal: 'a b' is not a valid tag name.", "这个名字不能当标签"},
		{OpCheckout, "fatal: pathspec 'x' did not match any file(s) known to git", "这条引用已经不在了"},
	}

	for _, c := range cases {
		f := Describe(c.op, errors.New(c.msg))
		if f.Title != c.title {
			t.Errorf("Describe(%q) 标题 = %q, want %q", c.msg, f.Title, c.title)
		}
		if f.Hint == "" {
			t.Errorf("Describe(%q) 没有给出「怎么办」", c.msg)
		}
		if f.Empty() {
			t.Errorf("Describe(%q) 不该是空失败", c.msg)
		}
		// 标题永远是人话，绝不裸甩 git 原文
		for _, bad := range []string{"fatal:", "error:", "CONFLICT"} {
			if strings.Contains(f.Title, bad) {
				t.Errorf("标题里不该出现 %q：%q", bad, f.Title)
			}
		}
	}
}

// 认不出的错误：兜底标题说清是哪一步没成，原话留给 Detail 可查。
func TestDescribeUnknownKeepsDetail(t *testing.T) {
	f := Describe(OpPush, errors.New("fatal: something we have never seen"))
	if f.Title != "推送没成功" {
		t.Errorf("标题 = %q, want 推送没成功", f.Title)
	}
	if !strings.Contains(f.Detail, "something we have never seen") {
		t.Errorf("Detail 该保留 git 原话: %q", f.Detail)
	}
}

// 竞态不是错误怪罪用户：「这个改动已经变了」要自成一档。
func TestDescribeHunkGone(t *testing.T) {
	f := Describe(OpStage, ErrHunkGone)
	if f.Title != "这个改动已经变了" {
		t.Errorf("标题 = %q", f.Title)
	}
	if f.Hint == "" {
		t.Error("该告诉用户下一步（刷过了，重看一眼）")
	}
}

// Detail 挑错行留原话，最多 3 行、200 字。
func TestGitDetailTrims(t *testing.T) {
	msg := "warning: 一行废话\nfatal: 真的错\nremote: 远端的话\nhint: 提示\n"
	if got := gitDetail(msg); strings.Contains(got, "废话") {
		t.Errorf("警告行不该被挑进来: %q", got)
	}

	long := "fatal: " + strings.Repeat("很长的中文错误信息", 40)
	got := gitDetail(long)
	if n := len([]rune(got)); n > detailMax+1 { // +1 是截断省略号
		t.Errorf("Detail 长度 %d 该被截到 %d 以内", n, detailMax)
	}
}

// hint 是 git 的建议，不是「出了什么事」—— 有真的错误行时，建议要让位。
//
// 这条是「拉取时分支分叉了」逼出来的：那条消息是八行 hint 加末尾一句
// fatal，按行数截只会截到 hint 的残句，用户看到的 Detail 就成了
// 「hint: or --ff-only on the command line to override the configured …」。
func TestGitDetailPrefersErrorOverHint(t *testing.T) {
	msg := "hint: 第一句建议\nhint: 第二句建议\nhint: 第三句建议\n" +
		"fatal: 真正出了什么事"
	got := gitDetail(msg)
	if !strings.Contains(got, "真正出了什么事") {
		t.Errorf("有 fatal 时该留 fatal：%q", got)
	}
	if strings.Contains(got, "建议") {
		t.Errorf("有真错误时不该再摆 git 的建议：%q", got)
	}

	// 只有建议时也不留空
	only := "hint: 第一句建议\nhint: 第二句建议"
	if got := gitDetail(only); !strings.Contains(got, "建议") {
		t.Errorf("只有 hint 时它就是全部内容：%q", got)
	}
}

// 缩进的续行跟着它上头那一行走（checkout 被挡时下面挂的是文件名清单）。
// 上头的行没被挑走时，续行也不该自己冒出来。
func TestGitDetailKeepsContinuations(t *testing.T) {
	msg := "error: The following untracked working tree files would be overwritten:\n" +
		"\tb.txt\n\tinternal/x.go\nPlease move or remove them before you switch branches."
	got := gitDetail(msg)
	for _, want := range []string{"b.txt", "internal/x.go"} {
		if !strings.Contains(got, want) {
			t.Errorf("续行 %q 该跟着走：%q", want, got)
		}
	}
	if strings.Contains(got, "Please move") {
		t.Errorf("没被挑走的那行底下的话不该跟进来：%q", got)
	}
}

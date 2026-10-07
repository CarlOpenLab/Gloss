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
		{OpPull, "CONFLICT (content): Merge conflict in a.txt\nAutomatic merge failed; fix conflicts and then commit the result.", "拉取撞上了冲突"},
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

		// —— 阶段 D：卡在合并/变基中间、分支与远程的收尾情形 ——
		// 原话全部照实测抄（git 2.50.1），连换行都保留：模式匹配的就是它们。
		{OpPush, "! [rejected]        v1 -> v1 (already exists)\nerror: failed to push some refs to '/tmp/bare'\nhint: Updates were rejected because the tag already exists in the remote.", "远程上已经有这个标签"},
		{OpForcePush, "! [rejected]        victim -> victim (stale info)\nerror: failed to push some refs to '/tmp/bare'", "远程上有你还没见过的提交"},
		{OpPush, "fatal: No configured push destination.", "这个仓库没有推送目标"},

		{OpRebase, "fatal: no rebase in progress", "这一步已经不在进行中了"},
		{OpCherryPick, "error: no cherry-pick or revert in progress\nfatal: cherry-pick failed", "这一步已经不在进行中了"},
		{OpMerge, "fatal: There is no merge in progress (MERGE_HEAD missing).", "这一步已经不在进行中了"},
		{OpMerge, "fatal: There is no merge to abort (MERGE_HEAD missing).", "这一步已经不在进行中了"},
		{OpMerge, "fatal: You have not concluded your merge (MERGE_HEAD exists).", "上一步还没收尾"},
		{OpRebase, "fatal: It seems that there is already a rebase-merge directory, and\n" +
			"I wonder if you are in the middle of another rebase.", "上一步还没收尾"},

		{OpMerge, "error: Merging is not possible because you have unmerged files.\n" +
			"hint: Fix them up in the work tree, and then use 'git add/rm <file>'\n" +
			"fatal: Exiting because of an unresolved conflict.", "还有文件没解决完"},
		{OpCherryPick, "error: Cherry-picking is not possible because you have unmerged files.\n" +
			"fatal: cherry-pick failed", "还有文件没解决完"},
		// 变基的「还没解决完」不叫 unmerged files：逐文件一句 needs merge，
		// 再加一句 You must edit（实测）。
		{OpRebase, "a.txt: needs merge\nYou must edit all merge conflicts and then\n" +
			"mark them as resolved using git add", "还有文件没解决完"},
		{OpCheckout, "error: you need to resolve your current index first\na.txt: needs merge", "有冲突没收尾"},
		{OpStash, "error: could not write index\na.txt: needs merge", "有冲突没收尾"},
		// 删除-修改冲突里点了「没有内容」的那一边（实测 git 2.50 的原话）。
		// 界面本来靠 stage 提前把它换成「接受删除」，撞到这里说明状态在
		// 点下去的一瞬间变了。
		{OpConflict, "error: path 'f.txt' does not have our version", "这一边已经没有这个文件了"},
		{OpConflict, "error: path 'f.txt' does not have their version", "这一边已经没有这个文件了"},

		{OpCherryPick, "The previous cherry-pick is now empty, possibly due to conflict resolution.\n" +
			"If you wish to commit it anyway, use:\n\n    git commit --allow-empty\n\n" +
			"Otherwise, please use 'git cherry-pick --skip'", "这一步没有产生任何改动"},
		{OpCherryPick, "error: commit 6f358a43796a5118ea78706a84dd2cec98badb01 is a merge but no -m option was given.\n" +
			"fatal: cherry-pick failed", "这是一个合并提交"},
		{OpMerge, "CONFLICT (content): Merge conflict in a.txt\n" +
			"Automatic merge failed; fix conflicts and then commit the result.", "合并撞上了冲突"},
		{OpRebase, "Auto-merging a.txt\nCONFLICT (content): Merge conflict in a.txt\n" +
			"error: could not apply 9a9a9a9... topic", "变基撞上了冲突"},
		{OpStash, "Auto-merging a.txt\nCONFLICT (content): Merge conflict in a.txt\n" +
			"Unmerged paths:\n\tboth modified:   a.txt", "储藏的改动和手头的文件冲突了"},

		{OpRebase, "error: cannot rebase: You have unstaged changes.\nerror: Please commit or stash them.", "有未提交的改动挡着"},
		{OpPull, "error: cannot pull with rebase: You have unstaged changes.\nerror: Please commit or stash them.", "有未提交的改动挡着"},
		{OpBranch, "fatal: a branch named 'main' already exists", "本地已经有这条分支了"},
		{OpBranch, "fatal: 'bad name' is not a valid branch name\nhint: See `man git check-ref-format`", "这个名字不能当分支名"},
		{OpBranch, "error: the branch 'unmerged' is not fully merged\n" +
			"hint: If you are sure you want to delete it, run 'git branch -D unmerged'", "这条分支上有还没合并的提交"},
		{OpBranch, "error: cannot delete branch 'main' used by worktree at '/private/tmp/x'", "这条分支正被用着"},
		{OpAmend, "fatal: You have nothing to amend.", "还没有提交可以改"},
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

// 「这个文件已经没有冲突了」是同一类竞态：冲突在点下去和跑起来之间被
// 解决掉了。它必须走 sentinel 这一档，不能被下面的模式表当成别的事。
func TestDescribeNotConflicted(t *testing.T) {
	f := Describe(OpConflict, ErrNotConflicted)
	if f.Title != "这个文件已经没有冲突了" {
		t.Errorf("标题 = %q", f.Title)
	}
	if f.Hint == "" {
		t.Error("该告诉用户下一步（列表已经重新读过）")
	}
}

// 认不出的错误也要说清「是哪一步没成」——写操作多了之后，兜底标题
// 是用户唯一能拿到的线索。
func TestOpTitles(t *testing.T) {
	cases := []struct {
		op   Op
		want string
	}{
		{OpStage, "这次暂存没成功"},
		{OpCommit, "提交没成功"},
		{OpCheckout, "切换没成功"},
		{OpTag, "打标签没成功"},
		{OpFetch, "抓取没成功"},
		{OpPull, "拉取没成功"},
		{OpPush, "推送没成功"},
		{OpMerge, "合并没成功"},
		{OpRebase, "变基没成功"},
		{OpCherryPick, "拣选没成功"},
		{OpRevert, "回退没成功"},
		{OpReset, "重置没成功"},
		{OpBranch, "分支操作没成功"},
		{OpStash, "储藏没成功"},
		{OpAmend, "修改上次提交没成功"},
		{OpConflict, "解决冲突没成功"},
		{OpForcePush, "强制推送没成功"},
		{OpAm, "应用补丁没成功"},
	}
	for _, c := range cases {
		if got := opTitle(c.op); got != c.want {
			t.Errorf("opTitle(%d) = %q, want %q", c.op, got, c.want)
		}
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

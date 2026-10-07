package gitrepo

import (
	"errors"
	"strings"
)

// Failure 是一次 git 写操作失败整理成界面能用的说法。
//
// 和 internal/llm/failure.go 同一个形状：Title 说「出了什么事」，Hint 说
// 「能怎么办」，Detail 是 git 自己的原话 —— 前两句是我们替它翻译的，
// 翻译可能不准，原话留着，遇到怪问题才有得对。Title 为空 = 没有失败。
type Failure struct {
	Title  string
	Hint   string
	Detail string
}

// Empty 表示这不是一次失败。
func (f Failure) Empty() bool { return f.Title == "" }

// Op 是发起这次写操作的动作，用来挑「认不出具体错误时」的兜底说法。
// 零值是 OpNone（没有操作在进行），界面拿它当「写闸门空着」。
type Op uint8

const (
	OpNone Op = iota
	OpStage
	OpUnstage
	OpCommit
	OpCheckout
	OpTag
	OpFetch
	OpPull
	OpPush
	OpMerge
	OpRebase
	OpCherryPick
	OpRevert
	OpReset
	OpBranch
	OpStash
	OpAmend
	OpConflict
	OpForcePush
	OpAm // 用户自己在终端里跑的补丁序列停在半路（git am）
)

// Describe 把 git 的错误整理成人话。
//
// git 的错误信息生来不是给 GUI 用户看的：一句 "error: pathspec 'x' did
// not match any file(s) known to git" 对命令行用户是清楚的，对点了下按钮
// 的人就是一坨英文。模式表翻常见的那些，认不出的也绝不把原文当标题 ——
// 原话永远留在 Detail 里可查。
func Describe(op Op, err error) Failure {
	if err == nil {
		return Failure{}
	}
	if errors.Is(err, ErrHunkGone) {
		return Failure{
			Title: "这个改动已经变了",
			Hint:  "文件在点下去之后被改过（或者前一次操作挪了行号），列表已经刷新，重新看一眼再操作。",
		}
	}
	if errors.Is(err, ErrNoRemote) {
		return Failure{
			Title: "这个仓库没有远程",
			Hint:  "在终端里 git remote add origin <地址> 加一个，回来再推。",
		}
	}
	if errors.Is(err, ErrNoBranch) {
		return Failure{
			Title: "现在没有分支在手上",
			Hint:  "HEAD 是分离的（检出过标签或某个提交）。先新建一条分支，或者检出一条已有的，再推送 —— 推上去的东西总得有个名字。",
		}
	}
	if errors.Is(err, ErrNotConflicted) {
		return Failure{
			Title: "这个文件已经没有冲突了",
			Hint:  "界面拿的是旧状态（多半已经解决掉了）。列表已经重新读过 —— 刚才那一下没有改坏任何东西。",
		}
	}
	if errors.Is(err, ErrBranchNotMerged) {
		return Failure{
			Title:  "这条分支上有还没合并的提交",
			Hint:   "删掉它，上面那些提交就再没有分支认领了。确认不要了就强制删除；拿不准就先把它合并进来。",
			Detail: err.Error(),
		}
	}

	msg := err.Error()
	low := strings.ToLower(msg)
	f := Failure{Title: opTitle(op), Detail: gitDetail(msg)}

	switch {
	// —— 凭据与 SSH。全部保持「在终端里登录一次」的出路，我们没有凭证输入 UI。——
	case strings.Contains(low, "could not read username"),
		strings.Contains(low, "could not read password"),
		strings.Contains(low, "terminal prompts disabled"):
		f.Title = "远程要你登录"
		f.Hint = "在终端里对这个仓库跑一次 git fetch，把凭据（钥匙串 / 令牌）登好再回来重试。"
	case strings.Contains(low, "authentication failed"),
		strings.Contains(low, "invalid username or password"),
		strings.Contains(low, "http 401"):
		f.Title = "远程没认这个账号"
		f.Hint = "GitHub 这类服务不收账号密码：改用个人访问令牌（PAT）或 SSH 地址。"
	case strings.Contains(low, "permission denied (publickey)"):
		f.Title = "远程不认这把 SSH key"
		f.Hint = "确认公钥已经加到远端；在终端里 ssh -T git@主机 试一下。"
	case strings.Contains(low, "host key verification failed"):
		f.Title = "这个主机的身份还没确认"
		f.Hint = "在终端里 ssh -T git@主机，按提示确认一次指纹。"

	// —— 网络 ——
	case strings.Contains(low, "could not resolve host"),
		strings.Contains(low, "couldn't connect to server"),
		strings.Contains(low, "failed to connect to"),
		strings.Contains(low, "connection timed out"),
		strings.Contains(low, "connection refused"),
		strings.Contains(low, "network is unreachable"):
		f.Title = "连不上远程"
		f.Hint = "检查网络，以及 git remote -v 里的地址。"

	// —— 推送被拒 ——
	//
	// stale info 和标签已存在都排在最前：它们的原话里都带 `! [rejected]`，
	// 落到下面那条泛化的 [rejected] 上会被说成「远程有你本地没有的提交」——
	// 对着一条标签说「先拉取」是答非所问。
	case strings.Contains(low, "stale info"):
		f.Title = "远程上有你还没见过的提交"
		f.Hint = "强制推送用的是 --force-with-lease：远程得还是你上次抓取时的样子才肯推。先抓取看一眼别人改了什么 —— 确认要覆盖，再点一次。"
	case strings.Contains(low, "tag already exists in the remote"):
		f.Title = "远程上已经有这个标签"
		f.Hint = "标签不能就地覆盖。确认要改它，在终端里 git push --force origin refs/tags/<名字>。"
	case strings.Contains(low, "non-fast-forward"),
		strings.Contains(low, "fetch first"),
		strings.Contains(low, "[rejected]"):
		f.Title = "远程有你本地没有的提交"
		f.Hint = "先拉取（Pull），合完再推。这条分支要是你刚变基过的，用「远程 ▸ 强制推送」覆盖远程那一份 —— 它只在远程还是你上次看到的样子时才肯推，别人新推的东西不会被吃掉。"
	case strings.Contains(low, "protected branch"),
		strings.Contains(low, "pre-receive hook declined"),
		strings.Contains(low, "gh006"),
		strings.Contains(low, "gh013"):
		f.Title = "远程拒绝了这个推送"
		f.Hint = "这个分支受保护，改动要走 Pull Request。"
	case strings.Contains(low, "no configured push destination"):
		f.Title = "这个仓库没有推送目标"
		f.Hint = "在终端里 git remote add origin <地址> 加一条远程。"

	// —— 拉取 ——
	case strings.Contains(low, "need to specify how to reconcile divergent branches"):
		f.Title = "本地和远程分叉了"
		f.Hint = "在终端里选一次：git config pull.rebase false（合并）或 true（变基）。"
	case strings.Contains(low, "has no upstream branch"),
		strings.Contains(low, "no tracking information"):
		f.Title = "这条分支还没有上游"
		f.Hint = "点「推送」会自动用 origin 建一条上游；也可以 git branch --set-upstream-to 指一条。"
	case strings.Contains(low, "upstream branch of your current branch does not match"):
		// 上游设在一条**不同名**的远程分支上：push.default 默认的 simple 会
		// 拒绝推（推过去到底用哪个名字？git 不替用户决定）。这是用户自己配的
		// 上游，我们不去改它 —— 说清楚，让他选。
		f.Title = "上游和本地分支不同名"
		f.Hint = "上游指向别的名字，git 不肯替你决定推成哪个。在终端里 git push origin <本地分支名> 明确一次。"
	case strings.Contains(low, "push.default is") && strings.Contains(low, `"nothing"`):
		f.Title = "git 配置成不推任何东西"
		f.Hint = "你的 git 配了 push.default = nothing。在终端里 git push origin <分支名> 明确指定要推哪条。"
	case strings.Contains(low, "you are not currently on a branch"),
		strings.Contains(low, "not currently on any branch"):
		// 分离 HEAD 上拉取/推送都会撞这句。出路不是「重试」，是先有分支。
		f.Title = "现在没有分支在手上"
		f.Hint = "HEAD 是分离的 —— 先新建一条分支（或者检出已有的分支），再拉取和推送。"

	// —— 正卡在合并/变基中间 ——
	//
	// 这一组必须排在下面那条泛化的「有冲突」之前：它们的原话里也带
	// conflict / merge 字样（"Exiting because of an unresolved conflict"、
	// "is now empty, possibly due to conflict resolution"），落进泛化那条
	// 就丢掉「该点哪颗按钮」—— 而那是用户此刻唯一需要知道的事。
	//
	// 「还没收尾」和「没有可收尾的」是两个相反的处境，分开说：前者指路
	// 状态条，后者是竞态（多半刚在终端里做完了），指路刷新。
	case strings.Contains(low, "no rebase in progress"),
		strings.Contains(low, "no cherry-pick or revert in progress"),
		strings.Contains(low, "no merge in progress"),
		strings.Contains(low, "no merge to abort"):
		f.Title = "这一步已经不在进行中了"
		f.Hint = "界面显示的是旧状态（多半已经做完或者被中止了）。刷新一下看最新状况 —— 刚才那一下没有改坏任何东西。"
	case strings.Contains(low, "have not concluded your merge"),
		strings.Contains(low, "already a rebase-merge directory"),
		strings.Contains(low, "already a rebase-apply directory"),
		strings.Contains(low, "already in progress"):
		f.Title = "上一步还没收尾"
		f.Hint = "仓库还停在合并或变基中间（顶上的状态条写着是哪一步）。先「继续」或者「中止」把它了结，再做这件事。"
	case strings.Contains(low, "unmerged files"),
		// rebase --continue 撞未解决时说的话和 merge/cherry-pick 不一样：
		// 没有 "unmerged files"，是逐文件一句 "a.txt: needs merge" 加
		// "You must edit all merge conflicts"（实测 git 2.50）。
		strings.Contains(low, "you must edit all merge conflicts"),
		strings.Contains(low, "unresolved conflict"):
		f.Title = "还有文件没解决完"
		f.Hint = "文件列表里标 ! 的就是没解决的：逐个挑一边（或者手工改完「标记为已解决」），再点状态条里的「继续」。"
	case strings.Contains(low, "you need to resolve your current index first"),
		strings.Contains(low, "could not write index"),
		strings.Contains(low, "needs merge"):
		f.Title = "有冲突没收尾"
		f.Hint = "仓库还停在没解决的冲突里（顶上的状态条写着是哪一步）—— 切换、储藏、开新的一步都得先把它了结：解决完「继续」，或者「中止」。"
	case strings.Contains(low, "does not have our version"),
		strings.Contains(low, "does not have their version"):
		// 删除-修改冲突：那一方在 index 里根本没有文件版本。界面本来靠
		// stage 提前把按钮换成了「接受删除」，撞到这里说明状态在点下去的
		// 一瞬间变了 —— 照实话说着，出路指回卡片。
		f.Title = "这一边已经没有这个文件了"
		f.Hint = "对方那边把它删掉了。刷新之后看文件卡片：挑另一边就是接受删除，也可以手工处理完「标记为已解决」。"
	case strings.Contains(low, "is now empty"):
		f.Title = "这一步没有产生任何改动"
		f.Hint = "解决后的结果和当前分支一模一样，git 不肯造一个空提交。点状态条里的「跳过」，接着走后面的。"
	case strings.Contains(low, "is a merge but no -m option was given"):
		f.Title = "这是一个合并提交"
		f.Hint = "合并提交连着两条历史，git 不知道按哪边拣。要拣的话得在终端里写明 git cherry-pick -m 1 <哈希>（1 = 留在当前分支那边）。"

	// —— 有冲突 ——
	// 通用那条排在这一组的最后：合并、变基、拣选、回退、储藏、拉取都会
	// 走到它，拉取只是其中之一。标题按 op 说清是哪一步 —— 光说「有冲突」，
	// 用户还得自己回头猜这是刚才哪一下，而那决定了他接下来按哪颗按钮。
	case strings.Contains(low, "conflict"),
		strings.Contains(low, "automatic merge failed"):
		f.Title = conflictTitle(op)
		if op == OpStash {
			// stash apply/pop 撞冲突不会留下可以「中止」的状态（不写
			// MERGE_HEAD，也没有 sequencer），状态条只会说「有 N 个冲突
			// 文件要解决」。不写这句，用户会去找一颗不存在的按钮。
			f.Hint = "列表里标 ! 的就是冲突文件：挑一边解决（右键菜单，或者手工改完「标记为已解决」）再提交。这一步没有「中止」—— 但储藏的条目还在，随时能取回。"
		} else {
			f.Hint = "列表里标 ! 的就是冲突文件：逐个挑一边解决，再点状态条里的「继续」；不想继续就「中止」，一切回到动手之前。"
		}

	// —— 工作区脏，切换/合并/变基被挡 ——
	//
	// 「暂存」不解决这类问题（改动还在工作区里），提交或储藏才解决 ——
	// 所以别照着老话写「先暂存」。
	case strings.Contains(low, "would be overwritten"),
		strings.Contains(low, "cannot rebase"),
		strings.Contains(low, "cannot pull with rebase"):
		f.Title = "有未提交的改动挡着"
		f.Hint = "先提交或者储藏这些改动再继续 —— 切换、合并、变基都要求先把手头这份放下。储藏会把它们收进一个条目，随时能取回。"

	// —— 仓库被占 / 引用消失 / 名称冲突 ——
	case strings.Contains(low, "index.lock"):
		f.Title = "另一个 git 进程正占着仓库"
		f.Hint = "等它跑完再试。别手动删 .git/index.lock —— 真有残留时 git 自己会处理。"
	case strings.Contains(low, "pathspec"),
		strings.Contains(low, "did not match any file"),
		strings.Contains(low, "unknown revision"),
		strings.Contains(low, "not a valid object name"):
		f.Title = "这条引用已经不在了"
		f.Hint = "刷新一下 —— 远端可能被 prune 掉了。"
	case strings.Contains(low, "a branch named") && strings.Contains(low, "already exists"):
		// 新建和重命名撞的是同一句话（实测），措辞得同时服务两条路 ——
		// 老话「直接检出本地那条」对重命名是答非所问，他要的正是这个名字。
		f.Title = "本地已经有这条分支了"
		f.Hint = "换个名字。想切到原来那条的话，关掉对话框，在分支列表里双击它。"
	case strings.Contains(low, "not a valid branch name"):
		f.Title = "这个名字不能当分支名"
		f.Hint = "别用空格和 ~ ^ : ? * [ \\ 这些字符；斜杠可以（feature/x 这种）。"
	case strings.Contains(low, "is not fully merged"):
		f.Title = "这条分支上有还没合并的提交"
		f.Hint = "删掉它，上面那些提交就再没有分支认领了。确认不要了就强制删除；拿不准就先把它合并进来。"
	case strings.Contains(low, "cannot delete branch"):
		f.Title = "这条分支正被用着"
		f.Hint = "它在一个工作区里检出着（当前分支，或者另一个 worktree），git 不让删。先切到别的分支再说。"
	case strings.Contains(low, "tag") && strings.Contains(low, "already exists"):
		f.Title = "已经有同名标签"
		f.Hint = "换一个名字。"
	case strings.Contains(low, "not a valid tag name"):
		f.Title = "这个名字不能当标签"
		f.Hint = "别用空格和 ~ ^ : ? * [ \\ 这些字符。"

	// —— 提交 ——
	case strings.Contains(low, "unable to auto-detect email address"),
		strings.Contains(low, "please tell me who you are"),
		strings.Contains(low, "empty ident name"):
		f.Title = "还没配 git 用户名和邮箱"
		f.Hint = "在终端里 git config --global user.name \"…\" 和 user.email \"…\" 配一次。"
	case strings.Contains(low, "gpg failed to sign"),
		strings.Contains(low, "no signing key"),
		strings.Contains(low, "secret key not available"):
		f.Title = "GPG 签名失败"
		f.Hint = "检查 user.signingkey，或者把 commit.gpgsign 关掉。"
	case strings.Contains(low, "empty commit message"):
		f.Title = "提交信息是空的"
		f.Hint = "写一句提交信息再提交。"
	case strings.Contains(low, "nothing to commit"):
		f.Title = "没有可提交的东西"
		f.Hint = "先把要提交的文件或改动块勾上。"
	case strings.Contains(low, "nothing to amend"):
		f.Title = "还没有提交可以改"
		f.Hint = "这个仓库还没有任何提交 ——「修改上次提交」得先有一条上次提交。先把第一次提交做出来。"
	case strings.Contains(low, "hook") && (strings.Contains(low, "exited with code") ||
		strings.Contains(low, "failed") || strings.Contains(low, "declined")):
		f.Title = "提交被仓库的钩子拦下了"
		f.Hint = "下面是钩子自己的原话，照它说的改。"

	// —— patch 打不上 ——
	case strings.Contains(low, "patch does not apply"),
		strings.Contains(low, "does not match index"),
		strings.Contains(low, "patch failed"):
		f.Title = "这段改动落不进 index"
		f.Hint = "文件可能刚被改过，列表已经刷新，重新试一次。"
	}
	return f
}

// conflictTitle 说清「冲突是哪一步撞上的」。
//
// 冲突的解法按操作不同：合并/拣选/回退解决完「继续」就收尾，变基后面
// 还排着一队提交，储藏根本没有可继续的步骤。只说「有冲突」，等于把
// 「接下来按哪颗按钮」的问题又丢回给用户。
func conflictTitle(op Op) string {
	switch op {
	case OpMerge:
		return "合并撞上了冲突"
	case OpRebase:
		return "变基撞上了冲突"
	case OpCherryPick:
		return "拣选撞上了冲突"
	case OpRevert:
		return "回退撞上了冲突"
	case OpPull:
		return "拉取撞上了冲突"
	case OpStash:
		return "储藏的改动和手头的文件冲突了"
	}
	return "有冲突要解决"
}

// opTitle 是「认不出具体错误」时的兜底标题：至少说清是哪一步没成。
func opTitle(op Op) string {
	switch op {
	case OpStage, OpUnstage:
		return "这次暂存没成功"
	case OpCommit:
		return "提交没成功"
	case OpCheckout:
		return "切换没成功"
	case OpTag:
		return "打标签没成功"
	case OpFetch:
		return "抓取没成功"
	case OpPull:
		return "拉取没成功"
	case OpPush:
		return "推送没成功"
	case OpMerge:
		return "合并没成功"
	case OpRebase:
		return "变基没成功"
	case OpCherryPick:
		return "拣选没成功"
	case OpRevert:
		return "回退没成功"
	case OpReset:
		return "重置没成功"
	case OpBranch:
		return "分支操作没成功"
	case OpStash:
		return "储藏没成功"
	case OpAmend:
		return "修改上次提交没成功"
	case OpConflict:
		return "解决冲突没成功"
	case OpForcePush:
		return "强制推送没成功"
	case OpAm:
		return "应用补丁没成功"
	}
	return "这个操作没成功"
}

// gitDetail 留 git 的原话：挑出像错误的几行（fatal:/error:/remote:/CONFLICT，
// 以及 push/fetch 那种以 ! 开头的逐引用状态行），一行都没挑出来就整段留着。
// 最多 3 行、约 200 字 —— Detail 是「出怪问题时对得上」的凭据，不是主体，
// 不能反过来把界面淹了。
//
// hint: 单列一档、排在后面：那是 git 的通用建议（「你可以 git config
// pull.rebase false」），而这类建议我们自己已经写过一句更贴切的。真正要比
// 对的是它上面那句结论。「拉取时分支分叉了」就是现成的例子：整条消息是
// 八行 hint 加末尾一行 fatal，按行数截只会截到 hint 的残句。
//
// warning: 丢得更干脆：警告不是这次失败本身，会挤掉真正要看的那句。
// （整段只有警告或只有 hint 时也不会丢字，见下面的兜底和分层。）
//
// 带前缀那一行底下的缩进续行跟着它走：git 习惯把细节挂在冒号后面，「这些
// 未跟踪文件会被覆盖」下面那份文件清单就是缩进的。只留带前缀的行，Detail
// 会停在一个冒号上 —— 而用户要的正是「是哪个文件挡着」。
//
// 同一档里超过 3 行时留**最后**那几行：结论通常在后头（前面是「patch
// failed」，最后才说「patch does not apply」）。
func gitDetail(msg string) string {
	msg = strings.ReplaceAll(msg, "\r\n", "\n")
	lines := strings.Split(msg, "\n")

	var errs, hints []string
	// bucket 是缩进续行该跟着去的那一档；它上面那行没被挑走（空行、warning、
	// 普通说明）时是 nil，续行也就一起不要了。
	var bucket *[]string
	for _, raw := range lines {
		l := strings.TrimSpace(raw)
		switch {
		case l == "":
			bucket = nil
		case strings.HasPrefix(l, "warning:"):
			bucket = nil
		case strings.HasPrefix(l, "hint:"):
			hints = append(hints, l)
			bucket = &hints
		case errLine(l):
			errs = append(errs, l)
			bucket = &errs
		case bucket != nil && (strings.HasPrefix(raw, "\t") || strings.HasPrefix(raw, "  ")):
			*bucket = append(*bucket, l)
		default:
			bucket = nil
		}
	}

	picked := errs
	if len(picked) == 0 {
		picked = hints
	}
	if len(picked) == 0 {
		picked = strings.Split(strings.TrimSpace(msg), "\n")
	}
	if len(picked) > 3 {
		picked = picked[len(picked)-3:]
	}
	return truncate(strings.Join(picked, "\n"), detailMax)
}

// errLine 说这一行像不像是「出了什么事」本身（而不是随行的解释和状态）。
func errLine(l string) bool {
	return strings.HasPrefix(l, "fatal:") || strings.HasPrefix(l, "error:") ||
		strings.HasPrefix(l, "remote:") || strings.HasPrefix(l, "!") ||
		strings.Contains(l, "CONFLICT")
}

// detailMax 是原话保留的长度上限，和 llm 那边取同一个数。
const detailMax = 200

// truncate 按「字符」截断（不是字节），别把中文劈成半个。
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

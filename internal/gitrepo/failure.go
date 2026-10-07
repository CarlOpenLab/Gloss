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
	case strings.Contains(low, "non-fast-forward"),
		strings.Contains(low, "fetch first"),
		strings.Contains(low, "[rejected]"):
		f.Title = "远程有你本地没有的提交"
		f.Hint = "先拉取（Pull），合完再推。"
	case strings.Contains(low, "protected branch"),
		strings.Contains(low, "pre-receive hook declined"),
		strings.Contains(low, "gh006"),
		strings.Contains(low, "gh013"):
		f.Title = "远程拒绝了这个推送"
		f.Hint = "这个分支受保护，改动要走 Pull Request。"

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
	case strings.Contains(low, "conflict"),
		strings.Contains(low, "automatic merge failed"):
		f.Title = "合并有冲突"
		f.Hint = "解决冲突文件（列表里标 !）之后提交；或者撤销这次合并。"

	// —— 切换被挡 ——
	case strings.Contains(low, "would be overwritten"):
		f.Title = "有未提交的改动挡着"
		f.Hint = "先提交或暂存这些文件，再切换。"

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
		f.Title = "本地已经有这条分支了"
		f.Hint = "直接检出本地那条。"
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

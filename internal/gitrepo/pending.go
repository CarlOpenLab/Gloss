package gitrepo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// 这个文件管「仓库正卡在哪一步写操作中间」：合并撞冲突、变基重放一半、
// 拣选/回退没走完 —— 它们各自会在 .git 底下留下标记文件和目录，
// 界面上那条「继续 / 跳过 / 中止」状态条就靠它。
//
// 全部是 stat + 读几个小文件，不跑 git 进程。调用点在 Load 里，每次刷新
// 都过一遍：装一个干净的仓库不该为这个多付一次子进程的钱。

// PendingKind 是卡住的那一步是什么。
type PendingKind uint8

const (
	PendingNone PendingKind = iota
	PendingMerge
	PendingRebase
	PendingAm // 用户自己在终端里跑的 git am 停在半路
	PendingCherryPick
	PendingRevert
)

// Pending 是仓库当前卡住的那一步。
//
// 零值（Kind == PendingNone）就是没卡住 —— 绝大多数时候。
type Pending struct {
	Kind PendingKind

	// Other 是「对方」那个提交：MERGE_HEAD / CHERRY_PICK_HEAD / REVERT_HEAD
	// 的哈希。合并里它是被并进来那条分支的尖端。
	Other string

	// Branch 是正在变基的分支（rebase-merge|apply/head-name 去掉 refs/heads/）。
	// 从分离态发起（`git rebase --onto` 那种）时是空的。
	//
	// 这段期间 git 的 HEAD 是分离的 —— 界面拿它来把分支芯片的话说对：
	// 「正在变基 main」而不是「分离 HEAD：a1b2c3d，先新建一条分支」，
	// 后者在变基中间是主动误导。
	Branch string

	// Onto 是变基目标（.../onto 的完整哈希）。UI 层拿它在引用表里找回
	// 人名（origin/main 之类），找不到就显示短哈希。
	Onto string

	// Step / Total 是变基进度（第 n / m 步）。读不到时是 0。
	Step  int
	Total int
}

// Active 表示有一步操作卡着。
func (p Pending) Active() bool { return p.Kind != PendingNone }

// pending 探测当前卡住的操作。
//
// 顺序不是随便排的：变基用 cherry-pick 的机制重放提交，老版本 git 撞冲突时
// 会**同时**留下 rebase-merge/ 和 CHERRY_PICK_HEAD —— 判错一个，用户点
// 「继续」发出的就是 `git cherry-pick --continue`，git 回一句
// "no cherry-pick in progress"，而这正是最需要它做对的那一刻。
// （实测 git 2.50 的 merge 后端只留 rebase-merge/ + REBASE_HEAD、
// apply 后端只留 rebase-apply/，CHERRY_PICK_HEAD 并不出现；rebase 优先
// 这条顺序留着当防御，代价是几次 stat。）
func (r *Repo) pending() Pending {
	gd := r.GitDir
	if gd == "" {
		return Pending{}
	}

	// 变基两种后端：merge（rebase-merge/，git 2.26 起的默认）和
	// apply（rebase-apply/，也是 git am 用的目录）。
	if p, ok := r.pendingRebase(filepath.Join(gd, "rebase-merge"), false); ok {
		return p
	}
	if p, ok := r.pendingRebase(filepath.Join(gd, "rebase-apply"), true); ok {
		return p
	}

	if h := firstLine(filepath.Join(gd, "MERGE_HEAD")); h != "" {
		return Pending{Kind: PendingMerge, Other: h}
	}
	if h := firstLine(filepath.Join(gd, "CHERRY_PICK_HEAD")); h != "" {
		return Pending{Kind: PendingCherryPick, Other: h}
	}
	if h := firstLine(filepath.Join(gd, "REVERT_HEAD")); h != "" {
		return Pending{Kind: PendingRevert, Other: h}
	}
	return Pending{}
}

// pendingRebase 从一个变基状态目录里读进度。applyBackend 表示这是
// rebase-apply/ —— 它还有一个身份：git am 也用同一个目录，靠目录里的
// `applying` 文件区分（实测：am 撞冲突时 rebase-apply/applying 在，
// rebase 的 apply 后端不在）。
func (r *Repo) pendingRebase(dir string, applyBackend bool) (Pending, bool) {
	if !isDir(dir) {
		return Pending{}, false
	}
	p := Pending{Kind: PendingRebase}
	if applyBackend {
		if fileExists(filepath.Join(dir, "applying")) {
			p.Kind = PendingAm
		}
		// apply 后端：next 是「正在做第几条」（1 起），last 是总数。
		p.Step = atoiFile(filepath.Join(dir, "next"))
		p.Total = atoiFile(filepath.Join(dir, "last"))
	} else {
		// merge 后端：msgnum / end。
		p.Step = atoiFile(filepath.Join(dir, "msgnum"))
		p.Total = atoiFile(filepath.Join(dir, "end"))
	}
	p.Branch = strings.TrimPrefix(firstLine(filepath.Join(dir, "head-name")), "refs/heads/")
	if p.Branch == "detached HEAD" {
		// --onto 从分离态发起的变基：没有分支名可写，留空
		p.Branch = ""
	}
	p.Onto = firstLine(filepath.Join(dir, "onto"))
	return p, true
}

// Continue / Abort / Skip 把卡住的那一步往前推、退回去、或者跳过。
//
// 指令按 kind 拼 —— 五种状态的操作名和 git 子命令同名，一张表就够。
// 界面只管点，判断「现在能不能点」是 UI 层的事（还有冲突文件时「继续」
// 会撞 git 的 "unmerged files"，由 failure.go 翻成人话）。
func (r *Repo) Continue(ctx context.Context, kind PendingKind) error {
	return r.pendingCmd(ctx, kind, "--continue")
}

func (r *Repo) Abort(ctx context.Context, kind PendingKind) error {
	return r.pendingCmd(ctx, kind, "--abort")
}

func (r *Repo) Skip(ctx context.Context, kind PendingKind) error {
	// merge 没有 --skip：合并是你自己发起的一件事，要么做完要么中止。
	// 界面本来就不给这个按钮，这里只是不让它成为一条暗路。
	if kind == PendingMerge {
		return fmt.Errorf("git merge 没有 --skip：合并要么做完，要么中止")
	}
	return r.pendingCmd(ctx, kind, "--skip")
}

func (r *Repo) pendingCmd(ctx context.Context, kind PendingKind, flag string) error {
	cmd := ""
	switch kind {
	case PendingMerge:
		cmd = "merge"
	case PendingRebase:
		cmd = "rebase"
	case PendingAm:
		cmd = "am"
	case PendingCherryPick:
		cmd = "cherry-pick"
	case PendingRevert:
		cmd = "revert"
	default:
		return fmt.Errorf("没有正在进行的操作")
	}
	_, err := run(ctx, r.Root, cmd, flag)
	return err
}

// firstLine 读文件的第一行（去掉空白）。读不到返回空串 —— 探测用它，
// 「文件不在」是最常见的情况，不是错误。
//
// 取第一行而不是整个文件：MERGE_HEAD 在章鱼合并里有多行，别的地方也
// 可能带换行；探测只关心「在不在」和「第一个是谁」。
func firstLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
}

func atoiFile(path string) int {
	n, err := strconv.Atoi(firstLine(path))
	if err != nil {
		return 0
	}
	return n
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

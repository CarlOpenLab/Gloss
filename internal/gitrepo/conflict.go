package gitrepo

import (
	"context"
	"errors"
	"strings"
)

// 冲突文件逐个解决。
//
// 「挑一边」在 git 那边是两条命令：`git checkout --ours/--theirs -- <路径>`
// 把那一方的版本写进工作区，**然后还得 `git add`** —— checkout 只动工作区，
// index 里三个 stage 原封不动，不 add 的话文件看着解好了、git 仍然认为有
// 冲突（实测：checkout 之后 `ls-files -u` 还是三行）。add 那一步才是「解决」
// 本身。
//
// 两边里有一边删掉了文件时（删除-修改冲突），那一边的 `checkout --ours/
// --theirs` 会回 "does not have our version" —— 对它来说「用那一边」就是
// 接受删除，落成 `git rm -f`。哪边有内容由 index 的 stage 说了算：
// stage 2 = 我们这边（HEAD），stage 3 = 对方那边（被并入/被重放/储藏），
// 少哪个 stage 就是哪边删了它。

// ConflictSides 说一个冲突文件在 index 里还剩哪几方的版本。
//
// 零值（两边都没有）理论上不出现 —— 两边都删的东西不会成为冲突；真撞上
// （DD 那种纠缠在重命名里的形态）时哪边都 checkout 不出来，界面按「接受
// 删除」处理。
type ConflictSides uint8

const (
	SideOurs   ConflictSides = 1 << iota // stage 2：我们这边有内容
	SideTheirs                           // stage 3：对方那边有内容
)

// Ours / Theirs 说这一边在 index 里有没有内容。false = 这一边删了文件。
func (s ConflictSides) Ours() bool   { return s&SideOurs != 0 }
func (s ConflictSides) Theirs() bool { return s&SideTheirs != 0 }

// ErrNotConflicted 表示点的文件已经没有未解决的冲突了 —— 用户点下去和 git
// 真正执行之间，冲突在别处解决掉了（或者本来就不是冲突文件）。是正常竞态，
// 界面翻译成一句人话 + 自动刷新。
//
// 这个前置检查是必须的，不是保险：对没有冲突条目的路径，`git checkout
// --ours` 会退化成普通 checkout 静默执行（把 index 里的版本盖回工作区），
// `git rm -f` 会真的把文件删掉 —— 一个按旧状态点下来的按钮不该有这种后果。
var ErrNotConflicted = errors.New("这个文件已经没有冲突了")

// ResolveOurs 用「我们这边」的内容解决一个冲突文件。
//
// 「我们」是谁跟着卡住的操作变：合并/拣选/回退里是当前分支，变基里是变基
// 目标（git 的 --ours，和合并正好相反）。按钮文案那层负责把这件事说清，
// 这里只管 git 的事实。
func (r *Repo) ResolveOurs(ctx context.Context, path string) error {
	return r.resolveConflict(ctx, path, SideOurs)
}

// ResolveTheirs 用「对方那边」的内容解决一个冲突文件：被并入的分支、正在
// 重放的提交、被回退提交的父版本、储藏里的那份……
func (r *Repo) ResolveTheirs(ctx context.Context, path string) error {
	return r.resolveConflict(ctx, path, SideTheirs)
}

func (r *Repo) resolveConflict(ctx context.Context, path string, side ConflictSides) error {
	sides, err := r.pathSides(ctx, path)
	if err != nil {
		return err
	}
	if sides == 0 {
		return ErrNotConflicted
	}
	if sides&side == 0 {
		// 这一边在 index 里没有内容：那边把文件删了。「用那一边」就是接受
		// 删除 —— -f 不能省：文件带着冲突状态，git rm 少了它不肯动。
		_, err := run(ctx, r.Root, "rm", "-f", "--", path)
		return err
	}
	flag := "--ours"
	if side == SideTheirs {
		flag = "--theirs"
	}
	if _, err := run(ctx, r.Root, "checkout", flag, "--", path); err != nil {
		return err
	}
	return r.StageFile(ctx, path)
}

// pathSides 读一个路径的未合并 stage（没有未合并条目返回 0）。
//
// 执行前重新问一遍 index，而不是让界面把看到的那份传进来：从界面点下去到
// 这里跑起来隔着一次异步，这中间的冲突状态可能已经变了 —— 宁可多起一个
// ls-files，也不能拿过期的边去覆盖用户刚做好的解决。
func (r *Repo) pathSides(ctx context.Context, path string) (ConflictSides, error) {
	out, err := run(ctx, r.Root, "ls-files", "-u", "-z", "--", path)
	if err != nil {
		return 0, err
	}
	return parseSides(out)[path], nil
}

// unmergedSides 读**所有**未合并路径的 stage 情况。Load 只在真有冲突文件时
// 跑它（干净仓库一次都不多跑），拿到的这份给界面用来提前说清「哪边把文件
// 删了」。
//
// 读失败不算加载失败：少一份 stage 信息只影响卡片的说法，按钮做事的判断
// 在 resolveConflict 里自己重新问。返回 nil 让调用方原样落进零值。
func (r *Repo) unmergedSides(ctx context.Context) map[string]ConflictSides {
	out, err := run(ctx, r.Root, "ls-files", "-u", "-z")
	if err != nil {
		return nil
	}
	return parseSides(out)
}

// parseSides 解析 `git ls-files -u -z` 的输出。
//
// 每条记录是 `<mode> <sha> <stage>\t<路径>`，彼此以 NUL 分隔。-z 模式下
// 路径不做引用转义（空格、中文都是原字节），所以先按 NUL 切、再按**第一个**
// TAB 切：路径里允许有空格，TAB 不许（git 自己也不许）。
func parseSides(out []byte) map[string]ConflictSides {
	sides := map[string]ConflictSides{}
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		head, path, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(head)
		if len(fields) < 3 {
			continue
		}
		switch fields[2] {
		case "2":
			sides[path] |= SideOurs
		case "3":
			sides[path] |= SideTheirs
		}
	}
	return sides
}

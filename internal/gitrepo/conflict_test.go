package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// 冲突文件的逐个解决（conflict.go）：checkout --ours/--theirs 各拿的是
// 谁的内容、删除-修改怎么落成删除、以及「已经解决过的文件再点一次」必须
// 被挡住。
//
// 每一条的验收都回到 git 自己身上（ls-files -u、工作区的字节）：拿我们
// 自己的 Load 去验自己的写，两边一起错的时候测试还是绿的。

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s: %v", path, err)
	}
	return string(b)
}

// findFile 在加载结果里按路径找一个文件。
func findFile(t *testing.T, m *Model, path string) File {
	t.Helper()
	for _, f := range m.Files {
		if f.NewPath == path || f.OldPath == path {
			return f
		}
	}
	t.Fatalf("加载结果里没有 %s：%+v", path, m.Files)
	return File{}
}

// deleteModifyRepo 造一个删除-修改冲突的仓库并停在合并中间：
// oursDeleted 为真时是我们（main）这边删了 a.txt、对方改了它；为假则反过来。
func deleteModifyRepo(t *testing.T, oursDeleted bool) *Repo {
	t.Helper()
	r := tempRepo(t)
	git(t, r.Root, "checkout", "-qb", "side")
	if oursDeleted {
		write(t, filepath.Join(r.Root, "a.txt"), "side 改的\n")
	} else {
		git(t, r.Root, "rm", "-q", "a.txt")
	}
	git(t, r.Root, "commit", "-qam", "side 的改动")
	git(t, r.Root, "checkout", "-q", "main")
	if oursDeleted {
		git(t, r.Root, "rm", "-q", "a.txt")
	} else {
		write(t, filepath.Join(r.Root, "a.txt"), "main 改的\n")
	}
	git(t, r.Root, "commit", "-qam", "main 的改动")
	gitExpectFail(t, r.Root, "merge", "side")
	return r
}

// 变基里「用重放中的」拿到的必须是**正在重放那条提交**的内容 —— 全篇
// 最关键的一格。照合并的习惯理解（ours = 自己），这里会拿到变基目标的
// 代码，而且毫无提示。
func TestResolveTheirsInRebaseTakesReplayedCommit(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	git(t, r.Root, "checkout", "-q", "topic")
	gitExpectFail(t, r.Root, "rebase", "main")

	if err := r.ResolveTheirs(ctx, "a.txt"); err != nil {
		t.Fatalf("ResolveTheirs: %v", err)
	}
	if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "topic\n" {
		t.Errorf("a.txt = %q，want 正在重放那条提交的内容（topic）", got)
	}
	if out := gitOut(t, r.Root, "ls-files", "-u"); out != "" {
		t.Errorf("解决之后不该还有未合并条目：%q", out)
	}
}

// 变基里的「用变基目标的」= git 的 --ours（和合并相反的那一边）。
func TestResolveOursInRebaseTakesOntoTarget(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	git(t, r.Root, "checkout", "-q", "topic")
	gitExpectFail(t, r.Root, "rebase", "main")

	if err := r.ResolveOurs(ctx, "a.txt"); err != nil {
		t.Fatalf("ResolveOurs: %v", err)
	}
	if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "main\n" {
		t.Errorf("a.txt = %q，want 变基目标（main）的内容", got)
	}
	if out := gitOut(t, r.Root, "ls-files", "-u"); out != "" {
		t.Errorf("解决之后不该还有未合并条目：%q", out)
	}
}

// 合并里两边各拿各的：ours = 当前分支，theirs = 被并入的。
func TestResolveInMergeTakesBothSides(t *testing.T) {
	ctx := context.Background()

	ours := conflictRepo(t)
	gitExpectFail(t, ours.Root, "merge", "topic")
	if err := ours.ResolveOurs(ctx, "a.txt"); err != nil {
		t.Fatalf("ResolveOurs: %v", err)
	}
	if got := readText(t, filepath.Join(ours.Root, "a.txt")); got != "main\n" {
		t.Errorf("合并里 ours 给了 %q，want main 那份", got)
	}

	theirs := conflictRepo(t)
	gitExpectFail(t, theirs.Root, "merge", "topic")
	if err := theirs.ResolveTheirs(ctx, "a.txt"); err != nil {
		t.Fatalf("ResolveTheirs: %v", err)
	}
	if got := readText(t, filepath.Join(theirs.Root, "a.txt")); got != "topic\n" {
		t.Errorf("合并里 theirs 给了 %q，want topic 那份", got)
	}
}

// Load 把「哪边删了文件」读出来：对方删了 → 我们这边有 stage、对方没有；
// 我们删了 → 反过来。界面靠它提前说清「按下去是删除」。
func TestLoadConflictStages(t *testing.T) {
	ctx := context.Background()

	r := deleteModifyRepo(t, false) // 对方删了 a.txt
	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f := findFile(t, m, "a.txt")
	if f.Status != StatusConflict {
		t.Fatalf("a.txt 的状态 = %v，want 冲突", f.Status)
	}
	if !f.Stages.Ours() || f.Stages.Theirs() {
		t.Errorf("对方删了它：Stages = %+v，want 只有我们这边", f.Stages)
	}

	r2 := deleteModifyRepo(t, true) // 我们删了 a.txt
	m2, err := r2.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	f2 := findFile(t, m2, "a.txt")
	if f2.Stages.Ours() || !f2.Stages.Theirs() {
		t.Errorf("我们删了它：Stages = %+v，want 只有对方那边", f2.Stages)
	}

	// 普通两边都改的冲突：两边都在。
	r3 := conflictRepo(t)
	gitExpectFail(t, r3.Root, "merge", "topic")
	m3, err := r3.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if f3 := findFile(t, m3, "a.txt"); !f3.Stages.Ours() || !f3.Stages.Theirs() {
		t.Errorf("两边都改了：Stages = %+v，want 两边都在", f3.Stages)
	}

	// 干净仓库：没有冲突，Stages 全是零值。
	r4 := tempRepo(t)
	m4, err := r4.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, f := range m4.Files {
		if f.Stages != 0 {
			t.Errorf("干净仓库里 %s 的 Stages = %+v，want 零值", f.NewPath, f.Stages)
		}
	}
}

// 对方删了文件：「用对方那边的」就是接受删除 —— 文件从工作区消失，
// 未合并条目清零。
func TestResolveTheirsOnModifyDeleteAcceptsDeletion(t *testing.T) {
	r := deleteModifyRepo(t, false)
	ctx := context.Background()
	if err := r.ResolveTheirs(ctx, "a.txt"); err != nil {
		t.Fatalf("ResolveTheirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("接受删除之后文件该没了：%v", err)
	}
	if out := gitOut(t, r.Root, "ls-files", "-u"); out != "" {
		t.Errorf("解决之后不该还有未合并条目：%q", out)
	}
}

// 对方删了文件：「用当前分支的」= 留下我们改的那份。
func TestResolveOursOnModifyDeleteKeepsOurVersion(t *testing.T) {
	r := deleteModifyRepo(t, false)
	ctx := context.Background()
	if err := r.ResolveOurs(ctx, "a.txt"); err != nil {
		t.Fatalf("ResolveOurs: %v", err)
	}
	if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "main 改的\n" {
		t.Errorf("a.txt = %q，want 我们改的那份", got)
	}
	if out := gitOut(t, r.Root, "ls-files", "-u"); out != "" {
		t.Errorf("解决之后不该还有未合并条目：%q", out)
	}
}

// 我们删了文件：两个方向各自成立 —— 「用对方的」把文件拿回来，
// 「用当前分支的」保持删除。删除-修改的两个方向都落成 git rm 或 add，
// 不是想当然的 checkout。
func TestResolveOnDeleteModifyBothDirections(t *testing.T) {
	ctx := context.Background()

	take := deleteModifyRepo(t, true) // 我们删了、对方改了
	if err := take.ResolveTheirs(ctx, "a.txt"); err != nil {
		t.Fatalf("ResolveTheirs: %v", err)
	}
	if got := readText(t, filepath.Join(take.Root, "a.txt")); got != "side 改的\n" {
		t.Errorf("a.txt = %q，want 对方改的那份", got)
	}
	if out := gitOut(t, take.Root, "ls-files", "-u"); out != "" {
		t.Errorf("解决之后不该还有未合并条目：%q", out)
	}

	keep := deleteModifyRepo(t, true)
	if err := keep.ResolveOurs(ctx, "a.txt"); err != nil {
		t.Fatalf("ResolveOurs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(keep.Root, "a.txt")); !os.IsNotExist(err) {
		t.Errorf("保持删除之后文件该没了：%v", err)
	}
	if out := gitOut(t, keep.Root, "ls-files", "-u"); out != "" {
		t.Errorf("解决之后不该还有未合并条目：%q", out)
	}
}

// 已经解决过的文件再点一次：拒绝。对没有冲突条目的路径，`checkout
// --ours` 会静默退化成普通 checkout（把 index 里的版本盖回工作区）、
// `rm -f` 会真的把文件删掉 —— 一个按旧状态点下来的按钮不该有这种后果。
func TestResolveRefusesWhenNotConflicted(t *testing.T) {
	r := conflictRepo(t)
	ctx := context.Background()
	gitExpectFail(t, r.Root, "merge", "topic")
	write(t, filepath.Join(r.Root, "a.txt"), "解好的\n")
	git(t, r.Root, "add", "a.txt")

	if err := r.ResolveOurs(ctx, "a.txt"); !errors.Is(err, ErrNotConflicted) {
		t.Fatalf("已解决的文件该拒绝：err = %v", err)
	}
	if err := r.ResolveTheirs(ctx, "a.txt"); !errors.Is(err, ErrNotConflicted) {
		t.Fatalf("已解决的文件该拒绝：err = %v", err)
	}
	if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "解好的\n" {
		t.Errorf("拒绝的那一下不该动文件：a.txt = %q", got)
	}
}

// 储藏 apply 撞出来的冲突没有 MERGE_HEAD（Pending 是零值），但 stage
// 信息照读、两边照能挑 —— ours 是当前分支，theirs 是储藏里的那份。
func TestResolveTheirsInStashConflict(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	write(t, filepath.Join(r.Root, "a.txt"), "储藏里的\n")
	git(t, r.Root, "stash", "push", "-qm", "s")
	write(t, filepath.Join(r.Root, "a.txt"), "提交进去的\n")
	git(t, r.Root, "commit", "-qam", "c")
	gitExpectFail(t, r.Root, "stash", "apply")

	m, err := r.Load(ctx)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.Pending.Active() {
		t.Fatalf("stash apply 的冲突不该被当成可继续的状态：%+v", m.Pending)
	}
	f := findFile(t, m, "a.txt")
	if !f.Stages.Ours() || !f.Stages.Theirs() {
		t.Errorf("stash 冲突两边都在：Stages = %+v", f.Stages)
	}

	if err := r.ResolveTheirs(ctx, "a.txt"); err != nil {
		t.Fatalf("ResolveTheirs: %v", err)
	}
	if got := readText(t, filepath.Join(r.Root, "a.txt")); got != "储藏里的\n" {
		t.Errorf("a.txt = %q，want 储藏里的那份", got)
	}
}

// ls-files -u -z 的解析：路径里的空格和中文按原字节来（-z 不做转义），
// 记录之间以 NUL 分隔、头和路径之间以第一个 TAB 分 —— 这是量出来的格式，
// 钉在这里免得哪天「顺手」改回按空格切。
func TestParseSides(t *testing.T) {
	out := "100644 df967b96 1\tdir 空/b c.txt\x00" +
		"100644 e5cdc043 2\tdir 空/b c.txt\x00" +
		"100644 351be5bf 3\ta.txt\x00"
	sides := parseSides([]byte(out))
	if got := sides["dir 空/b c.txt"]; !got.Ours() || got.Theirs() {
		t.Errorf("带空格/中文的路径解析错：%+v", got)
	}
	if got := sides["a.txt"]; got.Ours() || !got.Theirs() {
		t.Errorf("a.txt 只该有对方那边：%+v", got)
	}
	if len(sides) != 2 {
		t.Errorf("路径数 = %d，want 2（同一个路径的三个 stage 该合并成一条）", len(sides))
	}
}

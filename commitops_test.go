package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 提交右键那几件事（commitops.go）在界面这一层的行为：菜单条目、重置
// 对话框的三档说明与那句「会丢什么」、新分支、复制哈希，以及卡在半路时
// 它们该不该亮。
//
// 和别的界面测试一个规矩：验收回到 git 自己身上（gitOut / headOf / 磁盘
// 上的字节），拿我们自己的 Load 去验自己的写，两边一起错的时候测试还是绿的。

// historyAppOnTopic 打开 branchApp，点侧栏的 topic，让它那条历史出来。
// 「topic 上的提交」只在 topic 的历史里有 —— 拣选/建分支/检出都要一条
// 当前分支上没有的提交才说得通。返回 app、tester 和那条提交。
func historyAppOnTopic(t *testing.T) (*app, *ui.Tester, gitrepo.Commit) {
	t.Helper()
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)

	if err := tt.Click("topic"); err != nil {
		t.Fatalf("点侧栏的 topic：%v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.tab != 1 || len(a.commits) == 0 {
		t.Fatalf("该看的是 topic 的历史：tab=%d commits=%d", a.tab, len(a.commits))
	}
	cm := a.commits[0]
	if cm.Subject != "topic 上的提交" {
		t.Fatalf("最上面那条是 %q，want topic 上的提交", cm.Subject)
	}
	return a, tt, cm
}

// 菜单里该有的条目一条不少，文案带上短哈希（用户能确认点的是哪一行）。
func TestCommitMenuOffersOps(t *testing.T) {
	_, tt, cm := historyAppOnTopic(t)

	items := rightClick(t, tt, cm.Subject)
	for _, want := range []string{
		"在 " + cm.Short + " 上新建标签…",
		"拣选这个提交（cherry-pick 到当前分支）",
		"回退这个提交（生成一条反向提交）",
		"重置 main 到 " + cm.Short + "…",
		"从 " + cm.Short + " 建分支…",
		"检出这个提交（分离 HEAD）",
		"复制哈希",
	} {
		if !menuHas(items, want) {
			t.Errorf("提交右键菜单里该有 %q，实际 %v", want, items)
		}
	}
}

// 拣选：main 的顶端多出一条和 topic 那条一样的提交，HEAD 还附在 main 上。
func TestCherryPickLandsOnCurrentBranch(t *testing.T) {
	a, tt, cm := historyAppOnTopic(t)
	dir := a.repoPath

	rightClick(t, tt, cm.Subject)
	if err := tt.ChooseMenuItem("拣选这个提交（cherry-pick 到当前分支）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, dir, "log", "-1", "--format=%s"); got != "topic 上的提交" {
		t.Errorf("顶端提交 = %q，want topic 上的提交", got)
	}
	if got := headOf(t, dir); got != "refs/heads/main" {
		t.Errorf("HEAD = %q，want 还在 main 上", got)
	}
	if got := gitOut(t, dir, "show", "HEAD:b.txt"); got != "只在这条分支上" {
		t.Errorf("拣过来的内容不对：%q", got)
	}
	// 收货地点在「改动」栏：拣选成不成，答案都在工作区那边
	if a.tab != 0 {
		t.Errorf("拣选完该回改动栏，实际 tab=%d", a.tab)
	}
	if !hasAnnouncement(tt.Announcements(), "已拣选 "+cm.Short) {
		t.Errorf("该播报拣了哪一条：%q", tt.Announcements())
	}
}

// 回退：生成一条反向提交，文件内容回到被回退那条之前。
func TestRevertCreatesInverseCommit(t *testing.T) {
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "第一条")
	writeFile(t, filepath.Join(dir, "a.txt"), "changed\n")
	runGit(t, dir, "commit", "-qam", "第二条：改了 a.txt")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	a.setTab(1)
	settle(t, a)
	tt.Frame()

	cm := a.commits[0]
	before, err := strconv.Atoi(gitOut(t, dir, "rev-list", "--count", "HEAD"))
	if err != nil {
		t.Fatalf("数提交数：%v", err)
	}

	items := rightClick(t, tt, cm.Subject)
	if !menuHas(items, "回退这个提交（生成一条反向提交）") {
		t.Fatalf("菜单里没有回退：%v", items)
	}
	if err := tt.ChooseMenuItem("回退这个提交（生成一条反向提交）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := fileText(t, dir, "a.txt"); got != "base\n" {
		t.Errorf("回退之后 a.txt = %q，want base", got)
	}
	if got := gitOut(t, dir, "rev-list", "--count", "HEAD"); got != strconv.Itoa(before+1) {
		t.Errorf("提交数 = %q，want 比之前多一条（反向提交）", got)
	}
	if got := gitOut(t, dir, "log", "-1", "--format=%s"); !strings.HasPrefix(got, "Revert") {
		t.Errorf("反向提交的标题 = %q，want Revert 开头", got)
	}
	if got := headOf(t, dir); got != "refs/heads/main" {
		t.Errorf("HEAD = %q，want 还在 main 上", got)
	}
}

// —— 重置 ——

// resetApp 造一个「两条提交 + 工作区脏」的仓库：a.txt 改了没提交、u.txt
// 未跟踪。硬重置会丢前者、不会碰后者 —— 对话框那句警告要数对。
// 返回 app、tester、目录，和最老那条提交（重置的目标）。
func resetApp(t *testing.T) (*app, *ui.Tester, string, gitrepo.Commit) {
	t.Helper()
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "第一条")
	writeFile(t, filepath.Join(dir, "b.txt"), "第二条加的\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "第二条")

	writeFile(t, filepath.Join(dir, "a.txt"), "改了没提交\n")
	writeFile(t, filepath.Join(dir, "u.txt"), "未跟踪\n")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	a.setTab(1)
	settle(t, a)
	tt.Frame()
	return a, tt, dir, a.commits[1]
}

// openResetDialogAt 右键那条提交并打开重置对话框。
func openResetDialogAt(t *testing.T, a *app, tt *ui.Tester, cm gitrepo.Commit) {
	t.Helper()
	items := rightClick(t, tt, cm.Subject)
	want := "重置 main 到 " + cm.Short + "…"
	if !menuHas(items, want) {
		t.Fatalf("提交右键菜单里该有 %q，实际 %v", want, items)
	}
	if err := tt.ChooseMenuItem(want); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.reset.on {
		t.Fatal("选了菜单之后重置对话框没打开")
	}
}

// 硬重置会把没提交的改动丢掉 —— 打开对话框时就数好，选中硬重置写在脸上；
// 未跟踪的文件不在数里（reset --hard 不碰它们）。
func TestResetDialogWarnsBeforeHardDiscard(t *testing.T) {
	a, tt, dir, cm := resetApp(t)
	before := gitOut(t, dir, "status", "--porcelain")

	openResetDialogAt(t, a, tt, cm)
	if !tt.HasText("重置 main 到 " + cm.Short) {
		t.Fatalf("对话框该说清重置谁到哪：%q", tt.Texts())
	}
	// 默认档是混合，说明就摆着混合那句
	if !tt.HasText("改动留在工作区、暂存区清空") {
		t.Errorf("默认档的说明不对：%q", tt.Texts())
	}

	if err := tt.Click("硬重置（--hard）"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("工作区里有 1 个文件带着没提交的改动，会一起丢（未跟踪的文件不动）。") {
		t.Errorf("硬重置前该数出会丢几个文件（未跟踪的不算）：%q", tt.Texts())
	}
	// 确认键上的字跟着选中的档走（「硬重置」，不是「确定」）
	if _, ok := tt.Find("硬重置"); !ok {
		t.Errorf("确认键该写完整档名：%q", tt.Texts())
	}

	// 取消：一个字都没动
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()
	if got := gitOut(t, dir, "status", "--porcelain"); got != before {
		t.Errorf("取消不该动仓库：\n%q\nwant\n%q", got, before)
	}
}

// 软重置：分支挪过去，改动原样留在暂存区 —— 文件一个不少、一个不丢。
func TestResetSoftKeepsChangesStaged(t *testing.T) {
	a, tt, dir, cm := resetApp(t)
	openResetDialogAt(t, a, tt, cm)

	if err := tt.Click("软重置（--soft）"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("软重置"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, dir, "rev-parse", "--short", "HEAD"); got != cm.Short {
		t.Errorf("HEAD = %s，want %s", got, cm.Short)
	}
	// index 没动：第二条加的 b.txt 现在是「已暂存的新增」
	if got := gitOut(t, dir, "diff", "--cached", "--name-only"); !strings.Contains(got, "b.txt") {
		t.Errorf("软重置之后改动该在暂存区：%q", got)
	}
	if got := fileText(t, dir, "b.txt"); got != "第二条加的\n" {
		t.Errorf("软重置不碰工作区：b.txt = %q", got)
	}
	if got := fileText(t, dir, "a.txt"); got != "改了没提交\n" {
		t.Errorf("手头没提交的改动也该原样在：a.txt = %q", got)
	}
	if !hasAnnouncement(tt.Announcements(), "已重置 main 到 "+cm.Short+"，改动留在暂存区") {
		t.Errorf("该播报哪一种重置：%q", tt.Announcements())
	}
}

// 混合重置：暂存区清空，工作区一个文件不动 —— b.txt 还在磁盘上，只是
// 从「已暂存」变成了未跟踪。
func TestResetMixedLeavesWorktreeUntouched(t *testing.T) {
	a, tt, dir, cm := resetApp(t)
	openResetDialogAt(t, a, tt, cm)

	// 默认就是混合，直接按确认键
	if err := tt.Click("混合重置"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, dir, "rev-parse", "--short", "HEAD"); got != cm.Short {
		t.Errorf("HEAD = %s，want %s", got, cm.Short)
	}
	if got := gitOut(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("混合重置之后暂存区该是空的：%q", got)
	}
	if got := fileText(t, dir, "b.txt"); got != "第二条加的\n" {
		t.Errorf("工作区不该被动：b.txt = %q", got)
	}
	if got := gitOut(t, dir, "status", "--porcelain"); !strings.Contains(got, "?? b.txt") {
		t.Errorf("b.txt 该变成未跟踪：%q", got)
	}
}

// 硬重置：分支、暂存区、工作区一起回到那条提交 —— 第二条加的 b.txt 跟着
// 没了，手头没提交的改动丢了，未跟踪的 u.txt 不动。
func TestResetHardDiscardsWorkAndMovesBranch(t *testing.T) {
	a, tt, dir, cm := resetApp(t)
	openResetDialogAt(t, a, tt, cm)

	if err := tt.Click("硬重置（--hard）"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("硬重置"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, dir, "rev-parse", "--short", "HEAD"); got != cm.Short {
		t.Errorf("HEAD = %s，want %s", got, cm.Short)
	}
	if _, err := os.Stat(filepath.Join(dir, "b.txt")); !os.IsNotExist(err) {
		t.Errorf("第二条加的 b.txt 该没了：%v", err)
	}
	if got := fileText(t, dir, "a.txt"); got != "base\n" {
		t.Errorf("没提交的改动该丢了：a.txt = %q", got)
	}
	// 未跟踪的不动 —— 对话框那句警告也是这么说的
	if got := fileText(t, dir, "u.txt"); got != "未跟踪\n" {
		t.Errorf("未跟踪文件不该动：u.txt = %q", got)
	}
	if a.tab != 0 {
		t.Errorf("重置完该回改动栏，实际 tab=%d", a.tab)
	}
	if !hasAnnouncement(tt.Announcements(), "已硬重置 main 到 "+cm.Short) {
		t.Errorf("该播报硬重置：%q", tt.Announcements())
	}
}

// —— 新建分支 ——

// 默认勾着「建好就切过去」：从 topic 那条提交建一条新分支并站上去，
// b.txt 跟着过来。
func TestNewBranchFromCommitSwitches(t *testing.T) {
	a, tt, cm := historyAppOnTopic(t)
	dir := a.repoPath

	items := rightClick(t, tt, cm.Subject)
	want := "从 " + cm.Short + " 建分支…"
	if !menuHas(items, want) {
		t.Fatalf("菜单里该有 %q，实际 %v", want, items)
	}
	if err := tt.ChooseMenuItem(want); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !tt.HasText("新建分支") || !tt.HasText("从 "+cm.Short+" · "+cm.Subject+" 起步") {
		t.Fatalf("对话框该说清从哪儿起步：%q", tt.Texts())
	}
	if !tt.Focused("分支名称") {
		t.Error("名称框开局该有焦点")
	}

	tt.Type("feature/x")
	tt.Frame()
	if err := tt.Click("创建分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := headOf(t, dir); got != "refs/heads/feature/x" {
		t.Errorf("HEAD = %q，want 附到 feature/x 上", got)
	}
	if got := fileText(t, dir, "b.txt"); got != "只在这条分支上\n" {
		t.Errorf("新分支该从那条提交起步：b.txt = %q", got)
	}
	if a.tab != 0 {
		t.Errorf("切过去之后该回改动栏，实际 tab=%d", a.tab)
	}
	if !hasAnnouncement(tt.Announcements(), "已新建并切换到 feature/x") {
		t.Errorf("该播报建了什么、切到哪：%q", tt.Announcements())
	}
}

// 不勾「建好就切过去」：分支建出来，HEAD 留在原地，侧栏里多一条。
func TestNewBranchWithoutSwitchLeavesHead(t *testing.T) {
	a, tt, cm := historyAppOnTopic(t)
	dir := a.repoPath

	rightClick(t, tt, cm.Subject)
	if err := tt.ChooseMenuItem("从 " + cm.Short + " 建分支…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("建好就切过去"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	// 点过勾选框之后焦点跟着走了，先把光标点回名称框再打字
	if err := tt.Click("分支名称"); err != nil {
		t.Fatal(err)
	}
	tt.Type("feature/x")
	tt.Frame()
	if err := tt.Click("创建分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := headOf(t, dir); got != "refs/heads/main" {
		t.Errorf("没勾切换，HEAD 该留在 main：%q", got)
	}
	if got := gitOut(t, dir, "rev-parse", "--verify", "refs/heads/feature/x"); got == "" {
		t.Error("分支该建出来了")
	}
	found := false
	for _, r := range a.refs {
		if r.Name == "feature/x" {
			found = true
		}
	}
	if !found {
		t.Errorf("侧栏引用里该多出 feature/x：%+v", a.refs)
	}
	if !hasAnnouncement(tt.Announcements(), "已新建分支 feature/x") {
		t.Errorf("该播报建了什么：%q", tt.Announcements())
	}
}

// 重名：git 拒绝，话翻好留在对话框里，名字还在框里 —— 改一下就能再试。
func TestNewBranchDuplicateNameStaysInDialog(t *testing.T) {
	a, tt, cm := historyAppOnTopic(t)

	rightClick(t, tt, cm.Subject)
	if err := tt.ChooseMenuItem("从 " + cm.Short + " 建分支…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("topic") // 已经有了
	tt.Frame()
	if err := tt.Click("创建分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if !a.newBranch.on {
		t.Fatal("建失败了，对话框该开着")
	}
	if !tt.HasText("本地已经有这条分支了") {
		t.Errorf("该把重名翻成人话：%q", tt.Texts())
	}
	if a.newBranch.name != "topic" {
		t.Errorf("名字该留在框里：%q", a.newBranch.name)
	}
}

// 空名字不预判，交给 git 拒（” is not a valid branch name），话照样翻好。
func TestNewBranchEmptyNameExplains(t *testing.T) {
	a, tt, cm := historyAppOnTopic(t)

	rightClick(t, tt, cm.Subject)
	if err := tt.ChooseMenuItem("从 " + cm.Short + " 建分支…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("创建分支"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if !a.newBranch.on {
		t.Fatal("空名字被拒，对话框该开着")
	}
	if !tt.HasText("这个名字不能当分支名") {
		t.Errorf("该把 git 的拒收翻成人话：%q", tt.Texts())
	}
}

// —— 复制哈希、检出这个提交、卡半路时灰着 ——

// 「复制哈希」写进剪贴板（菜单回调拿不到 Context，晚一帧落地，测试也照着
// 帧来问）。
func TestCopyHashReachesClipboard(t *testing.T) {
	_, tt, cm := historyAppOnTopic(t)

	rightClick(t, tt, cm.Subject)
	if err := tt.ChooseMenuItem("复制哈希"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if got := tt.Clipboard(); got != cm.Hash {
		t.Errorf("剪贴板 = %q，want %q", got, cm.Hash)
	}
	if !hasAnnouncement(tt.Announcements(), "已复制 "+cm.Hash) {
		t.Errorf("该播报复制了什么：%q", tt.Announcements())
	}
}

// 「检出这个提交」：HEAD 分离到那条提交上，界面回改动栏。
func TestDetachFromCommitMenu(t *testing.T) {
	a, tt, cm := historyAppOnTopic(t)
	dir := a.repoPath

	rightClick(t, tt, cm.Subject)
	if err := tt.ChooseMenuItem("检出这个提交（分离 HEAD）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := headOf(t, dir); got != "HEAD" {
		t.Errorf("该是分离 HEAD：%q", got)
	}
	if got := gitOut(t, dir, "rev-parse", "HEAD"); got != cm.Hash {
		t.Errorf("HEAD 在 %s，want %s", got, cm.Hash)
	}
	if a.tab != 0 {
		t.Errorf("检出之后该回改动栏，实际 tab=%d", a.tab)
	}
	if !hasAnnouncement(tt.Announcements(), "已检出 "+cm.Short+"（分离 HEAD）") {
		t.Errorf("该播报检出了哪一条：%q", tt.Announcements())
	}
}

// 卡在合并/变基中间：拣选、回退、重置灰着（git 本来也会拦，而 reset
// --hard 更狠 —— 它会静默把没收尾的合并抹掉）；新建标签不受影响。
func TestCommitOpsDisabledDuringPending(t *testing.T) {
	a, _ := mergeConflictApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	a.setTab(1)
	settle(t, a)
	tt.Frame()

	if len(a.commits) == 0 {
		t.Fatal("卡在合并里也该看得到历史")
	}
	// 挑那条标题独一无二的提交：冲突仓库里最新那条的标题就是「main」，
	// 和侧栏的分支同名 —— rightClick 找的是第一处匹配，会点到分支上去。
	cm := a.commits[len(a.commits)-1]
	rightClick(t, tt, cm.Subject)

	for _, label := range []string{
		"拣选这个提交（cherry-pick 到当前分支）",
		"回退这个提交（生成一条反向提交）",
		"重置 main 到 " + cm.Short + "…",
	} {
		if err := tt.ChooseMenuItem(label); err == nil || !strings.Contains(err.Error(), "disabled") {
			t.Errorf("卡在半路时 %q 该是灰的：err = %v", label, err)
		}
	}
	// 标签不影响工作区，git 也不拦 —— 不该跟着一起灰
	if err := tt.ChooseMenuItem("在 " + cm.Short + " 上新建标签…"); err != nil {
		t.Fatalf("新建标签不该被一起灰掉：%v", err)
	}
	settle(t, a)
	if !a.tag.on {
		t.Error("选了新建标签之后对话框该开着")
	}
}

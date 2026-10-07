package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 检出和打标签在界面这一层的行为：侧栏的右键菜单、双击、打标签对话框。
//
// 和 stage_test.go 一个规矩：验收回到 git 自己身上（gitOut / headOf），
// 拿我们自己的 Load 去验自己的写，两边一起错的时候测试还是绿的。

// newRepoDir 建一个空仓库（配好身份），返回目录。
func newRepoDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

// headOf 是 HEAD 指向哪儿：refs/heads/x 表示附在那条分支上，HEAD 表示分离。
//
// 用 --symbolic-full-name 而不是 symbolic-ref --short：分支和标签重名时
// 后者会打印 "heads/v1"，看着像检出错地方了。
func headOf(t *testing.T, dir string) string {
	t.Helper()
	return gitOut(t, dir, "rev-parse", "--symbolic-full-name", "HEAD")
}

// branchApp 是一个两条分支的仓库：main 和 topic，topic 上多一个提交
// b.txt —— 切过去看得到文件列表真的变了。
func branchApp(t *testing.T) *app {
	t.Helper()
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")

	runGit(t, dir, "checkout", "-q", "-b", "topic")
	writeFile(t, filepath.Join(dir, "b.txt"), "只在这条分支上\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "topic 上的提交")
	runGit(t, dir, "checkout", "-q", "main")

	return loadApp(t, dir)
}

// doubleClick 在某个名字上双击。
//
// 中间不插 Frame：双击判的是「同一个元素、同一处、两次按下之间不到 500ms」
// （ui/input.go 的 lastPress），元素身份按帧算 —— 中间渲染一帧就可能换一个
// id，双击就散了。
func doubleClick(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	r, ok := tt.Find(name)
	if !ok {
		t.Fatalf("界面上找不到 %q：%q", name, tt.Texts())
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	tt.Release(x, y)
	tt.Press(x, y)
	tt.Release(x, y)
	tt.Frame()
}

// rightClick 在某个名字上按右键，返回菜单里的条目。
func rightClick(t *testing.T, tt *ui.Tester, name string) []string {
	t.Helper()
	r, ok := tt.Find(name)
	if !ok {
		t.Fatalf("界面上找不到 %q：%q", name, tt.Texts())
	}
	tt.RightClickAt(r.X+r.W/2, r.Y+r.H/2)
	return tt.Menu()
}

func menuHas(items []string, want string) bool {
	for _, s := range items {
		if s == want {
			return true
		}
	}
	return false
}

// 双击侧栏的分支 = 检出：HEAD 附到那条分支上，界面回到「改动」那一栏。
func TestDoubleClickBranchChecksOut(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 先去看一眼 topic 的历史（模拟「正在看一条别的分支」）：检出之后这份
	// 「在看哪条历史」必须作废 —— histRev 存的是引用的全名，切完分支它照样
	// 解析得到，照着读会得到一份和眼前对不上的历史。
	ref, ok := a.refByFull["refs/heads/topic"]
	if !ok {
		t.Fatalf("侧栏没读到 topic：%v（%s）", a.refsState, a.refErr)
	}
	a.selectRef("r:refs/heads/topic", *ref)
	settle(t, a)
	tt.Frame()
	if a.tab != 1 {
		t.Fatalf("点了分支该在历史 tab，实际 %d", a.tab)
	}
	// 展开状态也放一份：换分支之后它必须清掉（行号对不上）
	a.expanded = map[string]int{"g:u:a.txt:1": 20}

	doubleClick(t, tt, "topic")
	settle(t, a)
	tt.Frame()

	if got := headOf(t, a.repoPath); got != "refs/heads/topic" {
		t.Errorf("HEAD 指向 %q，want refs/heads/topic", got)
	}
	if a.branch != "topic" {
		t.Errorf("界面上的分支是 %q，want topic", a.branch)
	}
	if a.tab != 0 {
		t.Errorf("检出之后该回到改动 tab，实际 %d", a.tab)
	}
	if a.histRev != "" || a.commitSel != -1 || a.refSel != "" || a.stashRev != "" {
		t.Errorf("换完分支「在看哪条历史」该全部作废：histRev=%q commitSel=%d refSel=%q stashRev=%q",
			a.histRev, a.commitSel, a.refSel, a.stashRev)
	}
	if a.expanded != nil {
		t.Errorf("换分支后展开状态该作废（行号对不上），实际还留着 %v", a.expanded)
	}
	if a.opErr != nil {
		t.Errorf("这次检出该成功，却报错：%v", a.opErr)
	}
	if !tt.HasText("已切换到 topic") {
		t.Errorf("成功提示没出来：%q", tt.Texts())
	}
}

// 双击尾巴上那一次「抬起」不能被当成单击。
//
// 真实鼠标下按下和抬起之间隔着几十毫秒，检出完全可能在这中间做完 —— 那一下
// 抬起要是照单全收，用户刚双击检出一条分支，界面立刻又被切去历史。无头测试
// 每个事件都会渲染一帧，正好能把这个交错排出来：按下、抬起、按下（双击成立，
// 检出开跑）、等检出做完、再抬起。
func TestDoubleClickTrailingReleaseIgnored(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	r, ok := tt.Find("topic")
	if !ok {
		t.Fatalf("侧栏找不到 topic：%q", tt.Texts())
	}
	x, y := r.X+r.W/2, r.Y+r.H/2
	tt.Press(x, y)
	tt.Release(x, y)
	tt.Press(x, y)
	settle(t, a) // 检出在这中间做完
	tt.Release(x, y)
	settle(t, a)
	tt.Frame()

	if got := headOf(t, a.repoPath); got != "refs/heads/topic" {
		t.Fatalf("HEAD 指向 %q，want refs/heads/topic", got)
	}
	if a.tab != 0 || a.histRev != "" {
		t.Errorf("双击尾巴上那一次抬起被当成了单击：tab=%d histRev=%q", a.tab, a.histRev)
	}
}

// 双击已经在的那条分支：什么都不做，但要说一声。
//
// 不说的话，用户分不清「已经在这条分支上了」和「这个程序双击没反应」。
func TestDoubleClickCurrentBranchIsNoop(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	doubleClick(t, tt, "main")
	settle(t, a)
	tt.Frame()

	if a.op != gitrepo.OpNone {
		t.Errorf("当前分支上双击不该发起写操作，a.op = %v", a.op)
	}
	if a.opErr != nil {
		t.Errorf("这不是错误：%v", a.opErr)
	}
	if !tt.HasText("已经在这条分支上了") {
		t.Errorf("双击当前分支该有一句说明：%q", tt.Texts())
	}
}

// 右键一条分支：菜单里写着去向，选它 = 检出。
func TestRefContextMenuChecksOut(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	items := rightClick(t, tt, "topic")
	if !menuHas(items, "检出 topic") {
		t.Fatalf("菜单里该有「检出 topic」，实际 %v", items)
	}
	if err := tt.ChooseMenuItem("检出 topic"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := headOf(t, a.repoPath); got != "refs/heads/topic" {
		t.Errorf("HEAD 指向 %q，want refs/heads/topic", got)
	}
	if a.branch != "topic" {
		t.Errorf("界面上的分支是 %q，want topic", a.branch)
	}
	// 检出完人得留在「改动」那一栏：切完分支转头去看历史，是最常被抱怨的
	// 那种「它自己动了」。这一条也钉住双击尾巴上那一次单击被吃掉了 ——
	// 没吃掉的话它会再把界面切回历史。
	if a.tab != 0 {
		t.Errorf("检出之后该在改动 tab，实际 %d", a.tab)
	}
	if !tt.HasText("已切换到 topic") {
		t.Errorf("成功提示没出来：%q", tt.Texts())
	}
}

// 远程分支：本地没有同名分支时，检出 = 建一条本地分支并跟踪它。
// 菜单文案要把这件事说出来 —— 「检出并新建本地分支」，不是含糊的「检出」。
func TestRemoteRefMenuCreatesTrackingBranch(t *testing.T) {
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	runGit(t, dir, "remote", "add", "origin", dir)
	// 远程上有一条本地还没有的分支（拉下来之前的正常状态）
	runGit(t, dir, "update-ref", "refs/remotes/origin/feature", "HEAD")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	items := rightClick(t, tt, "feature")
	if !menuHas(items, "检出并新建本地分支 feature") {
		t.Fatalf("菜单里该写明会新建本地分支，实际 %v", items)
	}
	if err := tt.ChooseMenuItem("检出并新建本地分支 feature"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := headOf(t, a.repoPath); got != "refs/heads/feature" {
		t.Errorf("HEAD 指向 %q，want refs/heads/feature", got)
	}
	up := gitOut(t, dir, "for-each-ref", "--format=%(upstream)", "refs/heads/feature")
	if up != "refs/remotes/origin/feature" {
		t.Errorf("新分支的上游是 %q，want refs/remotes/origin/feature", up)
	}
	if a.opErr != nil {
		t.Errorf("这次检出该成功：%v", a.opErr)
	}
}

// 远程分支本地已经有了：菜单降级成「检出本地那条」，点下去是切过去，不是
// 撞一句 "a branch named 'x' already exists" 再自己想明白。
func TestRemoteRefWithLocalFallsBackToLocalBranch(t *testing.T) {
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	runGit(t, dir, "remote", "add", "origin", dir)
	runGit(t, dir, "update-ref", "refs/remotes/origin/feature", "HEAD")
	runGit(t, dir, "branch", "feature")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	// 侧栏里「feature」有两行（本地一条、远程一条），按名字点会点到哪一条
	// 全看遍历顺序 —— 所以两条的菜单都要能接受，唯独「新建本地分支」这条
	// 文案不能出现：本地已经有了，再写「新建」就是骗人。
	items := rightClick(t, tt, "feature")
	if menuHas(items, "检出并新建本地分支 feature") {
		t.Fatalf("本地已经有 feature 了，远程那条不该还写着「新建」：%v", items)
	}
	label := ""
	for _, want := range []string{"检出 feature（本地已经有了）", "检出 feature"} {
		if menuHas(items, want) {
			label = want
			break
		}
	}
	if label == "" {
		t.Fatalf("菜单里既没有本地那条也没有远程那条的检出项：%v", items)
	}
	if err := tt.ChooseMenuItem(label); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := headOf(t, a.repoPath); got != "refs/heads/feature" {
		t.Errorf("HEAD 指向 %q，want refs/heads/feature（附在分支上，不是分离）", got)
	}
	if a.opErr != nil {
		t.Errorf("这次检出该成功：%v", a.opErr)
	}
}

// 检出标签：分离 HEAD，而且界面上要看得出来。
func TestCheckoutTagDetaches(t *testing.T) {
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	first := gitOut(t, dir, "rev-parse", "HEAD")
	runGit(t, dir, "tag", "v1")
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n改了\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "后来的提交")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	items := rightClick(t, tt, "v1")
	if !menuHas(items, "检出标签 v1（分离 HEAD）") {
		t.Fatalf("标签的菜单要说清会分离 HEAD，实际 %v", items)
	}
	if err := tt.ChooseMenuItem("检出标签 v1（分离 HEAD）"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := headOf(t, dir); got != "HEAD" {
		t.Errorf("检出标签之后 HEAD 该是分离的，实际指向 %q", got)
	}
	if gitOut(t, dir, "rev-parse", "HEAD") != first {
		t.Errorf("检出的不是 v1 那个提交")
	}
	if !a.detached {
		t.Error("a.detached 该是 true")
	}
	if !tt.HasText("分离 HEAD") {
		t.Errorf("工具栏该把分离 HEAD 说出来：%q", tt.Texts())
	}
}

// 工作区有东西挡着：检出失败，标题是人话，git 的原话留在 Detail 里。
func TestCheckoutBlockedShowsFriendlyBanner(t *testing.T) {
	a := branchApp(t)
	// topic 上有 b.txt，main 上没有 —— 工作区里放一个同名的未跟踪文件挡道
	writeFile(t, filepath.Join(a.repoPath, "b.txt"), "未跟踪的同名文件\n")

	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	rightClick(t, tt, "topic")
	if err := tt.ChooseMenuItem("检出 topic"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.opErr == nil {
		t.Fatal("这次检出该失败")
	}
	if a.opErr.Title != "有未提交的改动挡着" {
		t.Errorf("标题是 %q，want「有未提交的改动挡着」", a.opErr.Title)
	}
	if !strings.Contains(a.opErr.Detail, "b.txt") {
		t.Errorf("原话里该有那个文件名，好让用户知道是哪个：%q", a.opErr.Detail)
	}
	if got := headOf(t, a.repoPath); got != "refs/heads/main" {
		t.Errorf("检出失败了，HEAD 不该动，实际 %q", got)
	}
	if !tt.HasText("有未提交的改动挡着") {
		t.Errorf("失败要画在横幅上：%q", tt.Texts())
	}
}

// —— 打标签 ——

// tagDialogApp 打开一个仓库，切到历史 tab，并右键第一个提交打开对话框。
// 返回 app、tester 和那个提交。
func tagDialogApp(t *testing.T) (*app, *ui.Tester, gitrepo.Commit) {
	t.Helper()
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	a.setTab(1)
	settle(t, a)
	tt.Frame()

	cm := a.commits[0]
	if !tt.HasText(cm.Subject) {
		t.Fatalf("提交列表里没有 %q：%q", cm.Subject, tt.Texts())
	}
	items := rightClick(t, tt, cm.Subject)
	want := "在 " + cm.Short + " 上新建标签…"
	if !menuHas(items, want) {
		t.Fatalf("提交的右键菜单里该有 %q，实际 %v", want, items)
	}
	if err := tt.ChooseMenuItem(want); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()
	if !a.tag.on {
		t.Fatal("选了菜单之后对话框没打开")
	}
	return a, tt, cm
}

// 附注标签：打字、创建、git 那边真的多了一个 tag 对象，侧栏也看得见。
func TestTagDialogCreatesAnnotatedTag(t *testing.T) {
	a, tt, cm := tagDialogApp(t)
	dir := a.repoPath

	if !tt.HasText("新建标签") || !tt.HasText("打在 "+cm.Short+" · "+cm.Subject) {
		t.Fatalf("对话框该说清打在哪个提交上：%q", tt.Texts())
	}

	// 名称框开局就有焦点（不然打开对话框还得先点一下才能打字）
	if !tt.Focused("标签名称") {
		t.Error("名称框开局该有焦点")
	}
	tt.Type("v1.0")
	tt.Click("标签说明")
	tt.Type("第一个版本")
	settle(t, a)
	tt.Frame()

	// 输入框里的字不进 Labels（编辑器内容是画出来的，不是一段静态文字），
	// 所以断言看状态。两个框都要查：说明打进了名称框也是一样的「没报错」。
	if a.tag.name != "v1.0" || a.tag.msg != "第一个版本" {
		t.Fatalf("打进去的是 name=%q msg=%q，want v1.0 / 第一个版本", a.tag.name, a.tag.msg)
	}
	if err := tt.Click("创建标签"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.tag.on {
		t.Error("创建成功之后对话框该关掉")
	}
	if got := gitOut(t, dir, "cat-file", "-t", "v1.0"); got != "tag" {
		t.Fatalf("v1.0 该是个附注标签对象，实际 %q", got)
	}
	if got := gitOut(t, dir, "rev-parse", "v1.0^{commit}"); got != cm.Hash {
		t.Errorf("标签落在 %s，want %s", got, cm.Hash)
	}
	if body := gitOut(t, dir, "tag", "-n99", "-l", "v1.0"); !strings.Contains(body, "第一个版本") {
		t.Errorf("说明没写进标签对象：%q", body)
	}
	if !tt.HasText("v1.0") {
		t.Errorf("新标签该出现在侧栏里：%q", tt.Texts())
	}
	if a.opErr != nil {
		t.Errorf("这次打标签该成功：%v", a.opErr)
	}
}

// 轻量标签：取消勾选附注，说明框就没了，创建出来的是一条指向提交的 ref。
func TestTagDialogLightweightTag(t *testing.T) {
	a, tt, cm := tagDialogApp(t)
	dir := a.repoPath

	tt.Type("v2")
	tt.Click("附注标签")
	tt.Frame()
	// 说明只有附注标签用得上，取消勾选之后不该还留着一个空框要人填。
	// 查的是说明框自己那个无障碍名字：placeholder 不进 Labels，靠它判断
	// 只会永远为假（这行断言原来就是这么空转过去的）。
	if tt.HasText("标签说明") {
		t.Errorf("取消附注之后说明框该收起来：%q", tt.Texts())
	}
	tt.Click("创建标签")
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, dir, "cat-file", "-t", "v2"); got != "commit" {
		t.Errorf("轻量标签该直接指向 commit，实际 %q", got)
	}
	if got := gitOut(t, dir, "rev-parse", "v2^{commit}"); got != cm.Hash {
		t.Errorf("标签落在 %s，want %s", got, cm.Hash)
	}
}

// 重名：错误留在对话框里，名字和说明一个字都不丢，顶上那条横幅不许抢戏。
func TestTagDialogFailureStaysInDialog(t *testing.T) {
	a, tt, _ := tagDialogApp(t)
	dir := a.repoPath
	runGit(t, dir, "tag", "v1")

	tt.Type("v1")
	tt.Click("标签说明")
	tt.Type("这段话不能丢")
	settle(t, a)
	tt.Frame()

	if err := tt.Click("创建标签"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if !a.tag.on {
		t.Fatal("失败了对话框该还开着 —— 用户要在这里改名字")
	}
	if a.tag.name != "v1" || a.tag.msg != "这段话不能丢" {
		t.Errorf("失败之后填过的东西要留着，实际 name=%q msg=%q", a.tag.name, a.tag.msg)
	}
	if a.tag.err.Title != "已经有同名标签" {
		t.Errorf("对话框里的错误是 %q，want「已经有同名标签」", a.tag.err.Title)
	}
	if a.opErr != nil {
		t.Error("错误已经搬进对话框了，顶上不该再挂一条横幅")
	}
	if !tt.HasText("已经有同名标签") {
		t.Errorf("错误该画在对话框里：%q", tt.Texts())
	}
	if got := gitOut(t, dir, "tag", "-l"); got != "v1" {
		t.Errorf("重名的标签不该被覆盖或新建：%q", got)
	}
	// 改个名字再点一次，这次要成 —— 说明还是原来那句
	tt.Click("标签名称")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("v1.1")
	settle(t, a)
	tt.Frame()
	tt.Click("创建标签")
	settle(t, a)
	tt.Frame()

	if a.tag.on {
		t.Error("改名之后该创建成功")
	}
	if body := gitOut(t, dir, "tag", "-n99", "-l", "v1.1"); !strings.Contains(body, "这段话不能丢") {
		t.Errorf("改名之后说明该还在：%q", body)
	}
}

// 一眼看得出的坏名字：按钮灰着，点在灰按钮上什么都不会发生。
func TestTagDialogRejectsBadName(t *testing.T) {
	a, tt, _ := tagDialogApp(t)
	dir := a.repoPath

	tt.Type("带 空格")
	settle(t, a)
	tt.Frame()

	if !tt.HasText("名字里不能有空格") {
		t.Fatalf("该给出不能用的理由：%q", tt.Texts())
	}
	if err := tt.Click("创建标签"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.op != gitrepo.OpNone {
		t.Errorf("灰按钮点下去不该发起写操作，a.op = %v", a.op)
	}
	if got := gitOut(t, dir, "tag", "-l"); got != "" {
		t.Errorf("不该有标签被建出来：%q", got)
	}
}

// Esc 关掉对话框，什么都不留下。
func TestTagDialogEscapeCancels(t *testing.T) {
	a, tt, _ := tagDialogApp(t)

	tt.Type("v9")
	tt.Key(0, ui.KeyEscape)
	settle(t, a)
	tt.Frame()

	if a.tag.on {
		t.Error("Esc 该关掉对话框")
	}
	if a.tag.name != "" {
		t.Errorf("关掉之后状态该清干净，实际还留着 %q", a.tag.name)
	}
	if got := gitOut(t, a.repoPath, "tag", "-l"); got != "" {
		t.Errorf("Esc 不该建出标签：%q", got)
	}
}

// 菜单/命令栏那条「新建标签…」打的是 HEAD：rev 传空，交给 git 自己解析。
func TestNewTagAtHead(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	head := gitOut(t, a.repoPath, "rev-parse", "HEAD")
	a.newTagAtHead()
	settle(t, a)
	tt.Frame()

	if !a.tag.on {
		t.Fatal("对话框没打开")
	}
	if !tt.HasText("打在 main") {
		t.Errorf("对话框该说清打在 HEAD（main）上：%q", tt.Texts())
	}
	tt.Type("at-head")
	// 附注标签要说明，这条路上不填说明 —— 打成轻量标签
	tt.Click("附注标签")
	tt.Click("创建标签")
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "rev-parse", "at-head^{commit}"); got != head {
		t.Errorf("标签落在 %s，want HEAD 的 %s", got, head)
	}
}

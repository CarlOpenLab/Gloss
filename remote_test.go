package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 远程三件事在界面这一层：工具栏那三颗按钮、快捷键、以及成与不成之后屏幕上
// 留下什么。
//
// 验收回到**仓库之外**的那一份上（一个本地裸仓库）：推送的全部意义就是远程
// 上多出来一个提交，拿我们自己的 Load 去验自己的写，两边一起错的时候测试
// 还是绿的（和 stage_test / checkout_test 一个规矩）。

// bareRemote 建一个本地裸仓库当远程：不起服务器、不碰网络，但在 git 那边
// 是完完全全的另一次推和拉。
func bareRemote(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "--bare", "-b", "main")
	return dir
}

// remoteApp 是一个连着真远程的仓库：main 上有一个提交，还没推上去。
func remoteApp(t *testing.T) (*app, string) {
	t.Helper()
	bare := bareRemote(t)
	dir := newRepoDir(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "base\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	runGit(t, dir, "remote", "add", "origin", bare)
	return loadApp(t, dir), bare
}

// cloneOf 是「另一个人」的工作副本：同一个远程的另一份克隆。
func cloneOf(t *testing.T, bare string) string {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "clone", "-q", bare, ".")
	runGit(t, dir, "config", "user.email", "other@example.com")
	runGit(t, dir, "config", "user.name", "Other")
	return dir
}

// commitIn 在 dir 里写一个文件并提交。
func commitIn(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, name), content)
	runGit(t, dir, "add", name)
	runGit(t, dir, "commit", "-m", msg)
}

// remoteHead 是远程那条分支现在指向哪儿。
func remoteHead(t *testing.T, bare, branch string) string {
	t.Helper()
	return gitOut(t, bare, "rev-parse", "refs/heads/"+branch)
}

// 点「推送」，提交真的到了远程上 —— 这一整块功能存在的意义。
func TestPushSendsToRemote(t *testing.T) {
	a, bare := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if err := tt.Click("推送"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	want := gitOut(t, a.repoPath, "rev-parse", "HEAD")
	if got := remoteHead(t, bare, "main"); got != want {
		t.Errorf("远程 main = %s，本地 HEAD = %s —— 推送没落到远程上", got, want)
	}
	// 第一次推要顺手把上游设好，不然下一次还得再折腾一遍
	if up := gitOut(t, a.repoPath, "for-each-ref", "--format=%(upstream)", "refs/heads/main"); up != "refs/remotes/origin/main" {
		t.Errorf("推完的上游 = %q，want refs/remotes/origin/main", up)
	}
	// 成功走 Toast：读屏软件听到的就是这句话
	if !hasAnnouncement(tt.Announcements(), "已推送 main") {
		t.Errorf("该播报「已推送 main」：%q", tt.Announcements())
	}
	if a.opErr != nil {
		t.Errorf("成功的一次不该留失败横幅：%+v", *a.opErr)
	}
}

// 没有远程的仓库（本地新建的）：工具栏上根本不画那三颗，快捷键也什么都不做。
//
// 藏起来而不是画成灰的：有没有远程是仓库的固有属性，不是一时的状态 ——
// 三颗永远灰着的按钮占掉的是「刷新」的地方。快捷键那条路要堵住更实在的原因：
// `git fetch --all` 在没有远程时**静默成功**，真放它跑，用户会收到一句
// 「已抓取远程的更新」—— 一个什么都没做的谎。
func TestNoRemoteButtonsWithoutRemote(t *testing.T) {
	a := testApp(t) // 本地仓库，没有远程
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if len(a.remotes) != 0 {
		t.Fatalf("前提就不对：这个仓库有远程 %v", a.remotes)
	}
	for _, label := range []string{"抓取", "拉取", "推送"} {
		if _, ok := tt.Find(label); ok {
			t.Errorf("没有远程时工具栏上不该有 %q", label)
		}
	}

	// 快捷键那条路也堵着：按钮不在，⇧⌘F 却按得下去
	tt.Key(ui.Cmd|ui.Shift, ui.KeyF)
	settle(t, a)
	tt.Frame()
	if a.op != gitrepo.OpNone || a.opNote != "" {
		t.Errorf("没有远程时 ⇧⌘F 也该什么都不做：op=%v note=%q", a.op, a.opNote)
	}
}

// 抓取：别人推上来的提交，点一下就在本地的远程分支上出现；工作区一动不动。
func TestFetchUpdatesRemoteRefsWithoutTouchingWorktree(t *testing.T) {
	a, bare := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 先把 main 推上去，作为「别人的起点」
	runGit(t, a.repoPath, "push", "-q", "-u", "origin", "main")
	other := cloneOf(t, bare)
	commitIn(t, other, "b.txt", "别人写的\n", "别人的提交")
	runGit(t, other, "push", "-q", "origin", "main")

	before := gitOut(t, a.repoPath, "rev-parse", "HEAD")
	filesBefore := len(a.files)

	if err := tt.Click("抓取"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "rev-parse", "refs/remotes/origin/main"); got != remoteHead(t, bare, "main") {
		t.Errorf("抓取之后 origin/main = %s，远程上是 %s", got, remoteHead(t, bare, "main"))
	}
	if got := gitOut(t, a.repoPath, "rev-parse", "HEAD"); got != before {
		t.Errorf("抓取不该动本地分支：HEAD = %s，之前是 %s", got, before)
	}
	if len(a.files) != filesBefore {
		t.Errorf("抓取不该动文件列表：%d → %d", filesBefore, len(a.files))
	}
	if !hasAnnouncement(tt.Announcements(), "已抓取远程的更新") {
		t.Errorf("该播报抓取成功：%q", tt.Announcements())
	}
}

// 拉取：远程的新提交合进当前分支，工作区跟着变。
func TestPullBringsCommitsIntoWorktree(t *testing.T) {
	a, bare := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	runGit(t, a.repoPath, "push", "-q", "-u", "origin", "main")
	other := cloneOf(t, bare)
	commitIn(t, other, "c.txt", "别人的一行\n", "远程上的提交")
	runGit(t, other, "push", "-q", "origin", "main")

	if err := tt.Click("拉取"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if got := gitOut(t, a.repoPath, "rev-parse", "HEAD"); got != remoteHead(t, bare, "main") {
		t.Errorf("拉取之后 HEAD = %s，远程是 %s", got, remoteHead(t, bare, "main"))
	}
	// 拉下来的文件是**已提交**的，所以它出现在工作区磁盘上，而不在改动列表里
	// （列表列的是没提交的东西）—— 这里容易想当然地断言错。
	if _, err := os.Stat(filepath.Join(a.repoPath, "c.txt")); err != nil {
		t.Errorf("拉下来的文件该在工作区里：%v", err)
	}
}

// 拉取撞上分叉（本地和远程各有一个对方没有的提交）：git 自己不肯动，
// 横幅要把「先选一次合并还是变基」说出来。
//
// 这是分叉之后第一次拉取的**真实**结局 —— 比冲突还靠前：没配过 pull.rebase
// 的仓库根本走不到合并那一步。这条测试逼出来一个真 bug：原来的模式串是照着
// 想象的错误消息写的，git 的实际原话里那句在末尾的 fatal 行上。
func TestPullDivergedExplainsHowToChoose(t *testing.T) {
	a, bare := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	runGit(t, a.repoPath, "push", "-q", "-u", "origin", "main")
	other := cloneOf(t, bare)
	commitIn(t, other, "a.txt", "别人写的版本\n", "别人的改动")
	runGit(t, other, "push", "-q", "origin", "main")
	commitIn(t, a.repoPath, "a.txt", "我写的版本\n", "我的改动")

	if err := tt.Click("拉取"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.opErr == nil {
		t.Fatal("分叉时拉取该留下横幅")
	}
	if a.opErr.Title != "本地和远程分叉了" {
		t.Errorf("标题 = %q（原话：%q）", a.opErr.Title, a.opErr.Detail)
	}
	if !strings.Contains(a.opErr.Hint, "pull.rebase") {
		t.Errorf("该给出选哪边的办法：%q", a.opErr.Hint)
	}
	// Detail 留的是那句 fatal，不是它上面八行 hint 的残句
	if !strings.Contains(a.opErr.Detail, "Need to specify how to reconcile") {
		t.Errorf("Detail 该留最后那句结论：%q", a.opErr.Detail)
	}
}

// 配好 pull.rebase=false 之后，同一个分叉会真的合并 —— 撞冲突。
// 横幅说人话，冲突文件按冲突的样子（`!`）出现在列表里：光说「合并有冲突」
// 而不指出是哪个文件，用户下一步还是不知道去哪儿。
func TestPullConflictShowsBannerAndFiles(t *testing.T) {
	a, bare := remoteApp(t)
	// 选一次「合并」：git 2.27 起不选就不肯拉，这一步是用户自己在终端里做的
	runGit(t, a.repoPath, "config", "pull.rebase", "false")
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	runGit(t, a.repoPath, "push", "-q", "-u", "origin", "main")
	other := cloneOf(t, bare)
	commitIn(t, other, "a.txt", "别人写的版本\n", "别人的改动")
	runGit(t, other, "push", "-q", "origin", "main")
	commitIn(t, a.repoPath, "a.txt", "我写的版本\n", "我的改动")

	if err := tt.Click("拉取"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.opErr == nil {
		t.Fatal("冲突该留下横幅")
	}
	if a.opErr.Title != "合并有冲突" {
		t.Errorf("标题 = %q（原话：%q）", a.opErr.Title, a.opErr.Detail)
	}
	var found *gitrepo.File
	for i := range a.files {
		if a.files[i].NewPath == "a.txt" {
			found = &a.files[i]
		}
	}
	if found == nil {
		t.Fatalf("冲突文件该在列表里：%v", a.files)
	}
	if found.Status != gitrepo.StatusConflict {
		t.Errorf("a.txt 的状态 = %v，want 冲突", found.Status)
	}
}
func TestPushRejectedShowsFriendlyBanner(t *testing.T) {
	a, bare := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	runGit(t, a.repoPath, "push", "-q", "-u", "origin", "main")
	other := cloneOf(t, bare)
	commitIn(t, other, "d.txt", "别人先推的\n", "别人先推的提交")
	runGit(t, other, "push", "-q", "origin", "main")

	// 我这边也提交一个：两边分叉，推送必被拒
	commitIn(t, a.repoPath, "e.txt", "我这边也提交了\n", "我这边的提交")

	if err := tt.Click("推送"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()

	if a.opErr == nil {
		t.Fatal("推送被拒该留下横幅")
	}
	if a.opErr.Title != "远程有你本地没有的提交" {
		t.Errorf("标题 = %q", a.opErr.Title)
	}
	if !tt.HasText("远程有你本地没有的提交") {
		t.Errorf("横幅该画在屏幕上：%q", tt.Texts())
	}
	if !strings.Contains(a.opErr.Detail, "rejected") {
		t.Errorf("Detail 该留 git 的原话：%q", a.opErr.Detail)
	}
	// 失败之后还站在原处：远程不该被动过
	if got := remoteHead(t, bare, "main"); got == gitOut(t, a.repoPath, "rev-parse", "HEAD") {
		t.Error("推送明明被拒了，远程却动到了本地这个提交上")
	}
}

// 分离 HEAD 上：拉取和推送没有落点，抓取照常。
func TestRemoteButtonsOnDetachedHead(t *testing.T) {
	a, _ := remoteApp(t)
	runGit(t, a.repoPath, "push", "-q", "-u", "origin", "main")
	runGit(t, a.repoPath, "checkout", "-q", "--detach", "HEAD")
	a.reload()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if !a.detached {
		t.Fatal("前提就不对：这会儿该是分离 HEAD")
	}

	if err := tt.Click("推送"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()
	if a.op != gitrepo.OpNone {
		t.Errorf("分离 HEAD 上点灰着的「推送」不该发起写操作，a.op = %v", a.op)
	}

	// 抓取不碰当前分支，照常能用 —— 三颗一起灰掉是把「没有落点」当成了
	// 「和远程没关系」。
	if err := tt.Click("抓取"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	tt.Frame()
	if !hasAnnouncement(tt.Announcements(), "已抓取远程的更新") {
		t.Errorf("分离 HEAD 上抓取该照常工作：%q", tt.Announcements())
	}
}

// 快捷键绕得过灰按钮，绕不过判断：分离 HEAD 上按 ⇧⌘U，得到的是一句
// 说得清的失败，而不是把短哈希当成分支名推出去（git 会报「src refspec
// 不匹配」，那句话谁读都摸不着头脑）。
func TestPushShortcutOnDetachedExplains(t *testing.T) {
	a, bare := remoteApp(t)
	runGit(t, a.repoPath, "push", "-q", "-u", "origin", "main")
	runGit(t, a.repoPath, "checkout", "-q", "--detach", "HEAD")
	a.reload()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyU)
	settle(t, a)
	tt.Frame()

	if a.opErr == nil {
		t.Fatal("该留下一条说得清的失败横幅")
	}
	if a.opErr.Title != "现在没有分支在手上" {
		t.Errorf("标题 = %q", a.opErr.Title)
	}
	if got := gitOut(t, bare, "for-each-ref", "--format=%(refname)"); got != "refs/heads/main" {
		t.Errorf("远程上不该凭这一下多出任何分支：%q", got)
	}
}

// 进行中的那几秒：按钮灰着，工具栏上写着在做什么。
func TestToolbarShowsRemoteOpInFlight(t *testing.T) {
	a, _ := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 直接摆出「正在抓取」的状态：真实的抓取几毫秒就跑完，抓不稳这两帧。
	a.op = gitrepo.OpFetch
	a.opName = "正在抓取…"
	tt.Frame()

	if !tt.HasText("正在抓取…") {
		t.Errorf("工具栏该写着在做什么：%q", tt.Texts())
	}
	if err := tt.Click("推送"); err != nil {
		t.Fatal(err)
	}
	if a.op != gitrepo.OpFetch {
		t.Errorf("有操作在飞的时候不该再起一个：a.op = %v", a.op)
	}
}

// 菜单里那三条和工具栏那三颗是同一件事的两种入口：可用性必须是同一套判断
// （syncMenu 和 remoteBar 都问 canReachRemote）—— 菜单亮着而按钮是灰的，
// 用户不知道该信哪个。
func TestRemoteMenuMirrorsButtons(t *testing.T) {
	a, _ := remoteApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	m := buildAppMenu(a)
	tt.Frame()

	wantEnabled(t, m, "抓取", true)
	wantEnabled(t, m, "拉取", true)
	wantEnabled(t, m, "推送", true)

	// 点菜单和点按钮走的是同一条命令
	clickMenuItem(t, m, "抓取")
	settle(t, a)
	tt.Frame()
	if !hasAnnouncement(tt.Announcements(), "已抓取远程的更新") {
		t.Errorf("点菜单里的「抓取」该真的抓一次：%q", tt.Announcements())
	}

	// 分离 HEAD：拉取和推送没有落点，抓取照常
	runGit(t, a.repoPath, "checkout", "-q", "--detach", "HEAD")
	a.reload()
	settle(t, a)
	tt.Frame()
	if !a.detached {
		t.Fatal("前提就不对：这会儿该是分离 HEAD")
	}
	wantEnabled(t, m, "拉取", false)
	wantEnabled(t, m, "推送", false)
	wantEnabled(t, m, "抓取", true)
}

// 没有远程的仓库：菜单里那三条也是灰的。
func TestRemoteMenuWithoutRemote(t *testing.T) {
	a := testApp(t)
	m := buildAppMenu(a)
	ui.NewTester(a.view, 1280, 860).Frame()

	wantEnabled(t, m, "抓取", false)
	wantEnabled(t, m, "拉取", false)
	wantEnabled(t, m, "推送", false)
}

// 灰按钮要说得出自己为什么灰。
//
// 框架不给禁用元素报 hover，所以 tooltip 挂在外层盒子上 —— 这条链路没有
// 别的地方会替它报错，坏了就是一片沉默的灰。
func TestDisabledRemoteButtonExplainsItself(t *testing.T) {
	a, _ := remoteApp(t)
	runGit(t, a.repoPath, "checkout", "-q", "--detach", "HEAD")
	a.reload()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	if !a.detached {
		t.Fatal("前提就不对：这会儿该是分离 HEAD")
	}
	r, ok := tt.Find("推送")
	if !ok {
		t.Fatalf("界面上找不到「推送」：%q", tt.Texts())
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	// 框架的 tooltip 要悬停满 600ms 才画（ui/widgets.go 的 Tooltip）
	time.Sleep(700 * time.Millisecond)
	tt.Frame()

	if !tt.HasText("HEAD 是分离的") {
		t.Errorf("悬停在灰掉的推送上该说出原因：%q", tt.Texts())
	}
}

// hasAnnouncement 说播报里有没有这句话（包含匹配）。
func hasAnnouncement(all []string, want string) bool {
	for _, s := range all {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}

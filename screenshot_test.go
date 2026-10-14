package main

import (
	"context"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// realTitleBar 是真实窗口里红绿灯占的地方（在本机量出来的：窗口用
// TitleBarHidden + TrafficLightPosition{X:20,Y:19}）。无头渲染默认没有
// 窗口控件，不摆上它就看不出侧栏顶上会不会被关闭按钮压住。
var realTitleBar = ui.TitleBar{Height: 52, Left: 80}

// still 关掉这个 tester 上的动效。
//
// 截图要的是「稳下来之后长什么样」，而它在无头环境下不一定稳得下来：
// 淡入、颜色过渡这些都按真实时间走（tester 没有自己的钟），而两张截图
// 之间只隔几微秒 —— 拍到的是动画的第一帧：底色刚亮起还几乎全透明。
// 接上系统的「减弱动态效果」这一档，框架就会直接摆到终点，拍到的正是
// 用户看惯了的那一帧。
func still(t *ui.Tester) {
	t.SetPreferences(ui.Preferences{ReduceMotion: true})
}

// 这个文件是人工检查工具：把界面渲染成 PNG，用眼看排版对不对。
// 只在 SHOT=1 时跑，不进常规测试。
func TestDumpScreenshot(t *testing.T) {
	if os.Getenv("SHOT") == "" {
		t.Skip("设置 SHOT=1 才跑")
	}
	a := richApp(t)
	// 展示远程分支落后与超前的 HeroUI Action Pill 状态
	a.remotes = []string{"origin"}
	a.ahead = 2
	a.behind = 1

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	// 先空跑一帧让列表把行高量出来，否则首帧的字号/行高还没稳定
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	writePNG(t, "/tmp/de-shot-light.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-dark.png", tt.Image())

	// 全部暂存后的状态截图（展示 masterStageBox 勾选、全部取消按钮、已暂存状态胶囊）
	a.stageAll()
	settle(t, a)
	tt.SetDark(false)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-staged-light.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-staged-dark.png", tt.Image())
	a.unstageAll()
	settle(t, a)
	tt.SetDark(false)
	tt.Frame()

	// 最近项目下拉弹窗（HeroUI 风格）。它挂在工具栏那块仓库名上，
	// 而那块只在侧栏收起时才摆出来（开着的时候项目名归顶栏的标签）。
	a.sidebarOn = false
	a.repoDropdownOpen = true
	tt.SetDark(false)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-dropdown-light.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-dropdown-dark.png", tt.Image())
	a.repoDropdownOpen = false
	a.sidebarOn = true
	tt.SetDark(false)
	tt.Frame()

	// 再单独看一眼带语法高亮的那个文件（它在列表靠下的位置，默认看不到）
	a.fileFilter = "router"
	tt.SetDark(false)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-code-light.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-code-dark.png", tt.Image())

	// split 视图：左侧旧、右侧新，配不上的那一侧留白
	a.fileFilter = ""
	a.split = true
	tt.SetDark(false)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-split.png", tt.Image())

	// 分支对比视图（Fork 风格）：两分支比对栏、差异文件、diff
	a.startCompare("main", "feature", true)
	a.compare.state = loadReady
	a.compare.commits = []gitrepo.Commit{
		{Short: "a1b2c3d", Subject: "feat: add user authentication"},
		{Short: "e4f5g6h", Subject: "fix: update router handler"},
	}
	tt.SetDark(false)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-compare.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-compare-dark.png", tt.Image())
	a.exitCompare()
	tt.SetDark(false)

	// 模型设置对话框
	a.openSettings()
	a.editCfg.Providers[0].APIKey = "sk-ant-xxxxxxxxxxxxxxxx"
	tt.Frame()
	writePNG(t, "/tmp/de-shot-settings.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-settings-dark.png", tt.Image())
	tt.SetDark(false)

	// 展示 Magpie 本地网关接入状态
	a.magpieOnline = true
	a.magpieModels = []string{"opencode-go/deepseek-v4.1-flash", "opencode-go/claude-haiku-5-5", "codex-2/gpt-6-sol"}
	a.connectMagpie()
	tt.Frame()
	writePNG(t, "/tmp/de-shot-settings-magpie.png", tt.Image())
	a.settingsOn = false

	// 设置里的「更新」页。无头环境里 updater 插件没注册（mygo.Updater.Enabled()
	// 为 false），所以这里伪造一份可用状态，看的是打包发行版里真正会出现的排版。
	origLoadPrefs := loadUpdatePrefs
	loadUpdatePrefs = func() updatePrefs {
		return updatePrefs{
			available:          true,
			automaticChecks:    true,
			automaticDownloads: false,
			lastCheck:          time.Date(2026, 10, 14, 9, 30, 0, 0, time.Local),
		}
	}
	a.openSettings()
	a.settingsTab = settingsTabUpdates
	tt.Frame()
	writePNG(t, "/tmp/de-shot-settings-updates.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-settings-updates-dark.png", tt.Image())
	tt.SetDark(false)
	loadUpdatePrefs = origLoadPrefs
	a.settingsOn = false

	// 折叠：第一个文件标成已 review，它就在 diff 里折起来了。
	// 前面试筛选的时候列表滚下去过，这里先滚回顶上。
	a.toggleViewed(a.files[0])
	a.diffList.ScrollTo(0, ui.Start)
	tt.Frame()
	tt.Frame()
	writePNG(t, "/tmp/de-shot-folded.png", tt.Image())
	a.toggleViewed(a.files[0])

	// 文件行右键菜单截不进图：菜单是系统的原生菜单，不在这一帧的画面里
	// （测试里只能读它的条目，见 filemenu_test.go）。这里改拍它点开之后的
	// 那个「查看文件历史…」弹窗 —— 挑一列提交、点一条跳去历史 tab。
	if len(a.files) > 0 {
		now := time.Now()
		a.fileHist.on = true
		a.fileHist.path = pathOf(a.files[0])
		a.fileHist.state = loadReady
		a.fileHist.commits = []gitrepo.Commit{
			{Hash: "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", Short: "a1b2c3d", Subject: "feat: add router middleware",
				Author: "carl", When: now.Add(-2 * time.Hour)},
			{Hash: "e4f5g6h7i8j9e4f5g6h7i8j9e4f5g6h7i8j9e4f5", Short: "e4f5g6h", Subject: "fix: handle empty route list",
				Author: "carl", When: now.Add(-26 * time.Hour)},
			{Hash: "1122334455661122334455661122334455661122", Short: "1122334", Subject: "refactor: split handler from routing",
				Author: "mika", When: now.AddDate(0, 0, -9)},
		}
		tt.Frame()
		writePNG(t, "/tmp/de-shot-filehist.png", tt.Image())
		tt.SetDark(true)
		tt.Frame()
		writePNG(t, "/tmp/de-shot-filehist-dark.png", tt.Image())
		tt.SetDark(false)
		a.closeFileHistory()
		tt.Frame()
	}

	// ⌘K 命令栏
	a.openPalette()
	tt.Frame()
	writePNG(t, "/tmp/de-shot-palette.png", tt.Image())
	a.closePalette()

	// 查找：栏开着、有命中，一眼看出「当前那条」和「其余的」深浅不同
	a.find.on = true
	a.find.query = "CHANGED"
	tt.Frame()
	tt.Frame()
	writePNG(t, "/tmp/de-shot-find.png", tt.Image())
	tt.SetDark(true)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-find-dark.png", tt.Image())
	tt.SetDark(false)
	a.closeFind()

	// 空状态：没打开仓库时第一眼看到的东西
	empty := newApp()
	et := ui.NewTester(empty.view, 1280, 860)
	still(et)
	et.Frame()
	et.SetTitleBar(realTitleBar)
	et.Frame()
	writePNG(t, "/tmp/de-shot-empty.png", et.Image())
	et.SetDark(true)
	et.Frame()
	writePNG(t, "/tmp/de-shot-empty-dark.png", et.Image())

	// 打开了一个不是仓库的文件夹：首屏最常踩的那个坑 —— 拖错了文件夹，
	// 看到的是一句 git 的英文。这里看的就是那句话摆在哪儿、多大。
	bad := newApp()
	bad.openRepo(t.TempDir())
	settle(t, bad)
	bt := ui.NewTester(bad.view, 1280, 860)
	still(bt)
	bt.Frame()
	bt.SetTitleBar(realTitleBar)
	bt.Frame()
	writePNG(t, "/tmp/de-shot-open-failed.png", bt.Image())

	// 窄窗口：窗口的最小宽度（main.go 里设的 720）。工具栏摆不下时
	// 右边的动作按钮该收进「»」菜单，而不是被裁掉或者把这一行挤折。
	tt.SetSize(720, 620)
	tt.Frame()
	writePNG(t, "/tmp/de-shot-narrow.png", tt.Image())
	tt.SetSize(1280, 860)
	tt.Frame()

	// 半暂存的仓库：一部分改动进了 index，文件列表分成「已暂存 / 未暂存」
	// 两段，勾选框一半是勾上的，hunk 头上是「取消暂存」。
	half := richApp(t)
	runGit(t, half.repoPath, "add", "README.md")
	half.reload()
	settle(t, half)
	ht2 := ui.NewTester(half.view, 1280, 860)
	still(ht2)
	ht2.Frame()
	ht2.SetTitleBar(realTitleBar)
	ht2.Frame()
	writePNG(t, "/tmp/de-shot-sections.png", ht2.Image())
	ht2.SetDark(true)
	ht2.Frame()
	writePNG(t, "/tmp/de-shot-sections-dark.png", ht2.Image())

	// 提交栏的两种活态：输入了消息（按钮亮起来）、正在提交（转圈）
	half.commitMsg = "把 README 的措辞改一下"
	half.commitFocused = true
	ht2.SetDark(false)
	ht2.Frame()
	writePNG(t, "/tmp/de-shot-commit.png", ht2.Image())
	half.op, half.opName = gitrepo.OpCommit, "正在提交…"
	ht2.Frame()
	writePNG(t, "/tmp/de-shot-commit-busy.png", ht2.Image())
	half.op = gitrepo.OpNone

	// 失败横幅：git 的原话留在 Detail 里，Title/Hint 是人话
	half.opErr = &gitrepo.Failure{
		Title:  "提交被仓库的钩子拦下了",
		Hint:   "下面是钩子自己的原话，照它说的改。",
		Detail: "husky - pre-commit hook exited with code 1 (error)\nremote: ✗ 有文件没跑 gofmt",
	}
	ht2.Frame()
	writePNG(t, "/tmp/de-shot-op-error.png", ht2.Image())
	ht2.SetDark(true)
	ht2.Frame()
	writePNG(t, "/tmp/de-shot-op-error-dark.png", ht2.Image())
	half.opErr = nil
	ht2.SetDark(false)

	// 历史 tab：上半是提交列表（带拓扑图），下半是该提交的「文件列表 + diff」。
	// 用带分支的仓库 —— 一条直线的仓库看不出泳道画得对不对，也看不出
	// 主线换泳道之后颜色还连不连续。
	hist := graphApp(t)
	ht := ui.NewTester(hist.view, 1280, 860)
	still(ht)
	ht.Frame()
	ht.SetTitleBar(realTitleBar)
	hist.setTab(1)
	settle(t, hist)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-history.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-history-dark.png", ht.Image())

	// 换个合并提交、切到 Split：上栏那一行连出的线最多，下栏左右对照也出来了
	hist.split = true
	if err := ht.Click("合并支线一"); err != nil {
		t.Fatal(err)
	}
	settle(t, hist)
	ht.SetDark(false)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-history-split.png", ht.Image())

	// 打标签对话框：历史里右键一个提交点出来的那张表单
	cm := hist.commits[0]
	hist.openTagDialog(cm.Hash, cm.Short+" · "+cm.Subject)
	hist.tag.name = "v1.0.0"
	hist.tag.msg = "第一个正式版本。\n\n- 提交拓扑图\n- 变更预检"
	ht.Frame()
	writePNG(t, "/tmp/de-shot-tag.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-tag-dark.png", ht.Image())

	// 重名：错误留在对话框里，名字和说明一个字都不能丢
	hist.tag.err = gitrepo.Describe(gitrepo.OpTag, errors.New("fatal: tag 'v1.0.0' already exists"))
	ht.SetDark(false)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-tag-error.png", ht.Image())
	hist.tag = tagState{}

	// 重置对话框：默认是混合档，三档的说明摆在一起。第二张换成硬重置，
	// 工作区里留一点没提交的改动 —— 那句「会丢什么」是这张卡片的重点。
	// （改一个已跟踪的文件：新加的文件是未跟踪，硬重置不碰，数不进警告。）
	writeFile(t, filepath.Join(hist.repoPath, "init.txt"), "初始提交\n手头又改了一行\n")
	hist.reload()
	settle(t, hist)
	tcm := hist.commits[0]
	hist.openResetDialog(tcm)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-reset.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-reset-dark.png", ht.Image())
	hist.reset.mode = gitrepo.ResetHard
	ht.SetDark(false)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-reset-hard.png", ht.Image())
	hist.reset = resetState{}

	// 新建分支对话框：从某条提交起步，默认勾着「建好就切过去」
	hist.openNewBranchDialog(tcm.Hash, tcm.Short+" · "+tcm.Subject)
	hist.newBranch.name = "feature/health-check"
	ht.Frame()
	writePNG(t, "/tmp/de-shot-newbranch.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-newbranch-dark.png", ht.Image())
	ht.SetDark(false)
	hist.newBranch = branchState{}

	// 重命名分支对话框：预填当前分支名
	hist.openRenameBranchDialog("feat2")
	hist.rename.newName = "feature/payment"
	ht.Frame()
	writePNG(t, "/tmp/de-shot-rename.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-rename-dark.png", ht.Image())
	ht.SetDark(false)
	hist.rename = renameState{}

	// 删除分支确认对话框
	hist.askConfirm(confirmState{
		title: "删除分支 feat2",
		body:  "确定要删除本地分支 feat2 吗？",
		yes:   "删除分支",
	})
	ht.Frame()
	writePNG(t, "/tmp/de-shot-delete-branch.png", ht.Image())
	hist.confirm = confirmState{}

	// 变基确认对话框
	hist.promptRebase("feat2")
	ht.Frame()
	writePNG(t, "/tmp/de-shot-rebase-confirm.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-rebase-confirm-dark.png", ht.Image())
	ht.SetDark(false)
	hist.confirm = confirmState{}

	// 删除远程分支确认对话框
	hist.askConfirm(confirmState{
		title: "删除远程分支 origin/feature/payment",
		body:  "确定要从 origin 删除远程分支 feature/payment 吗？\n这会从远端彻底移除该分支，其他协作者将无法再访问它。你的本地分支不会被删除。",
		yes:   "删除远程分支",
	})
	ht.Frame()
	writePNG(t, "/tmp/de-shot-delete-remote-branch.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-delete-remote-branch-dark.png", ht.Image())
	ht.SetDark(false)
	hist.confirm = confirmState{}

	// 强制推送二次确认对话框
	hist.askConfirm(confirmState{
		title: "强制推送 main",
		body:  "将使用 --force-with-lease 覆盖 origin/main。\n这会改写远程提交历史。如果别人在你上次抓取后向该分支推送了新提交，Git 会安全地拒绝本次操作。",
		yes:   "强制推送",
	})
	ht.Frame()
	writePNG(t, "/tmp/de-shot-force-push.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-force-push-dark.png", ht.Image())
	ht.SetDark(false)
	hist.confirm = confirmState{}

	// 储藏工作区改动对话框
	hist.openStashDialog()
	hist.stashDlg.message = "正在重构的临时改动"
	ht.SetDark(false)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-stash.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-stash-dark.png", ht.Image())
	ht.SetDark(false)
	hist.stashDlg = stashState{}

	// 丢弃储藏二次确认对话框
	hist.promptStashDrop(gitrepo.Ref{Name: "stash@{0}", Subject: "On main: 正在重构的临时改动"})
	ht.SetDark(false)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-stash-drop.png", ht.Image())
	ht.SetDark(true)
	ht.Frame()
	writePNG(t, "/tmp/de-shot-stash-drop-dark.png", ht.Image())
	ht.SetDark(false)
	hist.confirm = confirmState{}

	// 分离 HEAD：工具栏那枚分支芯片换成警示色，把话说全
	det := branchApp(t)
	runGit(t, det.repoPath, "checkout", "-q", "--detach", "HEAD")
	det.reload()
	settle(t, det)
	dt := ui.NewTester(det.view, 1280, 860)
	still(dt)
	dt.Frame()
	dt.SetTitleBar(realTitleBar)
	dt.Frame()
	writePNG(t, "/tmp/de-shot-detached.png", dt.Image())
	dt.SetDark(true)
	dt.Frame()
	writePNG(t, "/tmp/de-shot-detached-dark.png", dt.Image())

	// 检出被工作区挡下：这里让 git 真报一次错再翻，拍到的才是用户会看到的话
	blk := branchApp(t)
	writeFile(t, filepath.Join(blk.repoPath, "b.txt"), "未跟踪的同名文件\n")
	blk.reload()
	settle(t, blk)
	if repo, err := gitrepo.Open(context.Background(), blk.repoPath); err == nil {
		ferr := repo.CheckoutLocal(context.Background(), "topic")
		f := gitrepo.Describe(gitrepo.OpCheckout, ferr)
		blk.opErr = &f
	}
	bt2 := ui.NewTester(blk.view, 1280, 860)
	still(bt2)
	bt2.Frame()
	bt2.SetTitleBar(realTitleBar)
	bt2.Frame()
	writePNG(t, "/tmp/de-shot-checkout-blocked.png", bt2.Image())
	bt2.SetDark(true)
	bt2.Frame()
	writePNG(t, "/tmp/de-shot-checkout-blocked-dark.png", bt2.Image())

	// 远程：工具栏上那三颗图标按钮（这个仓库连着真的裸仓库，所以它们是亮的）
	rem, bare := remoteApp(t)
	runGit(t, rem.repoPath, "push", "-q", "-u", "origin", "main")
	other := cloneOf(t, bare)
	commitIn(t, other, "b.txt", "别人先推的\n", "别人先推的提交")
	runGit(t, other, "push", "-q", "origin", "main")
	commitIn(t, rem.repoPath, "c.txt", "我这边也提交了\n", "我这边的提交")
	rem.reload()
	settle(t, rem)
	rt := ui.NewTester(rem.view, 1280, 860)
	still(rt)
	rt.Frame()
	rt.SetTitleBar(realTitleBar)
	rt.Frame()
	writePNG(t, "/tmp/de-shot-remote.png", rt.Image())
	if r, ok := rt.Find("推送"); ok {
		rt.Move(r.X+r.W/2, r.Y+r.H/2)
		rt.Frame()
		writePNG(t, "/tmp/de-shot-hover.png", rt.Image())
		rt.Move(-100, -100)
		rt.Frame()
	}
	rt.SetDark(true)
	rt.Frame()
	writePNG(t, "/tmp/de-shot-remote-dark.png", rt.Image())
	if r, ok := rt.Find("推送"); ok {
		rt.Move(r.X+r.W/2, r.Y+r.H/2)
		rt.Frame()
		writePNG(t, "/tmp/de-shot-hover-dark.png", rt.Image())
		rt.Move(-100, -100)
		rt.Frame()
	}
	rt.SetDark(false)

	// 推送被拒：还是让 git 真拒一次再翻 —— Detail 里那几行原话是 gitDetail
	// 从真实的输出里挑的，手拼一段拍不出它挑得对不对
	if repo, err := gitrepo.Open(context.Background(), rem.repoPath); err == nil {
		ferr := repo.Push(context.Background(), "main")
		f := gitrepo.Describe(gitrepo.OpPush, ferr)
		rem.opErr = &f
	}
	rt.Frame()
	writePNG(t, "/tmp/de-shot-remote-error.png", rt.Image())
	rt.SetDark(true)
	rt.Frame()
	writePNG(t, "/tmp/de-shot-remote-error-dark.png", rt.Image())
	rem.opErr = nil

	// 进行中：按钮灰着，工具栏上写着在做什么
	rem.op, rem.opName = gitrepo.OpFetch, "正在抓取…"
	rt.SetDark(false)
	rt.Frame()
	writePNG(t, "/tmp/de-shot-remote-busy.png", rt.Image())
	rem.op = gitrepo.OpNone

	// 拉取撞上冲突：横幅 + 冲突文件（`!`）在列表里。这是「拉取出错」最常见
	// 的那张脸，也让冲突状态在界面上的样子有个留底。
	con, cbare := remoteApp(t)
	runGit(t, con.repoPath, "config", "pull.rebase", "false") // 用户自己在终端里选的那次
	runGit(t, con.repoPath, "push", "-q", "-u", "origin", "main")
	co := cloneOf(t, cbare)
	commitIn(t, co, "a.txt", "别人写的版本\n", "别人的改动")
	runGit(t, co, "push", "-q", "origin", "main")
	commitIn(t, con.repoPath, "a.txt", "我写的版本\n", "我的改动")
	if repo, err := gitrepo.Open(context.Background(), con.repoPath); err == nil {
		ferr := repo.Pull(context.Background())
		f := gitrepo.Describe(gitrepo.OpPull, ferr)
		con.opErr = &f
	}
	con.reload()
	settle(t, con)
	ct := ui.NewTester(con.view, 1280, 860)
	still(ct)
	ct.Frame()
	ct.SetTitleBar(realTitleBar)
	ct.Frame()
	writePNG(t, "/tmp/de-shot-pull-conflict.png", ct.Image())

	// 合并/变基停在冲突上：琥珀色状态条（继续/跳过/中止）、冲突文件排在
	// 列表里、变基时分支芯片改口。这是阶段 D 的主角，四个状态各拍一张。
	mc, _ := mergeConflictApp(t)
	mt := ui.NewTester(mc.view, 1280, 860)
	still(mt)
	mt.Frame()
	mt.SetTitleBar(realTitleBar)
	mt.Frame()
	writePNG(t, "/tmp/de-shot-conflict-merge.png", mt.Image())
	mt.SetDark(true)
	mt.Frame()
	writePNG(t, "/tmp/de-shot-conflict-merge-dark.png", mt.Image())
	mt.SetDark(false)

	rb, _ := rebaseConflictApp(t)
	rtb := ui.NewTester(rb.view, 1280, 860)
	still(rtb)
	rtb.Frame()
	rtb.SetTitleBar(realTitleBar)
	rtb.Frame()
	writePNG(t, "/tmp/de-shot-conflict-rebase.png", rtb.Image())
	rtb.SetDark(true)
	rtb.Frame()
	writePNG(t, "/tmp/de-shot-conflict-rebase-dark.png", rtb.Image())
	rtb.SetDark(false)

	// 删除-修改冲突：对方删了文件、我这边改了它。卡片的白话不一样 ——
	// 按「用被并入的」是**接受删除**，不提前说清就是挖坑，这张留底。
	dc, _ := deleteConflictApp(t)
	dct := ui.NewTester(dc.view, 1280, 860)
	still(dct)
	dct.Frame()
	dct.SetTitleBar(realTitleBar)
	dct.Frame()
	writePNG(t, "/tmp/de-shot-conflict-delete.png", dct.Image())
	dct.SetDark(true)
	dct.Frame()
	writePNG(t, "/tmp/de-shot-conflict-delete-dark.png", dct.Image())
	dct.SetDark(false)

	// 确认框：中止合并之前的那一问。确认键写完整动作名（「中止合并」），
	// 焦点默认在「取消」上 —— 回车落在安全的那一边。
	mc.abortOp()
	mt.Frame()
	writePNG(t, "/tmp/de-shot-confirm.png", mt.Image())
	mt.SetDark(true)
	mt.Frame()
	writePNG(t, "/tmp/de-shot-confirm-dark.png", mt.Image())
	mt.SetDark(false)
	mc.confirm = confirmState{}

	// 有远程的仓库收窄到最小宽度：多了三颗图标，工具栏还能不能不折行地
	// 摆下（摆不下的按顺序进「»」）。没远程的窄窗是上面那张 de-shot-narrow。
	rt.SetSize(720, 620)
	rt.Frame()
	writePNG(t, "/tmp/de-shot-remote-narrow.png", rt.Image())
	rt.SetSize(1280, 860)
	rt.Frame()

	// 卡在冲突里的仓库收窄到最小宽度：状态条上两行话加三颗按钮，
	// 720px 下会不会把正文挤没、按钮会不会折行。
	mt.SetSize(720, 620)
	mt.Frame()
	writePNG(t, "/tmp/de-shot-conflict-narrow.png", mt.Image())
	mt.SetSize(1280, 860)
	mt.Frame()
}

func TestDumpToolbarOpCollision(t *testing.T) {
	if os.Getenv("SHOT") == "" {
		t.Skip("设置 SHOT=1 才跑")
	}
	a := richApp(t)
	a.repoName = "yunlue-web-ts7"
	a.repoPath = "/Users/carl/Desktop/guoji/yunlue-web-ts7"
	a.branch = "refactor/shared-components"
	a.sidebarOn = true
	a.tab = 1
	a.op = gitrepo.OpPush
	a.opName = "正在推送…"
	a.remotes = []string{"origin"}
	a.ahead = 0
	a.behind = 0

	tt := ui.NewTester(a.view, 1280, 860)
	still(tt)
	tt.Frame()
	tt.SetTitleBar(realTitleBar)
	tt.Frame()

	repoRect, _ := tt.Find("yunlue-web-ts7")
	branchRect, ok1 := tt.Find("refactor/shared-components")
	opRect, ok2 := tt.Find("正在推送…")
	t.Logf("repoRect: %+v", repoRect)
	t.Logf("branchRect: %+v (found=%v)", branchRect, ok1)
	t.Logf("opRect: %+v (found=%v)", opRect, ok2)
	writePNG(t, "/tmp/de-shot-collision.png", tt.Image())
}
func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

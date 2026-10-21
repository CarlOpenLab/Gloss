package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// —— Git 配置（设置里的「Git 配置」页）——
//
// 这一页只回答一个问题：**提交时 git 认为你是谁**。所以它管的就是
// user.name / user.email 这两项，别的键一概不碰 —— 分支、远程那些结构性
// 配置在别的面板里有正经入口，core.* 之类跟着具体操作走，摆在这里当普通
// 键值改，改错了不容易看出来，改对了也看不出效果。
//
// 三层里同名项的生效次序是 项目级 > 全局 > 系统级：
//
//   - 全局那份（一般是 ~/.gitconfig）跟着用户走，这台机器上每个仓库都吃它；
//   - 项目级那份（.git/config）只认一个仓库，可以「跟随全局」，也可以单独设；
//   - 系统级那份（/etc/gitconfig 之类）是整台机器的，只读摆出来。
//
// 只读的那层摆出来是有用的：全局没设、系统级设了的话，那一项照样生效，不看
// 系统级就说不清「这个名字到底哪儿来的」。
//
// 写盘交给命令行 git（`git config --global/--local`），和这个项目里所有别的
// git 操作一个路子：配置文件里的 include、条件、注释怎么写，就还是怎么写，
// 不由 Gloss 另造一份自己的解释。

// gitConfigRow 是表格里的一行。
//
// 编辑改的是 Value，Original 留着做对比：保存时只写真正动过的行。整份重写
// 会把用户文件里的注释、include 顺序、还有 Gloss 读不懂的写法一起抹平。
type gitConfigRow struct {
	Key      string
	Value    string
	Original string
	// Fresh 表示这一行是在界面上新加的（文件里本来没有）。
	// 它和「文件里有一条空值」得分开：前者删掉就是反悔，后者删掉要真去删。
	Fresh bool
	// Multi 说明这个键在配置文件里出现了不止一次（url.*.insteadof 这类）。
	// 只显示最后一条 —— git 取值也是最后一条赢 —— 但得让人知道保存会把它
	// 压成一条，否则改完再看会以为丢东西了。
	Multi bool
	// Deleted 是「这一行不要了」的意图。不立刻删，跟别的改动一起等保存，
	// 中途后悔还能点回来。
	Deleted bool
}

// changed 说明这一行要不要写回文件。
func (r *gitConfigRow) changed() bool {
	if r.Deleted || r.cleared() {
		// 文件里本来没有这一项，那就没什么可写也没什么可删。
		return !r.Fresh
	}
	return r.Value != r.Original
}

// removes 说明保存时该把这一项从文件里删掉，而不是写进去。
func (r *gitConfigRow) removes() bool { return r.Deleted || r.cleared() }

// cleared 说明这个框被清空了。
//
// 清空 = 这一层不再设它，而不是设成空字符串：`user.name = ""` 会让 git 拿空
// 名字去提交，不会有人想要这个结果。所以清空既有的项等于删掉它（下面的层
// 接着生效），清空一个本来就没有的项等于什么也不做。
func (r *gitConfigRow) cleared() bool { return strings.TrimSpace(r.Value) == "" }

// gitConfigGroup 是表格里的一栏（全局 / 当前项目 / 系统级）。
type gitConfigGroup struct {
	Scope string
	Title string
	Desc  string
	Icon  *ui.SVG
	// Files 是这一栏的条目来自哪些文件，摆出来给人指路用的：配置文件读
	// 不懂的地方（include、条件、手写格式）总要回文件里看。为空说明这一层
	// 一条都没有 —— git 只报读到的文件，读不到的不报路径。
	Files []string
	Rows  []*gitConfigRow
	// ReadOnly 的栏只展示：系统级配置改它要管理员权限，而且那是别人的东西，
	// 一个仓库客户端不该顺手替人动。
	ReadOnly bool
	// InheritGlobal 只对项目级那栏有意义：这个仓库跟随全局身份，还是自己单独
	// 设一份。跟随的意思就是 .git/config 里不写 user.name / user.email ——
	// 保存时把本来有的删掉，让全局那份生效。
	InheritGlobal bool
	// identityBackup 是切到「跟随全局」那一刻两行的样子，切回来按它恢复：
	// 用户敲了一半再切走切回，不该把敲的东西弄丢。
	identityBackup []*gitConfigRow
	// scopeSel 是「身份来源」那个分段控件的选中下标（0 跟随全局、1 单独设置）。
	//
	// 它必须存在这上面而不是渲染时的局部变量里：分段控件是在**下一帧**的输入
	// 阶段往这个指针里写选中的，指针要是每帧新建的局部变量，写进去的就是上一
	// 帧那个已经没人看的变量 —— 表现为「点得中、没反应」。
	scopeSel int
}

// gitIdentityKeys 是这一页管的两项：提交时署的身份。
//
// 只有这两项。git 的配置有几百个键，把整份配置文件摊在一个列表里改，最容易
// 出事的恰恰是那些结构性的（branch.*、remote.*）—— 它们有专门的入口，
// 在这里改等于绕开所有校验。这一页的职责窄一点，但每一条都改得明白。
var gitIdentityKeys = []string{"user.name", "user.email"}

func isIdentityKey(key string) bool {
	return key == "user.name" || key == "user.email"
}

// gitIdentityFieldLabel 是身份项在界面上的名字。
func gitIdentityFieldLabel(key string) string {
	if key == "user.email" {
		return T("邮箱", "Email")
	}
	return T("姓名", "Name")
}

// gitConfigState 是这一页的全部状态。
type gitConfigState struct {
	groups  []*gitConfigGroup
	loaded  bool
	loading bool
	// loadedFor 是这一份配置站在哪个目录读的：项目级配置只有站在仓库里才
	// 看得见，换了仓库就得重读。
	loadedFor string
	err       string
	// seq 守住异步读：换仓库、点重读都会让在飞的那一次作废。
	seq int
}

// gitConfigDir 是读配置时该站在哪里。
//
// 有仓库就站在仓库里（否则读不到项目级那一栏），没有就站在进程当前目录 ——
// 全局与系统级照样读得到，只是没有项目级。
//
// 先问标签页再问 repoPath：标签页是「你现在看的这个仓库」，而 repoPath 在
// 一次打开失败之后还记着上一次的路径。两个都没有时留空，git 就站在进程
// 当前目录上跑 —— 那是唯一一个不会读错别人配置的选择。
func (a *app) gitConfigDir() string {
	if tab := a.currentTab(); tab != nil && tab.path != "" {
		return tab.path
	}
	return a.repoPath
}

// ensureGitConfig 保证这一页拿到的是当前目录的配置，没有就去读一次。
//
// 惰性读：读这一趟要起 git 进程，没点进这一页的人不该为它买单。每打开一次
// 设置面板都会作废上一次的结果（见 openSettings），所以面板里看到的永远是
// 这一趟打开时的样子。
func (a *app) ensureGitConfig() {
	dir := a.gitConfigDir()
	if a.gitCfg.loading || (a.gitCfg.loaded && a.gitCfg.loadedFor == dir) {
		return
	}
	a.gitCfg.loading = true
	a.gitCfg.seq++
	seq := a.gitCfg.seq
	a.run(func() {
		entries, err := gitrepo.ReadConfig(context.Background(), dir)
		a.dispatch(func() {
			if a.gitCfg.seq != seq {
				return
			}
			a.gitCfg.loading = false
			a.gitCfg.err = ""
			if err != nil {
				// 读失败就什么都不摆：半份配置比一句「没读到」更容易让人改错。
				a.gitCfg.err = err.Error()
				a.gitCfg.loaded = true
				a.gitCfg.loadedFor = dir
				a.gitCfg.groups = nil
				return
			}
			a.gitCfg.groups = buildGitConfigGroups(entries)
			a.gitCfg.loaded = true
			a.gitCfg.loadedFor = dir
		})
	})
}

// reloadGitConfig 是「重新读取」：把草稿丢掉，从文件里再读一遍。
func (a *app) reloadGitConfig() {
	a.gitCfg.loaded = false
	a.ensureGitConfig()
}

// buildGitConfigGroups 把读回来的条目分成界面上三栏。
//
// 顺序就是显示顺序：能改的两栏在前（全局改得最多），只读的系统级垫后。
func buildGitConfigGroups(entries []gitrepo.ConfigEntry) []*gitConfigGroup {
	groups := []*gitConfigGroup{
		{
			Scope: gitrepo.ScopeGlobal,
			Title: T("全局配置", "Global"),
			Desc:  T("这台机器上你的每一个仓库都吃它", "Applies to every repository on this machine"),
			Icon:  iconGlobe,
		},
		{
			Scope: gitrepo.ScopeLocal,
			Title: T("当前项目", "Current Project"),
			Desc:  T("只作用于当前仓库", "Applies to the current repository only"),
			Icon:  iconFolder,
		},
		{
			Scope:    gitrepo.ScopeSystem,
			Title:    T("系统级", "System"),
			Desc:     T("整台机器的配置，这里只读", "Machine-wide; read-only here"),
			Icon:     iconShield,
			ReadOnly: true,
		},
	}
	byScope := map[string]*gitConfigGroup{}
	for _, g := range groups {
		byScope[g.Scope] = g
	}
	seen := map[*gitConfigGroup]map[string]*gitConfigRow{}
	for _, e := range entries {
		g := byScope[e.Scope]
		if g == nil {
			// git 报的「unknown」：macOS 上 /Applications/Xcode.app/…/gitconfig
			// 这类厂商文件就是这一档。它从效果上就是系统级（谁都用、谁也不该
			// 在 GUI 里改），归到那一栏展示。
			if e.Scope == "unknown" {
				g = byScope[gitrepo.ScopeSystem]
			} else {
				continue
			}
		}
		// 文件路径照收（不管这一项是不是身份项）：它要回答的是「这一层是从
		// 哪些文件读出来的」，用户得知道保存会落到哪个文件上。
		if e.File != "" && !containsString(g.Files, e.File) {
			g.Files = append(g.Files, e.File)
		}
		// 不是身份项的一律不上界面。它们仍然好好待在配置文件里，只是这一页
		// 不碰 —— 分支、远程这些结构性配置在别的面板里有正经入口。
		if !isIdentityKey(e.Key) {
			continue
		}
		if seen[g] == nil {
			seen[g] = map[string]*gitConfigRow{}
		}
		if row, ok := seen[g][e.Key]; ok {
			// 同名多条只留最后一条（git 取值也是最后一条赢），并记下
			// 「这里原本不止一条」——保存会把它压成一条。
			row.Value = e.Value
			row.Original = e.Value
			row.Multi = true
			continue
		}
		row := &gitConfigRow{Key: e.Key, Value: e.Value, Original: e.Value}
		seen[g][e.Key] = row
		g.Rows = append(g.Rows, row)
	}
	// 能改的两栏固定摆出姓名与邮箱两项：文件里没有也摆，用户才有地方填。
	// 系统级只读，只摆文件里真有的 —— 只读的地方摆两个空框没意义。
	for _, scope := range []string{gitrepo.ScopeGlobal, gitrepo.ScopeLocal} {
		g := byScope[scope]
		for _, key := range gitIdentityKeys {
			if seen[g][key] != nil {
				continue
			}
			g.Rows = append(g.Rows, &gitConfigRow{Key: key, Fresh: true})
		}
	}
	// 项目级默认跟不跟随全局：.git/config 里本来没这两项，那就是跟随。
	local := byScope[gitrepo.ScopeLocal]
	local.InheritGlobal = true
	for _, r := range local.Rows {
		if !r.Fresh {
			local.InheritGlobal = false
			break
		}
	}
	local.scopeSel = 1
	if local.InheritGlobal {
		local.scopeSel = 0
	}
	for _, g := range groups {
		sortRowsByKey(g.Rows)
	}
	return groups
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func sortRowsByKey(rows []*gitConfigRow) {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
}

// gitSettingsPane 是「Git 配置」这一页。
func (a *app) gitSettingsPane(c *ui.Context, pal palette) {
	a.ensureGitConfig()

	ui.Scroll(c).FillWidth().FillHeight().
		Padding(settingsPadTop, settingsPadX, settingsPadBot, settingsPadX).Children(func() {
		ui.Column(c).Gap(22).FillWidth().MaxWidth(620).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(12).Children(func() {
				ui.Text(c, T("改的是 git 自己的配置文件，不是 Gloss 的设置。这里只管提交时署的身份（姓名与邮箱）—— 分支、远程那些在各自的面板里改。同名项按 项目级 > 全局 > 系统级 生效；保存时只写动过的那几项，配置文件里的注释和顺序都不动。",
					"These are git's own config files, not Gloss preferences. This page covers only the identity commits are signed with (name and email) — branches and remotes live in their own panes. For duplicate keys, local beats global beats system; saving touches only what you changed, leaving comments and ordering intact.")).
					FontSize(textBodySM).TextColor(pal.textMuted).LineHeight(1.4).Grow(1).MinWidth(0)
				a.gitConfigReloadButton(c, pal)
			})

			if a.gitCfg.loading && len(a.gitCfg.groups) == 0 {
				ui.Text(c, T("正在读取配置…", "Reading config…")).FontSize(textBodySM).TextColor(pal.inkSubtle)
			}
			if a.gitCfg.err != "" {
				gitConfigErrorCard(c, pal, T("读 Git 配置没成功", "Could not read git config"), a.gitCfg.err)
			}
			for _, g := range a.gitCfg.groups {
				a.gitConfigGroupCard(c, pal, g)
			}
		})
	})
}

// gitValueWidth 是身份值那一列的宽度：宽窗口固定 320，窄窗口留出
// 210 的名字格之后把余量让给它，再窄就收到 90。
func (a *app) gitValueWidth(c *ui.Context) float32 {
	winW, _ := c.Size()
	panelW := clamp(winW-40, 640, 860)
	cardW := panelW - settingsNavW - 2*settingsCardGap
	contentW := cardW - 2*settingsPadX
	return clamp(contentW-210-12, 90, 320)
}

// gitConfigReloadButton 是这一页右上角那颗「重新读取」。
//
// 配置文件是别的东西也能改的（命令行、别的 git 客户端），面板开着的时候
// 那边改完这边不会知道 —— 得留一条明路把它重读一遍。
func (a *app) gitConfigReloadButton(c *ui.Context, pal palette) {
	btn := secondaryAction(c, pal, T("重新读取", "Reload"),
		T("从文件里重新读一遍，丢掉还没保存的改动", "Re-read from disk; discards unsaved edits"), false)
	if btn.Clicked() {
		a.reloadGitConfig()
	}
}

// gitConfigErrorCard 是整页读失败时的说法。
//
// 借失败横幅那一套颜色和图标（warnIcon + Danger）：读不到配置文件是
// 「这件事没成」，不是一条中性的说明 —— 摆成灰色的，人一眼扫过去会当
// 它是注释。
func gitConfigErrorCard(c *ui.Context, pal palette, title, detail string) {
	t := c.Theme()
	ui.Column(c).Gap(3).FillWidth().Padding(10, 14).Radius(radiusMD).
		Background(t.Danger.Alpha(0.10)).Border(1, t.Danger.Alpha(0.35)).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			warnIcon(c, 14, t.Danger)
			ui.Text(c, title).FontSize(textBodySM).FontWeight(weightSemibold).TextColor(pal.ink)
		})
		if detail != "" {
			// git 的原话：等宽、小一号 —— 它是证据不是结论。
			ui.Text(c, detail).Font("monospace").FontSize(textCaption).TextColor(pal.inkMuted)
		}
	})
}

// gitConfigGroupCard 画一栏：标题、说明、来源文件，然后是身份那两项。
func (a *app) gitConfigGroupCard(c *ui.Context, pal palette, g *gitConfigGroup) {
	// 项目级这栏多一句「身份从哪儿来」：跟随全局，还是这个仓库单独设一份。
	// 另外两栏没得选 —— 全局本来就是「全局那份」，系统级只读。
	local := g.Scope == gitrepo.ScopeLocal
	ui.Column(c).Gap(8).FillWidth().Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Padding(0, 4).Children(func() {
			drawIcon(c, g.Icon, 14, pal.textMuted)
			ui.Text(c, g.Title).FontSize(textBodySM).FontWeight(weightSemibold).TextColor(pal.textMuted)
			if g.ReadOnly {
				specChip(c, pal, T("只读", "Read-only"), ui.Transparent, 18, textCaption)
			}
		})
		desc := g.Desc
		if len(g.Files) > 0 {
			desc += "　" + strings.Join(g.Files, "　·　")
		}
		if desc != "" {
			ui.Text(c, desc).FontSize(textBodySM).TextColor(pal.textFaint).SingleLine().
				LineHeight(1.35)
		}

		ui.Column(c).Radius(settingsGroupR).Background(pal.chip).
			FillWidth().Padding(4, 16).Children(func() {
			// 没打开仓库时项目级没什么可改的：摆出两个空框、点保存也不知道往
			// 哪儿写。
			if local && a.gitConfigDir() == "" {
				gitConfigEmptyRow(c, pal, a.gitConfigEmptyNote(g))
				return
			}
			// 系统级那一栏可能一项身份都没有（很常见）：说明白没读到，比摆两个
			// 空框强。
			if len(g.Rows) == 0 {
				gitConfigEmptyRow(c, pal, a.gitConfigEmptyNote(g))
				return
			}
			if local {
				a.gitIdentityScopeRow(c, pal, g)
				settingDivider(c, pal)
			}
			// 跟随全局：这一层什么都不写，摆出生效的那份让人看明白提交时署谁。
			if local && g.InheritGlobal {
				a.gitIdentityInheritedRow(c, pal)
				return
			}
			for i, row := range g.Rows {
				if i > 0 {
					settingDivider(c, pal)
				}
				if g.ReadOnly {
					gitConfigReadOnlyRow(c, pal, row)
				} else {
					a.gitConfigEditorRow(c, pal, g, row)
				}
			}
		})
	})
}

// gitIdentityScopeRow 画项目级那栏的「身份从哪儿来」。
//
// 这不是一个摆了就完了的开关：选「跟随全局」意味着保存时要把 .git/config 里
// 的 user.name / user.email 删掉（让全局那份生效），所以切换时改的是两行的
// 状态，见 setIdentityInherit；保存路径只认行的状态。
func (a *app) gitIdentityScopeRow(c *ui.Context, pal palette, g *gitConfigGroup) {
	note := T("这个仓库用下面填的值；留空就直接用全局那份。",
		"This repository uses the values below; leave a field empty to fall back to the global one.")
	if g.InheritGlobal {
		note = T("提交时用全局那份身份，这个仓库的 .git/config 里不写 user.name / user.email。",
			"Commits use the global identity; this repository keeps no user.name / user.email of its own.")
	}
	ui.Column(c).Gap(5).Padding(11, 0).FillWidth().Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(12).FillWidth().Children(func() {
			// 名字在左、控件在右：space-between，不是把控件摆在名字后面。
			ui.Text(c, T("身份来源", "Identity")).Grow(1).MinWidth(0).SingleLine().
				FontSize(textBodySM).TextColor(pal.ink)
			seg := settingsSegmented(c, pal, &g.scopeSel,
				T("跟随全局", "Inherit global"), T("单独设置", "Set here"))
			seg.Label(T("身份来源", "Identity"))
		})
		ui.Text(c, note).FontSize(textCaption).TextColor(pal.inkSubtle)
	})
	// 分段控件往 g.scopeSel 里写选中：这一帧看到的就是用户刚点的那一下。
	if (g.scopeSel == 0) != g.InheritGlobal {
		a.setIdentityInherit(g, g.scopeSel == 0)
	}
}

// setIdentityInherit 把项目级身份切到「跟随全局」或「这个仓库单独设」。
//
// 跟随全局的意思就是 .git/config 里不写这两项：所以这里不是记一个布尔值就完
// 事，得把两行改到对应状态 —— 文件里本来有的标记成待删（保存时删掉），用户
// 刚敲进来的清空（等于没填）。切回来按切换前的样子原样恢复，免得敲了一半切
// 走再切回来，敲的东西没了。
func (a *app) setIdentityInherit(g *gitConfigGroup, inherit bool) {
	if g.InheritGlobal == inherit {
		return
	}
	if inherit {
		g.identityBackup = nil
		for _, r := range g.Rows {
			backup := *r
			g.identityBackup = append(g.identityBackup, &backup)
			if r.Fresh {
				r.Value = ""
				r.Deleted = false
				continue
			}
			r.Deleted = true
		}
	} else if len(g.identityBackup) == len(g.Rows) {
		for i, r := range g.Rows {
			*r = *g.identityBackup[i]
		}
		g.identityBackup = nil
	}
	g.InheritGlobal = inherit
}

// gitIdentityInheritedRow 画「跟随全局」时的两行：值来自最终生效的那一层
// （项目级没设就看全局、再看系统级），只读 —— 这一层现在什么都不写。
func (a *app) gitIdentityInheritedRow(c *ui.Context, pal palette) {
	for i, key := range gitIdentityKeys {
		if i > 0 {
			settingDivider(c, pal)
		}
		val := a.gitIdentityFrom(key, gitrepo.ScopeGlobal, gitrepo.ScopeSystem)
		if val == "" {
			val = T("未设置 —— 提交时 git 会用主机名与邮箱兜底", "Not set — git falls back to hostname and email")
		}
		ui.Row(c).Padding(11, 0).AlignItems(ui.Center).Gap(12).FillWidth().Children(func() {
			gitIdentityKeyCell(c, pal, key, false, false)
			ui.Spacer(c)
			ui.Text(c, val).FontSize(textBodySM).TextColor(pal.inkMuted).
				Shrink(1).SingleLine().Ellipsis("…").TextAlign(ui.End)
		})
	}
}

// gitIdentityFrom 依次在给定几层里找某一项的值，先找到的先用。
//
// 看的是草稿里的值（不是文件里的原始值）：用户刚在全局那栏改完名字、还没保存，
// 项目级输入框的提示就该是改完的那个 —— 眼睛看见的和将要用上的是同一份。
func (a *app) gitIdentityFrom(key string, scopes ...string) string {
	for _, scope := range scopes {
		for _, g := range a.gitCfg.groups {
			if g.Scope != scope {
				continue
			}
			for _, r := range g.Rows {
				if r.Key == key && !r.cleared() && !r.Deleted {
					return r.Value
				}
			}
		}
	}
	return ""
}

// gitIdentityInherited 是「这一层不设时」会生效的值：项目级看全局与系统级，
// 全局看系统级。用作输入框的提示 —— 清空它就是要回到这个值。
func (a *app) gitIdentityInherited(key, scope string) string {
	if scope == gitrepo.ScopeGlobal {
		return a.gitIdentityFrom(key, gitrepo.ScopeSystem)
	}
	return a.gitIdentityFrom(key, gitrepo.ScopeGlobal, gitrepo.ScopeSystem)
}

// gitConfigEmptyRow 是一栏里没有内容时那句说明。
func gitConfigEmptyRow(c *ui.Context, pal palette, note string) {
	ui.Box(c).Padding(10, 14).FillWidth().Children(func() {
		ui.Text(c, note).FontSize(textCaption).TextColor(pal.inkSubtle)
	})
}

// gitConfigEmptyNote 说清这一栏为什么是空的。
func (a *app) gitConfigEmptyNote(g *gitConfigGroup) string {
	if g.Scope == gitrepo.ScopeLocal {
		if a.gitConfigDir() == "" {
			return T("先打开一个仓库，才能在这里改这个仓库的身份。",
				"Open a repository first to edit the identity it commits with.")
		}
		return T("这个仓库没有自己的身份设置。跟随全局就行 —— 选「单独设置」才需要填。",
			"This repository sets no identity of its own; the global one applies.")
	}
	return T("这一层没有设 user.name / user.email。",
		"This level sets neither user.name nor user.email.")
}

// gitConfigEditorRow 是可编辑的一行：姓名（user.name）或邮箱（user.email）。
//
// 清空 = 这一层不再设它（见 gitConfigRow.cleared）：本来就有的会被删掉，本来
// 没有的就什么也不写。提示文字给的是「下面几层现在生效的值」，清空之后会用
// 哪个名字，眼睛直接看得见。
func (a *app) gitConfigEditorRow(c *ui.Context, pal palette, g *gitConfigGroup, r *gitConfigRow) {
	ui.Row(c).Padding(11, 0).AlignItems(ui.Center).Gap(10).FillWidth().Children(func() {
		gitIdentityKeyCell(c, pal, r.Key, r.cleared(), r.Multi)
		if r.Deleted {
			// 待删的行不给编辑：这一行现在唯一有意义的事是「恢复」。
			ui.Text(c, T("保存时从这一层删掉", "Removed from this level on save")).FontSize(textCaption).
				TextColor(pal.inkSubtle).Grow(1)
			if secondaryAction(c, pal, T("恢复", "Restore"), "", false).Clicked() {
				r.Deleted = false
			}
			return
		}
		// 输入框自带一个名字：里面的字是画出来的，不进无障碍标签表 —— 不写这
		// 一行，读屏软件念的是一串「编辑框」，说不出这是给哪个仓库、哪一项填的。
		// 名字在左、输入框贴右：space-between，中间的空隙由 Spacer 吃掉。
		ui.Spacer(c)
		// 输入框自带一个名字：里面的字是画出来的，不进无障碍标签表 —— 不写这
		// 一行，读屏软件念的是一串「编辑框」，说不出这是给哪个仓库、哪一项填的。
		input := ui.TextInput(c, &r.Value).Width(a.gitValueWidth(c)).Shrink(0).
			Label(gitIdentityInputLabel(g.Scope, r.Key))
		if inherited := a.gitIdentityInherited(r.Key, g.Scope); inherited != "" {
			input.Placeholder(Tf("留空则用 %s", "Empty uses %s", inherited))
		}
	})
}

// gitIdentityInputLabel 给身份输入框一个能认出来的名字。
//
// 一页里有四个（全局/项目 × 姓名/邮箱），框里没字的时候长得一模一样，读屏
// 软件和自动化测试只能靠这个名字分辨谁是谁。
func gitIdentityInputLabel(scope, key string) string {
	if scope == gitrepo.ScopeGlobal {
		return Tf("全局配置 %s", "Global %s", key)
	}
	return Tf("项目配置 %s", "Project %s", key)
}

// gitIdentityKeyCell 是行左那个名字格：界面上叫什么（姓名/邮箱），下面小字
// 是它落的键名 —— 写进文件的就是后者，两个都摆出来省得猜。
func gitIdentityKeyCell(c *ui.Context, pal palette, key string, dim, multi bool) {
	ui.Column(c).Width(210).Shrink(0).Gap(2).Children(func() {
		col := pal.ink
		if dim {
			col = pal.inkTertiary
		}
		ui.Text(c, gitIdentityFieldLabel(key)).FontSize(textBodySM).TextColor(col).SingleLine()
		caption := key
		if multi {
			caption += "　" + T("原有多条，保存后合并为一条", "Multiple entries; merged into one on save")
		}
		ui.Text(c, caption).FontSize(textCaption).TextColor(pal.inkSubtle).SingleLine()
	})
}

// gitConfigReadOnlyRow 是只读的一行：值排在名字右边，只看得见改不了。
func gitConfigReadOnlyRow(c *ui.Context, pal palette, r *gitConfigRow) {
	ui.Row(c).Padding(11, 0).AlignItems(ui.Center).Gap(12).FillWidth().Children(func() {
		gitIdentityKeyCell(c, pal, r.Key, false, r.Multi)
		ui.Spacer(c)
		ui.Text(c, r.Value).FontSize(textBodySM).TextColor(pal.inkMuted).
			Shrink(1).SingleLine().Ellipsis("…").TextAlign(ui.End)
	})
}

// gitConfigChange 是一条要落到文件上的改动。
type gitConfigChange struct {
	scope  string
	key    string
	value  string
	remove bool
}

// pendingGitConfigChanges 汇总这一份草稿里真正动过的行。
func (a *app) pendingGitConfigChanges() []gitConfigChange {
	var out []gitConfigChange
	for _, g := range a.gitCfg.groups {
		if g.ReadOnly {
			continue
		}
		for _, r := range g.Rows {
			if !r.changed() {
				continue
			}
			out = append(out, gitConfigChange{
				scope:  g.Scope,
				key:    r.Key,
				value:  r.Value,
				remove: r.removes(),
			})
		}
	}
	return out
}

// applyGitConfig 把动过的行写回 git 配置文件（设置面板点「保存」时走到这儿）。
//
// 丢到后台跑：一条改动一个 git 进程，还可能要等文件锁；而且失败（键名不合法、
// 配置文件只读）不该卡住正在关闭的设置面板 —— 结果是横幅和吐司该说的事。
func (a *app) applyGitConfig() {
	changes := a.pendingGitConfigChanges()
	if len(changes) == 0 {
		return
	}
	dir := a.gitCfg.loadedFor
	// 写完就得重读：git 会按自己的规矩重排文件（--replace-all 把键挪到它自己
	// 的位置上），界面上这份草稿不再等于文件里那份。
	a.gitCfg.loaded = false
	a.gitCfg.groups = nil

	a.run(func() {
		ctx := context.Background()
		var firstErr error
		done := 0
		for _, ch := range changes {
			var err error
			if ch.remove {
				err = gitrepo.UnsetConfig(ctx, dir, ch.scope, ch.key)
			} else {
				err = gitrepo.WriteConfig(ctx, dir, ch.scope, ch.key, ch.value)
			}
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", ch.key, err)
				}
				continue
			}
			done++
		}
		a.dispatch(func() {
			if firstErr != nil {
				f := gitrepo.Describe(gitrepo.OpGitConfig, firstErr)
				a.opErr = &f
				return
			}
			a.opNote = Tf("已更新 %d 项 Git 配置", "Updated %d Git config entries", done)
		})
	})
}

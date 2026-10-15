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
// 这一页管的是 **git 自己的配置**，不是 Gloss 的设置：全局那份（一般是
// ~/.gitconfig）跟着用户走，这台机器上每个仓库都吃它；项目级那份
// （.git/config）只认一个仓库；系统级那份（/etc/gitconfig 之类）是整台
// 机器的，只读摆出来。
//
// 三层同名时后面的盖前面的（项目级 > 全局 > 系统级），所以三栏摆在一页里 ——
// 只看一层说不清「我这条到底生效了没有」。
//
// 写盘交给命令行 git（`git config --global/--local`），和这个项目里所有别的
// git 操作一个路子：配置文件里那些 include、条件、注释怎么写，就还是怎么写，
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
	if r.Deleted {
		return !r.Fresh
	}
	return r.Value != r.Original
}

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
	// New 是栏底「新增一行」那两个输入框的内容。
	New newGitConfigInput
}

type newGitConfigInput struct{ Key, Value string }

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
		if e.File != "" && !containsString(g.Files, e.File) {
			g.Files = append(g.Files, e.File)
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
	t := c.Theme()
	a.ensureGitConfig()

	ui.Scroll(c).FillWidth().FillHeight().Padding(20, 28).Children(func() {
		ui.Column(c).Gap(18).FillWidth().Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				drawIcon(c, iconGitBranch, 15, t.Accent)
				ui.Text(c, T("Git 配置", "Git Config")).FontSize(textBody).FontWeight(weightSemibold).TextColor(pal.ink)
				ui.Spacer(c)
				a.gitConfigReloadButton(c, pal)
			})
			ui.Text(c, T("改的是 git 自己的配置文件，不是 Gloss 的设置。同名项按 项目级 > 全局 > 系统级 生效；保存时只写动过的行，配置文件里的注释和顺序都不动。",
				"These are git's own config files, not Gloss preferences. For duplicate keys, local beats global beats system; saving touches only the rows you changed, leaving comments and ordering intact.")).
				FontSize(11).TextColor(t.TextMuted)

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

// gitConfigGroupCard 画一栏：标题、说明、来源文件，然后是条目表。
func (a *app) gitConfigGroupCard(c *ui.Context, pal palette, g *gitConfigGroup) {
	t := c.Theme()
	ui.Column(c).Gap(6).FillWidth().Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			drawIcon(c, g.Icon, 14, t.Accent)
			ui.Text(c, g.Title).FontSize(textBodySM).FontWeight(weightSemibold).TextColor(pal.ink)
			if g.ReadOnly {
				specChip(c, pal, T("只读", "Read-only"), ui.Transparent, 18, textCaption)
			}
		})
		desc := g.Desc
		if len(g.Files) > 0 {
			desc += "　" + strings.Join(g.Files, "　·　")
		}
		if desc != "" {
			ui.Text(c, desc).FontSize(textCaption).TextColor(pal.inkSubtle).SingleLine()
		}

		ui.Column(c).Radius(radiusMD).Background(pal.surface2).Border(1, pal.hairline).
			FillWidth().Children(func() {
			if len(g.Rows) == 0 {
				ui.Box(c).Padding(10, 14).FillWidth().Children(func() {
					ui.Text(c, a.gitConfigEmptyNote(g)).FontSize(textCaption).TextColor(pal.inkSubtle)
				})
			}
			for i, row := range g.Rows {
				if i > 0 {
					settingDivider(c, pal)
				}
				if g.ReadOnly {
					gitConfigReadOnlyRow(c, pal, row)
				} else {
					a.gitConfigEditorRow(c, pal, row)
				}
			}
			if !g.ReadOnly {
				settingDivider(c, pal)
				a.gitConfigNewRow(c, pal, g)
			}
		})
	})
}

// gitConfigEmptyNote 说清这一栏为什么是空的。
func (a *app) gitConfigEmptyNote(g *gitConfigGroup) string {
	switch g.Scope {
	case gitrepo.ScopeLocal:
		if a.gitConfigDir() == "" {
			return T("先打开一个仓库，才能在这里改项目级配置。", "Open a repository first to edit its local config.")
		}
		return T("这个仓库还没有自己的配置。在这里加一项，只影响它自己。", "This repository has no local config yet. Anything added here affects only it.")
	case gitrepo.ScopeGlobal:
		return T("还没读到全局配置。加一项再保存，就会写进 git 的全局配置文件（一般是 ~/.gitconfig）。",
			"No global config yet. Adding an entry writes to git's global config file (usually ~/.gitconfig).")
	default:
		return T("没有读到系统级配置。", "No system-level config found.")
	}
}

// gitConfigEditorRow 是可编辑的一行：键名、值、删掉。
func (a *app) gitConfigEditorRow(c *ui.Context, pal palette, r *gitConfigRow) {
	ui.Row(c).Padding(7, 14).AlignItems(ui.Center).Gap(10).FillWidth().Children(func() {
		gitConfigKeyCell(c, pal, r)
		if r.Deleted {
			// 删掉的行不给编辑：那颗「恢复」才是这一行现在唯一有意义的事。
			ui.Text(c, T("保存时删除", "Removed on save")).FontSize(textCaption).
				TextColor(pal.inkSubtle).Grow(1)
			if secondaryAction(c, pal, T("恢复", "Restore"), "", false).Clicked() {
				r.Deleted = false
			}
			return
		}
		// 输入框自带一个名字：它里面的字是画出来的，不进无障碍标签表 ——
		// 不写这一行，读屏软件念的是一串「编辑框」，说不出这是哪个键的值。
		ui.TextInput(c, &r.Value).FillWidth().Grow(1).
			Label(Tf("%s 的值", "%s value", r.Key))
		if removeButton(c, pal) {
			r.Deleted = true
		}
	})
}

// gitConfigKeyCell 是行左那个键名格（固定宽度，键名长短不一时值输入框才对得齐）。
func gitConfigKeyCell(c *ui.Context, pal palette, r *gitConfigRow) {
	ui.Column(c).Width(210).Shrink(0).Gap(2).Children(func() {
		col := pal.ink
		if r.Deleted {
			col = pal.inkTertiary
		}
		ui.Text(c, r.Key).FontSize(textBodySM).TextColor(col).SingleLine()
		if r.Multi {
			ui.Text(c, T("原有多条，保存后合并为一条", "Multiple entries; merged into one on save")).
				FontSize(textCaption).TextColor(pal.inkSubtle).SingleLine()
		}
	})
}

// gitConfigReadOnlyRow 是只读的一行：值排在键名右边，只看得见改不了。
func gitConfigReadOnlyRow(c *ui.Context, pal palette, r *gitConfigRow) {
	ui.Row(c).Padding(7, 14).AlignItems(ui.Center).Gap(10).FillWidth().Children(func() {
		ui.Text(c, r.Key).Width(210).Shrink(0).FontSize(textBodySM).TextColor(pal.ink).SingleLine()
		ui.Text(c, r.Value).FontSize(textBodySM).TextColor(pal.inkMuted).Grow(1).SingleLine()
	})
}

// gitConfigNewRow 是栏底那一行「新增」：填键名和值，回车前先点「添加」。
//
// 两栏（全局 / 当前项目）的提示文字故意写得不一样：这两个输入框长得一模
// 一样，只认「值」两个字的话，摆在一页里就分不出往哪一栏写。提示里带一句
// 作用范围，正好也是填之前该想一下的事。
func (a *app) gitConfigNewRow(c *ui.Context, pal palette, g *gitConfigGroup) {
	keyHint := T("键名，如 core.autocrlf", "Key, e.g. core.autocrlf")
	valueHint := T("值，作用于所有仓库", "Value, applies to all repositories")
	// 两栏的输入框名字也得分开：它们是同一页上同一种控件，名字一样的话
	// 读屏软件报的是两句一样的话，用键盘切过去也不知道自己站在哪一栏。
	keyLabel := T("全局配置项名", "Global config key")
	valueLabel := T("全局配置项值", "Global config value")
	if g.Scope == gitrepo.ScopeLocal {
		keyHint = T("键名，如 user.email", "Key, e.g. user.email")
		valueHint = T("值，只作用于这个仓库", "Value, applies to this repository only")
		keyLabel = T("项目配置项名", "Project config key")
		valueLabel = T("项目配置项值", "Project config value")
	}
	ui.Row(c).Padding(7, 14).AlignItems(ui.Center).Gap(10).FillWidth().Children(func() {
		ui.TextInput(c, &g.New.Key).Width(210).Placeholder(keyHint).Label(keyLabel)
		ui.TextInput(c, &g.New.Value).FillWidth().Grow(1).Placeholder(valueHint).Label(valueLabel)
		if secondaryAction(c, pal, T("添加", "Add"), "", strings.TrimSpace(g.New.Key) == "").Clicked() {
			a.addGitConfigRow(g)
		}
	})
}

// addGitConfigRow 把栏底那两格填的内容加成一行。
//
// 键名已经有一行时不另起一行，而是把值写进那一行 —— 否则一保存就是
// 「同名两条」，而界面上一个键只能占一行。
func (a *app) addGitConfigRow(g *gitConfigGroup) {
	key := strings.TrimSpace(g.New.Key)
	if key == "" {
		return
	}
	value := g.New.Value
	for _, r := range g.Rows {
		if r.Key == key {
			r.Deleted = false
			r.Value = value
			g.New = newGitConfigInput{}
			return
		}
	}
	g.Rows = append(g.Rows, &gitConfigRow{Key: key, Value: value, Original: "", Fresh: true})
	sortRowsByKey(g.Rows)
	g.New = newGitConfigInput{}
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
				remove: r.Deleted,
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

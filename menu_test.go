package main

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// menuItems 把菜单（含各层子菜单）摊平成一个列表。
func menuItems(m *mygo.Menu) []*mygo.MenuItem {
	var out []*mygo.MenuItem
	var walk func(items []*mygo.MenuItem)
	walk = func(items []*mygo.MenuItem) {
		for _, it := range items {
			out = append(out, it)
			walk(it.Submenu)
		}
	}
	walk(m.Items())
	return out
}

// menuItem 按标签找一项，找不到就直接失败 —— 标签是测试唯一认识它的方式。
func menuItem(t *testing.T, m *mygo.Menu, label string) *mygo.MenuItem {
	t.Helper()
	for _, it := range menuItems(m) {
		if it.Label == label {
			return it
		}
	}
	t.Fatalf("菜单里没有 %q", label)
	return nil
}

// clickMenuItem 点一下菜单里的某项（和用户点菜单走的是同一条路）。
func clickMenuItem(t *testing.T, m *mygo.Menu, label string) {
	t.Helper()
	it := menuItem(t, m, label)
	it.Click(it, nil)
}

func wantChecked(t *testing.T, m *mygo.Menu, label string, want bool) {
	t.Helper()
	if got := menuItem(t, m, label).IsChecked(); got != want {
		t.Errorf("菜单「%s」的勾 = %v, want %v", label, got, want)
	}
}

func wantEnabled(t *testing.T, m *mygo.Menu, label string, want bool) {
	t.Helper()
	if got := menuItem(t, m, label).IsEnabled(); got != want {
		t.Errorf("菜单「%s」可用 = %v, want %v", label, got, want)
	}
}

// 菜单里那几个键，和 keys.go 里 handleKeys 认的必须是同一组。
//
// 这两半没法互相验证 —— 真实 macOS 应用里菜单先把键吃掉，视图那条路根本
// 走不到，测试走的又只有视图那条。所以在这里把对应关系钉死：改了菜单上的
// 键，这条测试会红，提醒你去 keys.go 改另一半（反过来也一样）。
func TestMenuAccelerators(t *testing.T) {
	m := buildAppMenu(newApp())

	want := map[string]string{
		"打开仓库…":  "CmdOrCtrl+O",
		"查看改动":   "CmdOrCtrl+1",
		"查看历史":   "CmdOrCtrl+2",
		"模型设置…":  "CmdOrCtrl+,",
		"刷新":     "CmdOrCtrl+R",
		"显示侧栏":   "CmdOrCtrl+Shift+B",
		"放大代码字号": "CmdOrCtrl+Plus",
		"缩小代码字号": "CmdOrCtrl+-",
		"恢复默认字号": "CmdOrCtrl+0",
		"命令栏…":   "CmdOrCtrl+K",
		"筛选文件":   "CmdOrCtrl+F",
		"新建分支…":  "CmdOrCtrl+Shift+N",
		"抓取":     "CmdOrCtrl+Shift+F",
		"拉取":     "CmdOrCtrl+Shift+P",
		"推送":     "CmdOrCtrl+Shift+U",
	}
	for label, accel := range want {
		if got := menuItem(t, m, label).Accelerator; got != accel {
			t.Errorf("「%s」的键 = %q, want %q", label, got, accel)
		}
	}

	// 一个键只能有一个主人。默认菜单坏就坏在这里：⌘R / ⌘0 / ⌘+ / ⌘−
	// 被它自己的「重新载入 / 缩放」占着，而那几个角色在原生界面上什么
	// 都不做 —— 按下去等于按了个空。
	seen := map[string]string{}
	for _, it := range menuItems(m) {
		if it.Accelerator == "" {
			continue
		}
		if prev, dup := seen[it.Accelerator]; dup {
			t.Errorf("「%s」和「%s」抢同一个键 %s", prev, it.Label, it.Accelerator)
		}
		seen[it.Accelerator] = it.Label
	}

	// 更根本的一条：网页的那几个角色根本不该出现在菜单里。它们天生带着
	// 上面说的那几个键，而且是给网页用的。
	for _, it := range menuItems(m) {
		switch it.Role {
		case mygo.RoleReload, mygo.RoleForceReload, mygo.RoleResetZoom,
			mygo.RoleZoomIn, mygo.RoleZoomOut:
			t.Errorf("菜单里不该有 %q 这个角色：它会抢走 ⌘R / ⌘0 / ⌘+ / ⌘−，"+
				"而且在这个原生界面上什么都不做", it.Role)
		}
	}
}

// 菜单里的勾和可用性跟着应用状态走：用快捷键切了 tab、收了侧栏，
// 拉开菜单时勾必须已经在新位置上。
func TestMenuMirrorsState(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	m := buildAppMenu(a)
	tt.Frame()

	wantChecked(t, m, "查看改动", true)
	wantChecked(t, m, "查看历史", false)
	wantChecked(t, m, "显示侧栏", true)
	wantChecked(t, m, "统一视图", true)
	wantChecked(t, m, "左右对照", false)
	wantEnabled(t, m, "刷新", true)

	// 这三个状态都不是为菜单改的，菜单这边没人被主动通知过 ——
	// 全靠每帧那一次 syncMenu 自己追上。
	tt.Key(ui.Cmd, ui.Key2)
	tt.Frame()
	tt.Key(ui.Cmd|ui.Shift, ui.KeyB)
	tt.Frame()
	a.setSplit(true)
	tt.Frame()
	settle(t, a)

	wantChecked(t, m, "查看改动", false)
	wantChecked(t, m, "查看历史", true)
	wantChecked(t, m, "显示侧栏", false)
	wantChecked(t, m, "统一视图", false)
	wantChecked(t, m, "左右对照", true)
}

// 没有仓库时，刷新和两个 tab 都没有作用对象，灰掉；打开仓库和字号
// 不依赖仓库，照常可点。
func TestMenuWithoutRepo(t *testing.T) {
	m := buildAppMenu(newApp())

	wantEnabled(t, m, "刷新", false)
	wantEnabled(t, m, "查看改动", false)
	wantEnabled(t, m, "查看历史", false)
	wantEnabled(t, m, "新建分支…", false)
	wantEnabled(t, m, "打开仓库…", true)
	wantEnabled(t, m, "放大代码字号", true)
	wantEnabled(t, m, "筛选文件", true)

	// 远程三条：连仓库都没有，更谈不上远程。
	wantEnabled(t, m, "抓取", false)
	wantEnabled(t, m, "拉取", false)
	wantEnabled(t, m, "推送", false)
}

// 打开失败和没打开过是一个口径：这时候 repoPath 也记着东西（试的是哪个
// 文件夹，标题栏要摆），但刷新和两个 tab 一样没有作用对象 —— 菜单、工具栏、
// setTab 三处必须同一套判断，不然灰的灰、亮的点了没反应。
func TestMenuAfterFailedOpen(t *testing.T) {
	a := newApp()
	a.openRepo(t.TempDir())
	settle(t, a)
	if a.state != loadFailed {
		t.Fatalf("该是失败态，实际 %v", a.state)
	}
	m := buildAppMenu(a)
	ui.NewTester(a.view, 1280, 860).Frame()

	wantEnabled(t, m, "刷新", false)
	wantEnabled(t, m, "查看改动", false)
	wantEnabled(t, m, "查看历史", false)
	wantEnabled(t, m, "打开仓库…", true)
}

// 点菜单里的条目和按对应的快捷键是同一条命令。
func TestMenuCommands(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	m := buildAppMenu(a)
	tt.Frame()

	clickMenuItem(t, m, "查看历史")
	settle(t, a)
	tt.Frame()
	if a.tab != 1 {
		t.Fatalf("点「查看历史」之后 tab = %d, want 1", a.tab)
	}

	clickMenuItem(t, m, "放大代码字号")
	settle(t, a)
	if got := a.codeSizeValue(); got != defSize+codeSizeStep {
		t.Errorf("点「放大代码字号」之后字号 = %v, want %v", got, defSize+codeSizeStep)
	}

	clickMenuItem(t, m, "显示侧栏")
	settle(t, a)
	tt.Frame()
	if a.sidebarOn {
		t.Error("点「显示侧栏」应该把侧栏收起来")
	}

	// 对话框开着的时候让位：视图那条路在模态下会被框架挡掉，菜单这条路
	// 框架挡不着，得自己让。两边不然就不一个脾气了。
	//
	// 模态是在对话框被提交的那一帧才立起来的，所以先画一帧再说 ——
	// 也正是用户真正看到的顺序（对话框先出现在屏幕上，然后才按键）。
	a.settingsOn = true
	tt.Frame()
	before := a.tab
	tt.Key(ui.Cmd, ui.Key1)
	tt.Frame()
	if a.tab != before {
		t.Fatalf("设置面板开着时快捷键不该切 tab（前提就错了）")
	}
	clickMenuItem(t, m, "查看改动")
	settle(t, a)
	tt.Frame()
	if a.tab != before {
		t.Errorf("设置面板开着时菜单也不该在背后切 tab：tab = %d, want %d", a.tab, before)
	}
}

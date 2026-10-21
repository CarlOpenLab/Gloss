package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSidebarDragResize(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	tt.Frame()

	initW := a.sidebarWidth(nil)
	if initW != sidebarDefaultW {
		t.Fatalf("侧栏初始宽度应该是 %.0f，实际 %.0f", sidebarDefaultW, initW)
	}

	// 悬停在侧栏右边缘 4px 范围内，光标应变为东西双向箭头
	tt.Move(sidebarDefaultW-2, 300)
	tt.Frame()
	if c := tt.Cursor(); c != ui.CursorResizeEW {
		t.Errorf("悬停在侧栏边缘光标应该是 CursorResizeEW，实际是 %v", c)
	}

	// 拖拽手柄向右拉 50px
	tt.Press(sidebarDefaultW-2, 300)
	tt.Move(sidebarDefaultW-2+50, 300)
	tt.Frame()
	tt.Release(sidebarDefaultW-2+50, 300)
	tt.Frame()

	newW := a.sidebarWidth(nil)
	if newW <= initW {
		t.Errorf("拖动后侧栏宽度应变大：before=%.0f, after=%.0f", initW, newW)
	}

	// 验证已持久化到 store
	if cfg := a.store.Config(); cfg.SidebarWidth != newW {
		t.Errorf("侧栏宽度应持久化到 store：want %.0f, got %.0f", newW, cfg.SidebarWidth)
	}

	// 拖拽手柄向左拉到超出下限，应被 clamp 保护在 sidebarMinW
	curW := a.sidebarWidth(nil)
	tt.Press(curW-2, 300)
	tt.Move(curW-2-300, 300)
	tt.Frame()
	tt.Release(curW-2-300, 300)
	tt.Frame()
	if minClamped := a.sidebarWidth(nil); minClamped != sidebarMinW {
		t.Errorf("侧栏宽度向左拖动应受 clamp 保护在 %.0f，实际 %.0f", sidebarMinW, minClamped)
	}

	// 双击复位
	a.resetSidebarWidth()
	tt.Frame()
	if got := a.sidebarWidth(nil); got != sidebarDefaultW {
		t.Errorf("复位后侧栏宽度应为 %.0f，实际 %.0f", sidebarDefaultW, got)
	}
}

func TestFilePaneDragResize(t *testing.T) {
	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	initW := a.filePaneWidth(nil)
	if initW <= 0 {
		t.Fatalf("文件列表栏初始宽度不应为空")
	}

	// 主卡与侧栏之间留了 shellCardGap，文件栏从卡片内沿开始。
	sepX := a.sidebarWidth(nil) + shellCardGap + initW

	// 悬停在文件列表分隔条上，光标应变为东西双向箭头
	tt.Move(sepX, 500)
	tt.Frame()
	if c := tt.Cursor(); c != ui.CursorResizeEW {
		t.Errorf("悬停在文件列表边缘光标应该是 CursorResizeEW，实际是 %v", c)
	}

	// 拖拽手柄向右拉 40px
	tt.Press(sepX, 500)
	tt.Move(sepX+40, 500)
	tt.Frame()
	tt.Release(sepX+40, 500)
	tt.Frame()

	newW := a.filePaneWidth(nil)
	if newW <= initW {
		t.Errorf("拖动后文件栏宽度应变大：before=%.0f, after=%.0f", initW, newW)
	}

	// 验证已持久化到 store
	if cfg := a.store.Config(); cfg.FilePaneWidth != newW {
		t.Errorf("文件列表栏宽度应持久化到 store：want %.0f, got %.0f", newW, cfg.FilePaneWidth)
	}

	// 拖拽向左拉到超出下限，应被 clamp 保护在 filePaneMinW
	curW := a.filePaneWidth(nil)
	curSep := a.sidebarWidth(nil) + shellCardGap + curW
	tt.Press(curSep, 500)
	tt.Move(curSep-300, 500)
	tt.Frame()
	tt.Release(curSep-300, 500)
	tt.Frame()
	if minClamped := a.filePaneWidth(nil); minClamped != filePaneMinW {
		t.Errorf("文件栏宽度向左拖动应受 clamp 保护在 %.0f，实际 %.0f", filePaneMinW, minClamped)
	}

	// 复位测试
	a.resetFilePaneWidth()
	tt.Frame()
	if got := a.filePaneWidth(nil); got != a.filePaneDefaultW() {
		t.Errorf("复位后文件栏宽度应为默认值 %.0f，实际 %.0f", a.filePaneDefaultW(), got)
	}
}

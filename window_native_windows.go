//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// Windows 的无边框窗口仍带着 WS_CAPTION。亚克力材质要把 DWM 的框架延伸进整个
// 客户区，而框架一延伸，DWM 就会在材质顶上画它自己的最小化 / 最大化 / 关闭
// 三颗按钮，压在我们自绘的那三颗上面 —— 无边框收掉的是 NonClient 的尺寸计算，
// 不是样式里的标题栏（2026-10-30 实机反馈）。
//
// 收掉 WS_CAPTION 就收掉了这三颗。样式里保留 WS_SYSMENU / WS_THICKFRAME 和
// 最小化 / 最大化两个 Box：Alt+Space 的系统菜单、贴边吸附、任务栏的最小化、
// 拖拽和缩放都还在。少了 WS_CAPTION 之后 DWM 不再自己决定圆角，显式要回
// Windows 11 的那一档（DWMWCP_ROUND），窗口外圈才和 shellWindowRadiusFor
// 写的 8 对得上。
const (
	// GWL_STYLE 的索引是 -16，按 32 位无符号传进去才不会在 uintptr 上溢出。
	gwlStyle                    = 0xFFFFFFF0
	wsCaption                   = 0x00C00000
	dwmwaWindowCornerPreference = 33
	dwmwcpRound                 = 2

	// SWP_NOSIZE | SWP_NOMOVE | SWP_NOZORDER | SWP_NOACTIVATE | SWP_FRAMECHANGED：
	// 只让系统重新算一遍框架，位置、大小、层叠和焦点都不动。
	swpFrameChangedOnly = 0x0037
)

var (
	user32                    = syscall.NewLazyDLL("user32.dll")
	dwmapi                    = syscall.NewLazyDLL("dwmapi.dll")
	procGetWindowLongW        = user32.NewProc("GetWindowLongW")
	procSetWindowLongW        = user32.NewProc("SetWindowLongW")
	procSetWindowPos          = user32.NewProc("SetWindowPos")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
)

// applyNativeChrome 收掉 Windows 自己画的三颗标题栏按钮，并把窗口圆角显式
// 要回 8。只在主线程、窗口刚建好时调用一次；样式会跟着窗口活到关闭。
//
// @param hwnd - 顶层窗口的 Win32 句柄，0 时什么都不做
func applyNativeChrome(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	style, _, _ := procGetWindowLongW.Call(hwnd, gwlStyle)
	procSetWindowLongW.Call(hwnd, gwlStyle, style&^wsCaption)
	// 样式改了要让系统重新算一遍框架，否则旧的标题栏会留到下一次尺寸变化。
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpFrameChangedOnly)

	round := int32(dwmwcpRound)
	procDwmSetWindowAttribute.Call(hwnd, dwmwaWindowCornerPreference,
		uintptr(unsafe.Pointer(&round)), unsafe.Sizeof(round))
}

//go:build !windows

package main

// applyNativeChrome 在 Windows 之外什么都不做：macOS 的窗口按钮由 AppKit 画，
// Linux 交给桌面主题，没有「DWM 把按钮画到材质上」这回事。
//
// @param hwnd - 别的平台上不碰它
func applyNativeChrome(hwnd uintptr) {}

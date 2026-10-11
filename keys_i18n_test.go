package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// K/KS 的翻译动作：macOS 上原样，别处把 ⌘ 换成 Ctrl、修饰键重排成
// Windows 的写法。测试不按当前平台断言（CI 可能在 macOS 上跑），而是直接
// 打 translateShortcut —— 那才是真正干活的函数，两个平台都走它。
func TestTranslateShortcut(t *testing.T) {
	cases := []struct{ in, want string }{
		{"⌘R", "Ctrl+R"},
		{"⌘O", "Ctrl+O"},
		{"⌘⏎", "Ctrl+Enter"},
		{"⌘1", "Ctrl+1"},
		{"⌘+", "Ctrl++"}, // Ctrl + 加号键：Windows 上就印作这样
		{"⌘−", "Ctrl+−"},
		{"⌘0", "Ctrl+0"},
		{"⌘,", "Ctrl+,"},
		{"⌘⇧A", "Ctrl+Shift+A"},
		{"⌘⇧B", "Ctrl+Shift+B"},
		{"⌘⇧N", "Ctrl+Shift+N"},
		{"⌘⇧F", "Ctrl+Shift+F"},
		{"⌘⇧P", "Ctrl+Shift+P"},
		{"⌘⇧U", "Ctrl+Shift+U"},
		{"⌘⇧D", "Ctrl+Shift+D"},
		{"⌘⇧[", "Ctrl+Shift+["},
		{"⌘⇧]", "Ctrl+Shift+]"},
		{"⌘J / Ctrl+`", "Ctrl+J / Ctrl+`"},
	}
	for _, c := range cases {
		if got := translateShortcut(c.in); got != c.want {
			t.Errorf("translateShortcut(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// KS 把「/」分段的每一段各自翻译，再用「 / 」拼回去 —— 一段里混着 macOS
// 和 Windows 两种写法的提示（「⌘J / Ctrl+`」）才读得通。
//
// 在 macOS 上它整条短路（原样返回），这一条只在非 macOS 上有断言可做。
func TestKSJoinsSegments(t *testing.T) {
	want := strings.Join([]string{translateShortcut("⌘J"), translateShortcut("Ctrl+`")}, " / ")
	if isMacPlatform() {
		if got := KS("⌘J / Ctrl+`"); got != "⌘J / Ctrl+`" {
			t.Errorf("macOS 上 KS 该原样返回，实际 %q", got)
		}
		return
	}
	if got := KS("⌘J / Ctrl+`"); got != want {
		t.Errorf("KS 拼接得不对：got %q, want %q", got, want)
	}
}

// macOS 上文案必须一字不动：这是用户唯一真正看见 ⌘ 的平台，多一个字符都算
// 改坏了原本排好的版式。
func TestKIdentityOnMac(t *testing.T) {
	if !isMacPlatform() {
		t.Skip("只在 macOS 上跑这一条")
	}
	for _, s := range []string{"⌘R", "⌘⇧A", "⌘J / Ctrl+`", ""} {
		if got := K(s); got != s {
			t.Errorf("K(%q) = %q，macOS 上该原样返回", s, got)
		}
		if got := KS(s); got != s {
			t.Errorf("KS(%q) = %q，macOS 上该原样返回", s, got)
		}
	}
}

// 平台判据必须和框架的 ui.Cmd 一致：ui.Cmd 是 Command 的平台上写 ⌘，是
// Ctrl 的平台上写 Ctrl —— 两边用同一个 GOOS 条件，才不可能对不上。
func TestPlatformMatchesFrameworkCmd(t *testing.T) {
	// ui.Cmd 在 darwin 上等于 Super，别处等于 Ctrl；反推不出 GOOS，但能
	// 验证 isMacPlatform 的判据确实是「Cmd 是不是 Super」。
	macByCmd := ui.Cmd == ui.Super
	if isMacPlatform() != macByCmd {
		t.Fatalf("isMacPlatform() = %v，但 ui.Cmd==ui.Super 是 %v —— 判据不一致",
			isMacPlatform(), macByCmd)
	}
}

// 空串不能翻出东西：界面上 hint 为空的命令（没有快捷键）到处都是，翻出个
// "Ctrl+" 出来就是一处莫名的装饰。
func TestTranslateShortcutEmpty(t *testing.T) {
	if got := translateShortcut(""); got != "" {
		t.Errorf("空串该还是空串，实际 %q", got)
	}
	if got := KS(""); got != "" {
		t.Errorf("空串该还是空串，实际 %q", got)
	}
}

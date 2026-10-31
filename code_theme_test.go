package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

func TestCodeThemePresetLookup(t *testing.T) {
	if p := codeThemePresetByID("vitesse"); p == nil {
		t.Fatal("vitesse 方案找不到了")
	}
	// 拼错、大小写对不上的名字都不是方案 —— 一律 nil，调用方落回默认。
	for _, id := range []string{"", "nope", "Vitesse"} {
		if p := codeThemePresetByID(id); p != nil {
			t.Errorf("codeThemePresetByID(%q) = %+v, want nil", id, p)
		}
	}
	// 出厂默认指着的必须是真实存在的方案，否则整套换装会悄悄失效。
	if codeThemePresetByID(defaultCodeTheme) == nil {
		t.Errorf("defaultCodeTheme %q 不是已注册的方案", defaultCodeTheme)
	}
}

func TestVitessePresetApplies(t *testing.T) {
	p := codeThemePresetByID("vitesse")
	if p == nil {
		t.Fatal("没有 vitesse 方案")
	}

	// 深色外观换的是 Vitesse Dark：底、文字和关键字都得对上。
	th := editor.DefaultTheme(ui.DarkTheme())
	p.apply(th, true)
	if th.Background != ui.Hex("#121212") {
		t.Errorf("深色底 = %v, want #121212", th.Background)
	}
	if th.Text != ui.Hex("#dbd7caee") {
		t.Errorf("深色文字 = %v, want #dbd7caee", th.Text)
	}
	if th.Tokens[editor.TokenKeyword].Color != ui.Hex("#4d9375") {
		t.Errorf("深色关键字 = %v, want #4d9375", th.Tokens[editor.TokenKeyword].Color)
	}

	// 浅色外观换的是 Vitesse Light。
	thL := editor.DefaultTheme(ui.LightTheme())
	p.apply(thL, false)
	if thL.Background != ui.Hex("#ffffff") {
		t.Errorf("浅色底 = %v, want #ffffff", thL.Background)
	}
	if thL.Tokens[editor.TokenString].Color != ui.Hex("#b56959") {
		t.Errorf("浅色字符串 = %v, want #b56959", thL.Tokens[editor.TokenString].Color)
	}

	// 选区是搜索跳转的落点，两套外观都得看得清（至少两成的实色）。
	for _, v := range []*codeThemeVariant{&p.dark, &p.light} {
		if v.selection.A < 50 {
			t.Errorf("选区太淡（alpha %d），跳转落点又找不着了", v.selection.A)
		}
	}
}

// TestVitessePresetCoversAllTokens 防着 mygo 增删 token 种类：每加一种，
// Vitesse 的色表就得有意识地补一支色（或决定留默认），别默默退回 GitHub 色。
func TestVitessePresetCoversAllTokens(t *testing.T) {
	p := codeThemePresetByID("vitesse")
	if p == nil {
		t.Fatal("没有 vitesse 方案")
	}
	for kind := 0; kind <= int(editor.TokenInvalid); kind++ {
		if p.dark.tokens[kind].A == 0 {
			t.Errorf("Vitesse 深色缺 editor.TokenKind(%d) 的颜色", kind)
		}
		if p.light.tokens[kind].A == 0 {
			t.Errorf("Vitesse 浅色缺 editor.TokenKind(%d) 的颜色", kind)
		}
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Nerd Font 的记号就那两种写法，外加不能把普通名字误判进来。
func TestIsNerdFontName(t *testing.T) {
	cases := []struct {
		name string
		nerd bool
	}{
		{"Maple Mono NF", true},            // 本机装的就是这种：家族名以 " NF" 结尾
		{"MapleMono-NF-Regular.ttf", true}, // 补丁记号只落在文件名上
		{"JetBrainsMono Nerd Font", true},  // v3 的家族名
		{"JetBrainsMonoNerdFont-Regular.ttf", true},
		{"MesloLGS NF", true},
		{"CaskaydiaCove Nerd Font Mono", true}, // 带变体后缀
		{"Fira Code", false},
		{"Menlo", false},
		{"JetBrains Mono", false}, // 普通版，不是补丁版
		{"NFSans", false},         // NF 开头不算：记号在后缀上
		{"Noto Sans", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isNerdFontName(c.name); got != c.nerd {
			t.Errorf("isNerdFontName(%q) = %v, want %v", c.name, got, c.nerd)
		}
	}
}

// 扫字体目录：只收等宽的、按族去重、认得出补丁版。
//
// 用系统里现成的两个字体文件当样本（一个等宽、一个比例），没有就跳过 ——
// 这份扫描在任何机器上都跑同一段代码，夹具只是喂给它的输入。
func TestScanFontsFiltersDedupesAndFlagsNerd(t *testing.T) {
	mono := "/System/Library/Fonts/Supplemental/Courier New.ttf"
	prop := "/System/Library/Fonts/Supplemental/Times New Roman.ttf"
	for _, p := range []string{mono, prop} {
		if _, err := os.Stat(p); err != nil {
			t.Skipf("这台机器没有样本字体 %s", p)
		}
	}

	dir := t.TempDir()
	copyFile(t, mono, filepath.Join(dir, "Courier New.ttf"))
	// 同一个族再来一份（另一个字重/另一个文件名）：下拉里只能出现一次。
	copyFile(t, mono, filepath.Join(dir, "Courier New Bold.ttf"))
	// 补丁版：家族名不变，记号在文件名上。
	copyFile(t, mono, filepath.Join(dir, "TestFont-NF-Regular.ttf"))
	// 比例字体：终端用不了，必须被挡在外面。
	copyFile(t, prop, filepath.Join(dir, "Times New Roman.ttf"))
	// 读不动的文件：跳过，不算错。
	if err := os.WriteFile(filepath.Join(dir, "broken.ttf"), []byte("not a font"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := scanFonts([]string{dir, filepath.Join(dir, "does-not-exist")})

	byName := map[string]installedFont{}
	for _, f := range got {
		byName[f.Family] = f
	}
	if _, ok := byName["Times New Roman"]; ok || len(got) != 1 {
		t.Fatalf("只该剩等宽的那一个族，实际 %+v", got)
	}
	courier, ok := byName["Courier New"]
	if !ok {
		t.Fatalf("没扫到 Courier New：%+v", got)
	}
	if !courier.Nerd {
		t.Errorf("文件名里带 -NF- 的该算补丁版：%+v", courier)
	}
}

// 选项顺序就是「Nerd Font 优先」这件事本身。
func TestTermFontOptionsOrderAndDedupe(t *testing.T) {
	a := newApp()
	// scanFonts 交出来的顺序：Nerd 在前、各自按名字 —— 这里照那个契约喂。
	a.fonts = []installedFont{
		{Family: "FiraCode Nerd Font", Nerd: true},
		{Family: "Maple Mono NF", Nerd: true},
		{Family: "Menlo"},
		{Family: "Zed Mono"},
	}
	opts := a.termFontOptions()

	if len(opts) == 0 || opts[0] != termFontGenericMono {
		t.Fatalf("第一个该是系统等宽：%q", opts)
	}
	wantHead := []string{termFontGenericMono, "FiraCode Nerd Font", "Maple Mono NF", "Menlo", "Zed Mono"}
	for i, want := range wantHead {
		if opts[i] != want {
			t.Fatalf("第 %d 个该是 %q，实际 %q（%q）", i, want, opts[i], opts)
		}
	}

	seen := map[string]int{}
	for _, o := range opts {
		seen[o]++
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("%q 出现了 %d 次", name, n)
		}
	}
	// 已装的（含 Nerd）在前，固定表在后 —— 但固定表里的候选还在，兜底能力没丢。
	for _, want := range []string{"SF Mono", "Consolas", "MesloLGS NF"} {
		if seen[want] == 0 {
			t.Errorf("固定表里的 %q 不该丢：%q", want, opts)
		}
	}
	if seen["Menlo"] != 1 {
		t.Errorf("Menlo 既在已装里又在固定表里，只该出现一次：%d 次", seen["Menlo"])
	}

	// 一次都没扫过时也要能画：只有系统等宽 + 固定表。
	empty := newApp().termFontOptions()
	if len(empty) != len(curatedTermFonts)+1 || empty[0] != termFontGenericMono {
		t.Errorf("没扫到字体时该只剩固定表：%d 个", len(empty))
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

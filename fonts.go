package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
)

// installedFont 是一支扫出来的、装在这台机器上的等宽字体。
//
// 只收等宽的：终端拿比例字体排出来的格子会和字形对不上（下拉里那份
// monospace 兜底的注释说的就是这个坑）。Nerd 只影响排序，不影响取用。
type installedFont struct {
	Family string
	Nerd   bool
}

// userFontDirs 是各平台上「用户自己装的」字体目录。
//
// 有意**不扫**系统字体目录：SF Mono / Menlo / Monaco / Consolas 这些由下拉里
// 那份固定的常见字体表兜着，而系统目录在 macOS 上是上千个文件、每个都得整份
// 解析（实测 ~11ms/个），为一个下拉不值当。用户装的通常几十个文件 —— 而且
// Nerd Font 全在这一档，正是「我装了 Maple Mono NF 却在下拉里看不到」那一批。
func userFontDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		return []string{filepath.Join(home, "Library", "Fonts"), "/Library/Fonts"}
	case "windows":
		return []string{
			filepath.Join(home, "AppData", "Local", "Microsoft", "Windows", "Fonts"),
			filepath.Join(home, "AppData", "Roaming", "Microsoft", "Windows", "Fonts"),
		}
	default: // linux 及其他 unix
		return []string{
			filepath.Join(home, ".local", "share", "fonts"),
			filepath.Join(home, ".fonts"),
		}
	}
}

// fontScanMaxFiles 是一次扫描最多看几个字体文件。
//
// 正常用户字体目录就是几十个；这个上限只防「有人把整个字体库 rsync 进了
// ~/Library/Fonts」把后台线程占住 —— 一个下拉的长相不值得。
const fontScanMaxFiles = 500

// scanFonts 扫一遍 dirs（递归），返回装着的等宽字体族：去重、Nerd 在前、按名字排。
//
// 读不动的东西一律跳过：目录不存在、权限不够、文件坏掉都只是「这支字体没扫到」，
// 不该让这颗下拉报错或者卡住。
func scanFonts(dirs []string) []installedFont {
	nerd := map[string]bool{} // 族名 → 是不是 Nerd 补丁版（同一个族好几个字重，取或）
	files := 0
	for _, dir := range dirs {
		if files >= fontScanMaxFiles {
			break
		}
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !isFontFile(path) {
				return nil
			}
			if files++; files > fontScanMaxFiles {
				return fs.SkipAll
			}
			families := monospaceFamiliesIn(path)
			if len(families) == 0 {
				return nil
			}
			// 家族名和文件名一起看：补丁版的记号有时只落在文件名上
			// （MapleMono-NF-Regular.ttf 的家族名是 "Maple Mono NF"，两边都认，
			// 但别的字体可能只留一边）。
			patch := isNerdFontName(filepath.Base(path))
			for _, fam := range families {
				nerd[fam] = nerd[fam] || patch || isNerdFontName(fam)
			}
			return nil
		})
	}

	out := make([]installedFont, 0, len(nerd))
	for fam, isNerd := range nerd {
		out = append(out, installedFont{Family: fam, Nerd: isNerd})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Nerd != out[j].Nerd {
			return out[i].Nerd
		}
		return out[i].Family < out[j].Family
	})
	return out
}

// isFontFile 是扫描愿意打开的扩展名。dfont / suit 这些老格式不认：go-text 读不了。
func isFontFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".ttf", ".ttc", ".otf", ".otc":
		return true
	}
	return false
}

// monospaceFamiliesIn 读一个字体文件，交出里面等宽的那些 face 的族名
// （字体集合如 .ttc 里可能有好几支）。
//
// 整份解析，不走「名字里带不带 mono」那种猜法：Menlo、Consolas、Maple Mono NF
// 里的前两个都不含 "mono"，猜法会把它们划掉；反过来，一个比例字体混进终端字体
// 下拉，选下去就是错行。实测用户字体目录一个文件 ~11ms，一次后台扫描一两百毫秒。
func monospaceFamiliesIn(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	loaders, err := opentype.NewLoaders(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	var out []string
	for _, ld := range loaders {
		f, err := font.NewFont(ld)
		if err != nil || !f.IsMonospace() {
			continue
		}
		if fam := strings.TrimSpace(f.Describe().Family); fam != "" {
			out = append(out, fam)
		}
	}
	return out
}

// isNerdFontName 判一个名字（族名或文件名）是不是 Nerd Font 补丁版。
//
// Nerd Fonts 的记号就两种写法：`<字体> Nerd Font`（v3，含 Mono / Propo 变体）
// 和早期的 ` NF` 后缀（MesloLGS NF、Maple Mono NF、MapleMono-NF-Regular.ttf）。
// 后者只在词边界上算数 —— 「NF 开头的普通文件名」不该被当补丁版。
func isNerdFontName(name string) bool {
	l := strings.ToLower(name)
	if strings.Contains(l, "nerd font") || strings.Contains(l, "nerdfont") {
		return true
	}
	for _, sep := range []string{" nf", "-nf", "_nf"} {
		i := strings.Index(l, sep)
		if i < 0 {
			continue
		}
		if rest := l[i+len(sep):]; rest == "" || rest[0] == '-' || rest[0] == '_' || rest[0] == ' ' {
			return true
		}
	}
	return false
}

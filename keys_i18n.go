package main

import (
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// 这个文件管「快捷键文案按平台显示什么」。
//
// 键位本身不用管：框架的 ui.Cmd 在 macOS 上是 Command、在别处自动是
// Control（ui/keys.go），所以 keys.go 里接键的逻辑和按的是什么键天然对齐。
// 麻烦只在**文案** —— 界面里写死的「⌘R」「⌘⇧A」按 macOS 的习惯写，到了
// Windows / Linux 上按钮照样显示 ⌘，而用户按下去的其实是 Ctrl。文案和按键
// 对不上，比不显示还糟：用户会以为自己按错了。
//
// 做法：所有用户可见的快捷键文案都经过 K()（单个键）或 KS()（一串键）出来,
// 它们按当前平台把 ⌘ 换成 Ctrl、把平台没有的修饰键排列抹平。文案里只写
// macOS 那一种形式，交给这里翻译 —— 和 T() 处理语言是同一套路子。
//
// 翻译策略（宁可粗不要漏）：
//   - ⌘ → Ctrl（⌃ 在中文 Windows 键盘上不印，用户认的是 Ctrl）
//   - ⌥ → Alt，⇧ → Shift，⌃ → Ctrl
//   - ⏎ → Enter，⌫ → Backspace，⌦ → Delete、⎋ → Esc（这些符号在
//     Windows / Linux 键盘上找不到）
//   - ；「⌘⇧[」这种修饰键顺序统一排成 Ctrl+Shift+[（macOS 的语法是
//     修饰键在前不带分隔符，Windows 的是相间带 +）
//
// 没有动的东西：原生菜单（menu.go）。那一份的键等价是 "CmdOrCtrl+R" 这种
// 交给框架的字符串，操作系统自己会画成「⌘R」或「Ctrl+R」，不必也不该在这里
// 再翻一遍。

// 平台差异的键名映射。只在非 macOS 上有意义 —— macOS 上文案原样保留。
var platformKeyNames = map[string]string{
	"⌘": "Ctrl",
	"⌥": "Alt",
	"⇧": "Shift",
	"⌃": "Ctrl",
	"⏎": "Enter",
	"⌫": "Backspace",
	"⌦": "Delete",
	"⎋": "Esc",
	"↑": "Up",
	"↓": "Down",
	"←": "Left",
	"→": "Right",
}

// 修饰键在非 macOS 上的排列顺序：按 Windows  accelerators 的习惯，
// Ctrl → Alt → Shift，且彼此之间用 + 隔开。
var platformModOrder = []string{"Ctrl", "Alt", "Shift"}

var (
	platformOnce sync.Once
	platformMac  bool
)

// isMacPlatform 报告当前是不是 macOS。和框架判 ui.Cmd 用同一个条件
// （runtime.GOOS == "darwin"），所以 UI.Cmd 是 Command 的平台上文案写
// Command，别处写 Ctrl —— 两边永远对得上。
func isMacPlatform() bool {
	platformOnce.Do(func() {
		// ui.Cmd 在 macOS 上是 Super，别处是 Ctrl。拿它和自己的值比一下，
		// 就不用在这个包里再引一次 runtime。
		platformMac = ui.Cmd == ui.Super
	})
	return platformMac
}

// K 翻译单个快捷键文案，如 "⌘R" → "Ctrl+R"。
//
// macOS 上原样返回（连分配都不做），所以绝大多数调用在测试和本机上走的是
// 快路。
func K(mac string) string {
	if isMacPlatform() || mac == "" {
		return mac
	}
	return translateShortcut(mac)
}

// KS 翻译一串快捷键文案，分隔符统一用「 / 」。
//
// 多键共用一处提示时用（「⌘1 / Ctrl+`」），避免调用方自己拼：Windows 上那
// 一段变成「Ctrl+1 / Ctrl+`」，两个键都是一个平台上的写法，读起来才像
// 同一套快捷键。键名之间若本来就没有 '/'，行为同 K。
func KS(mac string) string {
	if isMacPlatform() || mac == "" {
		return mac
	}
	parts := strings.Split(mac, "/")
	for i, p := range parts {
		parts[i] = translateShortcut(strings.TrimSpace(p))
	}
	return strings.Join(parts, " / ")
}

// translateShortcut 把一段 macOS 写法的快捷键翻成 Windows / Linux 写法。
//
// 修饰键放在前面（按 platformModOrder 的顺序），其余按键名按出现顺序跟在
// 后面，修饰键之间和修饰键与键名之间都用 + 相连。认不出的符号原样留下 ——
// 宁可留个 ⌘ 让用户愣一下，也不要把一个键名吃掉。
func translateShortcut(mac string) string {
	var mods []string
	var keys []string
	for _, r := range mac {
		s := string(r)
		if s == "+" {
			// 「⌘+ / ⌘−」里的 + 是「加号键」本身，不是分隔符 —— 拼回去时
			// 不能再补一个前缀分隔符，否则 ⌘+ 会变成 "Ctrl++"。
			keys = append(keys, s)
			continue
		}
		if name, ok := platformKeyNames[s]; ok {
			switch name {
			case "Ctrl", "Alt", "Shift":
				// 修饰键收进 mods。重复没关系（「⌘⌘」之类本来就写不出来），
				// 下面按 platformModOrder 重排时自然去重。
				mods = append(mods, name)
			default:
				keys = append(keys, name)
			}
			continue
		}
		// 普通字符（字母、数字、标点）原样收下。
		keys = append(keys, s)
	}

	var b strings.Builder
	for _, m := range platformModOrder {
		if contains(mods, m) {
			b.WriteString(m)
			b.WriteByte('+')
		}
	}
	// 按键名之间不加分隔符（「Ctrl+⇧[」里 [ 紧跟 Shift），但 + 本身要 —
	// 它是「加号键」，前面必须有分隔符才读得成 "Ctrl++"？不：macOS 的
	// 「⌘+」在 Windows 上就印作 "Ctrl++"（Ctrl 加加号键），这个双加号是
	// 惯例，不是笔误。所以按键名一律原样相接，+ 也原样跟在 Ctrl 后面。
	b.WriteString(strings.Join(keys, ""))
	return b.String()
}

// contains 报 s 里有没有 v。就三五个元素，线性扫比建 map 便宜。
func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// 按钮的一套皮（issue #7 问题三：「字体大小的修改和部分按钮是拟态UI，但其他
// 组件UI 不是拟态的，感觉 ui 不够统一」）。
//
// 全界面共用的控件走的是同一套语言：洗色底（chip）+ 1px 发丝边（hairline）+
// inkMuted 文字，悬停升 rowSelected（widgets.go 的 secondaryAction /
// primaryAction，fadeColors 收尾）。而设置页那两颗按钮自己另起了一套：
// darkSurfaceShadow(litSurfaceEdge(…)) —— 深色下三层外投影加一道内高光，
// 浅色下一层内阴影加一层外投影。
//
// 这不是「两颗按钮稍有不同的品味」：同一个设置页里， stepper 的 −/＋ 是拟态、
// 旁边的分段控件和开关不是；往下一格的「保存」是拟态，顶栏的「关闭设置」不是。
// 用户读到的是同一屏上两三套互相不认识的语言，而 DESIGN.md §4.1 写的是
// 「控件不投投影……暗色模式下不使用粗重投影」。
//
// 所以这里按函数静态扫：拟态那一套只准出现在**表面**的抬升上（raisedSurface /
// floatingSurface / popoverSurface —— 卡片、浮层、菜单确实需要从背景上浮起来），
// 不准出现在**控件**上。控件的高度差距靠洗色阶梯表达，不靠投影。

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// neumorphicHelpers 是拟态那一套的两个函数。键是函数名，值是换成什么。
//
// 只列这两个：litSurfaceEdge 是「一道内高光 + 一根 12% 白边」，
// darkSurfaceShadow 是「三层外投影」—— 两者叠起来才是用户说的「拟态」。
var neumorphicHelpers = map[string]string{
	"litSurfaceEdge":    "控件不描内高光：静息 chip、悬停 rowSelected、按下 rowSelected（§2.1 洗色阶梯）",
	"darkSurfaceShadow": "控件不投三层外阴影：层级靠明度阶梯和 1px 发丝边表达（§4.1）",
}

// surfaceLifters 是**允许**用拟态那一套的地方：卡片、浮层、菜单这些表面。
//
// 它们确实要从背景上浮起来，明度阶梯不够 —— 卡片压在亚克力上，没有一道硬边
// 和一点投影的话，边就是一团化开的灰。控件不在此列：控件坐在一个已知的面上，
// 它要的是「这一颗可点」，不是「这一颗浮起来」。
var surfaceLifters = map[string]bool{
	"raisedSurface":   true, // 工作卡
	"floatingSurface": true, // 下拉 / 右键浮层、对话框
	"popoverSurface":  true, // 菜单那层渐变 + 暖白
	"selectedSurface": true, // 侧栏选中行
}

// TestNoNeumorphicControls 断言拟态那一套只用在表面上，不用在控件上。
//
// 按 AST 扫而不是按子串扫：注释里常常正是在解释「这里为什么不用拟态」，
// 而 shell_style.go 自己就定义着这两个函数 —— 照子串扫会把定义处和解释处
// 一起拎出来。
func TestNoNeumorphicControls(t *testing.T) {
	for _, path := range designGuardFiles(t) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", path, err)
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if surfaceLifters[fn.Name.Name] {
				continue
			}
			// 函数自己就是定义处的话跳过（定义在 shell_style.go，不在上面的名单里，
			// 但它的函数名等于 helper 名本身）。
			if _, isHelper := neumorphicHelpers[fn.Name.Name]; isHelper {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				id, ok := call.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				fix, bad := neumorphicHelpers[id.Name]
				if !bad {
					return true
				}
				t.Errorf("%s: %s 里用了 %s —— 控件上的拟态和共用的洗色控件不是一套语言。"+
					" surfaceLifters 里的表面抬升才准用；%s",
					fset.Position(call.Pos()), fn.Name.Name, id.Name, fix)
				return true
			})
		}
	}
}

// TestSettingsButtonsUseTheSharedWashLanguage 断言设置页那两颗按钮和界面上
// 共用的控件走同一套语言。
//
// 上面那条守「不许用拟态」，这一条守「必须用共用的那一套」：只禁前一半的话，
// 把拟态删掉就完事，剩下的是一颗既没有洗色也没有发丝边的裸按钮 —— 比拟态更
// 不统一。所以两头一起盯。
func TestSettingsButtonsUseTheSharedWashLanguage(t *testing.T) {
	for _, path := range designGuardFiles(t) {
		src := readFile(t, path)
		for _, fn := range []string{"settingsButton", "settingsMiniButton"} {
			// 函数体从定义处取到下一个顶层声明之前，够用也不必引 AST。
			body := funcBody(src, fn)
			if body == "" {
				continue
			}
			if !strings.Contains(body, "pal.chip") && !strings.Contains(body, "pal.control") {
				t.Errorf("%s: %s 的静息底既不是 pal.chip 也不是 pal.control —— "+
					"其余控件用洗色阶梯（secondaryAction / primaryAction）", path, fn)
			}
			if !strings.Contains(body, "hairline") {
				t.Errorf("%s: %s 没有 1px 发丝边 —— 边界靠 hairline 划，不靠投影（§4.1）", path, fn)
			}
		}
	}
}

// readFile 读一个源文件，读不到就让测试直接失败。
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s 失败: %v", path, err)
	}
	return string(b)
}

// funcBody 取出 src 里 `func <name>(` 到下一个顶层 `func ` 之间的文本。
func funcBody(src, name string) string {
	i := strings.Index(src, "func "+name+"(")
	if i < 0 {
		return ""
	}
	rest := src[i:]
	if j := strings.Index(rest, "\nfunc "); j >= 0 {
		return rest[:j]
	}
	return rest
}

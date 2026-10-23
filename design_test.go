package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 这两条断言守的是 DESIGN.md §3 里两个「一改就散」的量：圆角与描边宽度。
//
// 之所以写成测试而不是只留在评审里：这两样东西的漂移方式很隐蔽 —— 新增一个
// 对话框时顺手写 `Radius(10)`，渲染出来和 `Radius(8)` 差两个像素，没人会在
// 一张截图里发现，但半年后「把卡片圆角整体调一档」就又得挨个调用点去认。
// 圆角从 1 到 99 攒出十一个值、描边 1.2/1.5 混用，就是这么来的。
//
// 字号**不在这里守**：规范只约束「成角色」的那些字号，密集列表的正文与元信息
// 本来就允许有自己的档（见 design.go 的注释），静态扫不出角色，只能靠评审。

// radiusLiteralExceptions 是为数不多「几何值不是设计 token」的地方。键是
// 「文件:字面量」，值必须写清为什么它是几何量 —— 加进来要付解释成本，这本身
// 就是一道闸。
var radiusLiteralExceptions = map[string]string{
	"markdown.go:1": "引用块左侧那条 2px 竖线的端头，圆角取它宽度的一半才是圆头；这是几何，不是设计 token",
}

// TestNoRawRadiusLiterals 断言非测试代码里不再出现圆角数字字面量。
func TestNoRawRadiusLiterals(t *testing.T) {
	for _, path := range designGuardFiles(t) {
		for _, lit := range designCallArgs(t, path, "Radius") {
			if _, ok := radiusLiteralExceptions[filepath.Base(path)+":"+lit]; ok {
				continue
			}
			t.Errorf("%s: Radius(%s) 是裸字面量 —— 请用 design.go 的 radiusXS/SM/MD/LG/XL/Pill；"+
				"确实属于几何量（而非设计 token）的话，加进 radiusLiteralExceptions 并写明理由",
				path, lit)
		}
	}
}

// TestNoDerivedRadii 断言没人从 t.Radius 上加减出一个新的圆角。
//
// `t.Radius - 2`、`t.Radius + 4` 这类派生值最坏的地方是**跟随**：今天 t.Radius
// 是 8，派生出来的是 6 和 12，看着都对；哪天把 t.Radius 调成 10，这两个数就跟着
// 变成 8 和 14，无声无息地滑出规范的六档，而改 t.Radius 的人只会以为自己只动了
// 框架自绘控件。
func TestNoDerivedRadii(t *testing.T) {
	for _, path := range designGuardFiles(t) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			if !isThemeRadius(bin.X) && !isThemeRadius(bin.Y) {
				return true
			}
			t.Errorf("%s: %s 是从 t.Radius 派生出来的圆角 —— 派生值会跟着 t.Radius 漂，"+
				"要用规则里的固定档就写 radiusXS/SM/MD/LG/XL",
				fset.Position(bin.Pos()), exprText(fset, bin))
			return true
		})
	}
}

// TestIntegerBorderWidths 断言描边宽度都是整数。
//
// 规范里只有 1px 发丝线和 2px 焦点环两种描边；1.2 与 1.5 这种值在半像素对齐的
// 渲染下会一格糊一格清（同一排按钮有的边看起来重些），而且它多半是「觉得 1px
// 太细、2px 太粗」的和稀泥，不是设计决定。
func TestIntegerBorderWidths(t *testing.T) {
	for _, path := range designGuardFiles(t) {
		for _, meth := range []string{"Border", "BorderWidth"} {
			for _, lit := range designCallArgs(t, path, meth) {
				if !strings.Contains(lit, ".") {
					continue
				}
				t.Errorf("%s: %s(%s, …) 不是整数描边 —— 规范只有 1px 发丝线与 2px 焦点环两档",
					path, meth, lit)
			}
		}
	}
}

// frameworkLookAlikes 是框架自绘控件里那几个「自带一套皮」的，非测试代码不该再用。
//
//   - ui.Button / ui.PrimaryButton：字号取 t.FontSize(13)、字重 400、高度跟着字形走，
//     而 §4.1 要求按钮是 12.5 / Medium / 固定高。40 个对话框底部全换成
//     primaryAction / secondaryAction 之后，这里留一道闸。
//   - ui.Modal：面板圆角写死成 t.Space(2.5)＝10，而 §3 给模态的是 xl(16)。
//     用 specModal，它只是把 DialogBase 的面板按规范摆一遍。
//
// 这两个都是**静默**的：用了框架版，界面照样渲染，只是悄悄偏离规范。
var frameworkLookAlikes = map[string]string{
	"ui.Button":        "用 secondaryAction（§4.1 次级按钮：Surface 1 + 发丝线 + Ink Muted）",
	"ui.PrimaryButton": "用 primaryAction（§4.1 主按钮：强调色实心 / 12.5px / Medium）",
	"ui.Modal":         "用 specModal（§3 模态圆角 xl(16)、§2.1 浮层 Surface 4）",
}

// TestNoFrameworkSkinnedControls 断言界面上不再出现框架自带一套皮肤的控件。
func TestNoFrameworkSkinnedControls(t *testing.T) {
	for _, path := range designGuardFiles(t) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", path, err)
		}
		for name, fix := range frameworkLookAlikes {
			// 只认调用，不认注释里提到它们的那些行（注释里常常正是在解释为什么不用）。
			for i, line := range strings.Split(string(src), "\n") {
				code := line
				if j := strings.Index(code, "//"); j >= 0 {
					code = code[:j]
				}
				if strings.Contains(code, name+"(") {
					t.Errorf("%s:%d: 还在用 %s —— %s", path, i+1, name, fix)
				}
			}
		}
	}
}

// designGuardFiles 列出需要守的源文件：本目录下的非测试 .go。
//
// 只看当前目录：internal/ 下是 diff、语法高亮这些纯逻辑包，不画界面。
func designGuardFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("读取源码目录失败: %v", err)
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, name)
	}
	if len(files) == 0 {
		t.Fatal("一个源文件都没扫到 —— 测试的工作目录不对？")
	}
	return files
}

// designCallArgs 取出 path 里所有 `.方法名(字面量, …)` 的那个字面量。
//
// 只认字面量实参：`Radius(radiusSM)` 与 `Radius(chipDot / 2)` 都是表达式，交给
// 类型系统去管。`Border` / `BorderWidth` 的第 2 个实参是颜色，所以只看第 1 个。
func designCallArgs(t *testing.T, path, method string) []string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", path, err)
	}
	var out []string
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != method {
			return true
		}
		lit, ok := call.Args[0].(*ast.BasicLit)
		if !ok {
			return true
		}
		if lit.Kind != token.INT && lit.Kind != token.FLOAT {
			return true
		}
		out = append(out, lit.Value)
		return true
	})
	return out
}

// isThemeRadius 判断表达式是不是 `t.Radius` 这类取框架圆角的写法。
func isThemeRadius(e ast.Expr) bool {
	sel, ok := e.(*ast.SelectorExpr)
	return ok && sel.Sel.Name == "Radius"
}

// exprText 把表达式还原成源码，用于报错信息。
func exprText(fset *token.FileSet, e ast.Expr) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, fset, e); err != nil {
		return "<表达式>"
	}
	return b.String()
}

// TestShellCardRadiusIsConcentric 守 §3 的「内外同心」：工作卡的圆角取
// 「窗口圆角 − 间距」，两圈弧才平行、8px 的灰带在角上和边上才一样宽。
//
// 这条关系最容易在换机器 / 升系统之后悄悄失效：macOS 26（Tahoe）把窗口角从
// 10 拉到了 28，卡片要是还写死 14，角上的灰带就被挤到 5px —— 差三个像素，
// 整张截图上看不出来，但那个角读起来像两张不相干的卡片叠在一起。
func TestShellCardRadiusIsConcentric(t *testing.T) {
	// 窗口角够大的平台（macOS 26 = 28），严格同心算出来仍然是张卡片。
	for _, goos := range []string{"darwin"} {
		win, card := shellWindowRadiusFor(goos), shellCardRadiusFor(goos)
		if want := win - shellCardGap; card != want {
			t.Errorf("%s: 工作卡圆角 %.0f ≠ 窗口圆角 %.0f − 间距 %d —— 两圈弧不平行",
				goos, card, win, shellCardGap)
		}
	}

	// 窗口角比卡片那一档还小的平台（Windows 11 = 8，间距也是 8），严格同心
	// 会算出 0 —— 那是把圆角抹平了，不是对齐。这时守住下限：卡片至少还是
	// radiusLG，宁可角上的灰带宽几像素。
	for _, goos := range []string{"darwin", "windows", "linux"} {
		if card := shellCardRadiusFor(goos); card < radiusLG {
			t.Errorf("%s: 工作卡圆角 %.0f 掉到 rounded.lg(%v) 之下 —— 卡片不能没有圆角",
				goos, card, radiusLG)
		}
	}
}

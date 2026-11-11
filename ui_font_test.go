// 界面字号（Win11 上正文偏小的那件事，issue #7 问题二）。
//
// 系统那条 TextScale 路在这里走不通：框架只把它乘到主题自己的 t.FontSize 上
// （ui/theme.go 的 follow），而 Gloss 界面上三百多处文字写的是 design.go 的
// token，谁都不看 t.FontSize —— 用户在 Windows 设置里把文本放大到 125%，
// Gloss 一个像素都不动。所以界面字号是 Gloss 自己的一个设置，跟着用户走，
// 而不是跟着系统走。
//
// 四件事要同时成立，缺一件这个设置就是假的：
//
//   - 调大一档，界面上的字真的变大（不是只改了个数）；
//   - 同一档位反复取用，字号不累加 —— 主题对象跨帧复用，累乘会让字号指数级
//     涨上去；
//   - 默认档就是规范里的定值，不是 1.0 倍之后再四舍五入出来的近似值；
//   - 每一档基准都过同一个缩放口子（静态那一半）。

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/store"
)

// uiFontBases 是所有会跟着界面字号走的基准档位。
//
// 收齐七档而不只是规范那六档：rowChipText（芯片徽章 10px）也是界面上的文字，
// 它不跟着放大的话，正文变大了、徽章还是原样，界面就一半大一半小 —— 那比
// 不缩放更难读，正是 issue #7 问题三在问的事。
var uiFontBases = []struct {
	name string
	tok  float32
	spec float32
}{
	{"textDisplay", textDisplay, 20},
	{"textHeadline", textHeadline, 16},
	{"textBody", textBody, 13},
	{"textBodySM", textBodySM, 12},
	{"textButton", textButton, 12.5},
	{"textCaption", textCaption, 11},
	{"rowChipText", rowChipText, 10},
}

// resetUIFontStep 把界面字号档位拨回默认，离开时再拨回去。
//
// 两头都要拨：档位是包级状态（自由函数里的 FontSize 也要读它，见
// TestUIFontScaleReachesEveryToken 里那 46 处），只注册 Cleanup 的话，进入这个
// 测试时读到的还是上一个测试临走时留下的档位 —— 同一条断言在两个顺序下给出
// 两个答案。进去时先归位，出去时再归位，测试才和顺序无关。
func resetUIFontStep(t *testing.T) {
	t.Helper()
	setUIFontSizeStep(uiFontSizeDefault)
	t.Cleanup(func() { setUIFontSizeStep(uiFontSizeDefault) })
}

// TestUIFontScaleResetsToSpec 断言默认档就是规范里的定值。
//
// 不是「约等于」：13.0 这一档在默认档下必须正好是 13.0。缩放写在 1.0 倍上，
// 却因为先乘再 round 而落成 12.9，用户看到的是「什么都没调，正文就小了一号」。
func TestUIFontScaleResetsToSpec(t *testing.T) {
	resetUIFontStep(t)
	for _, b := range uiFontBases {
		if b.tok != b.spec {
			t.Errorf("规范里 %s 应该是 %v，实际 %v", b.name, b.spec, b.tok)
		}
		if got := uiFontSize(b.tok); got != b.spec {
			t.Errorf("默认档下 %s = %v, want %v", b.name, got, b.spec)
		}
	}
}

// TestUIFontScaleGrowsText 断言调大一档，每一档基准都真的变大。
//
// 逐个档位盯而不只盯正文：任何一档没变大，界面就是一半大一半小。
func TestUIFontScaleGrowsText(t *testing.T) {
	resetUIFontStep(t)
	before := make([]float32, len(uiFontBases))
	for i, b := range uiFontBases {
		before[i] = uiFontSize(b.tok)
	}

	setUIFontSizeStep(uiFontSizeDefault + 1)

	for i, b := range uiFontBases {
		after := uiFontSize(b.tok)
		if after <= before[i] {
			t.Errorf("调大一档后 %s 从 %.2f 变成 %.2f，没变大", b.name, before[i], after)
		}
	}
}

// TestUIFontScaleShrinksText 断言调小一档，每一档都真的变小。
//
// 和上面那条是一对：只有放大的那条，缩小的方向可以是恒等函数，用户拧小旋钮
// 什么都不会发生。
func TestUIFontScaleShrinksText(t *testing.T) {
	resetUIFontStep(t)
	// 基准先在默认档下取一遍，再把档位拨小 —— 顺序反了的话，取到的是拨小之后的
	// 数，「变小」这一条就永远证不出来。
	big := make([]float32, len(uiFontBases))
	for i, b := range uiFontBases {
		big[i] = uiFontSize(b.tok)
	}
	setUIFontSizeStep(uiFontSizeDefault - 1)
	for i, b := range uiFontBases {
		small := uiFontSize(b.tok)
		if small >= big[i] {
			t.Errorf("调小一档后 %s 从 %.2f 变成 %.2f，没变小", b.name, big[i], small)
		}
	}
}

// TestUIFontScaleIsNotCompounded 断言字号是基准和档位的纯函数。
//
// 这条守的是累乘：界面上的字号是每帧现算的，而 design.go 的 token 是常量、
// 不会自己变大 —— 真正的风险是某个中间对象把缩放结果存回去再缩放一次
// （比如把 uiFontSize 的结果写进一个跨帧复用的 theme/style，下一帧又拿它当
// 基准）。同一个基准取五次必须还是同一个数，多取一次就涨一截的话，开着界面
// 几分钟字就涨出屏幕了。
func TestUIFontScaleIsNotCompounded(t *testing.T) {
	resetUIFontStep(t)
	setUIFontSizeStep(uiFontSizeDefault + 1)

	first := uiFontSize(textBody)
	if first == textBody {
		t.Fatalf("调大一档后正文还是 %.1f，缩放没生效", first)
	}
	// 取五次（模拟五帧）。任何一次变了，就是有人把结果存回去当了基准。
	for i := 0; i < 5; i++ {
		if got := uiFontSize(textBody); got != first {
			t.Fatalf("第 %d 次取正文档 = %.4f，第一次是 %.4f —— 字号被累乘了", i+1, got, first)
		}
	}
	// 拨回默认档必须正好回到规范值，而不是留在某个被放大过的数上。
	setUIFontSizeStep(uiFontSizeDefault)
	if got := uiFontSize(textBody); got != textBody {
		t.Errorf("拨回默认档后正文 = %v, want %v", got, textBody)
	}
}

// TestUIFontScaleReachesEveryToken 断言所有基准档都过同一个缩放口子。
//
// 这是「界面整体变大、不是一半变大」的静态那一半：三百多处 FontSize 里只要
// 有一处绕开了 uiFontSize，那一处就不跟着设置走 —— 而缩放只影响一部分文字，
// 比不缩放更难读。TestNoRawFontSizeLiterals 守字面量，这条守 token 也走同一条路。
//
// 按 AST 查而不是按子串查：`FontSize(uiFontSize(textBody))` 里含着
// `FontSize(textBody)` 这段子串（内层那次调用），用 strings.Contains 会把
// 正确的代码判成错的。
func TestUIFontScaleReachesEveryToken(t *testing.T) {
	want := map[string]bool{}
	for _, b := range uiFontBases {
		want[b.name] = true
	}

	for _, path := range designGuardFiles(t) {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", path, err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "FontSize" {
				return true
			}
			// FontSize(裸 token) 要改成 FontSize(uiFontSize(token))。
			id, ok := call.Args[0].(*ast.Ident)
			if !ok || !want[id.Name] {
				return true
			}
			t.Errorf("%s: FontSize(%s) 绕过了 uiFontSize，这一档不跟界面字号走",
				fset.Position(call.Pos()), id.Name)
			return true
		})
	}
}

// TestUIFontSizeStepClamps 断言档位收敛到范围内，且范围内的值原样通过。
//
// 档位会从配置文件里读回来（用户手改过、旧版本写过别的数），一个 99 落进来，
// 下一帧字号就是 12 倍 —— 界面直接涨出窗口。所以进 setUIFontSizeStep 的值必须
// 夹回 [min, max]；而范围内的值夹一遍必须还是自己，夹掉了就不是「档位」了。
func TestUIFontSizeStepClamps(t *testing.T) {
	resetUIFontStep(t)
	for _, step := range []float32{uiFontSizeMin, uiFontSizeDefault, uiFontSizeMax} {
		if got := setUIFontSizeStep(step); got != step {
			t.Errorf("档位 %v 被改成 %v，范围内的档位不该被改写", step, got)
		}
	}
	for _, bogus := range []float32{-99, -3, 99, 1000} {
		got := setUIFontSizeStep(bogus)
		if got < uiFontSizeMin || got > uiFontSizeMax {
			t.Errorf("档位 %v 收敛到 %v，应夹在 %v~%v 之间", bogus, got, uiFontSizeMin, uiFontSizeMax)
		}
	}
}

// TestUIFontSizeAppliesInTheRunningApp 从应用上读：设置改了，正在跑的界面
// 的字号跟着变。
func TestUIFontSizeAppliesInTheRunningApp(t *testing.T) {
	a := richApp(t)
	resetUIFontStep(t)

	if got := a.uiFontSizeValue(); got != uiFontSizeDefault {
		t.Fatalf("默认界面字号 = %v, want %v", got, uiFontSizeDefault)
	}
	a.setUIFontSize(uiFontSizeDefault + 1)
	if got := a.uiFontSizeValue(); got != uiFontSizeDefault+1 {
		t.Errorf("调大一档之后 = %v, want %v", got, uiFontSizeDefault+1)
	}
	// 缩放真的接上了：应用上改一档，正文档的字号要变大。
	if uiFontSize(textBody) <= textBody {
		t.Errorf("应用上调大一档后正文档还是 %.1f，缩放没接上", uiFontSize(textBody))
	}
	a.setUIFontSize(uiFontSizeDefault)
	if uiFontSize(textBody) != textBody {
		t.Errorf("拨回默认后正文档 = %v, want %v", uiFontSize(textBody), textBody)
	}
}

// TestUIFontSizePersists 界面字号要落进配置，下次启动才恢复得了。
func TestUIFontSizePersists(t *testing.T) {
	a := richApp(t)
	resetUIFontStep(t)

	// 拨到最大档，不越界 —— 越界的值会被夹回来，存进配置的就不是拨过去的那一格。
	a.setUIFontSize(uiFontSizeMax)

	if got := a.store.Config().UIFontSize; got != uiFontSizeMax {
		t.Errorf("配置里的界面字号 = %v, want %v", got, uiFontSizeMax)
	}
}

// TestUIFontSizeRowInSettings 设置页里要有一格能调界面字号。
//
// 这条守的是「用户够得着」：uiFontSize 那一套函数写得再对，设置页上没有入口，
// 用户也只能改代码字号 —— 那正是 issue #7 问题二的原话（「设置里，只能修改
// 代码字体大小」）。所以入口本身是要验收的一部分。
//
// 点的是 −/＋ 那两个小按钮（和代码字号同一套 stepper），不是行左边那行字：
// 行标题是纯文本，点了不做事。
func TestUIFontSizeRowInSettings(t *testing.T) {
	a := richApp(t)
	s := store.Open(t.TempDir())
	a.attachStore(s)
	settle(t, a)

	const winH float32 = settingsWindowH
	tt := settingsTester(t, a, settingsWindowW, settingsWindowH)
	a.settingsTab = settingsTabGeneral
	tt.Frame()

	if !tt.HasText(T("界面字体大小", "UI Font Size")) {
		t.Fatalf("通用偏好里没有界面字体大小这一行：%q", tt.Texts())
	}

	before := a.uiFontSizeValue()
	scrollTo(t, tt, T("放大界面字号", "Larger UI font"), winH)
	if err := tt.Click(T("放大界面字号", "Larger UI font")); err != nil {
		t.Fatalf("点「放大界面字号」: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if got := a.uiFontSizeValue(); got != before+1 {
		t.Errorf("点过 + 之后 = %v, want %v", got, before+1)
	}
	if uiFontSize(textBody) <= textBody {
		t.Errorf("界面上的字没跟着变大：正文档还是 %.1f", uiFontSize(textBody))
	}
	// 落盘：配置里要留下这一格，下次启动才恢复得了。
	if s.Config().UIFontSize != before+1 {
		t.Errorf("界面字号没存进配置文件：%v", s.Config().UIFontSize)
	}
}

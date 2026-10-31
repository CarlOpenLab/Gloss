package main

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

// —— 代码编辑器配色方案 ——
//
// 编辑器的整套配色（底色、行号、选区、活动行和语法高亮）出厂就是 Vitesse：
// 深浅外观各带一份色表，按窗口当前外观自动取用 —— 和 VS Code 装主题一个
// 道理，切外观的那一拍就换过去。以后想加别的方案（比如 Vitesse Black），
// 往 codeThemePresets 里添一份、把 defaultCodeTheme 指过去就行。
//
// Vitesse 这份色值逐条照官方主题（antfu/vscode-theme-vitesse，MIT）抄的，
// TextMate scope 到 mygo TokenKind 的映射写在每行注释里。

// defaultCodeTheme 是出厂方案的名字。
const defaultCodeTheme = "vitesse"

// codeThemeVariant 是一套外观下的完整配色。颜色为零表示「这一项不动，用
// 默认主题的」，所以方案只列和默认不一样的项。
type codeThemeVariant struct {
	bg, text         ui.Color
	lineNumber       ui.Color
	activeLineNumber ui.Color
	activeLine       ui.Color
	selection        ui.Color // 选区和未聚焦选区同一档色，跳转高亮不随焦点变
	cursor           ui.Color
	indentGuide      ui.Color
	tokens           []ui.Color // 按 editor.TokenKind 下标；零色不覆盖
}

// codeThemePreset 是一套方案：id 存配置，深浅两份色表按外观取。
type codeThemePreset struct {
	id    string
	dark  codeThemeVariant
	light codeThemeVariant
}

// codeThemePresets 是全部内置方案，Vitesse 是出厂默认的那份。
var codeThemePresets = []codeThemePreset{
	vitessePreset(),
}

// codeThemePresetByID 按 id 找方案；找不到返回 nil（调用方落回默认方案）。
func codeThemePresetByID(id string) *codeThemePreset {
	for i := range codeThemePresets {
		if codeThemePresets[i].id == id {
			return &codeThemePresets[i]
		}
	}
	return nil
}

// apply 把方案套到默认主题上：面板色逐项换，语法色按 kind 覆盖非零的。
func (p *codeThemePreset) apply(th *editor.Theme, dark bool) {
	v := &p.light
	if dark {
		v = &p.dark
	}
	th.Background = v.bg
	th.Text = v.text
	th.LineNumber = v.lineNumber
	th.ActiveLineNumber = v.activeLineNumber
	th.ActiveLine = v.activeLine
	th.Selection = v.selection
	th.InactiveSelection = v.selection
	th.Cursor = v.cursor
	th.IndentGuide = v.indentGuide
	for k, col := range v.tokens {
		if k < len(th.Tokens) && col.A != 0 {
			th.Tokens[k].Color = col
		}
	}
}

// codeThemeTokens 把「TokenKind → 色号」的表摊成按下标取的切片，超界的
// kind 丢掉 —— mygo 哪天增删 token 种类时这里不会越界。
func codeThemeTokens(m map[editor.TokenKind]string) []ui.Color {
	out := make([]ui.Color, int(editor.TokenInvalid)+1)
	for k, s := range m {
		if int(k) < len(out) {
			out[k] = ui.Hex(s)
		}
	}
	return out
}

// vitessePreset 是 Vitesse 深浅两份。选区一栏没有照抄原主题 —— VS Code 的
// 选区底（白/黑各半成的一成透明度）在本编辑器里几乎看不见，而搜索结果跳转
// 的落点就是选区；这里取主题自带的绿调出一档明确的底色，其余都是原值。
func vitessePreset() codeThemePreset {
	return codeThemePreset{
		id:    "vitesse",
		dark:  vitesseVariant(true),
		light: vitesseVariant(false),
	}
}

func vitesseVariant(dark bool) codeThemeVariant {
	if dark {
		return codeThemeVariant{
			bg:               ui.Hex("#121212"),
			text:             ui.Hex("#dbd7caee"), // 默认文字带一成透明度，Vitesse 的「One」
			lineNumber:       ui.Hex("#dedcd550"),
			activeLineNumber: ui.Hex("#bfbaaa"),
			activeLine:       ui.Hex("#181818"), // editor.lineHighlightBackground
			selection:        ui.Hex("#4d9375").Alpha(0.32),
			cursor:           ui.Hex("#4d9375"),
			indentGuide:      ui.Hex("#ffffff").Alpha(0.08), // #ffffff15
			tokens: codeThemeTokens(map[editor.TokenKind]string{
				editor.TokenText:        "#dbd7caee",
				editor.TokenKeyword:     "#4d9375", // keyword, tag, markup.heading, constant.language
				editor.TokenType:        "#5DA994", // support.type.primitive, entity.name.type
				editor.TokenFunction:    "#80a665", // entity.name.function, entity
				editor.TokenVariable:    "#bd976a", // variable identifier, entity.other.attribute-name
				editor.TokenProperty:    "#b8a965", // property, support
				editor.TokenConstant:    "#c99076", // constant, variable.language, support.constant
				editor.TokenNumber:      "#4C9A91", // constant.numeric, number
				editor.TokenString:      "#c98a7d", // string, attribute.value
				editor.TokenEscape:      "#e6cc77", // constant.character.escape
				editor.TokenRegexp:      "#c4704f", // source.regexp
				editor.TokenComment:     "#758575dd",
				editor.TokenOperator:    "#cb7676", // keyword.operator, storage
				editor.TokenPunctuation: "#666666", // delimiter.bracket, delimiter
				editor.TokenTag:         "#4d9375", // entity.name.tag
				editor.TokenAttribute:   "#bd976a", // entity.other.attribute-name
				editor.TokenMeta:        "#bd976a", // 装饰器/注解按标识符着色
				editor.TokenHeading:     "#4d9375", // markup.heading
				editor.TokenEmphasis:    "#dbd7caee",
				editor.TokenStrong:      "#dbd7caee",
				editor.TokenLink:        "#dedcd590", // markup.underline.link
				editor.TokenCode:        "#4d9375",   // markup.raw
				editor.TokenInserted:    "#85e89d",   // markup.inserted
				editor.TokenDeleted:     "#fdaeb7",   // markup.deleted, invalid
				editor.TokenInvalid:     "#fdaeb7",
			}),
		}
	}
	return codeThemeVariant{
		bg:               ui.Hex("#ffffff"),
		text:             ui.Hex("#393a34"),
		lineNumber:       ui.Hex("#393a3450"),
		activeLineNumber: ui.Hex("#4e4f47"),
		activeLine:       ui.Hex("#f7f7f7"),
		selection:        ui.Hex("#1e754f").Alpha(0.20),
		cursor:           ui.Hex("#1e754f"),
		indentGuide:      ui.Hex("#000000").Alpha(0.08), // #00000015
		tokens: codeThemeTokens(map[editor.TokenKind]string{
			editor.TokenText:        "#393a34",
			editor.TokenKeyword:     "#1e754f", // keyword, tag, markup.heading, constant.language
			editor.TokenType:        "#2e8f82", // support.type.primitive, entity.name.type
			editor.TokenFunction:    "#59873a", // entity.name.function, entity
			editor.TokenVariable:    "#b07d48", // variable identifier, entity.other.attribute-name
			editor.TokenProperty:    "#998418", // property, support
			editor.TokenConstant:    "#a65e2b", // constant, variable.language, support.constant
			editor.TokenNumber:      "#2f798a", // constant.numeric, number
			editor.TokenString:      "#b56959", // string, attribute.value
			editor.TokenEscape:      "#bda437", // constant.character.escape
			editor.TokenRegexp:      "#ab5e3f", // source.regexp
			editor.TokenComment:     "#a0ada0",
			editor.TokenOperator:    "#ab5959", // keyword.operator, storage
			editor.TokenPunctuation: "#999999", // delimiter.bracket, delimiter
			editor.TokenTag:         "#1e754f", // entity.name.tag
			editor.TokenAttribute:   "#b07d48", // entity.other.attribute-name
			editor.TokenMeta:        "#b07d48", // 装饰器/注解按标识符着色
			editor.TokenHeading:     "#1c6b48", // markup.heading
			editor.TokenEmphasis:    "#393a34",
			editor.TokenStrong:      "#393a34",
			editor.TokenLink:        "#393a3490", // markup.underline.link
			editor.TokenCode:        "#1c6b48",   // markup.raw
			editor.TokenInserted:    "#22863a",   // markup.inserted
			editor.TokenDeleted:     "#b31d28",   // markup.deleted, invalid
			editor.TokenInvalid:     "#b31d28",
		}),
	}
}

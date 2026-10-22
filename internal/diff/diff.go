// Package diff 把 unified git diff 文本解析成结构化数据模型，
// 供前端渲染和 AI 解释共用。零第三方依赖。
package diff

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// imageExts 是界面按「图片」预览的扩展名。
//
// 只收栅格图：SVG 是文本，diff 本身就能读，mygo 的 DecodeBitmap 也不收它。
// 这一份同时给文件模型用 —— 未跟踪的图片不必按「文本」读进来切行，界面
// 也不需要看到它的乱码。
var imageExts = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true,
	".webp": true, ".bmp": true,
}

// IsImagePath 说这个路径是不是一张可以直接摆上屏幕的图片。
func IsImagePath(path string) bool {
	return imageExts[strings.ToLower(filepath.Ext(path))]
}

// LineType 表示 diff 中一行的类型。
type LineType string

const (
	LineAdd     LineType = "add"
	LineDel     LineType = "del"
	LineContext LineType = "context"
)

// Line 是 diff 中的一行，OldNum/NewNum 为 0 表示该行在对应侧不存在。
type Line struct {
	Type    LineType `json:"type"`
	Content string   `json:"content"`
	OldNum  int      `json:"oldNum,omitempty"`
	NewNum  int      `json:"newNum,omitempty"`
}

// Hunk 是一个 @@ 代码块。
type Hunk struct {
	ID       string `json:"id"`
	Header   string `json:"header"`
	Section  string `json:"section,omitempty"` // @@ 后面的函数名等上下文
	OldStart int    `json:"oldStart"`
	OldLines int    `json:"oldLines"`
	NewStart int    `json:"newStart"`
	NewLines int    `json:"newLines"`
	Lines    []Line `json:"lines"`
}

// Raw 把 hunk 还原成 unified diff 文本，用于喂给模型。
func (h Hunk) Raw() string {
	var b strings.Builder
	b.WriteString(h.Header)
	b.WriteByte('\n')
	for _, l := range h.Lines {
		switch l.Type {
		case LineAdd:
			b.WriteByte('+')
		case LineDel:
			b.WriteByte('-')
		default:
			b.WriteByte(' ')
		}
		b.WriteString(l.Content)
		b.WriteByte('\n')
	}
	return b.String()
}

// NewStartLine 返回 hunk 覆盖到的第一个新侧行号。
//
// 新侧行数为 0 时（纯删除的 hunk），git 把头里的起始行号写成插入点之前
// 的那一行 —— `@@ -5,3 +4,0 @@` 的意思是「删掉旧文件 5-7 行，新文件第 4
// 行之后什么都不剩」，所以它覆盖的起点是第 5 行。
//
// 界面算「未改动区间」和还原旧文件都要靠这个约定，所以放在这里共用。
func (h Hunk) NewStartLine() int {
	if h.NewLines == 0 {
		return h.NewStart + 1
	}
	return h.NewStart
}

// NewEndLine 返回 hunk 覆盖到的最后一个新侧行号（纯删除的情形见 NewStartLine）。
func (h Hunk) NewEndLine() int {
	if h.NewLines == 0 {
		return h.NewStart
	}
	return h.NewStart + h.NewLines - 1
}

// FileDiff 是单个文件的变更。Status: added / deleted / modified / renamed。
type FileDiff struct {
	OldPath  string `json:"oldPath"`
	NewPath  string `json:"newPath"`
	Status   string `json:"status"`
	Binary   bool   `json:"binary,omitempty"` // 二进制文件：没有 hunk，界面显示「二进制文件已变更」
	Hunks    []Hunk `json:"hunks"`
	AddCount int    `json:"addCount"`
	DelCount int    `json:"delCount"`
}

// Path 是这个文件的主路径。删除的文件没有新路径，退回旧的。
func (f FileDiff) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// HunkID 是 hunk 的稳定标识：文件路径 + hunk 原文的内容哈希。
//
// 哈希覆盖的是 hunk.Raw()，也就是模型和界面看到的那份文本，@@ 行里的
// 行号也包括在内 —— 「同一处改动挪到了别的行」算两个不同的块。
// 界面用它当列表 key，AI 解释缓存也用它当 key 的一半。
func HunkID(path string, h Hunk) string {
	sum := sha256.Sum256([]byte(path + "|" + h.Raw()))
	return hex.EncodeToString(sum[:])[:16]
}

// AssignHunkIDs 给每个 hunk 算好 ID（见 HunkID）。
func (f *FileDiff) AssignHunkIDs() {
	for hi := range f.Hunks {
		f.Hunks[hi].ID = HunkID(f.Path(), f.Hunks[hi])
	}
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@\s?(.*)$`)

// Parse 解析完整的 git diff 输出（也兼容没有 diff --git 头的裸 patch）。
func Parse(raw string) []FileDiff {
	var files []FileDiff
	var cur *FileDiff
	var hunk *Hunk
	oldLine, newLine := 0, 0

	finishFile := func() {
		if cur != nil && (len(cur.Hunks) > 0 || cur.OldPath != "" || cur.NewPath != "") {
			files = append(files, *cur)
		}
		cur = nil
		hunk = nil
	}

	sc := bufio.NewScanner(strings.NewReader(raw))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()

		// diff --git 永远是一列顶格出现的新文件头，即使上个 hunk
		// 行数没完（比如用户粘贴了被截断的 diff）也要切文件
		if strings.HasPrefix(line, "diff --git ") {
			finishFile()
			cur = &FileDiff{Status: "modified"}
			rest := strings.TrimPrefix(line, "diff --git ")
			if i := strings.Index(rest, " b/"); i > 0 {
				cur.OldPath = strings.TrimPrefix(rest[:i], "a/")
				cur.NewPath = rest[i+3:]
			}
			continue
		}

		// 处于 hunk 内部：按 old/new 行数判断是否结束，避免把内容里的
		// "--- "/"+++ " 误判成文件头。
		if hunk != nil && (oldLine-hunk.OldStart < hunk.OldLines || newLine-hunk.NewStart < hunk.NewLines) {
			if strings.HasPrefix(line, `\`) { // "\ No newline at end of file"
				continue
			}
			c := byte(' ')
			content := ""
			if len(line) > 0 {
				c = line[0]
				content = line[1:]
			}
			switch c {
			case '+':
				hunk.Lines = append(hunk.Lines, Line{Type: LineAdd, Content: content, NewNum: newLine})
				newLine++
				cur.AddCount++
			case '-':
				hunk.Lines = append(hunk.Lines, Line{Type: LineDel, Content: content, OldNum: oldLine})
				oldLine++
				cur.DelCount++
			default: // ' ' 或格式异常，宽容地按 context 处理
				hunk.Lines = append(hunk.Lines, Line{Type: LineContext, Content: content, OldNum: oldLine, NewNum: newLine})
				oldLine++
				newLine++
			}
			continue
		}
		hunk = nil

		switch {
		case cur == nil:
			// 忽略 diff 之外的行（commit message 等）
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = "added"
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = "deleted"
		case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
			cur.Binary = true
		case strings.HasPrefix(line, "rename from "):
			cur.Status = "renamed"
			cur.OldPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			cur.NewPath = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "--- "):
			if len(cur.Hunks) > 0 {
				// 裸 patch 拼接：上一个文件已有 hunk，这里开始新文件
				finishFile()
				cur = &FileDiff{Status: "modified"}
			}
			if p := strings.TrimPrefix(line, "--- "); p == "/dev/null" {
				cur.Status = "added"
				cur.OldPath = ""
			} else {
				cur.OldPath = stripAB(p)
			}
		case strings.HasPrefix(line, "+++ "):
			if p := strings.TrimPrefix(line, "+++ "); p == "/dev/null" {
				cur.Status = "deleted"
				cur.NewPath = ""
			} else {
				cur.NewPath = stripAB(p)
			}
		case strings.HasPrefix(line, "@@ "):
			m := hunkRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			cur.Hunks = append(cur.Hunks, Hunk{
				Header:   line,
				Section:  m[5],
				OldStart: atoi(m[1]),
				OldLines: atoiDef(m[2], 1),
				NewStart: atoi(m[3]),
				NewLines: atoiDef(m[4], 1),
			})
			hunk = &cur.Hunks[len(cur.Hunks)-1]
			oldLine = hunk.OldStart
			newLine = hunk.NewStart
		}
	}
	finishFile()

	// 计算每个 hunk 的稳定 ID（内容哈希），用作前端 key 和解释缓存 key
	for fi := range files {
		files[fi].AssignHunkIDs()
	}
	return files
}

// IsBinaryContent 按 git 的判据说一份内容是不是二进制：前 8000 字节里
// 有没有 NUL。普通文本（哪怕不是 UTF-8）没有，图片、压缩包、可执行文件都有。
//
// 不能用 utf8.Valid 代替：NUL 是合法的 UTF-8，二进制内容能整份通过校验，
// 然后被当成文本画成一屏乱码。
func IsBinaryContent(content []byte) bool {
	head := content
	if len(head) > 8000 {
		head = head[:8000]
	}
	return bytes.IndexByte(head, 0) >= 0
}

// NewFileFromContent 把一个全新文件的内容合成成「整文件新增」的 FileDiff。
//
// 未跟踪的文件不会出现在 `git diff` 的输出里（git 根本不认为它属于版本控制），
// 但 review 时恰恰最需要看到它们，所以单独合成一份。
func NewFileFromContent(path string, content []byte) FileDiff {
	f := FileDiff{NewPath: path, Status: "added"}
	if len(content) == 0 {
		return f
	}
	// 二进制内容按行切开只会得到一堆乱码，再塞进 diff 视图就是几屏没法读
	// 的东西。标成二进制之后，图片由 preview 画，其余显示「二进制文件已变更」。
	if IsBinaryContent(content) {
		f.Binary = true
		return f
	}
	// 末尾的换行只是文件结束标记，不构成一个空行
	text := strings.TrimSuffix(string(content), "\n")
	lines := strings.Split(text, "\n")
	h := Hunk{
		Header:   fmt.Sprintf("@@ -0,0 +1,%d @@", len(lines)),
		NewStart: 1,
		NewLines: len(lines),
	}
	for i, l := range lines {
		h.Lines = append(h.Lines, Line{Type: LineAdd, Content: l, NewNum: i + 1})
	}
	f.Hunks = []Hunk{h}
	f.AddCount = len(lines)
	f.AssignHunkIDs()
	return f
}

func stripAB(p string) string {
	if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
		return p[2:]
	}
	return p
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atoiDef(s string, def int) int {
	if s == "" {
		return def
	}
	return atoi(s)
}

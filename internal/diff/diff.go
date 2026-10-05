// Package diff 把 unified git diff 文本解析成结构化数据模型，
// 供前端渲染和 AI 解释共用。零第三方依赖。
package diff

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

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

// AssignHunkIDs 给每个 hunk 算出稳定 ID（文件路径 + hunk 原文的内容哈希）。
// 界面用它当列表 key，AI 解释缓存也用它当 key 的一半——所以同一个 hunk
// 出现在不同 PR、不同分支里都会命中同一份缓存。
func (f *FileDiff) AssignHunkIDs() {
	for hi := range f.Hunks {
		h := &f.Hunks[hi]
		sum := sha256.Sum256([]byte(f.NewPath + "|" + h.Raw()))
		h.ID = hex.EncodeToString(sum[:])[:16]
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

// NewFileFromContent 把一个全新文件的内容合成成「整文件新增」的 FileDiff。
//
// 未跟踪的文件不会出现在 `git diff` 的输出里（git 根本不认为它属于版本控制），
// 但 review 时恰恰最需要看到它们，所以单独合成一份。
func NewFileFromContent(path string, content []byte) FileDiff {
	f := FileDiff{NewPath: path, Status: "added"}
	if len(content) == 0 {
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

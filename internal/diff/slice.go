package diff

import "strings"

// SliceHunk 从一份完整的 `git diff` 原文里切出「path 这个文件的 hunkID 那一段」：
// 文件头（diff --git 到第一个 @@ 之前）+ 那一段 hunk，字节原样。
//
// 为什么不在模型里把 Hunk.Raw() 拼回去：Parse 会丢掉
// "\ No newline at end of file" 这类标记（它们只影响「行尾有没有换行」，
// 不进 Lines），拿 Raw() 拼出来的 patch 少了它，git apply 会按另一套
// 行尾约定处理甚至直接拒收。所以这里在原文上按和 Parse 完全一致的规则扫
// （'\' 行不计数、'+' 计新侧、'-' 计旧侧、其余两侧都计），整段原文切出来。
//
// 找不到目标（路径不在、hunkID 不在、文件根本没有 hunk）返回 false ——
// 调用方借此发现「用户点下去和 git 真正执行之间文件被改过」，这是正常
// 竞态，不是错误。
func SliceHunk(raw, path, hunkID string) (string, bool) {
	lines := strings.Split(raw, "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "diff --git ") {
			continue
		}
		// 文件块到下一条 diff --git 为止。和 Parse 一样把顶格的 diff --git
		// 当作新文件头 —— hunk 内容行永远带 ' '/'+'/'-' 前缀，不会顶格。
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(lines[j], "diff --git ") {
				end = j
				break
			}
		}
		if blockHasPath(lines[i], path) {
			if out, ok := sliceInBlock(path, lines[i:end], hunkID); ok {
				return out, true
			}
		}
		i = end - 1
	}
	return "", false
}

// dropCR 按 bufio.ScanLines 的规矩丢掉行尾的一个 \r。
//
// Parse 是拿 bufio.Scanner 扫文本的，ScanLines 会吃掉行尾的 \r（CRLF 文件
// 在 patch 里就长这样）；SliceHunk 是在 strings.Split 的原文上走的，行里
// 还留着它。用来计算 ID 的重建必须和解析器逐字节一致，所以比照同一条规矩
// 掉一个 \r —— 注意只影响「算 ID」，切出去的原文里 \r 原样保留。
func dropCR(s string) string { return strings.TrimSuffix(s, "\r") }

// blockHasPath 判断一条 diff --git 头指向的文件是不是 path。路径推导和
// Parse 完全一样（按第一个 " b/" 切成两半），保证和模型里的路径对得上。
func blockHasPath(header, path string) bool {
	rest := strings.TrimPrefix(dropCR(header), "diff --git ")
	i := strings.Index(rest, " b/")
	if i <= 0 {
		return false
	}
	oldPath := strings.TrimPrefix(rest[:i], "a/")
	newPath := rest[i+3:]
	return path == newPath || path == oldPath
}

// sliceInBlock 在单个文件块内找出 hunkID 那一段，返回「文件头 + 该 hunk」。
//
// 文件头取到块内第一个 @@ 之前为止（而不是到目标 hunk 之前）—— 中间那些
// hunk 不属于这一刀。每段的 ID 用和 Parse 相同的重建方式计算，对上了才算。
func sliceInBlock(path string, block []string, hunkID string) (string, bool) {
	headerEnd := -1
	for k, line := range block {
		line = dropCR(line)
		if strings.HasPrefix(line, "@@ ") && hunkRe.MatchString(line) {
			headerEnd = k
			break
		}
	}
	if headerEnd < 0 {
		return "", false // 二进制、纯 mode 变更：没有 hunk 可切
	}

	for hs := headerEnd; hs < len(block); {
		line := dropCR(block[hs])
		m := hunkRe.FindStringSubmatch(line)
		if m == nil || !strings.HasPrefix(line, "@@ ") {
			hs++
			continue
		}

		h := Hunk{
			Header:   line,
			Section:  m[5],
			OldStart: atoi(m[1]), OldLines: atoiDef(m[2], 1),
			NewStart: atoi(m[3]), NewLines: atoiDef(m[4], 1),
		}
		oldLine, newLine := h.OldStart, h.NewStart
		he := hs + 1
		for he < len(block) && (oldLine-h.OldStart < h.OldLines || newLine-h.NewStart < h.NewLines) {
			l := block[he]
			he++
			if strings.HasPrefix(l, `\`) {
				continue // "\ No newline at end of file"：不计入行数，但留在切片里
			}
			c, content := byte(' '), ""
			if len(l) > 0 {
				c, content = l[0], dropCR(l[1:])
			}
			switch c {
			case '+':
				h.Lines = append(h.Lines, Line{Type: LineAdd, Content: content, NewNum: newLine})
				newLine++
			case '-':
				h.Lines = append(h.Lines, Line{Type: LineDel, Content: content, OldNum: oldLine})
				oldLine++
			default:
				h.Lines = append(h.Lines, Line{Type: LineContext, Content: content, OldNum: oldLine, NewNum: newLine})
				oldLine++
				newLine++
			}
		}
		// 行数刚好凑齐时，末尾那行缺换行的「\ No newline」标记是在计数判定
		// 之后出现的，但它属于这一段 patch，得跟着。
		for he < len(block) && strings.HasPrefix(block[he], `\`) {
			he++
		}

		if HunkID(path, h) == hunkID {
			kept := append(append([]string{}, block[:headerEnd]...), block[hs:he]...)
			return strings.Join(kept, "\n") + "\n", true
		}
		hs = he
	}
	return "", false
}

package main

import (
	"path/filepath"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 文件列表里的几个「这个文件的标识和名字」的纯函数。
//
// 它们原来住在 rows.go（那份把整份 diff 摊成扁平行数组的行模型）里。行模型
// 换成 mygo 的单文件 DiffView 之后没了，但 key 和显示名这两件事和渲染方式
// 无关，两边都要用同一套 —— 所以单独留在这儿。

// fileKey 是文件的稳定标识：路径 + 所属段落。
//
// 同一个路径可以同时在「已暂存」和「未暂存」两段里（暂存之后又改了），
// 两段的 hunk、行号、内容都不一样 —— 拿路径当 key，行模型的 key、内容
// 缓存、折叠状态会互相顶掉。SectionNone（历史提交、储藏的 diff）不加前缀：
// 那边的 key 和语义和分段之前完全一样。
func fileKey(f gitrepo.File) string {
	switch f.Section {
	case gitrepo.SectionStaged:
		return "s:" + pathOf(f)
	case gitrepo.SectionUnstaged:
		return "u:" + pathOf(f)
	}
	return "n:" + pathOf(f)
}

// pathOf 是给人看、给 git 口子用的纯路径（不带段落前缀）。
// 界面上的显示、模糊筛选、patch 切片的路径参数都用它，不用 fileKey。
func pathOf(f gitrepo.File) string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// displayPath 是文件头上显示的路径。删除的文件没有新路径，退回旧的。
func displayPath(f gitrepo.File) string {
	if f.NewPath != "" {
		return f.NewPath
	}
	if f.OldPath != "" {
		return f.OldPath
	}
	return filepath.Base(f.NewPath)
}

// tabWidth 是 tab 展开后的列宽。
//
// DiffView 和编辑器都是同一档：4 列和大多数编辑器的默认值一致，也和
// 之前自研渲染器里那个 expandTabs 一样。
const tabWidth = 4

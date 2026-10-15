package gitrepo

import (
	"context"
	"sort"
	"strings"
)

// WorktreeFile 是工作区里的一个路径。目前只带路径 —— 编辑器那一栏要的是
// 「这个仓库有哪些代码」，改动与否由调用方拿状态自己合。
type WorktreeFile struct {
	Path string
}

// WorktreeFiles 列出工作区里的全部文件：已跟踪的，加上未跟踪但没有被
// .gitignore 忽略的。
//
// 这是给「编辑器」视图的文件栏用的：它要的是「仓库里有哪些代码」，而不是
// 「哪些代码改了」—— 后者是 git status 的活。忽略文件不在其中，空的目录
// 也不会出现（git 不追踪目录），与「未跟踪」在状态里的含义一致。
//
// -z 走 NUL 分隔，core.quotePath=false 关掉引号转义：带空格、带中文的名字
// 原样出来，不用再解一层转义。
func (r *Repo) WorktreeFiles(ctx context.Context) ([]WorktreeFile, error) {
	out, err := run(ctx, r.Root,
		"-c", "core.quotePath=false",
		"ls-files", "-z", "--cached", "--others", "--exclude-standard",
	)
	if err != nil {
		return nil, err
	}

	var files []WorktreeFile
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		files = append(files, WorktreeFile{Path: p})
	}
	// ls-files 先吐已跟踪（按索引顺序）再吐未跟踪，混着来不利于建树；
	// 排一次序，树里同层就是整齐的。
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

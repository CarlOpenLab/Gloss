package gitrepo

import (
	"context"
	"strconv"
	"strings"
	"time"

	"diff-explainer/internal/diff"
)

// Commit 是历史里的一个提交。
type Commit struct {
	Hash    string    // 完整哈希
	Short   string    // 短哈希，界面上显示这个
	Author  string    // 作者名
	When    time.Time // 作者时间
	Subject string    // 提交信息的第一行
	Refs    string    // 指向它的分支/标签，如 "HEAD -> main, origin/main"
}

// Log 读最近的提交。skip 用来翻页。
func (r *Repo) Log(ctx context.Context, limit, skip int) ([]Commit, error) {
	if limit <= 0 {
		limit = 100
	}
	// 还没有任何提交时 git log 会直接报错，但那只是「没有历史」，
	// 不是出错 —— 先探一下 HEAD。
	if _, err := run(ctx, r.Root, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return nil, nil
	}
	// 用 %x00 分隔字段、%x1e 分隔记录：作者名和提交信息里可能有任何
	// 可打印字符，用普通分隔符会被内容里的逗号、竖线之类拆错。
	format := "%H%x00%h%x00%an%x00%aI%x00%D%x00%s%x1e"
	out, err := run(ctx, r.Root, "-c", "core.quotePath=false", "log", "--no-color",
		"--date=iso-strict", "--pretty=format:"+format,
		"--skip="+strconv.Itoa(skip), "-n", strconv.Itoa(limit))
	if err != nil {
		return nil, err
	}
	return parseLog(string(out)), nil
}

func parseLog(out string) []Commit {
	var commits []Commit
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		f := strings.Split(rec, "\x00")
		if len(f) < 6 {
			continue
		}
		when, err := time.Parse(time.RFC3339, f[3])
		if err != nil {
			when = time.Time{}
		}
		commits = append(commits, Commit{
			Hash:    f[0],
			Short:   f[1],
			Author:  f[2],
			When:    when,
			Refs:    strings.TrimSpace(f[4]),
			Subject: f[5],
		})
	}
	return commits
}

// CommitDiff 读一个提交相对其父提交的改动，产物和 Load 一样是 []File，
// 所以 diff 视图、语法高亮、✨ 解释全都能直接复用。
//
// 合并提交按第一个父提交比较（--first-parent）：跟第二个父比或者干脆
// 不给 diff 都没什么用，Fork 这类工具也是这么做的。
func (r *Repo) CommitDiff(ctx context.Context, hash string) ([]File, error) {
	args := append([]string{
		"-c", "core.quotePath=false",
		"show", "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/",
		"-M", "-U3", "--format=", "--first-parent",
	}, hash)
	out, err := run(ctx, r.Root, args...)
	if err != nil {
		return nil, err
	}
	return FilesFromDiff(diff.Parse(string(out))), nil
}

// FilesFromDiff 把解析好的 patch 包成可渲染的 File 列表。
//
// git status 那一路（Load）能拿到「未跟踪」「冲突」这类 patch 里没有的信息，
// 而按提交看 diff 时没有这些，状态只能从 patch 头推。
func FilesFromDiff(parsed []diff.FileDiff) []File {
	files := make([]File, 0, len(parsed))
	for i := range parsed {
		f := parsed[i]
		f.AssignHunkIDs()
		status := StatusModified
		switch f.Status {
		case "added":
			status = StatusAdded
		case "deleted":
			status = StatusDeleted
		case "renamed":
			status = StatusRenamed
		}
		// 指纹用 hunk 原文：同一个提交里的同一个文件，指纹始终一样，
		// 「已 review」的标记因此不会在重新打开时莫名其妙失效。
		files = append(files, File{
			FileDiff:    f,
			Status:      status,
			Fingerprint: fingerprint(f.Path(), hunksRaw(f)),
		})
	}
	return files
}

func hunksRaw(f diff.FileDiff) string {
	var b strings.Builder
	for i := range f.Hunks {
		b.WriteString(f.Hunks[i].Raw())
	}
	return b.String()
}

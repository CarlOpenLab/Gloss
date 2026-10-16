package gitrepo

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// Commit 是历史里的一个提交。
type Commit struct {
	Hash    string    // 完整哈希
	Short   string    // 短哈希，界面上显示这个
	Author  string    // 作者名
	When    time.Time // 作者时间
	Subject string    // 提交信息的第一行
	Refs    string    // 指向它的分支/标签，如 "HEAD -> main, origin/main"
	Parents []string  // 完整哈希的父提交；合并提交有两个以上，根提交没有
}

// Log 读最近的提交。skip 用来翻页。
//
// rev 是看哪条历史：分支名、标签名、任意 revision，空串表示当前的 HEAD。
// 侧栏点了哪条分支就传哪条 —— 不为空时不能退回 HEAD，那样点了分支看着没反应。
func (r *Repo) Log(ctx context.Context, rev string, limit, skip int) ([]Commit, error) {
	if limit <= 0 {
		limit = 100
	}
	// 还没有任何提交时 git log 会直接报错，但那只是「没有历史」，
	// 不是出错 —— 先探一下要看的那条引用在不在。
	probe := rev
	if probe == "" {
		probe = "HEAD"
	}
	if _, err := run(ctx, r.Root, "rev-parse", "--verify", "--quiet", probe); err != nil {
		return nil, nil
	}
	return r.log(ctx, []string{rev}, limit, skip)
}

// LogAll 读**所有分支**的提交：本地分支、远程跟踪分支、标签一起，按拓扑序排。
//
// 侧栏「所有分支」那一行用的就是它 —— 用户想看的是这个仓库总共在干什么，
// 不是某一条线的历史。储藏不在里面（git log --all 本来就不收 refs/stash），
// 那是储藏自己的视图。
//
// 空仓库直接返回空：--all 一条引用都没有时 git log 也是空输出，但顺带吐一
// 句 "does not have any commits yet" 到 stderr，按错误处理就错了。
func (r *Repo) LogAll(ctx context.Context, limit, skip int) ([]Commit, error) {
	if limit <= 0 {
		limit = 100
	}
	if _, err := run(ctx, r.Root, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return nil, nil
	}
	return r.log(ctx, nil, limit, skip)
}

// log 是 Log / LogAll 公共的身子：revs 是要看的 revision 列表（nil = HEAD）。
//
// 分开两个入口是因为「所有分支」不是一条 revision：--all 是个 flag，塞进
// rev 里会被 git 当成一条叫 "--all" 的引用名，报一句和真实原因无关的错。
func (r *Repo) log(ctx context.Context, revs []string, limit, skip int) ([]Commit, error) {
	// 用 %x00 分隔字段、%x1e 分隔记录：作者名和提交信息里可能有任何
	// 可打印字符，用普通分隔符会被内容里的逗号、竖线之类拆错。
	format := "%H%x00%h%x00%an%x00%aI%x00%D%x00%P%x00%s%x1e"
	// --topo-order：默认是按提交时间倒序，几条分支的提交会互相穿插，
	// 画出来的线是乱的。拓扑序保证父提交永远排在子提交之后，而且同一条
	// 线上的提交是连续的 —— 画泳道图的前提。
	args := []string{
		"-c", "core.quotePath=false", "log", "--no-color",
		"--date=iso-strict", "--topo-order", "--pretty=format:" + format,
		"--skip=" + strconv.Itoa(skip), "-n", strconv.Itoa(limit),
	}
	// revs 为空表示「所有分支」：--all 是 flag 不是 revision，得加在参数
	// 里。什么都不加的话 git log 退回 HEAD 一条线 —— 那正是「所有分支」
	// 这一行要避免的。
	if len(revs) == 0 {
		args = append(args, "--all")
	}
	for _, rev := range revs {
		if rev != "" {
			args = append(args, rev)
		}
	}
	out, err := run(ctx, r.Root, args...)
	if err != nil {
		return nil, err
	}
	return parseLog(string(out)), nil
}

// FileLog 读某个文件的提交历史（新到旧）。
//
// --follow 让它跟住重命名：文件改过名字之后，改名之前那些提交也在它的历史
// 里 —— 不然「这个文件的历史」会在改名那一刻凭空断掉，而那正是用户想看的
// 地方。（--follow 只认单个路径，这也是这里不收变参的原因。）
//
// 路径用 `--` 隔开：文件名可以是纯哈希的样子，不隔开会被当成 revision，
// 报一句和用户做的事毫不相干的错。
func (r *Repo) FileLog(ctx context.Context, path string, limit int) ([]Commit, error) {
	if limit <= 0 {
		limit = 100
	}
	if path == "" {
		return nil, nil
	}
	format := "%H%x00%h%x00%an%x00%aI%x00%D%x00%P%x00%s%x1e"
	args := []string{
		"-c", "core.quotePath=false", "log", "--no-color",
		"--date=iso-strict", "--follow", "--pretty=format:" + format,
		"-n", strconv.Itoa(limit), "--", path,
	}
	out, err := run(ctx, r.Root, args...)
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
		if len(f) < 7 {
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
			Parents: strings.Fields(f[5]), // 空格分隔，根提交是空串
			Subject: f[6],
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

// StashDiff 读一个储藏的内容，产物和 CommitDiff 一样是 []File。
//
// 名字传的是 stash@{n}，不是哈希 —— 储藏的改动不在它那个提交本身里
// （那个提交的第一个父是当时的 HEAD，直接 diff 出来的东西是错的），
// git stash show 才负责把 WIP 那次提交和它的父正确地摆在一起比。
func (r *Repo) StashDiff(ctx context.Context, name string) ([]File, error) {
	out, err := run(ctx, r.Root,
		"-c", "core.quotePath=false",
		"stash", "show", "-p", "--include-untracked",
		"--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/",
		"-M", "-U3", name)
	if err != nil {
		return nil, err
	}
	return FilesFromDiff(diff.Parse(string(out))), nil
}

// BranchDiff 读两个分支之间的改动（产物为 []File）。
// tripleDot 为 true 时用 base...target（自共同祖先分叉以来的改动，即 PR 模式）；
// 为 false 时用 base..target（两个分支最新指针的直接差异）。
func (r *Repo) BranchDiff(ctx context.Context, base, target string, tripleDot bool) ([]File, error) {
	op := "..."
	if !tripleDot {
		op = ".."
	}
	rev := base + op + target
	args := []string{
		"-c", "core.quotePath=false",
		"diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/",
		"-M", "-U3",
		rev,
	}
	out, err := run(ctx, r.Root, args...)
	if err != nil {
		return nil, err
	}
	return FilesFromDiff(diff.Parse(string(out))), nil
}

// BranchCommits 读从 base 到 target 之间的提交列表。
func (r *Repo) BranchCommits(ctx context.Context, base, target string, limit int) ([]Commit, error) {
	if limit <= 0 {
		limit = 100
	}
	rangeRev := base + ".." + target
	format := "%H%x00%h%x00%an%x00%aI%x00%D%x00%P%x00%s%x1e"
	args := []string{
		"-c", "core.quotePath=false", "log", "--no-color",
		"--date=iso-strict", "--topo-order", "--pretty=format:" + format,
		"-n", strconv.Itoa(limit),
		rangeRev,
	}
	out, err := run(ctx, r.Root, args...)
	if err != nil {
		return nil, err
	}
	return parseLog(string(out)), nil
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

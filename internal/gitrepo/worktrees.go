package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// WorktreeEntry 是 `git worktree list` 里的一项。
type WorktreeEntry struct {
	Path     string
	Head     string // 该工作区 HEAD 指向的提交（短哈希不保证，git 给全长）
	Branch   string // 短分支名；detached / bare 时为空
	Detached bool
	Bare     bool
	Prunable bool // 目录已经不在了，记录还没清
	Main     bool // 第一项：主工作区
}

// ErrWorktreeDirty 表示要移除的 worktree 里还有未提交的改动（或未跟踪文件）。
//
// git 的拒绝是一句英文长句，而界面上要据此弹「强制移除」的二次确认 ——
// 得有个能认出来的错，不能靠比对字符串。
var ErrWorktreeDirty = errors.New("worktree 里有未提交的改动")

// Worktrees 读仓库登记的全部工作区（含主工作区）。
//
// porcelain -z：记录之间是空字段（\0\0），路径里的空格、换行原样保留；
// 记录内还是「键 值」一行一项。git 把主工作区排在第一项。
func (r *Repo) Worktrees(ctx context.Context) ([]WorktreeEntry, error) {
	out, err := run(ctx, r.Root,
		"-c", "core.quotePath=false",
		"worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return parseWorktrees(string(out)), nil
}

func parseWorktrees(out string) []WorktreeEntry {
	var entries []WorktreeEntry
	var cur WorktreeEntry
	flush := func() {
		if cur.Path != "" {
			entries = append(entries, cur)
		}
		cur = WorktreeEntry{}
	}
	for _, tok := range strings.Split(out, "\x00") {
		if tok == "" { // 空字段 = 一条记录结束
			flush()
			continue
		}
		key, val, _ := strings.Cut(tok, " ")
		switch key {
		case "worktree":
			cur.Path = val
		case "HEAD":
			cur.Head = val
		case "branch":
			cur.Branch = strings.TrimPrefix(val, "refs/heads/")
		case "detached":
			cur.Detached = true
		case "bare":
			cur.Bare = true
		case "prunable":
			// "prunable gitdir file points to non-existent location" 这类
			// 带原因，认前缀。
			cur.Prunable = true
		}
	}
	flush()
	if len(entries) > 0 {
		entries[0].Main = true
	}
	return entries
}

// AddWorktree 把已有分支检出到一个新目录。目标已存在、分支已被别的
// worktree 检出之类的拒绝交给 git，译文在 failure.go。
func (r *Repo) AddWorktree(ctx context.Context, path, branch string) error {
	_, err := run(ctx, r.Root, "worktree", "add", path, branch)
	return err
}

// RemoveWorktree 移除一个 worktree 目录与登记。force 时连同未提交改动一起丢。
//
// 脏目录的拒绝单独认出来（ErrWorktreeDirty）：界面拿它决定要不要弹第二次
// 「强制移除」确认，而不是把所有失败都变成一句「移除没成功」。
func (r *Repo) RemoveWorktree(ctx context.Context, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	if _, err := run(ctx, r.Root, args...); err != nil {
		low := strings.ToLower(err.Error())
		if strings.Contains(low, "contains modified or untracked files") ||
			strings.Contains(low, "use --force to delete") {
			return fmt.Errorf("%w: %v", ErrWorktreeDirty, err)
		}
		return err
	}
	return nil
}

// PruneWorktrees 清掉目录已经不在了的 worktree 记录。之后那些分支才能
// 在主工作区重新检出。
func (r *Repo) PruneWorktrees(ctx context.Context) error {
	_, err := run(ctx, r.Root, "worktree", "prune")
	return err
}

// IsLinkedWorktree 说这个目录是不是一个 linked worktree（不是主工作区）。
//
// 判据是 Open 量好的 GitDir：linked worktree 的 .git 是指向 common dir
// 底下 worktrees/<name> 的一个文件，`--absolute-git-dir` 返回的不是
// Root/.git。子模块同理。
func (r *Repo) IsLinkedWorktree() bool {
	return filepath.Clean(r.GitDir) != filepath.Join(r.Root, ".git")
}

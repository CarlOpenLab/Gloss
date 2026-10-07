package gitrepo

import (
	"context"
	"errors"
	"strings"
)

// ErrNoChangesToStash 表示没有可储藏的未提交改动。
var ErrNoChangesToStash = errors.New("no local changes to save")

// 储藏管理：创建储藏、应用、应用并丢弃、删除储藏。

// StashPush 将当前工作区与暂存区的改动存入储藏（git stash push [-u] [-m <message>]）。
func (r *Repo) StashPush(ctx context.Context, message string, includeUntracked bool) error {
	args := []string{"stash", "push"}
	if includeUntracked {
		args = append(args, "--include-untracked")
	}
	message = strings.TrimSpace(message)
	if message != "" {
		args = append(args, "-m", message)
	}
	out, err := run(ctx, r.Root, args...)
	if err != nil {
		return err
	}
	if strings.Contains(string(out), "No local changes to save") {
		return ErrNoChangesToStash
	}
	return nil
}

// StashApply 将指定储藏的内容应用到当前工作区，不从储藏列表删除该条目（git stash apply <name>）。
func (r *Repo) StashApply(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "stash@{0}"
	}
	_, err := run(ctx, r.Root, "stash", "apply", name)
	return err
}

// StashPop 将指定储藏的内容应用到当前工作区，并在成功后将其从储藏列表丢弃（git stash pop <name>）。
func (r *Repo) StashPop(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "stash@{0}"
	}
	_, err := run(ctx, r.Root, "stash", "pop", name)
	return err
}

// StashDrop 从储藏列表中删除指定条目（git stash drop <name>）。
func (r *Repo) StashDrop(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "stash@{0}"
	}
	_, err := run(ctx, r.Root, "stash", "drop", name)
	return err
}

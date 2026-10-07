package gitrepo

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// 分支重命名与删除在 git 层的验收。

func TestRenameBranch(t *testing.T) {
	ctx := context.Background()

	t.Run("重命名非当前分支", func(t *testing.T) {
		r := twoCommitRepo(t)
		git(t, r.Root, "branch", "feature/old")
		if err := r.RenameBranch(ctx, "feature/old", "feature/new"); err != nil {
			t.Fatalf("RenameBranch: %v", err)
		}
		if got := revOut(t, r.Root, "rev-parse", "--verify", "refs/heads/feature/new"); got == "" {
			t.Errorf("新分支 feature/new 应该存在")
		}
		if got := gitOut(t, r.Root, "branch", "--list", "feature/old"); strings.TrimSpace(got) != "" {
			t.Errorf("旧分支 feature/old 该消失了，实际：%q", got)
		}
	})

	t.Run("重命名当前分支更新HEAD", func(t *testing.T) {
		r := twoCommitRepo(t)
		if err := r.RenameBranch(ctx, "main", "trunk"); err != nil {
			t.Fatalf("RenameBranch: %v", err)
		}
		if got := revOut(t, r.Root, "symbolic-ref", "--short", "HEAD"); got != "trunk" {
			t.Errorf("当前分支重命名后 HEAD = %q, want trunk", got)
		}
	})

	t.Run("目标重名被拒", func(t *testing.T) {
		r := twoCommitRepo(t)
		git(t, r.Root, "branch", "existing")
		err := r.RenameBranch(ctx, "existing", "main")
		if err == nil || !strings.Contains(err.Error(), "already exists") {
			t.Errorf("重命名至已存在分支应被拒：%v", err)
		}
	})
}

func TestDeleteBranch(t *testing.T) {
	ctx := context.Background()

	t.Run("已合并分支安全删除", func(t *testing.T) {
		r := twoCommitRepo(t)
		git(t, r.Root, "branch", "feature/merged")
		if err := r.DeleteBranch(ctx, "feature/merged", false); err != nil {
			t.Fatalf("DeleteBranch: %v", err)
		}
		if got := gitOut(t, r.Root, "branch", "--list", "feature/merged"); strings.TrimSpace(got) != "" {
			t.Errorf("已合并分支应该已被删除，实际：%q", got)
		}
	})

	t.Run("未合并分支-d返回ErrBranchNotMerged", func(t *testing.T) {
		r := twoCommitRepo(t)
		git(t, r.Root, "checkout", "-qb", "feature/unmerged")
		write(t, filepath.Join(r.Root, "extra.txt"), "unmerged commit\n")
		git(t, r.Root, "add", ".")
		git(t, r.Root, "commit", "-qm", "unmerged commit")
		git(t, r.Root, "checkout", "-q", "main")

		err := r.DeleteBranch(ctx, "feature/unmerged", false)
		if !errors.Is(err, ErrBranchNotMerged) {
			t.Fatalf("未合并分支应返回 ErrBranchNotMerged，实际：%v", err)
		}

		// 强制删除 -D 应该成功
		if err := r.DeleteBranch(ctx, "feature/unmerged", true); err != nil {
			t.Fatalf("DeleteBranch(force=true): %v", err)
		}
		if got := gitOut(t, r.Root, "branch", "--list", "feature/unmerged"); strings.TrimSpace(got) != "" {
			t.Errorf("强制删除后分支应该消失，实际：%q", got)
		}
	})

	t.Run("删除当前分支被拒", func(t *testing.T) {
		r := twoCommitRepo(t)
		err := r.DeleteBranch(ctx, "main", false)
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "cannot delete branch") {
			t.Errorf("删除当前分支应被拒：%v", err)
		}
	})
}

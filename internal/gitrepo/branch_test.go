package gitrepo

import (
	"context"
	"errors"
	"os"
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

func TestMerge(t *testing.T) {
	ctx := context.Background()

	t.Run("干净合并分支", func(t *testing.T) {
		r := twoCommitRepo(t)
		git(t, r.Root, "checkout", "-qb", "feature/add-b")
		write(t, filepath.Join(r.Root, "b.txt"), "hello b\n")
		git(t, r.Root, "add", "b.txt")
		git(t, r.Root, "commit", "-qm", "add b")

		git(t, r.Root, "checkout", "-q", "main")
		write(t, filepath.Join(r.Root, "c.txt"), "hello c\n")
		git(t, r.Root, "add", "c.txt")
		git(t, r.Root, "commit", "-qm", "add c")

		if err := r.Merge(ctx, "feature/add-b"); err != nil {
			t.Fatalf("Merge: %v", err)
		}

		// 合并后 b.txt 应该在 main 上
		data, err := os.ReadFile(filepath.Join(r.Root, "b.txt"))
		if err != nil || string(data) != "hello b\n" {
			t.Errorf("合并后 b.txt 内容异常：%v, %s", err, string(data))
		}
	})

	t.Run("冲突时返回错误并留存MERGE_HEAD", func(t *testing.T) {
		r := twoCommitRepo(t)
		git(t, r.Root, "checkout", "-qb", "feature/conflict")
		write(t, filepath.Join(r.Root, "a.txt"), "conflict branch\n")
		git(t, r.Root, "commit", "-am", "branch edit")

		git(t, r.Root, "checkout", "-q", "main")
		write(t, filepath.Join(r.Root, "a.txt"), "conflict main\n")
		git(t, r.Root, "commit", "-am", "main edit")

		err := r.Merge(ctx, "feature/conflict")
		if err == nil {
			t.Fatal("冲突合并应该返回错误")
		}
		m, err := r.Load(ctx)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if m.Pending.Kind != PendingMerge {
			t.Errorf("冲突后 Pending.Kind = %v, want PendingMerge", m.Pending.Kind)
		}
	})
}

func TestRebase(t *testing.T) {
	ctx := context.Background()

	t.Run("干净变基到指定分支", func(t *testing.T) {
		r := twoCommitRepo(t)
		git(t, r.Root, "checkout", "-qb", "feature/rebase")
		write(t, filepath.Join(r.Root, "feature.txt"), "feature data\n")
		git(t, r.Root, "add", "feature.txt")
		git(t, r.Root, "commit", "-qm", "feat commit")

		git(t, r.Root, "checkout", "-q", "main")
		write(t, filepath.Join(r.Root, "main.txt"), "main data\n")
		git(t, r.Root, "add", "main.txt")
		git(t, r.Root, "commit", "-qm", "main commit")

		git(t, r.Root, "checkout", "-q", "feature/rebase")
		if err := r.Rebase(ctx, "main"); err != nil {
			t.Fatalf("Rebase: %v", err)
		}

		// 变基后 feature 分支上应该包含 main.txt 和 feature.txt
		if _, err := os.Stat(filepath.Join(r.Root, "main.txt")); err != nil {
			t.Errorf("变基后应包含 main 的修改 main.txt: %v", err)
		}
		if _, err := os.Stat(filepath.Join(r.Root, "feature.txt")); err != nil {
			t.Errorf("变基后应包含自己的修改 feature.txt: %v", err)
		}
	})

	t.Run("冲突时返回错误并留存PendingRebase", func(t *testing.T) {
		r := twoCommitRepo(t)
		git(t, r.Root, "checkout", "-qb", "feature/rebase-conflict")
		write(t, filepath.Join(r.Root, "a.txt"), "branch conflict\n")
		git(t, r.Root, "commit", "-am", "branch edit")

		git(t, r.Root, "checkout", "-q", "main")
		write(t, filepath.Join(r.Root, "a.txt"), "main conflict\n")
		git(t, r.Root, "commit", "-am", "main edit")

		git(t, r.Root, "checkout", "-q", "feature/rebase-conflict")
		err := r.Rebase(ctx, "main")
		if err == nil {
			t.Fatal("冲突变基应该返回错误")
		}
		m, err := r.Load(ctx)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if m.Pending.Kind != PendingRebase {
			t.Errorf("冲突后 Pending.Kind = %v, want PendingRebase", m.Pending.Kind)
		}
	})
}

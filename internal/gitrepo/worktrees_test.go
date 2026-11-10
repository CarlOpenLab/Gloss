package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// worktreePath 是临时目录下一个还没创建的子目录。t.TempDir 在 macOS 上是
// /var/... 的符号链接，先解析掉再拼，和 Repo.Root 的形态一致（Open 拿
// --show-toplevel 的解析结果当根）。
func worktreePath(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("解析临时目录: %v", err)
	}
	return filepath.Join(base, "wt")
}

// Worktrees 要同时认出主工作区和 linked worktree：路径、分支、是不是
// 主工作区。IsLinkedWorktree 则从打开的那个目录自己回答这个问题。
func TestWorktreesListsMainAndLinked(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "branch", "topic")
	wt := worktreePath(t)
	if err := r.AddWorktree(ctx, wt, "topic"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	entries, err := r.Worktrees(ctx)
	if err != nil {
		t.Fatalf("Worktrees: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("该有主工作区和 linked 两项，实际 %+v", entries)
	}
	main, linked := entries[0], entries[1]
	if !main.Main || main.Path != r.Root {
		t.Errorf("第一项该是主工作区 %q 且 Main=true，实际 %+v", r.Root, main)
	}
	if main.Branch != "main" {
		t.Errorf("主工作区分支 = %q, want main", main.Branch)
	}
	if linked.Main {
		t.Errorf("linked 工作区不该标 Main: %+v", linked)
	}
	if linked.Path != wt || linked.Branch != "topic" {
		t.Errorf("linked 项 = %+v, want path=%q branch=topic", linked, wt)
	}
	if linked.Head == "" || linked.Detached {
		t.Errorf("linked 项该有 HEAD 且不是 detached: %+v", linked)
	}

	opened, err := Open(ctx, wt)
	if err != nil {
		t.Fatalf("Open(%s): %v", wt, err)
	}
	if !opened.IsLinkedWorktree() {
		t.Error("linked worktree 该被认出来")
	}
	if r.IsLinkedWorktree() {
		t.Error("主工作区不该被当成 linked worktree")
	}
}

// Refs 注解占用：linked 工作区检出的分支带上那个路径，主工作区的当前分支
// 标 Main。分支列表靠这几个字段画 WT 标记和右键菜单。
func TestRefsAnnotateWorktreeOccupancy(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "branch", "topic")
	wt := worktreePath(t)
	if err := r.AddWorktree(ctx, wt, "topic"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}

	refs, err := r.Refs(ctx)
	if err != nil {
		t.Fatalf("Refs: %v", err)
	}
	byFull := map[string]Ref{}
	for _, ref := range refs {
		byFull[ref.Full] = ref
	}

	topic, ok := byFull["refs/heads/topic"]
	if !ok {
		t.Fatal("分支列表里没有 topic")
	}
	if topic.Worktree != wt {
		t.Errorf("topic.Worktree = %q, want %q", topic.Worktree, wt)
	}
	if topic.WorktreeMain {
		t.Error("topic 不在主工作区，WorktreeMain 该为假")
	}
	if topic.WorktreePrunable {
		t.Error("正常的 linked 工作区不该标 prunable")
	}

	main, ok := byFull["refs/heads/main"]
	if !ok {
		t.Fatal("分支列表里没有 main")
	}
	if !main.WorktreeMain || main.Worktree != r.Root {
		t.Errorf("main 该标在主工作区，实际 Worktree=%q Main=%v", main.Worktree, main.WorktreeMain)
	}
}

// 移除有未提交内容的 worktree：普通移除认得出来是「脏」，强制移除才动它。
func TestRemoveWorktreeDirtyReturnsSentinel(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "branch", "topic")
	wt := worktreePath(t)
	if err := r.AddWorktree(ctx, wt, "topic"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	write(t, filepath.Join(wt, "dirty.txt"), "还没提交\n")

	err := r.RemoveWorktree(ctx, wt, false)
	if !errors.Is(err, ErrWorktreeDirty) {
		t.Fatalf("脏目录普通移除该返回 ErrWorktreeDirty，实际 %v", err)
	}
	if _, statErr := os.Stat(wt); statErr != nil {
		t.Fatalf("没强制之前目录该还在: %v", statErr)
	}

	if err := r.RemoveWorktree(ctx, wt, true); err != nil {
		t.Fatalf("强制移除: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("强制移除后目录该消失，stat err = %v", err)
	}
}

// 目录被外部删掉后记录还在：Worktrees 标 prunable，prune 之后分支重新可用。
func TestWorktreePrunableAndPrune(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	git(t, r.Root, "branch", "topic")
	wt := worktreePath(t)
	if err := r.AddWorktree(ctx, wt, "topic"); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if err := os.RemoveAll(wt); err != nil {
		t.Fatalf("删掉 worktree 目录: %v", err)
	}

	entries, err := r.Worktrees(ctx)
	if err != nil {
		t.Fatalf("Worktrees: %v", err)
	}
	var found *WorktreeEntry
	for i := range entries {
		if entries[i].Branch == "topic" {
			found = &entries[i]
		}
	}
	if found == nil {
		t.Fatalf("记录该还在（prune 之前），实际 %+v", entries)
	}
	if !found.Prunable {
		t.Errorf("目录没了该标 prunable: %+v", found)
	}

	if err := r.PruneWorktrees(ctx); err != nil {
		t.Fatalf("PruneWorktrees: %v", err)
	}
	entries, err = r.Worktrees(ctx)
	if err != nil {
		t.Fatalf("prune 后 Worktrees: %v", err)
	}
	if len(entries) != 1 || !entries[0].Main {
		t.Fatalf("prune 后只剩主工作区，实际 %+v", entries)
	}
	// 记录清掉之后 git 才肯在主工作区检出 topic。
	git(t, r.Root, "checkout", "-q", "topic")
}

// 错误译文：脏 worktree 带机器可读的 Code（移除失败要靠它弹强制确认），
// 分支被占用和兜底各说各的。
func TestDescribeWorktreeFailures(t *testing.T) {
	f := Describe(OpWorktree, ErrWorktreeDirty)
	if f.Code != FailureCodeWorktreeDirty {
		t.Errorf("Code = %v, want FailureCodeWorktreeDirty", f.Code)
	}
	if f.Title == "" || f.Hint == "" {
		t.Errorf("脏 worktree 该有标题和出路，实际 %+v", f)
	}

	f = Describe(OpCheckout, errors.New("fatal: 'topic' is already used by worktree at '/x/wt'"))
	if f.Title != "这条分支已经在别的工作区检出" {
		t.Errorf("占用检出标题 = %q", f.Title)
	}
	if f.Code != FailureCodeNone {
		t.Errorf("普通占用不该带 Code，实际 %v", f.Code)
	}

	f = Describe(OpWorktree, errors.New("fatal: something we have never seen"))
	if f.Title != "Worktree 操作没成功" {
		t.Errorf("兜底标题 = %q", f.Title)
	}
}

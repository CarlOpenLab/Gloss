package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 储藏与修改上次提交在 gitrepo 层的测试。

func TestStashPushApplyPopDrop(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	// 1. 测试空仓库修改时储藏：干净工作区报错
	err := r.StashPush(ctx, "nothing", false)
	if err == nil {
		t.Fatal("干净工作区执行 StashPush 应该报错")
	}

	// 2. 修改文件并创建未跟踪文件
	write(t, filepath.Join(r.Root, "a.txt"), "modified for stash\n")
	write(t, filepath.Join(r.Root, "untracked.txt"), "untracked file\n")

	// 储藏（包含未跟踪文件）
	if err := r.StashPush(ctx, "测试储藏", true); err != nil {
		t.Fatalf("StashPush: %v", err)
	}

	// 储藏后工作区应该变干净
	if got, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(got) != "one\ntwo\nthree\n" {
		t.Errorf("储藏后 a.txt 应恢复原样，实际：%q", string(got))
	}
	if _, err := os.Stat(filepath.Join(r.Root, "untracked.txt")); err == nil {
		t.Error("储藏后未跟踪文件 untracked.txt 应该被收进储藏")
	}

	// 储藏列表应包含此条目
	refs, err := r.Refs(ctx)
	if err != nil {
		t.Fatalf("Refs: %v", err)
	}
	var found bool
	for _, ref := range refs {
		if ref.Kind == RefStash {
			found = true
			if !strings.Contains(ref.Subject, "测试储藏") {
				t.Errorf("储藏 Subject = %q, want 含 '测试储藏'", ref.Subject)
			}
		}
	}
	if !found {
		t.Fatal("未在 Refs 中找到储藏条目")
	}

	// 3. 测试 StashApply：改动被恢复，但储藏仍在
	if err := r.StashApply(ctx, "stash@{0}"); err != nil {
		t.Fatalf("StashApply: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(got) != "modified for stash\n" {
		t.Errorf("Apply 后 a.txt = %q, want 'modified for stash\\n'", string(got))
	}
	if _, err := os.Stat(filepath.Join(r.Root, "untracked.txt")); err != nil {
		t.Errorf("Apply 后 untracked.txt 应该恢复: %v", err)
	}

	// 清空工作区重新测试 StashPop
	git(t, r.Root, "checkout", "--", "a.txt")
	_ = os.Remove(filepath.Join(r.Root, "untracked.txt"))

	// 4. 测试 StashPop：恢复改动且丢弃该储藏
	if err := r.StashPop(ctx, "stash@{0}"); err != nil {
		t.Fatalf("StashPop: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(r.Root, "a.txt")); string(got) != "modified for stash\n" {
		t.Errorf("Pop 后 a.txt = %q", string(got))
	}

	// 5. 再次保存并测试 StashDrop
	if err := r.StashPush(ctx, "待丢弃的储藏", true); err != nil {
		t.Fatalf("StashPush 2: %v", err)
	}
	if err := r.StashDrop(ctx, "stash@{0}"); err != nil {
		t.Fatalf("StashDrop: %v", err)
	}

	refsAfter, err := r.Refs(ctx)
	if err != nil {
		t.Fatalf("Refs: %v", err)
	}
	for _, ref := range refsAfter {
		if ref.Kind == RefStash {
			t.Errorf("Drop 之后不该有储藏，实际存在: %+v", ref)
		}
	}
}

func TestCommitAmendAndLastCommitMessage(t *testing.T) {
	r := twoCommitRepo(t)
	ctx := context.Background()

	// 验证 HasHead
	if !r.HasHead(ctx) {
		t.Fatal("有提交的仓库 HasHead 应该为 true")
	}

	msg, err := r.LastCommitMessage(ctx)
	if err != nil {
		t.Fatalf("LastCommitMessage: %v", err)
	}
	if msg != "second" {
		t.Errorf("LastCommitMessage = %q, want 'second'", msg)
	}

	// 暂存新文件并修改上次提交
	write(t, filepath.Join(r.Root, "amend_file.txt"), "amended content\n")
	if err := r.StageFile(ctx, "amend_file.txt"); err != nil {
		t.Fatalf("StageFile: %v", err)
	}

	if err := r.CommitAmend(ctx, "amended second commit"); err != nil {
		t.Fatalf("CommitAmend: %v", err)
	}

	newMsg, err := r.LastCommitMessage(ctx)
	if err != nil {
		t.Fatalf("LastCommitMessage after amend: %v", err)
	}
	if newMsg != "amended second commit" {
		t.Errorf("amend 后的提交信息 = %q, want 'amended second commit'", newMsg)
	}

	// 提交总数应该仍然是 2
	revList := strings.TrimSpace(gitOut(t, r.Root, "rev-list", "--count", "HEAD"))
	if revList != "2" {
		t.Errorf("提交总数 = %q, want 2", revList)
	}
}

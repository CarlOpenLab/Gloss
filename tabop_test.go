package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 写闸门按项目算：A 页有操作在飞，切到 B 页之后 B 页照样能写。
//
// 提交是最好观察的一处：它可能卡上几十秒（钩子、签名、大仓库），用户等
// 不及切去另一个项目干活，那一页不该跟着灰。A 页那次操作的结果也留在 A
// 页，切回来才落收尾（重读、清提交框）。
func TestWriteGateIsPerTab(t *testing.T) {
	dirA := newRepoDir(t)
	writeFile(t, filepath.Join(dirA, "a.txt"), "a\n")
	runGit(t, dirA, "add", ".")
	runGit(t, dirA, "commit", "-m", "init")

	a := loadApp(t, dirA)
	// A 页已经载入；把它存成标签页快照，再挂一个 B 页。
	a.tabs = []*repoTab{{path: a.repoPath, name: a.repoName, state: loadReady, branch: a.branch}}
	a.activeTab = 0
	a.saveActiveTab()

	dirB := newRepoDir(t)
	writeFile(t, filepath.Join(dirB, "b.txt"), "b\n")
	runGit(t, dirB, "add", ".")
	runGit(t, dirB, "commit", "-m", "init")
	a.tabs = append(a.tabs, &repoTab{path: dirB, name: filepath.Base(dirB), state: loadReady})

	release := make(chan struct{})
	running := make(chan struct{})
	doneN, doneRepo := 0, ""
	if !a.startOp(gitrepo.OpCommit, "正在提交…", func(context.Context, *gitrepo.Repo) (string, error) {
		close(running)
		<-release
		return "已提交", nil
	}, func(bool) {
		doneN++
		doneRepo = a.repoPath
	}) {
		t.Fatal("A 页该能开始提交")
	}
	select {
	case <-running:
	case <-time.After(5 * time.Second):
		t.Fatal("提交没跑起来")
	}

	a.switchTab(1)

	if a.op != gitrepo.OpNone {
		t.Fatalf("切到 B 页后写闸门仍是 %v，B 页被 A 页的提交挡住了", a.op)
	}
	if !a.startOp(gitrepo.OpStage, "正在暂存", func(context.Context, *gitrepo.Repo) (string, error) {
		return "", nil
	}, nil) {
		t.Fatal("B 页的写操作该放行")
	}

	close(release)
	settle(t, a)

	if doneN != 0 {
		t.Fatalf("A 页的收尾跑在了 B 页上（repo=%q），该等切回 A 页再跑", doneRepo)
	}
	if a.op != gitrepo.OpNone {
		t.Fatalf("B 页自己的操作结束后写闸门是 %v", a.op)
	}

	a.switchTab(0)
	settle(t, a)

	if doneN != 1 {
		t.Fatalf("切回 A 页应收尾一次，实际 %d 次", doneN)
	}
	if doneRepo != filepath.Clean(dirA) {
		t.Errorf("收尾时看到的仓库是 %q，该是 %q", doneRepo, filepath.Clean(dirA))
	}
	if a.op != gitrepo.OpNone {
		t.Errorf("A 页的操作早结束了，闸门却还是 %v", a.op)
	}
}

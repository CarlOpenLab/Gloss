package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 侧栏本地分支行尾巴上那截「和上游差几个提交」。
func TestTrackLabel(t *testing.T) {
	cases := []struct {
		name string
		ref  gitrepo.Ref
		want string
	}{
		{"没有上游不画", gitrepo.Ref{Kind: gitrepo.RefLocal, Name: "x"}, ""},
		{"同步不画", gitrepo.Ref{Kind: gitrepo.RefLocal, Upstream: "origin/x"}, ""},
		{"只超前", gitrepo.Ref{Kind: gitrepo.RefLocal, Upstream: "origin/x", Ahead: 2}, "↑2"},
		{"只落后", gitrepo.Ref{Kind: gitrepo.RefLocal, Upstream: "origin/x", Behind: 1}, "↓1"},
		{"两边都有", gitrepo.Ref{Kind: gitrepo.RefLocal, Upstream: "origin/x", Ahead: 3, Behind: 4}, "↑3 ↓4"},
		{"上游没了", gitrepo.Ref{Kind: gitrepo.RefLocal, Upstream: "origin/x", UpstreamGone: true}, "上游已删除"},
		// 远程分支、标签、储藏都没有上游可言 —— 计数只属于本地分支
		{"远程分支不画", gitrepo.Ref{Kind: gitrepo.RefRemote, Upstream: "origin/x", Ahead: 2}, ""},
		{"标签不画", gitrepo.Ref{Kind: gitrepo.RefTag, Upstream: "origin/x", Ahead: 2}, ""},
		{"储藏不画", gitrepo.Ref{Kind: gitrepo.RefStash, Upstream: "origin/x", Ahead: 2}, ""},
	}
	for _, c := range cases {
		if got := trackLabel(c.ref); got != c.want {
			t.Errorf("%s：trackLabel = %q, want %q", c.name, got, c.want)
		}
	}
}

// 从「所有分支」点回一条具体分支：那一屏作废，回到那条分支的历史。
//
// 两个状态互斥（histAll 开着时 histRev 是空串），漏清任何一个都会让
// loadCommits 读错东西 —— 要么还在看全部，要么看的是一条空引用。
func TestSelectRefLeavesAllBranches(t *testing.T) {
	a := histApp(t)
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	a.setTab(1)
	settle(t, a)

	a.selectAllRefs()
	settle(t, a)
	if !a.histAll {
		t.Fatal("点了「所有分支」之后 histAll 该是 true")
	}

	ref, ok := a.refByFull["refs/heads/main"]
	if !ok {
		t.Fatal("夹具里没有 main")
	}
	a.selectRef(refLeaf+ref.Full, *ref)
	settle(t, a)

	if a.histAll {
		t.Error("点了具体分支之后 histAll 该作废")
	}
	if a.histRev != "refs/heads/main" {
		t.Errorf("histRev = %q, want refs/heads/main", a.histRev)
	}
	// 「所有分支」那一行不再处于选中态：选中跟着 histAll 走，它现在是
	// false，而 refSel 指向 main。
	if a.refSel != refLeaf+"refs/heads/main" {
		t.Errorf("refSel = %q, want 本地分支那一行", a.refSel)
	}
}

// 上游已经被远程删掉的分支，行尾那截字用危险色（红），不是压暗。
//
// 那是一件得处理的事 —— 推上去会新建一条远程分支，而不是更新原来那条。
// 压暗成和其它计数一样，等于让人看一眼就过去。
func TestGoneUpstreamIsRed(t *testing.T) {
	a, _ := remoteApp(t)
	dir := a.repoPath
	runGit(t, dir, "checkout", "-q", "-b", "topic")
	commitIn(t, dir, "t.txt", "t\n", "topic commit")
	runGit(t, dir, "push", "-q", "-u", "origin", "topic")
	runGit(t, dir, "checkout", "-q", "main")
	// 远程上删掉它，再 prune —— 本地 topic 的上游就此作废
	runGit(t, dir, "push", "-q", "origin", "--delete", "topic")
	runGit(t, dir, "fetch", "-q", "--prune", "origin")

	a.reload()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()

	if !tt.HasText(T("上游已删除", "upstream gone")) {
		t.Fatalf("侧栏没说上游已删除：%q", tt.Texts())
	}
	box, ok := tt.Find(T("上游已删除", "upstream gone"))
	if !ok {
		t.Fatal("找不到那一行字")
	}
	if !textIsRed(tt, box) {
		t.Errorf("上游已删除该是红色，实际取到的像素都不是红的")
	}

	// 计数照旧是压暗的灰，不跟着变红 —— 一列里只有「出了事」的那个显眼
	runGit(t, dir, "update-ref", "refs/remotes/origin/topic", "HEAD")
	runGit(t, dir, "branch", "--set-upstream-to=origin/topic", "topic")
	a.reload()
	settle(t, a)
	tt.Frame()
	if !tt.HasText("↑1") {
		t.Fatalf("设回上游之后该显示超前计数：%q", tt.Texts())
	}
	box, ok = tt.Find("↑1")
	if !ok {
		t.Fatal("找不到 ↑1")
	}
	if textIsRed(tt, box) {
		t.Error("普通计数不该是红的")
	}
}

// textIsRed 在这一行字的框里横向扫一遍，看有没有一个红压倒性高的像素。
//
// 不能只取一个点：抗锯齿让多数字素停在灰阶上，而字与字之间全是背景色。
func textIsRed(tt *ui.Tester, box ui.Rect) bool {
	y := int(box.Y + box.H/2)
	for x := int(box.X); x < int(box.X+box.W); x++ {
		p := pixelAt(tt, x, y)
		if p[0] > 150 && p[1] < 120 && p[2] < 120 {
			return true
		}
	}
	return false
}

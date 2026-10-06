package gitrepo

import (
	"context"
	"path/filepath"
	"testing"

	"diff-explainer/internal/diff"
)

// 建一个带远程的仓库：本地 main + feature，一个标签，一个储藏，
// 远程 origin 上也有分支，还会生成 refs/remotes/origin/HEAD 这个符号引用。
func refRepo(t *testing.T) *Repo {
	t.Helper()
	r := tempRepo(t)
	dir := r.Root

	// 远程：一个裸仓库，推上去，再让 origin/HEAD 指到 main。
	// set-head 是唯一能稳定造出 refs/remotes/origin/HEAD 的办法 ——
	// clone 出来的仓库自带它，这里手工造。
	bare := filepath.Join(t.TempDir(), "origin.git")
	git(t, dir, "init", "--bare", "-b", "main", bare)
	git(t, dir, "remote", "add", "origin", bare)
	git(t, dir, "push", "-q", "-u", "origin", "main")
	git(t, dir, "remote", "set-head", "origin", "main")

	git(t, dir, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(dir, "b.txt"), "feature\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "支线提交")

	git(t, dir, "checkout", "-q", "main")
	git(t, dir, "tag", "v1.0")

	// 储藏：改一个已跟踪文件再 stash
	write(t, filepath.Join(dir, "a.txt"), "one\ntwo\nthree\nSTASHED\n")
	git(t, dir, "stash", "push", "-q", "-m", "半成品")

	return r
}

func TestRefsGroups(t *testing.T) {
	r := refRepo(t)
	refs, err := r.Refs(context.Background())
	if err != nil {
		t.Fatalf("Refs: %v", err)
	}

	byName := map[string]Ref{}
	for _, ref := range refs {
		byName[ref.Name] = ref
	}

	main, ok := byName["main"]
	if !ok || main.Kind != RefLocal {
		t.Fatalf("没读到本地分支 main：%+v", refs)
	}
	if main.Full != "refs/heads/main" {
		t.Errorf("Full = %q, want refs/heads/main —— 喂给 Log 的是它", main.Full)
	}
	if !main.Head {
		t.Error("main 是当前分支，Head 该是 true")
	}
	if len(main.Hash) != 7 {
		t.Errorf("短哈希不对：%q", main.Hash)
	}
	if main.When.IsZero() {
		t.Error("提交时间没解析出来")
	}

	if f, ok := byName["feature"]; !ok || f.Kind != RefLocal {
		t.Errorf("没读到本地分支 feature：%+v", refs)
	} else if f.Head {
		t.Error("feature 不是当前分支，Head 该是 false")
	}

	if tag, ok := byName["v1.0"]; !ok || tag.Kind != RefTag {
		t.Errorf("没读到标签 v1.0：%+v", refs)
	} else if tag.Full != "refs/tags/v1.0" {
		t.Errorf("标签的 Full = %q", tag.Full)
	}

	remote, ok := byName["origin/main"]
	if !ok || remote.Kind != RefRemote {
		t.Fatalf("没读到远程分支 origin/main：%+v", refs)
	}
	if remote.Remote != "origin" {
		t.Errorf("Remote = %q, want origin", remote.Remote)
	}
	// 叶子上显示的是去掉 remote 前缀的名字
	if got := remote.Label(); got != "main" {
		t.Errorf("远程分支的 Label = %q, want main", got)
	}
	if got := byName["main"].Label(); got != "main" {
		t.Errorf("本地分支的 Label 不该被改：%q", got)
	}

	stash, ok := byName["stash@{0}"]
	if !ok || stash.Kind != RefStash {
		t.Fatalf("没读到储藏：%+v", refs)
	}
	// 储藏提交的主题自带 "On <分支>: " 前缀。不削掉它 —— 一眼能看出这个
	// 储藏是在哪条分支上攒的，比单看一句信息有用。
	if stash.Subject != "On main: 半成品" {
		t.Errorf("储藏的信息 = %q, want \"On main: 半成品\"", stash.Subject)
	}
}

// refs/remotes/origin/HEAD 是符号引用，不是一条真的分支线。列出来会多一条
// 和 main 重复的条目 —— 而且它指向 main，画到拓扑图上两条线会撞在一起。
func TestRefsSkipsSymbolicRemoteHead(t *testing.T) {
	r := refRepo(t)
	refs, err := r.Refs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if ref.Name == "origin/HEAD" {
			t.Fatalf("origin/HEAD 是符号引用，不该列出来：%+v", refs)
		}
	}
}

// 空仓库（一次提交都没有）没有 refs，不该报错。
func TestRefsEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	git(t, dir, "init", "-b", "main")

	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := r.Refs(context.Background())
	if err != nil {
		t.Fatalf("空仓库的 Refs 不该报错：%v", err)
	}
	if len(refs) != 0 {
		t.Errorf("空仓库不该有 refs，got %+v", refs)
	}
}

// 分支和标签同名时，短名会喂给 git 的消歧规则，结果是标签赢 —— 点分支
// 看到标签的历史，而且不报错。所以侧栏点一项必须拿 Ref.Full 去查。
func TestRefsFullNameDisambiguates(t *testing.T) {
	r := refRepo(t)
	dir := r.Root
	ctx := context.Background()

	// main 上打个标签 dup，再开一条同名分支 dup 并多提交一次
	git(t, dir, "tag", "dup")
	git(t, dir, "checkout", "-q", "-b", "dup")
	write(t, filepath.Join(dir, "c.txt"), "dup\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "分支上的提交")
	git(t, dir, "checkout", "-q", "main")

	refs, err := r.Refs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var branch, tag Ref
	for _, ref := range refs {
		switch {
		case ref.Kind == RefLocal && ref.Name == "dup":
			branch = ref
		case ref.Kind == RefTag && ref.Name == "dup":
			tag = ref
		}
	}
	if branch.Full == "" || tag.Full == "" {
		t.Fatalf("同名的分支和标签都该在：%+v", refs)
	}

	// 先把危险本身钉住：短名 "dup" 被 git 解析成了标签。
	short, err := r.Log(ctx, "dup", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(short) == 0 || short[0].Subject != "init" {
		t.Errorf("前提变了：短名 dup 现在解析到 %v，不再是标签那条", subjects(short))
	}

	// Full 各查各的
	got, err := r.Log(ctx, branch.Full, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Subject != "分支上的提交" {
		t.Errorf("refs/heads/dup 的历史 = %v，want 从「分支上的提交」开始", subjects(got))
	}

	got, err = r.Log(ctx, tag.Full, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0].Subject != "init" {
		t.Errorf("refs/tags/dup 的历史 = %v，want 从 init 开始", subjects(got))
	}
}

// 没有储藏时 git stash list 是空输出 + 退出码 0，不该被当成错误。
func TestRefsNoStash(t *testing.T) {
	r := tempRepo(t)
	refs, err := r.Refs(context.Background())
	if err != nil {
		t.Fatalf("没有储藏不该报错：%v", err)
	}
	for _, ref := range refs {
		if ref.Kind == RefStash {
			t.Errorf("不该有储藏：%+v", ref)
		}
	}
}

// Log 传 rev 就只看那条引用的历史 —— 侧栏点分支全靠它。
func TestLogOnRev(t *testing.T) {
	r := refRepo(t)
	ctx := context.Background()

	// main 停在 init 上（feature 那条提交没合回来）
	head, err := r.Log(ctx, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(head) != 1 || head[0].Subject != "init" {
		t.Fatalf("HEAD 的历史 = %v, want [init]", subjects(head))
	}

	feat, err := r.Log(ctx, "feature", 100, 0)
	if err != nil {
		t.Fatalf("Log(feature): %v", err)
	}
	if len(feat) != 2 {
		t.Fatalf("feature 的历史 = %d 条，want 2：%v", len(feat), subjects(feat))
	}
	if feat[0].Subject != "支线提交" {
		t.Errorf("feature 最近的提交 = %q", feat[0].Subject)
	}

	// 标签当 rev 用，等价于它指向的提交
	tagged, err := r.Log(ctx, "v1.0", 100, 0)
	if err != nil {
		t.Fatalf("Log(v1.0): %v", err)
	}
	if len(tagged) != 1 || tagged[0].Subject != "init" {
		t.Errorf("v1.0 的历史 = %v, want [init]", subjects(tagged))
	}

	// 不存在的分支要返回「没有历史」而不是报错，也不能退回 HEAD ——
	// 退回去的话点了分支看着像没反应。
	none, err := r.Log(ctx, "no-such-branch", 100, 0)
	if err != nil {
		t.Fatalf("不存在的引用不该报错：%v", err)
	}
	if len(none) != 0 {
		t.Errorf("不存在的引用该是空历史，got %v", subjects(none))
	}
}

// 储藏的改动读得出来。
//
// 这一步计划里标了风险：列储藏和读储藏的改动是两码事，后者走的是
// git stash show -p，产物未必和 FilesFromDiff 对得上（少了 hunk 头、
// 或者路径前缀不对，diff 视图就是一片空白）。所以拿真储藏验一遍，
// 而不是想当然。
func TestStashDiff(t *testing.T) {
	r := tempRepo(t)
	dir := r.Root

	// 已跟踪的文件改一下
	write(t, filepath.Join(dir, "a.txt"), "one\ntwo\nthree\n塞进去的一行\n")
	// 再来一个未跟踪的 —— 不带 --include-untracked 的话它不会出现
	write(t, filepath.Join(dir, "new.txt"), "未跟踪的内容\n")
	git(t, dir, "stash", "push", "-q", "-u", "-m", "半成品")

	files, err := r.StashDiff(context.Background(), "stash@{0}")
	if err != nil {
		t.Fatalf("StashDiff: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("读到 %d 个文件，want 2（改过的 + 新加的）：%+v", len(files), files)
	}

	byPath := map[string]File{}
	for _, f := range files {
		byPath[f.NewPath] = f
	}

	mod, ok := byPath["a.txt"]
	if !ok {
		t.Fatalf("没有 a.txt：%+v", files)
	}
	if len(mod.Hunks) != 1 {
		t.Fatalf("a.txt 有 %d 段改动，want 1", len(mod.Hunks))
	}
	h := mod.Hunks[0]
	if h.Header == "" {
		t.Error("a.txt 的 hunk 没有头 —— diff 视图画不出 @@ 那行")
	}
	// 那段里得看得到塞进去的那行
	found := false
	for _, ln := range h.Lines {
		if ln.Content == "塞进去的一行" && ln.Type == diff.LineAdd {
			found = true
		}
	}
	if !found {
		t.Errorf("改动内容不在 hunk 里：%+v", h.Lines)
	}
	if mod.AddCount != 1 {
		t.Errorf("a.txt 新增 %d 行，want 1", mod.AddCount)
	}
	if mod.Status != StatusModified {
		t.Errorf("a.txt 状态 = %v, want modified", mod.Status)
	}

	// 未跟踪的那个文件也得在，且算新增
	added, ok := byPath["new.txt"]
	if !ok {
		t.Fatalf("没读到未跟踪的 new.txt：%+v", files)
	}
	if added.Status != StatusAdded {
		t.Errorf("new.txt 状态 = %v, want added", added.Status)
	}
}

// 储藏的名字不存在时给个错误，不能把整个界面拖垮。
func TestStashDiffMissing(t *testing.T) {
	r := tempRepo(t)
	if _, err := r.StashDiff(context.Background(), "stash@{7}"); err == nil {
		t.Error("不存在的储藏该报错")
	}
}

package gitrepo

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 提交信息里带分隔符不能把解析搞乱 —— 这正是用 \x00/\x1e 而不是逗号竖线的原因。
func TestLogParsesFields(t *testing.T) {
	r := tempRepo(t)
	write(t, filepath.Join(r.Root, "a.txt"), "one\ntwo\nthree\nfour\n")
	git(t, r.Root, "commit", "-am", "a: 中文 subject, with commas | and 竖线")

	commits, err := r.Log(context.Background(), "", 10, 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("提交数 = %d, want 2", len(commits))
	}
	// 最近的排前面
	c := commits[0]
	if c.Subject != "a: 中文 subject, with commas | and 竖线" {
		t.Errorf("subject = %q", c.Subject)
	}
	if c.Author != "Test" {
		t.Errorf("author = %q", c.Author)
	}
	if len(c.Hash) != 40 || c.Short != c.Hash[:7] {
		t.Errorf("hash = %q short = %q", c.Hash, c.Short)
	}
	if c.When.IsZero() {
		t.Error("时间没解析出来")
	}
	if commits[1].Subject != "init" {
		t.Errorf("第二个提交 = %q, want init", commits[1].Subject)
	}
}

// skip/limit 用来翻页。
func TestLogPaging(t *testing.T) {
	r := tempRepo(t)
	for _, s := range []string{"2", "3", "4"} {
		write(t, filepath.Join(r.Root, "a.txt"), "x\n"+s+"\n")
		git(t, r.Root, "commit", "-am", "c"+s)
	}

	page1, err := r.Log(context.Background(), "", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	page2, err := r.Log(context.Background(), "", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 || len(page2) != 2 {
		t.Fatalf("每页条数不对：%d, %d", len(page1), len(page2))
	}
	if page1[0].Subject != "c4" || page1[1].Subject != "c3" {
		t.Errorf("第一页 = %q", subjects(page1))
	}
	if page2[0].Subject != "c2" || page2[1].Subject != "init" {
		t.Errorf("第二页 = %q", subjects(page2))
	}
}

func subjects(cs []Commit) []string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Subject)
	}
	return out
}

// 父提交要读出来 —— 提交拓扑图（泳道图）全靠它。
func TestLogReadsParents(t *testing.T) {
	r := tempRepo(t)
	write(t, filepath.Join(r.Root, "a.txt"), "one\ntwo\nthree\nCHANGED\n")
	git(t, r.Root, "commit", "-am", "second")

	commits, err := r.Log(context.Background(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 {
		t.Fatalf("提交数 = %d, want 2", len(commits))
	}
	if len(commits[0].Parents) != 1 || commits[0].Parents[0] != commits[1].Hash {
		t.Errorf("第二个提交的父应该是第一个：%v vs %v", commits[0].Parents, commits[1].Hash)
	}
	// 根提交没有父 —— 画图时这条泳道到此为止
	if len(commits[1].Parents) != 0 {
		t.Errorf("根提交不该有父：%v", commits[1].Parents)
	}
}

// 合并提交有两个父，顺序是 git 自己定的（第一个是「当前分支」那一侧）。
func TestLogMergeCommitParents(t *testing.T) {
	r := tempRepo(t)
	base := head(t, r.Root)

	git(t, r.Root, "checkout", "-q", "-b", "feature")
	write(t, filepath.Join(r.Root, "f.txt"), "feature\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "支线")
	feature := head(t, r.Root)

	git(t, r.Root, "checkout", "-q", "main")
	write(t, filepath.Join(r.Root, "m.txt"), "main\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "主线")
	mainSide := head(t, r.Root)

	git(t, r.Root, "merge", "-q", "--no-ff", "-m", "合并支线", "feature")

	commits, err := r.Log(context.Background(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) == 0 || commits[0].Subject != "合并支线" {
		t.Fatalf("第一个应该是合并提交：%v", subjects(commits))
	}
	got := commits[0].Parents
	if len(got) != 2 {
		t.Fatalf("合并提交应该有两个父：%v", got)
	}
	// merge 时人在 main 上，所以第一个父是 main 那一侧
	if got[0] != mainSide || got[1] != feature {
		t.Errorf("父的顺序不对：%v（main=%v feature=%v）", got, mainSide, feature)
	}
	if base == "" {
		t.Error("没拿到基线提交")
	}
}

// --topo-order：父提交永远排在子提交后面 —— 哪怕父提交的日期比子提交还新。
//
//	init(2099) ─┬─ X(2020) ─┐
//	            └─ Y(2010) ─┴─ M(2030)   ← main 在这
//
// 默认的 git log 按提交时间倒序排。走到 M 时它把两个父都放进队列，
// 先弹出 X(2020)，于是 init(2099) 进了队列 —— 它比还在队列里等着的
// Y(2010) 新，就先被弹出来了。可 init 是 Y 的父提交，排在 Y 前面就成了
// 逆序。日期被改过、从别处 fetch 过、rebase 过的仓库都会撞上这种情况。
//
// 所以这条测试必须把日期真的造拧：光在分支上提交几个再合并，时间序碰巧
// 也是拓扑序，测不出 --topo-order 有没有生效。
func TestLogIsTopological(t *testing.T) {
	r := tempRepo(t)

	// 根提交的日期挪到未来 —— 它是最老的提交，却最「新」
	future := "2099-01-01T00:00:00+00:00"
	gitDated(t, r.Root, future, "commit", "--amend", "--no-edit", "--date="+future)

	// 两条兄弟支线，日期一旧一新
	git(t, r.Root, "branch", "y")
	write(t, filepath.Join(r.Root, "x.txt"), "x\n")
	git(t, r.Root, "add", ".")
	gitDated(t, r.Root, "2020-01-01T00:00:00+00:00", "commit", "-m", "X")

	git(t, r.Root, "checkout", "-q", "y")
	write(t, filepath.Join(r.Root, "y.txt"), "y\n")
	git(t, r.Root, "add", ".")
	gitDated(t, r.Root, "2010-01-01T00:00:00+00:00", "commit", "-m", "Y")

	git(t, r.Root, "checkout", "-q", "main")
	gitDated(t, r.Root, "2030-01-01T00:00:00+00:00", "merge", "-q", "--no-ff", "-m", "合并", "y")

	commits, err := r.Log(context.Background(), "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 4 {
		t.Fatalf("提交数 = %d, want 4（%v）", len(commits), subjects(commits))
	}

	// 前提先立住：日期得真的是拧的，否则这条测试又成了一句废话 ——
	// 时间序碰巧等于拓扑序，去掉 --topo-order 也一样过。
	// 找根提交不能靠「排在最后一个」，那本身就是拓扑序的性质，顺序一错
	// 就找错了人，报出来的错也会驴唇不对马嘴。
	root := -1
	for i, c := range commits {
		if len(c.Parents) == 0 {
			root = i
		}
	}
	if root < 0 {
		t.Fatal("没找到根提交")
	}
	for _, c := range commits {
		if c.When.After(commits[root].When) {
			t.Fatalf("日期没造拧：根提交 %v 不是最新的（%v 比它新）",
				commits[root].When, c.When)
		}
	}

	pos := map[string]int{}
	for i, c := range commits {
		pos[c.Hash] = i
	}
	for _, c := range commits {
		for _, p := range c.Parents {
			pi, ok := pos[p]
			if !ok {
				continue // 超出这一页了，不管
			}
			if pi <= pos[c.Hash] {
				// 父必须排在子后面（下标更大）
				t.Errorf("%s（第 %d 个）的父 %s 排在第 %d 个 —— 拓扑序反了",
					c.Short, pos[c.Hash], p[:7], pi)
			}
		}
	}
}

// gitDated 在指定时刻跑一条 git 命令：作者时间和提交时间都定死。
//
// 注意 --amend 不吃 GIT_AUTHOR_DATE（它会原样保留原来的作者时间），那种
// 情况得自己补一个 --date=。merge 反过来，它不认 --date，只认环境变量。
func gitDated(t *testing.T, dir, when string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(gitEnv(), "GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// head 读当前 HEAD 的完整哈希。
func head(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// 空仓库没有提交，Log 应该是空列表而不是错误。
func TestLogEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := r.Log(context.Background(), "", 10, 0)
	if err != nil {
		t.Fatalf("空仓库的 Log 不该报错：%v", err)
	}
	if len(commits) != 0 {
		t.Errorf("空仓库应该没有提交，got %d", len(commits))
	}
}

// 按提交看 diff：产物要和 Load 一样能直接喂给界面。
func TestCommitDiff(t *testing.T) {
	r := tempRepo(t)
	write(t, filepath.Join(r.Root, "a.txt"), "one\nCHANGED\nthree\n")
	write(t, filepath.Join(r.Root, "b.txt"), "new file\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "rm", "--quiet", "-f", "a.txt") // 顺带删掉一个
	write(t, filepath.Join(r.Root, "a.txt"), "one\nCHANGED\nthree\n")
	git(t, r.Root, "add", "a.txt")
	git(t, r.Root, "commit", "-m", "second")

	commits, err := r.Log(context.Background(), "", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	files, err := r.CommitDiff(context.Background(), commits[0].Hash)
	if err != nil {
		t.Fatalf("CommitDiff: %v", err)
	}

	byPath := map[string]File{}
	for _, f := range files {
		byPath[f.Path()] = f
	}
	b, ok := byPath["b.txt"]
	if !ok || b.Status != StatusAdded {
		t.Errorf("b.txt 应该是新增，got %+v", b.Status)
	}
	if !ok || b.AddCount != 1 || b.DelCount != 0 {
		t.Errorf("b.txt 的增删数不对：+%d -%d", b.AddCount, b.DelCount)
	}
	if a, ok := byPath["a.txt"]; ok && a.Status != StatusModified {
		t.Errorf("a.txt 应该是修改，got %v", a.Status)
	}
	// 每个 hunk 都要有 ID，界面靠它当列表 key，解释缓存也靠它
	for _, f := range files {
		for _, h := range f.Hunks {
			if h.ID == "" {
				t.Errorf("%s 的 hunk 没有 ID", f.Path())
			}
		}
	}
}

// 根提交（没有父提交）也要能看 diff，不能报错也不能空手而归。
func TestCommitDiffOnRootCommit(t *testing.T) {
	r := tempRepo(t)
	commits, err := r.Log(context.Background(), "", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	files, err := r.CommitDiff(context.Background(), commits[0].Hash)
	if err != nil {
		t.Fatalf("根提交的 CommitDiff 失败：%v", err)
	}
	if len(files) != 1 || files[0].Path() != "a.txt" {
		t.Fatalf("根提交应该显示 a.txt 是新增：%+v", files)
	}
	if files[0].Status != StatusAdded {
		t.Errorf("根提交里的文件应该是新增，got %v", files[0].Status)
	}
	if len(files[0].Hunks) == 0 {
		t.Error("根提交的 diff 应该带 hunk")
	}
}

// 重命名在历史里也要认出来（-M 生效）。
func TestCommitDiffDetectsRename(t *testing.T) {
	r := tempRepo(t)
	// 内容够长才容易被判定成重命名而不是删一个加一个
	long := strings.Repeat("line of text\n", 40)
	write(t, filepath.Join(r.Root, "old.txt"), long)
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "add long")
	git(t, r.Root, "mv", "old.txt", "new.txt")
	git(t, r.Root, "commit", "-m", "rename")

	commits, _ := r.Log(context.Background(), "", 1, 0)
	files, err := r.CommitDiff(context.Background(), commits[0].Hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("应该只有一个文件：%+v", files)
	}
	if files[0].Status != StatusRenamed {
		t.Errorf("状态 = %v, want renamed", files[0].Status)
	}
	if files[0].OldPath != "old.txt" || files[0].NewPath != "new.txt" {
		t.Errorf("路径不对：%q → %q", files[0].OldPath, files[0].NewPath)
	}
	// 单纯重命名没有内容改动，界面据此显示「仅重命名」
	if len(files[0].Hunks) != 0 {
		t.Errorf("纯重命名不该有 hunk：%+v", files[0].Hunks)
	}
}

// 对比两个分支的改动与提交列表（BranchDiff、BranchCommits）。
func TestBranchDiffAndCommits(t *testing.T) {
	r := tempRepo(t)
	// base 提交
	write(t, filepath.Join(r.Root, "common.txt"), "base\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "init")

	// 分支 feature
	git(t, r.Root, "branch", "feature")

	// main 上加一个提交
	write(t, filepath.Join(r.Root, "main_only.txt"), "hello main\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "commit on main")

	// 切到 feature，加两个提交
	git(t, r.Root, "checkout", "feature")
	write(t, filepath.Join(r.Root, "feature.txt"), "hello feature\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "commit on feature 1")

	write(t, filepath.Join(r.Root, "feature.txt"), "hello feature updated\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "commit on feature 2")

	// 1. 三点对比（PR 模式）：main...feature
	// 只看 feature 从 main 分叉之后新改了什么（feature.txt），不含 main 上的 main_only.txt
	files, err := r.BranchDiff(context.Background(), "main", "feature", true)
	if err != nil {
		t.Fatalf("BranchDiff(main...feature): %v", err)
	}
	if len(files) != 1 || files[0].Path() != "feature.txt" {
		t.Fatalf("三点对比应该只有 feature.txt：%+v", files)
	}

	// 2. 两点对比（直接对比）：main..feature
	// 包含 feature.txt（新增）和 main_only.txt（删除）
	filesDirect, err := r.BranchDiff(context.Background(), "main", "feature", false)
	if err != nil {
		t.Fatalf("BranchDiff(main..feature): %v", err)
	}
	if len(filesDirect) != 2 {
		t.Fatalf("两点对比应该有两个文件：%+v", filesDirect)
	}

	// 3. 提交列表：main..feature
	// 包含 feature 上的两个提交
	commits, err := r.BranchCommits(context.Background(), "main", "feature", 10)
	if err != nil {
		t.Fatalf("BranchCommits: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("提交数 = %d, want 2", len(commits))
	}
	if commits[0].Subject != "commit on feature 2" {
		t.Errorf("最新提交 = %q", commits[0].Subject)
	}
}

// 「所有分支」看的是所有引用合起来的历史，不是 HEAD 那一条线。
//
// 判据是「HEAD 走不到的提交也在里面」：只比较条数的话，一个恰好同样长的
// 历史也能蒙混过关。
func TestLogAllCoversEveryBranch(t *testing.T) {
	r := tempRepo(t)
	for _, s := range []string{"2", "3"} {
		write(t, filepath.Join(r.Root, "a.txt"), "x\n"+s+"\n")
		git(t, r.Root, "commit", "-am", "c"+s)
	}
	// 支线：从 init 分出去，HEAD 走不到
	git(t, r.Root, "checkout", "-q", "-b", "side", "HEAD~2")
	write(t, filepath.Join(r.Root, "side.txt"), "side\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "支线提交")
	git(t, r.Root, "checkout", "-q", "main")

	head, err := r.Log(context.Background(), "", 10, 0)
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	all, err := r.LogAll(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("LogAll: %v", err)
	}
	if len(all) <= len(head) {
		t.Fatalf("LogAll 读到 %d 条，HEAD 那条 %d 条 —— 该更多", len(all), len(head))
	}
	var found bool
	for _, c := range all {
		if c.Subject == "支线提交" {
			found = true
		}
	}
	if !found {
		t.Errorf("支线提交不在 LogAll 里：%v", all)
	}
	// 储藏不算「分支」：--all 不收 refs/stash，硬塞进去会打断拓扑图
	write(t, filepath.Join(r.Root, "a.txt"), "dirty\n")
	git(t, r.Root, "stash", "push", "-q", "-m", "半成品")
	all, err = r.LogAll(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("LogAll（有储藏）: %v", err)
	}
	for _, c := range all {
		if strings.Contains(c.Subject, "WIP on") {
			t.Errorf("储藏不该出现在所有分支的历史里：%q", c.Subject)
		}
	}
}

// 空仓库里 --all 也是「没有历史」，不是出错。
func TestLogAllEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	commits, err := r.LogAll(context.Background(), 10, 0)
	if err != nil {
		t.Fatalf("LogAll 在空仓库上报错：%v", err)
	}
	if len(commits) != 0 {
		t.Errorf("空仓库该没有提交，实际 %d 条", len(commits))
	}
}

package gitrepo

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// 提交信息里带分隔符不能把解析搞乱 —— 这正是用 \x00/\x1e 而不是逗号竖线的原因。
func TestLogParsesFields(t *testing.T) {
	r := tempRepo(t)
	write(t, filepath.Join(r.Root, "a.txt"), "one\ntwo\nthree\nfour\n")
	git(t, r.Root, "commit", "-am", "a: 中文 subject, with commas | and 竖线")

	commits, err := r.Log(context.Background(), 10, 0)
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

	page1, err := r.Log(context.Background(), 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	page2, err := r.Log(context.Background(), 2, 2)
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

// 空仓库没有提交，Log 应该是空列表而不是错误。
func TestLogEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-b", "main")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	commits, err := r.Log(context.Background(), 10, 0)
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

	commits, err := r.Log(context.Background(), 1, 0)
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
	commits, err := r.Log(context.Background(), 1, 0)
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

	commits, _ := r.Log(context.Background(), 1, 0)
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

package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// RestoreFile 把工作区的改动丢回去：改了内容的回到 index 里那份，删掉的
// 也一起恢复回来。
func TestRestoreFile(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	write(t, filepath.Join(r.Root, "a.txt"), "one\nTWO CHANGED\nthree\n")
	if err := r.RestoreFile(ctx, "a.txt"); err != nil {
		t.Fatalf("RestoreFile: %v", err)
	}
	if got := read(t, filepath.Join(r.Root, "a.txt")); got != "one\ntwo\nthree\n" {
		t.Errorf("丢弃之后该回到 index 里那份，实际 %q", got)
	}

	// 删掉的文件也该被恢复回来
	if err := os.Remove(filepath.Join(r.Root, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if err := r.RestoreFile(ctx, "a.txt"); err != nil {
		t.Fatalf("RestoreFile（删除）: %v", err)
	}
	if got := read(t, filepath.Join(r.Root, "a.txt")); got != "one\ntwo\nthree\n" {
		t.Errorf("删掉的文件该被恢复回来，实际 %q", got)
	}

	// 路径给空串等于什么都没做（不报错）
	if err := r.RestoreFile(ctx, "", ""); err != nil {
		t.Errorf("空路径不该报错：%v", err)
	}
}

// RemoveFiles 删工作区里的文件；已经不在的不算失败。
func TestRemoveFiles(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	write(t, filepath.Join(r.Root, "fresh.txt"), "hi\n")
	if err := r.RemoveFiles(ctx, "fresh.txt"); err != nil {
		t.Fatalf("RemoveFiles: %v", err)
	}
	if _, err := os.Stat(filepath.Join(r.Root, "fresh.txt")); !os.IsNotExist(err) {
		t.Errorf("文件该被删掉：%v", err)
	}
	// 再删一次：已经不在，不算失败
	if err := r.RemoveFiles(ctx, "fresh.txt"); err != nil {
		t.Errorf("删一个已经不在的文件不该报错：%v", err)
	}
}

// 路径跑到仓库外面的必须被挡住：这个函数真的会删文件，不能由着路径走。
func TestRemoveFilesRejectsEscape(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	outside := filepath.Join(filepath.Dir(r.Root), "outside.txt")
	write(t, outside, "don't touch\n")
	t.Cleanup(func() { _ = os.Remove(outside) })

	for _, p := range []string{"../outside.txt", outside, "/etc/hosts", ""} {
		if err := r.RemoveFiles(ctx, p); err == nil && p != "" {
			t.Errorf("路径 %q 该被拒绝", p)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("仓库外面的文件不该被动过：%v", err)
	}
	// 空路径是「没有要删的」，不报错
	if err := r.RemoveFiles(ctx, ""); err != nil {
		t.Errorf("空路径不该报错：%v", err)
	}
}

// AddIgnore 往仓库根的 .gitignore 追加一行，带前导斜杠；同一行不重复写；
// 原来没有 .gitignore 就新建。
func TestAddIgnore(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()
	gitignore := filepath.Join(r.Root, ".gitignore")

	// 原来没有这个文件
	if err := r.AddIgnore(ctx, "build/out.txt"); err != nil {
		t.Fatalf("AddIgnore: %v", err)
	}
	if got := read(t, gitignore); got != "/build/out.txt\n" {
		t.Errorf("该新建 .gitignore 并写一行，实际 %q", got)
	}

	// 再写一条，接在后面
	if err := r.AddIgnore(ctx, "fresh.go"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, gitignore); got != "/build/out.txt\n/fresh.go\n" {
		t.Errorf("第二条该接在后面，实际 %q", got)
	}

	// 重复的同一行不写第二遍
	if err := r.AddIgnore(ctx, "fresh.go"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, gitignore); got != "/build/out.txt\n/fresh.go\n" {
		t.Errorf("同一行不该写第二遍，实际 %q", got)
	}

	// 原有内容末尾没有换行时先补一个
	write(t, gitignore, "*.log")
	if err := r.AddIgnore(ctx, "x.txt"); err != nil {
		t.Fatal(err)
	}
	if got := read(t, gitignore); got != "*.log\n/x.txt\n" {
		t.Errorf("末尾没换行时该先补一个，实际 %q", got)
	}

	// 忽略之后 git 真的不看它了
	write(t, filepath.Join(r.Root, "x.txt"), "noise\n")
	if out := gitOut(t, r.Root, "status", "--porcelain"); strings.Contains(out, "x.txt") {
		t.Errorf("忽略之后 x.txt 不该出现在 status 里：%q", out)
	}
}

// FileLog 只列碰过这个文件的提交，新的在前，并且跟得住改名。
func TestFileLog(t *testing.T) {
	r := tempRepo(t)
	ctx := context.Background()

	write(t, filepath.Join(r.Root, "b.txt"), "b\n")
	git(t, r.Root, "add", ".")
	git(t, r.Root, "commit", "-m", "add b")

	write(t, filepath.Join(r.Root, "b.txt"), "b\nmore\n")
	git(t, r.Root, "add", "b.txt")
	git(t, r.Root, "commit", "-m", "edit b")

	// 另一个文件也在改：它不该混进 b.txt 的历史
	write(t, filepath.Join(r.Root, "a.txt"), "one\ntwo\nthree\nfour\n")
	git(t, r.Root, "add", "a.txt")
	git(t, r.Root, "commit", "-m", "edit a")

	commits, err := r.FileLog(ctx, "b.txt", 10)
	if err != nil {
		t.Fatalf("FileLog: %v", err)
	}
	if len(commits) != 2 {
		t.Fatalf("b.txt 有 2 次提交，读到 %d 条：%+v", len(commits), subjects(commits))
	}
	if commits[0].Subject != "edit b" || commits[1].Subject != "add b" {
		t.Errorf("顺序该是新的在前，实际 %v", subjects(commits))
	}
	if commits[0].Hash == "" || commits[0].Short == "" {
		t.Errorf("哈希该读出来：%+v", commits[0])
	}

	// 改名之后仍然跟得住
	git(t, r.Root, "mv", "b.txt", "renamed.txt")
	git(t, r.Root, "commit", "-m", "rename b")
	commits, err = r.FileLog(ctx, "renamed.txt", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 3 {
		t.Fatalf("--follow 该把改名前的两次也带上，读到 %d 条：%v", len(commits), subjects(commits))
	}

	// 从没提交过的路径：空历史，不是错误
	commits, err = r.FileLog(ctx, "nope.txt", 10)
	if err != nil {
		t.Errorf("没有历史的路径不该报错：%v", err)
	}
	if len(commits) != 0 {
		t.Errorf("没有历史的路径该是空的，读到 %d 条", len(commits))
	}
	if commits, err := r.FileLog(ctx, "", 10); err != nil || commits != nil {
		t.Errorf("空路径该直接返回空：%v %v", commits, err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读 %s: %v", path, err)
	}
	return string(b)
}

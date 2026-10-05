package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Contents 读取工作区里某个文件的内容。
func (r *Repo) Contents(path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(r.Root, path))
}

// ContentsAt 读取某个版本里该文件的内容。rev 可以是 HEAD、某个 commit
// 或者 :0（index）。
func (r *Repo) ContentsAt(ctx context.Context, rev, path string) ([]byte, error) {
	if rev == "" {
		rev = "HEAD"
	}
	return run(ctx, r.Root, "show", rev+":"+path)
}

// NewLines 返回文件在「新的一侧」的内容按行切开的结果。
//
// 已删除的文件没有新内容，返回 nil。返回的切片不含末尾换行造成的空串，
// 所以 len() 就是文件的真实行数 —— 界面靠它算 diff 末尾那段
// 「未改动区间」有多长。
//
// 内容超过 limit 字节时返回 nil：与其把几 MB 的文件切行塞进界面，
// 不如干脆不高亮、也不算末尾区间。
func (r *Repo) NewLines(path string, limit int) []string {
	data, err := r.Contents(path)
	if err != nil || len(data) == 0 || len(data) > limit {
		return nil
	}
	return SplitLines(string(data))
}

// SplitLines 把文件内容切成行，去掉末尾换行造成的那个空串。
func SplitLines(s string) []string {
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

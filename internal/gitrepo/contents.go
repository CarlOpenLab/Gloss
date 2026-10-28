package gitrepo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
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

// SizeAt 返回某个版本里文件的字节数，不把内容读出来。
//
// 给「先看多大再决定读不读」用（图片预览）：一张几十 MB 的图读完再发现
// 超限，内存已经花掉了。cat-file -s 只查对象头，代价和一次 stat 差不多。
func (r *Repo) SizeAt(ctx context.Context, rev, path string) (int64, error) {
	if rev == "" {
		rev = "HEAD"
	}
	out, err := run(ctx, r.Root, "cat-file", "-s", rev+":"+path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
}

// indexHashes 批量取一组路径在 index（stage 0）里的 blob 哈希，不读内容。
// 二进制文件的指纹靠它们（见 model.go 的填充循环）：没有 hunk 可读，只有
// 对象哈希能说明「内容换没换」。取不到的路径不出现在结果里，调用方按
// 空串处理 —— 冲突条目挂在 stage 1/2/3，不算「已暂存内容」。
func (r *Repo) indexHashes(ctx context.Context, paths []string) map[string]string {
	out := make(map[string]string, len(paths))
	for _, chunk := range chunkPaths(paths, 128) {
		args := append([]string{"ls-files", "--stage", "-z", "--"}, chunk...)
		raw, err := run(ctx, r.Root, args...)
		if err != nil {
			continue
		}
		for _, rec := range bytes.Split(raw, []byte{0}) {
			meta, path, ok := bytes.Cut(rec, []byte{'\t'})
			if !ok {
				continue
			}
			fields := strings.Fields(string(meta)) // "<mode> <sha> <stage>"
			if len(fields) != 3 || fields[2] != "0" {
				continue
			}
			out[string(path)] = fields[1]
		}
	}
	return out
}

// headHashes 批量取一组路径在 HEAD 里的 blob 哈希。取不到（文件刚删、
// 权限不对）的路径不出现在结果里，调用方按空串处理。
func (r *Repo) headHashes(ctx context.Context, paths []string) map[string]string {
	out := make(map[string]string, len(paths))
	for _, chunk := range chunkPaths(paths, 128) {
		args := append([]string{"ls-tree", "-z", "HEAD", "--"}, chunk...)
		raw, err := run(ctx, r.Root, args...)
		if err != nil {
			continue
		}
		for _, rec := range bytes.Split(raw, []byte{0}) {
			meta, path, ok := bytes.Cut(rec, []byte{'\t'})
			if !ok {
				continue
			}
			fields := strings.Fields(string(meta)) // "<mode> <type> <sha>"
			if len(fields) != 3 || fields[1] != "blob" {
				continue
			}
			out[string(path)] = fields[2]
		}
	}
	return out
}

// worktreeHashes 批量算工作区文件的 blob 哈希（git 的哈希算法，不落库）。
// 走 git hash-object 而不是在这里读文件：它按流算，几百 MB 的文件不会
// 整份进内存；路径里有什么字符也不用担心，-- 之后一律当路径。
func (r *Repo) worktreeHashes(ctx context.Context, paths []string) map[string]string {
	out := make(map[string]string, len(paths))
	for _, chunk := range chunkPaths(paths, 128) {
		args := append([]string{"hash-object", "--"}, chunk...)
		raw, err := run(ctx, r.Root, args...)
		if err != nil {
			continue
		}
		hashes := strings.Fields(string(raw))
		if len(hashes) != len(chunk) {
			continue // 输出和输入一一对应才配得上对
		}
		for i, h := range hashes {
			out[chunk[i]] = h
		}
	}
	return out
}

// chunkPaths 把路径列表切成每批最多 n 个：git 子进程的命令行有长度上限
// （Windows 整条 32k），一批塞进几百个长路径会直接放不下。
func chunkPaths(paths []string, n int) [][]string {
	var out [][]string
	for len(paths) > 0 {
		c := min(n, len(paths))
		out = append(out, paths[:c])
		paths = paths[c:]
	}
	return out
}

// ConflictContents returns the three useful texts for a conflicted path:
// the common ancestor (stage 1), our side (stage 2), and their side (stage 3).
// A missing stage means that side deleted the file. The worktree is returned as
// the editable result, including conflict markers when Git wrote them.
func (r *Repo) ConflictContents(ctx context.Context, path string) (base, ours, theirs, worktree []byte, err error) {
	for _, item := range []struct {
		rev string
		out *[]byte
	}{
		{":1", &base},
		{":2", &ours},
		{":3", &theirs},
	} {
		data, e := r.ContentsAt(ctx, item.rev, path)
		if e != nil {
			// A missing stage is the normal representation of a delete/modify
			// conflict, not a failed load.
			continue
		}
		*item.out = data
	}
	worktree, err = r.Contents(path)
	return
}

// WriteContents replaces a worktree file without touching the index. The
// embedded resolver stages it separately after the write has succeeded.
//
// 已存在的文件保留它原来的权限位：内置编辑器保存一个 0755 的脚本时，不该
// 顺手把可执行位抹掉 —— 那是一次没人要求的改动，而且 git status 也不会报。
// 新文件才按 0644 建。
func (r *Repo) WriteContents(path string, data []byte) error {
	full := filepath.Join(r.Root, path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(full); err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(full, data, mode)
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

// NewLinesAt 和 NewLines 一样，但读的是指定版本（rev 语义同 ContentsAt，
// 比如 ":0" 是 index）。
//
// 已暂存段必须用它：那一段 diff 的「新侧」是 index 里的样子，不是工作区的
// 样子。半暂存的文件（暂存之后又改了）两者不一样 —— 读工作区会让展开的
// 未改动区间行号错位、语法高亮整段对不上，而且错得静默无声。
func (r *Repo) NewLinesAt(ctx context.Context, rev, path string, limit int) []string {
	data, err := r.ContentsAt(ctx, rev, path)
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

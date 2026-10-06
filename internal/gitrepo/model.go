package gitrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// maxUntrackedBytes 是未跟踪文件读进来渲染的上限。超过就不渲染内容，
// 只标注「文件过大」——几 MB 的日志文件塞进 diff 视图对谁都没好处。
const maxUntrackedBytes = 1 << 20

// maxUntracked 是未跟踪文件的处理条数上限。
//
// Status 用 --untracked-files=all 展开未跟踪目录，万一用户没配 .gitignore
// （比如仓库里有个没被忽略的 node_modules），列出来的文件可能上万条，
// 逐条读内容会把加载拖到没法用。超出部分直接跳过，界面上体现为
// 「文件数」比实际少 —— 这是有意的取舍，宁可少显示也不要卡死。
const maxUntracked = 500

// File 是一个可以直接渲染的文件变更：解析好的 hunk + git status 的三态信息。
type File struct {
	diff.FileDiff

	// Status 是显示用的变更类型，来自 git status（比只看 patch 更准，
	// 能区分「未跟踪」和「新增」）。
	Status Status

	// Staged / Unstaged 区分「已暂存」和「工作区还有改动」。
	// v1 界面上先把它们合并成一张卡片显示（像 godiff 那样），
	// 但数据从第一天就是分开的 —— 下一步做暂存/提交时不用回头改数据层。
	Staged   bool
	Unstaged bool

	// Fingerprint 是文件内容的指纹，用于「已 review」标记：
	// 内容一变指纹就变，已读状态自动失效。
	Fingerprint string

	// TooLarge 表示文件超过读取上限，没有渲染内容。
	TooLarge bool
}

// Model 是仓库当前状态的完整快照，界面每次刷新重建一份。
type Model struct {
	Repo     *Repo
	Branch   string
	Files    []File
	TotalAdd int
	TotalDel int
}

// Load 读取工作区相对 HEAD 的全部变更（含已暂存、未暂存、未跟踪）。
func (r *Repo) Load(ctx context.Context) (*Model, error) {
	changes, err := r.Status(ctx)
	if err != nil {
		return nil, err
	}
	branch, err := r.Branch(ctx)
	if err != nil {
		return nil, err
	}

	// git diff HEAD 同时包含已暂存和未暂存的改动，两者合成一张卡片显示。
	// 未跟踪文件不在此列，下面单独处理。
	raw, err := run(ctx, r.Root, diffArgs(r.baseRev(ctx))...)
	if err != nil {
		return nil, err
	}
	parsed := diff.Parse(string(raw))
	byPath := make(map[string]*diff.FileDiff, len(parsed))
	for i := range parsed {
		byPath[parsed[i].NewPath] = &parsed[i]
	}

	m := &Model{Repo: r, Branch: branch}
	untracked := 0
	for _, ch := range changes {
		f := File{
			Status:      ch.Status,
			Staged:      ch.Staged,
			Unstaged:    ch.Unstaged,
			Fingerprint: fingerprint(ch.Path, parsedFingerprintSource(byPath[ch.Path], ch)),
		}

		switch {
		case ch.Status == StatusUntracked:
			if untracked >= maxUntracked {
				continue
			}
			untracked++
			// 未跟踪文件 git diff 不会输出，自己读内容合成一份
			content, err := readCapped(filepath.Join(r.Root, ch.Path), maxUntrackedBytes)
			if err != nil {
				continue // 读不到（权限、目录、刚好被删）就跳过，不打断整体加载
			}
			f.FileDiff = diff.NewFileFromContent(ch.Path, content)
			f.Fingerprint = fingerprint(ch.Path, string(content))
			if f.AddCount == 0 && len(content) > 0 {
				f.TooLarge = true
			}

		case byPath[ch.Path] != nil:
			f.FileDiff = *byPath[ch.Path]

		default:
			// 只在 git status 里出现、patch 里没有的情况：mode 变更、
			// 空文件新增、子模块指针变化等。给一个只有状态没有内容的条目，
			// 至少让用户知道这个文件变了。
			f.FileDiff = diff.FileDiff{NewPath: ch.Path, OldPath: ch.OldPath}
		}

		if ch.Status == StatusRenamed && ch.OldPath != "" {
			f.OldPath = ch.OldPath
		}
		f.Status = ch.Status

		m.Files = append(m.Files, f)
		m.TotalAdd += f.AddCount
		m.TotalDel += f.DelCount
	}
	return m, nil
}

// parsedFingerprintSource 取用于算指纹的文本：优先用 patch 原文（能看到
// 相对 HEAD 的净变更），没有就用状态本身。
func parsedFingerprintSource(f *diff.FileDiff, ch Change) string {
	if f == nil {
		return string(ch.Status) + "\x00" + ch.OldPath
	}
	var b []byte
	for i := range f.Hunks {
		b = append(b, f.Hunks[i].Raw()...)
	}
	return string(b)
}

// fingerprint 算出内容指纹。用 sha256 截断到 16 位十六进制：
// 这里只是「内容变了没有」的标记，不涉及安全，哈希强度不是重点，
// 短一点更好存也更好比对。
func fingerprint(path, content string) string {
	sum := sha256.Sum256([]byte(path + "\x00" + content))
	return hex.EncodeToString(sum[:])[:16]
}

// readCapped 读取文件，最多读 limit 字节。读到文件尾（含空文件）不算错误。
func readCapped(path string, limit int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, limit)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf[:n], nil
}

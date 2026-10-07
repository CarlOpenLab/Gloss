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

// Section 说明一个 File 属于文件列表的哪一段。
//
// 历史提交、储藏里的 diff 两段都不是（SectionNone）—— 那边的 key 和语义
// 保持和分段之前完全一样。
type Section uint8

const (
	SectionNone     Section = iota
	SectionStaged           // index vs HEAD：已暂存
	SectionUnstaged         // worktree vs index：未暂存（含未跟踪、冲突）
)

// File 是一个可以直接渲染的文件变更：解析好的 hunk + git status 的三态信息。
type File struct {
	diff.FileDiff

	// Status 是这一段里该文件的变更类型（已暂存段用 index 侧的类型，
	// 未暂存段用工作区侧的类型）。
	Status Status

	// Section 说明这个 File 属于哪一段。两段各自的 diff 来自各自的基线，
	// 同一个路径可以同时出现在两段里（暂存之后又改了）。
	Section Section

	// Staged / Unstaged 是「这一段」的归属标志：SectionStaged 的条目
	// Staged=true、Unstaged=false，反之亦然。两个独立布尔是为了历史
	// 调用方读起来顺，不表示一个 File 能同时属于两段。
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
	Detached bool // HEAD 不在任何分支上（检出过标签/提交，或变基到一半）
	Remotes  []string
	Files    []File
	TotalAdd int
	TotalDel int

	// Pending 是仓库正卡在哪一步写操作中间（合并/变基/拣选/回退/am 撞了
	// 冲突停着）。零值 = 没卡住。
	Pending Pending
}

// Load 读取工作区相对 HEAD 的全部变更，按「已暂存」「未暂存」分成两段。
//
// 两段的 diff 必须来自各自的基线：已暂存 = index vs HEAD
// （`git diff --cached <base>`），未暂存 = worktree vs index（`git diff`）。
// 这不只是为了显示 —— hunk 的 ID 是「路径 + 带行号的原文」的哈希，换一条
// 基线行号就全变；界面上展示的 hunk 和点击暂存时重新跑出来的 hunk 只有
// 基线相同才对得上号（见 write.go 里 StageHunk 的重新切片）。
//
// Files 里「已暂存」是连续的前缀，之后才是未暂存 —— 界面的分组和段落键
// 都建立在这条性质上。
func (r *Repo) Load(ctx context.Context) (*Model, error) {
	changes, err := r.Status(ctx)
	if err != nil {
		return nil, err
	}
	branch, detached, err := r.branchInfo(ctx)
	if err != nil {
		return nil, err
	}
	remotes, err := r.Remotes(ctx)
	if err != nil {
		remotes = nil // 远程读不到不算加载失败，只是远程操作点不了
	}

	rawStaged, err := run(ctx, r.Root, diffArgs("--cached", r.baseRev(ctx))...)
	if err != nil {
		return nil, err
	}
	rawUnstaged, err := run(ctx, r.Root, diffArgs()...)
	if err != nil {
		return nil, err
	}
	staged := indexByPath(diff.Parse(string(rawStaged)))
	unstaged := indexByPath(diff.Parse(string(rawUnstaged)))

	m := &Model{Repo: r, Branch: branch, Detached: detached, Remotes: remotes, Pending: r.pending()}
	appendFile := func(f File) {
		m.Files = append(m.Files, f)
		m.TotalAdd += f.AddCount
		m.TotalDel += f.DelCount
	}

	// 已暂存段（Files 的连续前缀）。
	for _, ch := range changes {
		if ch.StagedStatus == "" {
			continue
		}
		fd := staged[ch.Path]
		appendFile(fileFromDiff(ch, fd, ch.StagedStatus, ch.Path, ch.OldPath, SectionStaged))
	}

	// 未暂存段：普通改动、未跟踪（合成整文件）、冲突（只有一张空卡片）。
	untracked := 0
	for _, ch := range changes {
		switch {
		case ch.Status == StatusUntracked:
			if untracked >= maxUntracked {
				continue
			}
			untracked++
			content, err := readCapped(filepath.Join(r.Root, ch.Path), maxUntrackedBytes)
			if err != nil {
				continue // 读不到（权限、目录、刚好被删）就跳过，不打断整体加载
			}
			f := File{
				FileDiff:    diff.NewFileFromContent(ch.Path, content),
				Status:      StatusUntracked,
				Section:     SectionUnstaged,
				Unstaged:    true,
				Fingerprint: fingerprint(ch.Path, string(content)),
			}
			if f.AddCount == 0 && len(content) > 0 {
				f.TooLarge = true
			}
			appendFile(f)

		case ch.Status == StatusConflict:
			// 冲突内容的 patch 是合并格式（diff --cc、@@@ 头），解析器不认；
			// 给一张没有内容的卡片，解决路径是整文件 `git add`。
			appendFile(File{
				FileDiff:    diff.FileDiff{NewPath: ch.Path},
				Status:      StatusConflict,
				Section:     SectionUnstaged,
				Unstaged:    true,
				Fingerprint: fingerprint(ch.Path, string(StatusConflict)),
			})

		case ch.UnstagedStatus != "":
			appendFile(fileFromDiff(ch, unstaged[ch.Path], ch.UnstagedStatus, ch.Path, ch.OldPath, SectionUnstaged))
		}
	}
	return m, nil
}

// fileFromDiff 组装分段后的一个 File：优先用该段 patch 解析出的内容，
// patch 里没有（mode 变更、空文件新增、子模块指针变化等）就给一个只有
// 状态没有内容的条目，至少让用户知道这个文件变了。
func fileFromDiff(ch Change, fd *diff.FileDiff, status Status, path, oldPath string, section Section) File {
	f := File{
		Status:      status,
		Section:     section,
		Fingerprint: fingerprint(path, parsedFingerprintSource(fd, status, oldPath)),
	}
	if section == SectionStaged {
		f.Staged = true
	} else {
		f.Unstaged = true
	}
	if fd != nil {
		f.FileDiff = *fd
	} else {
		f.FileDiff = diff.FileDiff{NewPath: path, OldPath: oldPath}
	}
	if status == StatusRenamed && oldPath != "" {
		f.OldPath = oldPath
	}
	return f
}

// indexByPath 把解析出的文件按主路径建索引。
//
// 用 Path()（新路径优先、退回旧路径）而不是 NewPath：删除的文件新路径是
// 空的，按 NewPath 建索引时它们全落到同一个空键上，界面上就成了「没有可
// 显示的文本差异」。
func indexByPath(files []diff.FileDiff) map[string]*diff.FileDiff {
	m := make(map[string]*diff.FileDiff, len(files))
	for i := range files {
		m[files[i].Path()] = &files[i]
	}
	return m
}

// parsedFingerprintSource 取用于算指纹的文本：优先用该段 patch 原文，
// 没有 patch 就用状态本身。
func parsedFingerprintSource(f *diff.FileDiff, status Status, oldPath string) string {
	if f == nil {
		return string(status) + "\x00" + oldPath
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

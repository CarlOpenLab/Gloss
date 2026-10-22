package gitrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// maxUntrackedBytes 是未跟踪文件读进来渲染的上限。超过就不渲染内容，
// 只标注「文件过大」——几 MB 的日志文件塞进 diff 视图对谁都没好处。
const maxUntrackedBytes = 1 << 20

// maxUntrackedImageBytes 是未跟踪图片读进来的上限。
//
// 比文本那一档宽得多：图片本来就不是拿来「按行看」的，读它只为判二进制和
// 算内容指纹；1 MB 会把常见的截图截断，指纹也就跟着半截算。真正的解码
// 上限在界面那边（见 imagereview.go），这里只求指纹完整。
const maxUntrackedImageBytes = 32 << 20

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

	// Stages 说（冲突文件）在 index 里还剩哪几方的版本：缺 stage 2 是我们
	// 这边删了它，缺 stage 3 是对方删了它。只在 Status == StatusConflict
	// 时有意义；别的文件是零值。
	//
	// 界面拿它把「哪边删了文件」提前说清 —— 删除-修改冲突里，一颗写着
	// 「用对方那边的」的按钮按下去是**把文件删掉**，不说清就是挖坑。
	Stages ConflictSides
}

// Model 是仓库当前状态的完整快照，界面每次刷新重建一份。
type Model struct {
	Repo     *Repo
	Branch   string
	Detached bool // HEAD 不在任何分支上（检出过标签/提交，或变基到一半）
	// Remotes 是远程名。RemoteURLs 是同一个远程配的 URL（提交详情卡拿它
	// 拼「在网页上看这个提交」的地址）—— 两条一起读，见 RemoteURLs。
	Remotes    []string
	RemoteURLs map[string]string
	Files      []File
	TotalAdd   int
	TotalDel   int

	// Pending 是仓库正卡在哪一步写操作中间（合并/变基/拣选/回退/am 撞了
	// 冲突停着）。零值 = 没卡住。
	Pending Pending

	// HasHead 表示仓库是否已有提交（HEAD 存在，可用于修改上次提交）。
	HasHead bool

	// Ahead 和 Behind 是当前分支相对配置上游的超前和落后提交数。
	Ahead  int
	Behind int

	// Upstream 是当前分支跟踪的远程分支名（如 origin/main）。若无上游则为空。
	Upstream string

	// Worktree 是工作区里的全部文件（已跟踪 + 未跟踪但未被忽略），
	// 供「编辑器」视图的文件栏列举整个仓库。读不到时为空 ——
	// 它只影响那一栏的完整性，不该把整次加载判失败。
	Worktree []WorktreeFile
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
	statusRaw, err := r.statusRaw(ctx)
	if err != nil {
		return nil, err
	}
	changes := parseStatus(statusRaw)
	ahead, behind, upstream := parseStatusBranch(statusRaw)
	branch, detached, err := r.branchInfo(ctx)
	if err != nil {
		return nil, err
	}
	remotes, err := r.Remotes(ctx)
	if err != nil {
		remotes = nil // 远程读不到不算加载失败，只是远程操作点不了
	}
	remoteURLs := r.RemoteURLs(ctx, remotes)
	worktree, err := r.WorktreeFiles(ctx)
	if err != nil {
		worktree = nil // 同上：编辑器那一栏少点文件，不影响仓库状态本身
	}

	base := r.baseRev(ctx)
	rawStaged, err := run(ctx, r.Root, diffArgs("--cached", base)...)
	if err != nil {
		return nil, err
	}
	rawUnstaged, err := run(ctx, r.Root, diffArgs()...)
	if err != nil {
		return nil, err
	}
	staged := indexByPath(diff.Parse(string(rawStaged)))
	unstaged := indexByPath(diff.Parse(string(rawUnstaged)))

	m := &Model{
		Repo:       r,
		Branch:     branch,
		Detached:   detached,
		Remotes:    remotes,
		RemoteURLs: remoteURLs,
		Pending:    r.pending(),
		HasHead:    base != emptyTree,
		Ahead:      ahead,
		Behind:     behind,
		Upstream:   upstream,
		Worktree:   worktree,
	}
	appendFile := func(f File) {
		m.Files = append(m.Files, f)
		m.TotalAdd += f.AddCount
		m.TotalDel += f.DelCount
	}

	// 有冲突文件时多跑一条 ls-files -u，把每个路径的 stage 情况读回来
	// （哪边删了文件）。判据是「有没有冲突文件」而不是 Pending 是否为空：
	// 储藏 apply/pop 撞出来的冲突不写 MERGE_HEAD，Pending 是零值，但那些
	// 文件同样需要逐边解决的说明。干净仓库一次都不多跑。
	var sides map[string]ConflictSides
	for _, ch := range changes {
		if ch.Status == StatusConflict {
			sides = r.unmergedSides(ctx)
			break
		}
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
			// 图片按预览上限读：1 MB 的截图很常见，用文本那一档会把大半
			// 图片截断，解出来就是「图片已损坏」。反正内容只用来判二进制和
			// 算指纹，截断的部分不影响这两件事。
			limit := maxUntrackedBytes
			if diff.IsImagePath(ch.Path) {
				limit = maxUntrackedImageBytes
			}
			content, err := readCapped(filepath.Join(r.Root, ch.Path), limit)
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
			// 二进制的 AddCount 本来就是 0，那不是「读不下」。
			if !f.Binary && f.AddCount == 0 && len(content) > 0 {
				f.TooLarge = true
			}
			appendFile(f)

		case ch.Status == StatusConflict:
			// 冲突内容的 patch 是合并格式（diff --cc、@@@ 头），解析器不认；
			// 给一张没有内容的卡片，解决路径是整文件 `git add` 或挑一边
			// （conflict.go）。
			appendFile(File{
				FileDiff:    diff.FileDiff{NewPath: ch.Path},
				Status:      StatusConflict,
				Section:     SectionUnstaged,
				Unstaged:    true,
				Fingerprint: fingerprint(ch.Path, string(StatusConflict)),
				Stages:      sides[ch.Path],
			})

		case ch.UnstagedStatus != "":
			appendFile(fileFromDiff(ch, unstaged[ch.Path], ch.UnstagedStatus, ch.Path, ch.OldPath, SectionUnstaged))
		}
	}

	// 二进制文件的内容指纹得另外补：hunk 里没有内容，光靠 patch 文本
	// 认不出「换了一张图」。不补这一步，图片改了界面还显示上一张，
	// 「已 review」也不会失效。
	for i := range m.Files {
		f := &m.Files[i]
		if !f.Binary || f.Status == StatusUntracked {
			continue
		}
		if h := r.binaryFingerprint(ctx, *f); h != "" {
			f.Fingerprint = fingerprint(strconv.Itoa(int(f.Section))+"/"+string(f.Status)+"/"+pathKey(*f), h)
		}
	}
	return m, nil
}

// binaryFingerprint 取二进制文件在该段里「新的一侧」的 blob 哈希。
//
// 只走 git 的对象哈希，不把内容读进内存：几百 MB 的视频文件也只是一次
// 哈希计算。取不到（文件刚好被删、权限不对）就返回空，调用方保留原来的
// 指纹 —— 界面显示旧内容，总比整次加载失败好。
func (r *Repo) binaryFingerprint(ctx context.Context, f File) string {
	path := f.Path()
	switch {
	case f.Section == SectionStaged && f.Status == StatusDeleted:
		h, _ := r.BlobHash(ctx, "HEAD", f.OldPath)
		return h
	case f.Section == SectionStaged:
		h, _ := r.BlobHash(ctx, ":0", path)
		return h
	case f.Status == StatusDeleted:
		h, _ := r.BlobHash(ctx, ":0", path)
		return h
	default:
		h, _ := r.WorktreeHash(ctx, path)
		return h
	}
}

// pathKey 是给指纹用的路径：已删除的文件没有 NewPath，退回旧的。
func pathKey(f File) string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
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

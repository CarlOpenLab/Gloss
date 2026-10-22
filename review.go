package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 改动区右边那一栏：一次看一个文件，用 mygo 的 editor.DiffView 画。
//
// 为什么不把整份改动摊成一张长列表（旧的做法）：那条路要自己重写 diff 的
// 每一块 —— 逐行铺、按行号取高亮、自己算未改动区间、自己做查找和虚拟化。
// mygo 的 DiffView 已经把这件事做完了：词级内联高亮、未改动区间折叠、可
// 编辑的右侧、按段操作、查找栏、按帧只画看得见的那几行。Gloss 留着那份
// 自研渲染器的唯一结果是两套 diff 长得不一样，而且框架升级它不会跟着好。
//
// 换掉的代价是「一份改动只显示一个文件」，所以左边那栏文件列表从「导航」
// 变成了「导航 + 概览」：它一直在，总数、每个文件的增删统计、已 review 的
// 勾都在那儿。浏览大改动时用 ↑/↓ 在文件之间走，不再是滚一列几万行。
//
// 状态跟着选中的文件走，跨帧保留：缓冲区一起活着，撤销历史、滚动位置、
// 展开过的未改动区间、光标全都在 —— 这些都是「一个编辑器」该有的东西，
// 每帧重建就等于每帧把它们擦掉。

// reviewState 是这一栏的状态。
type reviewState struct {
	// key 是当前显示的文件（fileKey：路径 + 段落），fp 是它当时的指纹。
	// 两个都不一样才重读 —— 指纹会跟着内容变，暂存完一个 hunk 之后同一
	// 个文件得把新的两侧拿进来。
	key string
	fp  string
	// path 是已加载（或正在加载）的纯路径，异步回调用它认领结果。
	path    string
	loading bool
	err     string

	old, new *editor.Buffer
	diff     *editor.DiffState

	// editable 说右侧那份能不能改并写回工作区。
	editable bool
	// conflict 说这个文件是带着冲突标记的那种（可改、可挑边、保存即解决）。
	conflict bool
	// stageable 说这一段能不能按 hunk 暂存（门槛和 hunkStageable 一致）。
	stageable bool
	// patchOnly 说这个文件只能照 patch 画：PR 检视那种只有 diff 文本、
	// 两侧原文都不在本地的情况。
	patchOnly bool

	// —— 冲突专用 ——
	// baseline 是左侧拿哪一份当基线（当前分支 / 对方 / 共同祖先）。
	baseline conflictBaseline
	ours     *editor.Buffer
	theirs   *editor.Buffer
	base     *editor.Buffer
	// regions 是当前缓冲区里解析出的冲突块，regionsVer 是解析时那份文本的
	// 版本号 —— 每帧重扫一遍整份文本没必要，改过才重扫。
	regions    []conflictRegion
	regionsVer uint64

	// hunkIdx 是 J / K 跳到的那一段在 Hunks() 里的下标。换文件时归 -1：
	// 「还没跳过」和「停在第 0 段」是两件事，前者往前该落到第一段。
	hunkIdx int

	// —— 图片专用（见 imagereview.go）——
	//
	// image 说这个文件按图片预览画：diff 里没有可读的行，两侧读回来的
	// 是字节而不是文本。imgOld/imgNew 是解好的位图，meta 是格式 + 尺寸 +
	// 体积那一行；两个 err 分开存，一侧坏了不该把另一侧也藏起来。
	image      bool
	imgOld     *ui.Bitmap
	imgNew     *ui.Bitmap
	imgOldMeta string
	imgNewMeta string
	imgOldErr  string
	imgNewErr  string
}

// conflictBaseline 是冲突文件的左侧基线。
type conflictBaseline uint8

const (
	baselineOurs   conflictBaseline = iota // 当前分支（git 的 --ours）
	baselineTheirs                         // 对方那边
	baselineBase                           // 共同祖先（stage 1）
)

// reviewDraft 是「改了还没存」的那一份。
type reviewDraft struct {
	fp  string // 记下它是对着哪一版内容改的
	buf *editor.Buffer
}

// stashReviewDraft 在换缓冲区之前，把改过的那一份留在草稿里。
func (a *app) stashReviewDraft() {
	s := &a.review
	if !s.editable || s.new == nil || !s.new.Modified() || s.key == "" {
		return
	}
	if a.reviewDrafts == nil {
		a.reviewDrafts = map[string]reviewDraft{}
	}
	a.reviewDrafts[s.key] = reviewDraft{fp: s.fp, buf: s.new}
}

// takeReviewDraft 取回某个文件的草稿（指纹对不上就当没有）。
func (a *app) takeReviewDraft(key, fp string) *editor.Buffer {
	d, ok := a.reviewDrafts[key]
	if !ok || d.fp != fp {
		return nil
	}
	return d.buf
}

// pruneReviewDrafts 丢掉已经不在这份列表里的草稿（提交走了、切仓库了）。
func (a *app) pruneReviewDrafts(files []gitrepo.File) {
	if len(a.reviewDrafts) == 0 {
		return
	}
	live := make(map[string]bool, len(files))
	for i := range files {
		live[fileKey(files[i])] = true
	}
	for k := range a.reviewDrafts {
		if !live[k] {
			delete(a.reviewDrafts, k)
		}
	}
}

// reviewTarget 取改动区该显示哪个文件：文件栏当前选中的那个，没选就退到第一个。
func (a *app) reviewTarget() *gitrepo.File {
	if len(a.files) == 0 {
		return nil
	}
	i := a.fileSel
	if i < 0 || i >= len(a.files) {
		i = 0
	}
	return &a.files[i]
}

// syncReview 让改动区和文件栏的选中对齐。每帧开头调一次，重复调用是安全的。
func (a *app) syncReview() {
	// 终端和「编辑器」视图里这一栏不在屏幕上：不为看不见的东西读盘。
	if a.viewMode != viewModeGit {
		return
	}
	if !a.hasDiff() {
		a.review = reviewState{}
		return
	}
	f := a.reviewTarget()
	if f == nil {
		a.review = reviewState{}
		return
	}
	// PR 检视只有 diff 文本（gh 给的 patch），两侧原文不在本地，按 patch 画。
	if a.prReview.on {
		// 失败也算「这一版试过了」：二进制文件的 patch 没有行可画，
		// 不认它会每帧重新解析一次。
		if a.review.key == fileKey(*f) && (a.review.patchOnly || a.review.err != "") {
			return
		}
		a.setupPatchReview(*f)
		return
	}
	if a.review.loading {
		return
	}
	// 装好的三种收尾：文本 diff、图片预览、加载失败。失败也要算「这一版
	// 试过了」—— 否则每帧都会重读同一个烧不进界面的二进制文件。
	if a.review.key == fileKey(*f) && a.review.fp == f.Fingerprint &&
		(a.review.diff != nil || a.review.image || a.review.err != "") {
		return
	}
	a.loadReview(*f)
}

// setupPatchReview 把一个 PR 里的文件按 patch 摆进改动区。
//
// 只有 diff 文本时看不了「这个文件全长什么样」，但一段一段的改动本身是
// 齐的：mygo 的 SetPatch 正是为这种输入准备的（每个 hunk 顶着自己的头）。
func (a *app) setupPatchReview(f gitrepo.File) {
	s := &a.review
	*s = reviewState{key: fileKey(f), fp: f.Fingerprint, path: pathOf(f), hunkIdx: -1}
	// 二进制文件在 patch 里只有一句「Binary files … differ」，没有行可画。
	// 图片尤其要说清楚：别处都能看到画面，只有这里看不到，是 PR 检视的
	// 数据源只有 patch 文本，不是文件本身坏了。
	if f.Binary {
		if isImagePreview(f) {
			s.err = T("PR 检视里只有 patch 文本，看不到这张图片的内容",
				"PR review only has the patch text, so this image's content isn't available")
		} else {
			s.err = T("二进制文件没有可以用行显示的内容", "A binary file has no lines to show")
		}
		return
	}
	patch, err := editor.ParsePatch(filePatchText(f))
	if err != nil || len(patch) == 0 {
		s.err = T("这个文件的改动读不出来", "This file's changes can't be read")
		return
	}
	s.patchOnly = true
	s.diff = &editor.DiffState{}
	s.diff.SetPatch(patch[0])
}

// filePatchText 把解析好的一个文件变动拼回 unified diff 文本。
//
// 拼的是「文件头 + 每个 hunk 的原文」：hunk 的原文在解析时就整行留着了
// （见 internal/diff 的 Raw），文件头则是 ParsePatch 认路径要用的那三行。
func filePatchText(f gitrepo.File) string {
	oldPath, newPath := f.OldPath, f.NewPath
	if oldPath == "" {
		oldPath = "/dev/null"
	} else {
		oldPath = "a/" + oldPath
	}
	if newPath == "" {
		newPath = "/dev/null"
	} else {
		newPath = "b/" + newPath
	}

	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", f.OldPath, f.NewPath)
	fmt.Fprintf(&b, "--- %s\n+++ %s\n", oldPath, newPath)
	for i := range f.Hunks {
		b.WriteString(f.Hunks[i].Raw())
	}
	return b.String()
}

// loadReview 读选中文件的两侧原文，建好缓冲区。
//
// 读的是「这个文件在这一段里相对的两个版本」：未暂存段是 index vs 工作区、
// 已暂存段是 HEAD vs index、历史里是父提交 vs 该提交、储藏是储藏的两侧。
// 哪一侧在这个版本里不存在（新增、删除）就给空文本 —— 那不是错误，是
// 「这一侧本来就没有这个文件」。
func (a *app) loadReview(f gitrepo.File) {
	s := &a.review
	key, path, fp := fileKey(f), pathOf(f), f.Fingerprint
	// 换掉缓冲区之前先把改过的那一份留起来（见 reviewDrafts）。
	a.stashReviewDraft()
	root := a.repoPath
	oldRev, newRev, diskNew, ok := a.reviewRevs(f)
	if !ok {
		a.setupPatchReview(f)
		return
	}
	oldPath, newPath := f.OldPath, f.NewPath

	// 图片走另一条路：没有行可切，两侧读回来的是字节（见 imagereview.go）。
	if isImagePreview(f) {
		a.loadReviewImage(f)
		return
	}

	// 其余二进制文件（压缩包、可执行文件、字体……）没有「行」可画。
	// 这里必须按标记拦下来，不能指望后面那道 utf8 检查：NUL 是合法的
	// UTF-8，二进制内容能整份通过 utf8.ValidString，画出来是一屏乱码。
	if f.Binary {
		*s = reviewState{key: key, fp: fp, path: path}
		s.err = Tf("%s 是二进制文件，改动区没有可显示的内容 —— 文件列表里可以整份暂存或丢弃",
			"%s is a binary file, so there are no lines to show — stage or discard it whole from the file list",
			path)
		return
	}

	*s = reviewState{key: key, fp: fp, path: path, loading: true}

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, root)
		var oldText, newText string
		var ours, theirs, base []byte
		tooLarge := false
		if err == nil {
			oldText, tooLarge = readReviewSide(ctx, repo, oldRev, oldPath, false)
			if newText, tooLarge = readReviewSide(ctx, repo, newRev, newPath, diskNew); !tooLarge {
				tooLarge = len(oldText) > maxReviewBytes || len(newText) > maxReviewBytes
			}
			if f.Status == gitrepo.StatusConflict {
				var worktree []byte
				base, ours, theirs, worktree, err = repo.ConflictContents(ctx, path)
				if err != nil {
					// 读不到三方版本不算致命：改动区还能按「基线 vs 工作区」
					// 画出来，只是挑边的两颗按钮没有底料（保存仍然能用）。
					base, ours, theirs = nil, nil, nil
					newText, tooLarge = readReviewSide(ctx, repo, newRev, newPath, diskNew)
					err = nil
				} else {
					newText = string(worktree)
				}
			}
		}
		a.dispatch(func() {
			if a.review.path != path || a.review.key != key {
				return // 读盘期间又切走了，这份结果作废
			}
			s := &a.review
			s.loading = false
			if err != nil {
				s.err = err.Error()
				return
			}
			if tooLarge {
				// 超大文件整个塞进编辑器会把界面拖死（缓冲区、词法切分、
				// 布局都是按整份文本算的）。说清楚能怎么办，别硬画。
				s.err = Tf("%s 超过 %d MB，改动区不展开它 —— 文件列表里可以整份暂存或丢弃，也可以「编辑代码」打开",
					"%s is over %d MB, so the review pane leaves it out — stage or discard it whole from the file list, or open it with Edit Code",
					path, maxReviewBytes>>20)
				return
			}
			if !utf8.ValidString(oldText) || !utf8.ValidString(newText) {
				s.err = Tf("%s 里有二进制内容，改动区画不了", "%s has binary content the diff can't show", path)
				return
			}
			a.fillReview(f, oldText, newText, ours, theirs, base)
		})
	})
}

// loadReviewImage 读选中图片的两侧字节，解成位图摆进改动区。
//
// 和 loadReview 是同一套「哪两个版本在比」（reviewRevs），只是读回来的是
// 字节、产出的是位图。冲突图片没有可挑的「块」：两侧摆的是我们和对方，
// 怎么落盘还是文件列表里那条整文件动作（或冲突条上的挑一边）。
func (a *app) loadReviewImage(f gitrepo.File) {
	s := &a.review
	key, path, fp := fileKey(f), pathOf(f), f.Fingerprint
	// 图片不可编辑，草稿只可能是之前当文本编辑过的那一份；照样先收起来，
	// 换回来时还在（reviewDrafts 按 key 存）。
	a.stashReviewDraft()
	root := a.repoPath
	oldRev, newRev, diskNew, ok := a.reviewRevs(f)
	if !ok {
		a.setupPatchReview(f)
		return
	}
	oldPath, newPath := f.OldPath, f.NewPath
	conflict := f.Status == gitrepo.StatusConflict

	*s = reviewState{key: key, fp: fp, path: path, loading: true, image: true, conflict: conflict}

	a.run(func() {
		ctx := context.Background()
		repo, err := gitrepo.Open(ctx, root)
		var oldSide, newSide imageSide
		if err == nil {
			if conflict {
				// 冲突的图片读不了「基线 vs 工作区」：工作区里那份通常就是
				// 我们这边，摆出来和左边重复。三方里只取有用的一对。
				_, ours, theirs, _, cerr := repo.ConflictContents(ctx, path)
				if cerr != nil {
					err = cerr
				} else {
					oldSide, newSide = decodeImageSide(ours), decodeImageSide(theirs)
				}
			} else {
				oldData, oldReadErr := readReviewImageSide(ctx, repo, oldRev, oldPath, false)
				newData, newReadErr := readReviewImageSide(ctx, repo, newRev, newPath, diskNew)
				oldSide, newSide = decodeImageSide(oldData), decodeImageSide(newData)
				// 读之前的大小检查失败要单独说：没有数据不是「这一侧没有
				// 这个文件」，是「有，但太大」。读失败（文件不在这个版本里）
				// 继续按空处理，和文本那条路一致。
				if errors.Is(oldReadErr, errImageTooLarge) {
					oldSide = imageSide{err: oversizedImageMessage()}
				}
				if errors.Is(newReadErr, errImageTooLarge) {
					newSide = imageSide{err: oversizedImageMessage()}
				}
			}
		}
		a.dispatch(func() {
			if a.review.path != path || a.review.key != key {
				return // 读盘期间又切走了，这份结果作废
			}
			s := &a.review
			s.loading = false
			if err != nil {
				s.err = err.Error()
				return
			}
			s.imgOld, s.imgOldMeta, s.imgOldErr = oldSide.bitmap, oldSide.meta, oldSide.err
			s.imgNew, s.imgNewMeta, s.imgNewErr = newSide.bitmap, newSide.meta, newSide.err
		})
	})
}

// oversizedImageMessage 是「先查大小发现超限」那句话。decodeImageSide 里
// 也有一份同样意思的说明（读到内容之后才超限），措辞保持一致。
func oversizedImageMessage() string {
	return Tf("图片超过 %s 的预览上限", "The image is over the %s preview limit",
		humanBytes(maxPreviewImageBytes))
}

// maxReviewBytes 是摆进改动区的文件大小上限。
//
// 和以前那个「读进内存做行号换算」的上限同一个数量级：再大的文件（生成的
// 代码、打包产物）切词法、算 diff、排版都要按整份文本算，摆上去只会拖死
// 界面，而用户想看的「改了什么」用文件列表那几行字也能交代。
const maxReviewBytes = 2 << 20

// readReviewSide 读某一侧的原文。rev 为 ":" 开头时当 index，空串当空文件。
// 第二个返回值说「这一侧超过上限」。
func readReviewSide(ctx context.Context, repo *gitrepo.Repo, rev, path string, disk bool) (string, bool) {
	if path == "" {
		return "", false
	}
	if disk {
		data, err := repo.Contents(path)
		if err != nil {
			return "", false
		}
		return string(data), len(data) > maxReviewBytes
	}
	if rev == "" {
		return "", false
	}
	data, err := repo.ContentsAt(ctx, rev, path)
	if err != nil {
		// 这一侧没有这个文件（新增/删除），或这个版本读不出来（空仓库的
		// HEAD）。都当空文本：界面画出来就是「整个文件是新增的」，
		// 和 git 自己的说法一致。
		return "", false
	}
	return string(data), len(data) > maxReviewBytes
}

// reviewRevs 说这个文件的这一段是「哪两个版本在比」。
//
// diskNew 为 true 时新侧要读工作区（磁盘上那一份），否则都从 git 里取。
// ok 为 false 表示两个版本拿不到（PR 检视），调用方按 patch 画。
func (a *app) reviewRevs(f gitrepo.File) (oldRev, newRev string, diskNew, ok bool) {
	switch {
	case a.compare.on:
		return a.compare.base, a.compare.target, false, true
	case a.tab == 1 && a.stashName != "":
		// 储藏的改动不在它那个提交本身里（第一个父是当时的 HEAD），
		// git stash show 比的是 WIP 那次提交和它的第一个父。
		return a.stashName + "^", a.stashName, false, true
	case a.tab == 1:
		if a.commitSel < 0 || a.commitSel >= len(a.commits) {
			return "", "", false, false
		}
		hash := a.commits[a.commitSel].Hash
		return hash + "^", hash, false, true
	case f.Section == gitrepo.SectionStaged:
		return "HEAD", ":0", false, true
	default:
		return ":0", "", true, true
	}
}

// fillReview 拿两侧原文把 DiffView 装起来。只在主线程、读盘回来之后调。
func (a *app) fillReview(f gitrepo.File, oldText, newText string, ours, theirs, base []byte) {
	s := &a.review
	lang := editor.LanguageFor(pathOf(f))
	makeBuffer := func(text string) *editor.Buffer {
		b := editor.NewBuffer(text)
		b.SetLanguage(lang)
		return b
	}

	s.conflict = f.Status == gitrepo.StatusConflict
	s.editable = a.reviewEditable(f)
	if s.conflict {
		s.ours = makeBuffer(string(ours))
		s.theirs = makeBuffer(string(theirs))
		s.base = makeBuffer(string(base))
	}
	if !s.conflict {
		s.old = makeBuffer(oldText)
	} else {
		// 冲突文件的左侧默认是「我们这边」：那正是「用当前分支的」按钮
		// 会写进去的东西，两边说的是同一件事。
		s.old = s.ours
	}
	// 改过还没存的草稿接着用：切走再切回来，刚打的字还在。
	if buf := a.takeReviewDraft(fileKey(f), f.Fingerprint); buf != nil {
		s.new = buf
	} else {
		s.new = makeBuffer(newText)
	}

	s.stageable = a.hunkStageable(f)
	s.hunkIdx = -1
	s.diff = &editor.DiffState{
		Old:          s.old,
		New:          s.new,
		Layout:       a.reviewLayout(),
		Inline:       editor.DiffInlineWordAlt,
		Indicators:   editor.DiffBars,
		ContextLines: 3,
		Editable:     s.editable,
		HunkActions:  s.stageable || s.editable,
	}
	if s.editable {
		// ⌘S 让出来交给 Gloss：DiffView 自己认走的话就没机会保存了
		// （和内置编辑器同一个理由）。
		s.diff.Bindings = []editor.Binding{{Mods: ui.Cmd, Key: ui.KeyS, Command: editor.NoCommand}}
	}
	// regionsVer 用「不可能相等的哨兵」起手：一个从没被改过的缓冲区
	// Version() 就是 0，拿 0 当初值的话第一帧会以为「已经扫过了」。
	s.regions, s.regionsVer = nil, unknownVersion
}

// reviewEditable 说这个文件的右侧能不能改。
//
// 只有未暂存段的右侧是磁盘上那一份，写回去才是「保存」；已暂存段的右侧
// 是 index 里的内容，写回磁盘等于拿 index 盖掉工作区 —— 那不是保存。
// 历史、储藏、对比、PR 检视一份都不许改。
func (a *app) reviewEditable(f gitrepo.File) bool {
	if a.tab != 0 || a.compare.on || a.prReview.on {
		return false
	}
	if f.Section != gitrepo.SectionUnstaged {
		return false
	}
	if f.Binary || f.TooLarge || f.NewPath == "" {
		return false
	}
	return true
}

// reviewLayout 把工具栏那个「左右对照 / 统一」的选择翻译成 DiffView 的布局。
func (a *app) reviewLayout() editor.DiffLayout {
	if a.split {
		return editor.DiffSplit
	}
	return editor.DiffUnified
}

// reviewPane 画右边那一栏：栏头 + （冲突时）挑边条 + diff + （可改时）底栏。
func (a *app) reviewPane(c *ui.Context, pal palette) {
	s := &a.review
	a.reviewHeader(c, pal)

	f := a.reviewTarget()
	if f != nil && s.conflict && (s.diff != nil || s.image) {
		a.reviewConflictBar(c, pal, *f)
	}

	switch {
	case s.loading || (s.diff == nil && !s.image && s.err == ""):
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(8).Children(func() {
				ui.Spinner(c).Size(18, 18)
				ui.Text(c, T("正在读取这个文件…", "Loading file…")).FontSize(textBodySM).
					TextColor(pal.inkMuted)
			})
		})
		return
	case s.err != "":
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Fill().Center().Gap(10).Padding(24).Children(func() {
				headline(c, pal, T("这个文件画不出 diff", "This file's diff can't be shown"))
				ui.Text(c, s.err).Font("monospace").FontSize(textCaption).
					SingleLine().TextColor(pal.inkMuted)
			})
		})
		return
	case s.image:
		if f != nil {
			a.reviewImageBody(c, pal, f)
		}
		return
	}

	s.diff.Layout = a.reviewLayout()
	th := editor.DefaultTheme(c.Theme())
	th.Background = pal.codeBg
	th.Text = pal.code
	s.diff.Theme = th
	s.diff.Font = ui.Font{Family: "monospace", Size: a.codeSizeValue()}
	s.diff.TabSize = tabWidth

	if s.conflict && f != nil {
		a.syncConflictRegions(c, pal)
	} else {
		s.diff.Annotations = nil
		s.diff.RenderAnnotation = nil
	}
	if s.stageable || s.editable {
		// 按 key 认领，不按下标：按下这一帧和回调执行之间列表可能重读过
		// （暂存一下就会），下标指到的就不是同一个文件了。
		key := s.key
		s.diff.RenderHunkActions = func(c *ui.Context, bar ui.Element, act *editor.DiffHunkActions) {
			a.reviewHunkActions(c, pal, bar, key, act)
		}
	} else {
		s.diff.RenderHunkActions = nil
		s.diff.HunkActions = false
	}

	ui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
		editor.DiffView(c, s.diff).Fill()
	})
}

// reviewHeader 是改动区顶上那条：文件路径 + 状态 + 增删 + 整文件暂存 + 已 review。
//
// 原来这些信息散在每个文件的头行上（一份改动里重复出现几十次）；现在一次
// 只显示一个文件，它就固定在这一条上，不再跟着滚。
func (a *app) reviewHeader(c *ui.Context, pal palette) {
	s := &a.review
	t := c.Theme()
	f := a.reviewTarget()
	busy := a.op != gitrepo.OpNone

	ui.Row(c).Height(34).Shrink(0).FillWidth().Padding(0, 12).Gap(8).
		AlignItems(ui.Center).Background(pal.surface2).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
		if f == nil {
			ui.Text(c, T("未选择文件", "No file selected")).FontSize(textBodySM).
				TextColor(pal.inkMuted)
			return
		}
		if f.Status != gitrepo.StatusUntracked {
			specChip(c, pal, string(f.Status.Letter()), statusColor(f.Status, pal), 20, textCaption)
		}
		path := displayPath(*f)
		ui.Text(c, path).Font("monospace").FontSize(textBodySM).Shrink(2).
			SingleLine().TextColor(pal.ink)
		if s.image {
			specChip(c, pal, T("图片", "Image"), ui.Transparent, 20, textCaption)
		}
		ui.Spacer(c)

		if s.diff != nil && !s.patchOnly {
			added, deleted := s.diff.Stats()
			if added > 0 {
				ui.Textf(c, "+%d", added).Font("monospace").FontSize(textCaption).Bold().
					FontFeatures("tnum").TextColor(pal.addBar)
			}
			if deleted > 0 {
				ui.Textf(c, "−%d", deleted).Font("monospace").FontSize(textCaption).Bold().
					FontFeatures("tnum").TextColor(pal.delBar)
			}
		}
		if s.diff != nil && s.new != nil && s.new.Modified() {
			// 底栏的「保存到工作区」撤掉之后，保存这件事只剩 ⌘S；改过的
			// 时候把键位就写在提示里，不用再翻菜单。
			ui.Text(c, T("已编辑 · ⌘S 保存", "Edited · ⌘S to save")).
				FontSize(textCaption).TextColor(t.Warning)
		}

		// 整文件暂存：只有工作区那一段（历史、储藏、对比、PR 检视都不能
		// 拿 index 做什么）。
		if a.tab == 0 && !a.compare.on && !a.prReview.on && f.Section != gitrepo.SectionNone {
			if stageFileButton(c, pal, stageStateOf(*f), busy).Clicked() {
				a.toggleStageFile(*f)
			}
		}

		// 已 review 的勾：一份改动几十个文件时，靠它把看过的划掉。
		viewed := a.isViewed(*f)
		v := viewed
		if ui.Checkbox(c, &v, T("已 review", "Reviewed")).Changed() {
			a.toggleViewed(*f)
		}
	})
}

// reviewConflictBar 是冲突文件顶上那条：说清处境，把整文件的三条出路摆出来。
//
// 逐块的选择在下面的冲突块上（每块一组按钮），这条管的是「整个文件」：
// 全部采用某一边 —— 那是遇到「这个文件我不在乎」时最快的路。
func (a *app) reviewConflictBar(c *ui.Context, pal palette, f gitrepo.File) {
	t := c.Theme()
	act := a.conflictActionsFor(f)
	busy := a.op != gitrepo.OpNone

	ui.Column(c).Shrink(0).FillWidth().Padding(8, 12).Gap(6).
		Background(pal.surface2).
		BorderWidth(0, 0, 1, 0).BorderColor(pal.hairline).Children(func() {
		ui.Text(c, act.note).FontSize(textCaption).TextColor(t.TextMuted)
		ui.Row(c).Wrap().Gap(6).AlignItems(ui.Center).Children(func() {
			if secondaryAction(c, pal, act.ours, act.oursTip, busy).Shrink(0).Clicked() {
				a.resolveFile(f, gitrepo.SideOurs, act.ours)
			}
			if secondaryAction(c, pal, act.theirs, act.theirsTip, busy).Shrink(0).Clicked() {
				a.resolveFile(f, gitrepo.SideTheirs, act.theirs)
			}
			if secondaryAction(c, pal, T("在编辑器打开", "Open in Editor"),
				T("在系统默认编辑器里手工改", "Edit by hand in the system editor"), busy).
				Shrink(0).Clicked() {
				a.openInEditor(f)
			}
			ui.Spacer(c)
			// 图片没有「左侧基线」可选：两边摆的就是两份文件本身，
			// 逐块挑边那套不适用，整份落盘走上面两颗按钮。
			if !a.review.image {
				ui.Text(c, T("左侧基线", "Compare with")).FontSize(textCaption).
					TextColor(pal.inkSubtle)
				a.reviewBaselineButton(c, pal, T("当前分支", "Current"), baselineOurs)
				a.reviewBaselineButton(c, pal, T("对方", "Theirs"), baselineTheirs)
				a.reviewBaselineButton(c, pal, T("共同祖先", "Base"), baselineBase)
			}
			// 底栏那条「保存并标记已解决」撤掉之后，解决完的冲突得有个
			// 落盘的地方：这里是整份文件的收尾动作，摆在基线选择之后。
			// 只有改过才出现 —— 没动过的时候它按下去也是空操作。
			if s := &a.review; s.new != nil && s.new.Modified() {
				if primaryAction(c, pal, T("保存并标记已解决", "Save & Resolve"),
					T("写入工作区并 git add，冲突才算解决", "Write and git add: that's what resolves a conflict"), busy).
					Clicked() {
					a.saveReview()
				}
			}
		})
	})
}

// reviewBaselineButton 切冲突文件左侧的基线。
func (a *app) reviewBaselineButton(c *ui.Context, pal palette, label string, b conflictBaseline) {
	s := &a.review
	sel := s.baseline == b
	btn := ui.ButtonBase(c).Height(22).Shrink(0).Padding(0, 8).Radius(radiusSM).
		Cursor(ui.CursorPointer).Label(label).Tooltip(T("用这份内容作为左侧基线", "Compare against this version"))
	if sel {
		btn.Background(pal.selection).Border(1, pal.hairlineStrong)
	} else if btn.Hovered() {
		btn.Background(pal.surface3).Border(1, pal.hairlineStrong)
	} else {
		btn.Background(ui.Transparent).Border(1, pal.hairline)
	}
	btn.Children(func() { ui.Text(c, label).FontSize(textCaption).TextColor(pal.inkMuted) })
	if btn.Clicked() && !sel {
		a.setReviewBaseline(b)
	}
}

// setReviewBaseline 换冲突文件左侧的基线。
//
// 只换缓冲区引用、不重读：三方版本都在手上（冲突文件本来就要读三份），
// 换基线是「拿哪一份当左侧」的事，动数据反而慢一拍。
func (a *app) setReviewBaseline(b conflictBaseline) {
	s := &a.review
	s.baseline = b
	if buf := s.baselineBuffer(); buf != nil && s.diff != nil {
		s.diff.Old = buf
	}
}

// baselineBuffer 取当前基线对应的缓冲区（冲突文件的左侧）。
func (s *reviewState) baselineBuffer() *editor.Buffer {
	switch s.baseline {
	case baselineTheirs:
		return s.theirs
	case baselineBase:
		return s.base
	}
	return s.ours
}

// syncConflictRegions 重扫冲突块并给每一块挂上操作条。
//
// 每帧都问一次「这一块还在不在」：用户在缓冲区里手工把标记删掉之后，那块
// 的按钮就该跟着消失。重扫只在文本版本变了的时候做（改一次扫一次，不是
// 每帧扫）。
func (a *app) syncConflictRegions(c *ui.Context, pal palette) {
	s := &a.review
	if s.new == nil {
		return
	}
	if v := s.new.Version(); v != s.regionsVer {
		s.regionsVer = v
		s.regions = parseConflictRegions(bufferLines(s.new))
	}
	anns := s.diff.Annotations[:0]
	for i, r := range s.regions {
		anns = append(anns, editor.DiffAnnotation{
			Side: editor.DiffNew, Line: r.start, Height: conflictActionH, Key: i,
		})
	}
	s.diff.Annotations = anns
	if len(s.regions) == 0 {
		s.diff.RenderAnnotation = nil
		return
	}
	s.diff.RenderAnnotation = func(c *ui.Context, an editor.DiffAnnotation) {
		a.reviewRegionActions(c, pal, an)
	}
}

// conflictActionH 是每块冲突下面那条操作条的高度。
const conflictActionH = 26

// unknownVersion 是一个不可能和任何缓冲区版本相等的值：拿它当「这份文本
// 还没扫过」的标记（0 是有效版本号）。
const unknownVersion = ^uint64(0)

// reviewRegionActions 画某一块冲突的操作条：左右两侧各一颗、外加「都要」。
func (a *app) reviewRegionActions(c *ui.Context, pal palette, an editor.DiffAnnotation) {
	s := &a.review
	i, _ := an.Key.(int)
	if i < 0 || i >= len(s.regions) {
		return
	}
	r := s.regions[i]
	oursLabel, theirsLabel := conflictRegionHint(r)

	ui.Row(c).FillWidth().Padding(0, 2).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Textf(c, T("第 %d 处", "Block %d"), i+1).FontSize(textCaption).TextColor(pal.inkSubtle)
		if smallConflictAction(c, pal, Tf("留下 %s", "Keep %s", oursLabel), oursLabel).Clicked() {
			a.applyRegionChoice(i, conflictTakeOurs)
		}
		if smallConflictAction(c, pal, Tf("留下 %s", "Keep %s", theirsLabel), theirsLabel).Clicked() {
			a.applyRegionChoice(i, conflictTakeTheirs)
		}
		if smallConflictAction(c, pal, T("两个都留", "Keep Both"), "").Clicked() {
			a.applyRegionChoice(i, conflictTakeBoth)
		}
		if smallConflictAction(c, pal, T("都不要", "Keep Neither"), "").Clicked() {
			a.applyRegionChoice(i, conflictTakeNone)
		}
	})
}

// smallConflictAction 是冲突块上的一颗小按钮。tip 非空时挂上悬停说明
// （按钮上只写得出侧的名字，说明里把「这是哪一边」补齐）。
func smallConflictAction(c *ui.Context, pal palette, label, tip string) ui.Element {
	btn := ui.ButtonBase(c).Height(20).Shrink(0).Padding(0, 7).Radius(radiusXS).
		Cursor(ui.CursorPointer).Label(label).Background(pal.surface3).
		Border(1, pal.hairline).Tooltip(tip)
	if btn.Hovered() {
		btn.Background(pal.surface4).Border(1, pal.hairlineStrong)
	}
	btn.Children(func() { ui.Text(c, label).FontSize(textCaption).TextColor(pal.inkMuted) })
	return btn
}

// applyRegionChoice 把某一处冲突按选择的结果改掉（只改缓冲区，不动磁盘）。
//
// 落盘和 git add 在冲突条那颗「保存并标记已解决」（或 ⌘S）上：和内置
// 编辑器一个规矩 —— 看一眼结果再决定存不存，撤销（⌘Z）永远回得到刚才那一版。
func (a *app) applyRegionChoice(i int, choice conflictChoice) {
	s := &a.review
	if s.new == nil {
		return
	}
	regions := parseConflictRegions(bufferLines(s.new))
	if i < 0 || i >= len(regions) {
		return
	}
	r := regions[i]
	repl := strings.Join(conflictChoiceLines(r, choice), "\n")
	start := lineStartOffset(s.new, r.start)
	end := lineStartOffset(s.new, r.end+1)
	// 这一块后面还有内容（或者文件本来以换行收尾）时，替换文本自己带一个
	// 换行，免得把下面的行粘上来。
	if end > start && !strings.HasSuffix(repl, "\n") && (r.end+1 < s.new.LineCount() ||
		strings.HasSuffix(s.new.String(), "\n")) {
		repl += "\n"
	}
	s.new.Edit(editor.Edit{Start: start, End: end, Text: repl})
	s.regionsVer = unknownVersion // 强制下一帧重扫
}

// bufferLines 把缓冲区按行取出来（不带末尾那个空行）。
func bufferLines(b *editor.Buffer) []string {
	n := b.LineCount()
	if n == 0 {
		return nil
	}
	lines := make([]string, n)
	for i := range n {
		lines[i] = b.Line(i)
	}
	return lines
}

// lineStartOffset 是第 line 行（0 基）在缓冲区里的字节偏移，越界给文本长度。
func lineStartOffset(b *editor.Buffer, line int) int {
	if line >= b.LineCount() {
		return b.Len()
	}
	off := 0
	for i := range line {
		off += len(b.Line(i)) + 1
	}
	return off
}

// reviewHunkActions 是某一段改动上的两颗按钮：按段暂存 / 按段还原。
//
// key 是「摆这颗按钮时显示的是哪个文件（fileKey）」，不是下标：按钮的回调
// 下一帧才执行，这中间列表可能已经被重读过了（暂存一下就会），按下标回去
// 查会指到别的文件上 —— 那意味着把 A 文件的这一段打进 B 文件的 index。
func (a *app) reviewHunkActions(c *ui.Context, pal palette, bar ui.Element, key string, act *editor.DiffHunkActions) {
	fi := a.fileIndexByKey(key)
	if fi < 0 {
		return // 这个文件已经不在这份列表里了（被提交走、被筛掉）
	}
	bar.Row().Gap(4)
	bar.Children(func() {
		if a.review.stageable {
			reverse := a.fileSection(fi) == gitrepo.SectionStaged
			label := T("暂存这段", "Stage")
			tip := T("把这一段放进暂存区", "Add this change to the index")
			if reverse {
				label = T("取消暂存这段", "Unstage")
				tip = T("把这一段从暂存区里撤出来", "Take this change out of the index")
			}
			if smallConflictAction(c, pal, label, tip).Clicked() {
				a.stageReviewHunk(fi, act.Hunk, reverse)
			}
			return
		}
		if a.review.editable {
			if smallConflictAction(c, pal, T("还原本段", "Revert"),
				T("把这一段改回左侧的样子（还没落盘）", "Put this block back to the left side, in memory only")).Clicked() {
				act.Revert()
			}
		}
	})
}

// fileSection 取第 fi 个文件属于哪一段（越界给零值）。
func (a *app) fileSection(fi int) gitrepo.Section {
	if fi < 0 || fi >= len(a.files) {
		return gitrepo.SectionNone
	}
	return a.files[fi].Section
}

// fileIndexByKey 按 fileKey 找文件在列表里的下标，没有返回 -1。
func (a *app) fileIndexByKey(key string) int {
	if key == "" {
		return -1
	}
	for i := range a.files {
		if fileKey(a.files[i]) == key {
			return i
		}
	}
	return -1
}

// stageReviewHunk 按 DiffView 给出的那一段把改动放进 / 撤出 index。
//
// 不用 hunk ID 那一套（git 自己切的 -U3 段）：DiffView 的段是它按两侧原文
// 算出来的，和 git 的切法不一定一样，所以直接把行范围拼成补丁交给 git
// apply。两侧原文就是屏幕上那两份，点下去打进去的正是看到的东西。
func (a *app) stageReviewHunk(fi int, hr editor.DiffHunk, reverse bool) {
	if fi < 0 || fi >= len(a.files) {
		return
	}
	f := a.files[fi]
	// 屏幕上的两份文本必须是这个文件的：选中换了但还没读完（loading）时
	// 手上那两份是上一个文件的，拿它们生成的补丁会打进别的文件。
	if fileKey(f) != a.review.key {
		return
	}
	if !a.hunkStageable(f) || a.review.old == nil || a.review.new == nil {
		return
	}
	path := pathOf(f)
	oldText, newText := a.review.old.String(), a.review.new.String()
	rng := diff.HunkRange{
		OldStart: hr.OldStart, OldEnd: hr.OldEnd,
		NewStart: hr.NewStart, NewEnd: hr.NewEnd,
	}
	label, kind := T("正在暂存这段改动", "Staging this change"), gitrepo.OpStage
	if reverse {
		label, kind = T("正在取消暂存这段改动", "Unstaging this change"), gitrepo.OpUnstage
	}
	a.startOp(kind, label, func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
		if reverse {
			return "", repo.UnstageRange(ctx, path, oldText, newText, rng)
		}
		return "", repo.StageRange(ctx, path, oldText, newText, rng)
	}, func(bool) {
		// 失败也要重读：行范围对不上说明界面拿的是已经过期的那一份。
		a.reloadAfterOp()
	})
}

// 改动区底下原来还有一条「保存到工作区 / 丢弃」的横条，现在不画了。
//
// 它把「看 diff」和「写 diff」两件事缝在同一条底边上：常驻的说明文字
// （「改完保存到工作区，不替你暂存」）在每次看代码时都在重复同一句话，
// 两颗按钮又只有在真的改过之后才有用。未保存的编辑现在由栏头的
// 「已编辑 · ⌘S 保存」提示，⌘S 落盘（见 keys.go），切走时草稿还在
// （见 takeReviewDraft）—— 少一条横条，diff 多出一整行的视野。

// saveReview 把右侧改好的内容存下去。
//
// 冲突文件走 ResolveManual：写盘 + git add 是一件事（git 那边「解决」的
// 定义就是这个 add）。别的文件只写盘，暂不暂存由用户自己在文件栏里点。
func (a *app) saveReview() {
	s := &a.review
	if s.new == nil || s.diff == nil || a.op != gitrepo.OpNone {
		return
	}
	path := s.path
	text := s.new.String()
	buf := s.new
	conflict := s.conflict

	a.startOp(gitrepo.OpWrite, Tf("正在保存 %s", "Saving %s", path),
		func(ctx context.Context, repo *gitrepo.Repo) (string, error) {
			if conflict {
				if err := repo.ResolveManual(ctx, path, []byte(text)); err != nil {
					return "", err
				}
				return Tf("已解决 %s", "Resolved %s", path), nil
			}
			if err := repo.WriteContents(path, []byte(text)); err != nil {
				return "", err
			}
			return Tf("已保存 %s", "Saved %s", path), nil
		}, func(ok bool) {
			if ok && a.review.new == buf {
				buf.MarkSaved()
			}
			a.reloadAfterOp()
		})
}

// revertReview 丢掉未保存的编辑，重新读一份。
//
// 不在内存里回滚缓冲区：磁盘上的内容可能已经被别处改过（另一个编辑器、
// 终端里的 git），重读顺带把两侧都对齐。
func (a *app) revertReview() {
	f := a.reviewTarget()
	if f == nil {
		return
	}
	a.review.key = ""
	a.review.fp = ""
	a.loadReview(*f)
}

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 改动区：一次一个文件，用 mygo 的 DiffView 画。
//
// 这些测试盯的是「换掉渲染方式之后，原来那些行为还成不成立」：选中哪个文件
// 显示哪个、按段暂存、冲突逐块挑边、未暂存段能改能存。渲染细节（词级高亮、
// 折叠、滚动）归 mygo 自己的测试，这里只管 Gloss 接上去的那一层。

// settleReview 让改动区把选中那个文件真的读出来。
//
// 读盘是渲染那一帧发起的（syncReview 在 view 里），所以顺序是「跑一帧 →
// 等后台 → 再跑一帧」。
func settleReview(t *testing.T, a *app) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	settle(t, a)
	tt.Frame()
	settle(t, a)
	return tt
}

// waitReview 等改动区把当前选中的文件装好（跑几帧、等后台）。
//
// 「装好」有三种收尾：文本 diff、图片预览、加载失败（二进制文件之类）。
// 三者之外继续等，免得把「读不出来」误当成「还没读完」。
func waitReview(t *testing.T, a *app, tt *ui.Tester) {
	t.Helper()
	for range 6 {
		tt.Frame()
		settle(t, a)
		f := a.reviewTarget()
		done := a.review.diff != nil || a.review.image || a.review.err != ""
		if f != nil && a.review.key == fileKey(*f) && !a.review.loading && done {
			// 再画一帧：读盘是在上一帧之后完成的，就绪的画面得这一帧才有。
			tt.Frame()
			return
		}
	}
	t.Fatalf("改动区一直没装上：key=%q loading=%v err=%q", a.review.key, a.review.loading, a.review.err)
}

// reviewText 是改动区当前这份 diff 的两侧文本（拼在一起，方便断言「看得见
// 什么」）。代码是 DiffView 自己画的，不是 ui.Text 元素，按屏幕文字找不到它。
func reviewText(a *app) string {
	if a.review.old == nil || a.review.new == nil {
		return ""
	}
	return a.review.old.String() + "\n" + a.review.new.String()
}

// 改动区默认显示第一个文件，栏头写着它的路径和增删统计。
func TestReviewShowsFirstFile(t *testing.T) {
	a := richApp(t)
	tt := settleReview(t, a)
	if a.review.key == "" {
		t.Fatalf("改动区没装上：%q", a.review.err)
	}
	if a.review.key != fileKey(a.files[0]) {
		t.Errorf("默认该显示第一个文件 %s，实际 %s", fileKey(a.files[0]), a.review.key)
	}
	if !tt.HasText(displayPath(a.files[0])) {
		t.Errorf("栏头该写清在看哪个文件：%q", tt.Texts())
	}
}

// 两侧原文都从 git 里读：未暂存段是 index vs 工作区，已暂存段是 HEAD vs index。
//
// 读错基线的后果是静默的 —— 屏幕上还是像模像样的 diff，只是比错了对象。
func TestReviewReadsBothSides(t *testing.T) {
	a := richApp(t)
	f := listedFile(t, a, "README.md")
	a.selectFileKey(fileKey(f))
	settleReview(t, a)

	if a.review.new == nil || a.review.old == nil {
		t.Fatalf("两侧都该有缓冲区：%q", a.review.err)
	}
	if got := a.review.new.String(); !strings.Contains(got, "新的一行") {
		t.Errorf("右侧该是工作区那份：\n%s", got)
	}
	if got := a.review.old.String(); strings.Contains(got, "新的一行") {
		t.Errorf("左侧该是 index 那份（这次改动还没暂存过，index 里还是旧的）：\n%s", got)
	}

	// 暂存之后再读：右侧换成 index 那一份，左侧换成 HEAD 那一份。
	runGit(t, a.repoPath, "add", "README.md")
	a.reload()
	settle(t, a)
	staged := listedFile(t, a, "README.md")
	if staged.Section != gitrepo.SectionStaged {
		t.Fatalf("暂存之后该在已暂存段，实际 %v", staged.Section)
	}
	a.selectFileKey(fileKey(staged))
	settleReview(t, a)
	if got := a.review.new.String(); !strings.Contains(got, "新的一行") {
		t.Errorf("已暂存段的右侧该是 index：\n%s", got)
	}
	if got := a.review.old.String(); !strings.Contains(got, "旧的一行") {
		t.Errorf("已暂存段的左侧该是 HEAD 那一份：\n%s", got)
	}
}

// 按段暂存：DiffView 给的那一段，点一下只把这一段放进 index。
func TestReviewStageHunk(t *testing.T) {
	a := richApp(t)
	a.selectFileKey(fileKey(listedFile(t, a, "README.md")))
	settleReview(t, a)

	hunks := a.review.diff.Hunks()
	if len(hunks) == 0 {
		t.Fatal("夹具该有改动段")
	}
	// README 是整块改的：暂存这一段之后，工作区那份不该再出现在未暂存段。
	a.stageReviewHunk(a.fileSel, hunks[0], false)
	settle(t, a)

	if got := indexContents(t, a.repoPath, "README.md"); !strings.Contains(got, "新的一行") {
		t.Errorf("index 里该有这一段改动：%q", got)
	}
	if f := listedFile(t, a, "README.md"); f.Section != gitrepo.SectionStaged {
		t.Errorf("暂存之后该只剩已暂存那一段，实际 %v", f.Section)
	}
}

// 取消暂存：已暂存段的那一段退回 index 之外（工作区不动）。
func TestReviewUnstageHunk(t *testing.T) {
	a := richApp(t)
	runGit(t, a.repoPath, "add", "README.md")
	a.reload()
	settle(t, a)

	staged := listedFile(t, a, "README.md")
	if staged.Section != gitrepo.SectionStaged {
		t.Fatalf("夹具该把 README 暂存起来，实际 %v", staged.Section)
	}
	a.selectFileKey(fileKey(staged))
	settleReview(t, a)

	hunks := a.review.diff.Hunks()
	if len(hunks) == 0 {
		t.Fatal("夹具该有改动段")
	}
	a.stageReviewHunk(a.fileSel, hunks[0], true)
	settle(t, a)

	if got := indexContents(t, a.repoPath, "README.md"); strings.Contains(got, "新的一行") {
		t.Errorf("取消暂存后 index 该回到 HEAD 那一份：%q", got)
	}
	if got := readDisk(t, a.repoPath+"/README.md"); !strings.Contains(got, "新的一行") {
		t.Errorf("取消暂存不该动工作区：%q", got)
	}
}

// countHunks 数一份 diff 里有几段改动。
//
// 按行首的 @@ 数，不能直接数 "@@ " 这个子串 —— 带函数名的 hunk 头里
// （`@@ -3,7 +3,7 @@ line 2: some code here`）它出现两次。
func countHunks(diffText string) int {
	n := 0
	for _, line := range strings.Split(diffText, "\n") {
		if strings.HasPrefix(line, "@@ ") {
			n++
		}
	}
	return n
}

// diffHoverX 是「在改动区正文里扫」用的那个 x：落在左边那一栏的代码上，
// 具体是哪一栏不重要 —— 重要的是别落在行号槽或分隔条上。
const diffHoverX = float32(900)

// hoverDiffBody 把鼠标在改动区正文里扫一遍，直到某处让 expect 那串字出现，
// 返回那一处按钮的位置。
//
// DiffView 的按段按钮只挂在「指针底下那一段」上（mygo 的 HunkActions：
// 指针或光标所在的那一段才摆按钮），而每一段在屏幕上的位置取决于字号、行高
// 和前面折起来多少行。测试里不去复算这套排版，扫一遍找它 —— 找到了就等于
// 证明「这一段在界面上真点得到」。
func hoverDiffBody(t *testing.T, tt *ui.Tester, expect string) (ui.Rect, bool) {
	t.Helper()
	for y := float32(150); y < 640; y += 3 {
		tt.Move(diffHoverX, y)
		tt.Frame()
		if r, ok := tt.Find(expect); ok {
			return r, true
		}
	}
	return ui.Rect{}, false
}

// clickHunkAction 点某一段改动上那颗按钮（「暂存这段」/「取消暂存这段」）。
//
// 按钮只挂在指针底下那一段上，而它画在那一段的右端：从代码区一步跳到按钮
// 中心，这一跳就换了「指针底下是哪一段」，按钮会先收起来 —— 测试里得像真人
// 一样把指针分几步挪过去再按。r 是 hoverDiffBody 找到的那个位置。
func clickHunkAction(t *testing.T, tt *ui.Tester, r ui.Rect) {
	t.Helper()
	x, y := r.X+r.W/2, r.Y+r.H/2
	const steps = 4
	for i := 1; i <= steps; i++ {
		tt.Move(diffHoverX+(x-diffHoverX)*float32(i)/steps, y)
		tt.Frame()
	}
	tt.Press(x, y)
	tt.Frame()
	tt.Release(x, y)
	tt.Frame()
}

// focusDiffBody 在改动区正文里点一处，把焦点交给右边的编辑器。
//
// DiffView 自己画文字（不是 ui.Text 元素），按坐标点；横向试几个位置直到
// 打字真的进了缓冲区 —— 布局一动，正好的那个 x 也跟着动，写死一个数只会让
// 这条测试在某次改版之后莫名其妙地红。
func focusDiffBody(t *testing.T, tt *ui.Tester, a *app) {
	t.Helper()
	for _, x := range []float32{620, 700, 780, 860, 940, 1020, 1100} {
		tt.ClickAt(x, 200)
		tt.Frame()
		tt.Type("x")
		tt.Frame()
		if a.review.new.Modified() {
			return
		}
	}
	t.Fatal("点进改动区正文之后打字该进到这个文件的缓冲区里")
}

// 按段暂存：鼠标停在某一段上，那一颗「暂存这段」点下去只把这一段放进 index，
// 同一个文件别的那一段留在工作区 —— 这是按段暂存的全部意义。
func TestReviewHunkButtonStagesOnlyThatHunk(t *testing.T) {
	a := richApp(t)
	a.fileFilter = "handler"
	a.applyFilter()
	tt := settleReview(t, a)

	if f := listedFile(t, a, "internal/server/handler.go"); len(f.Hunks) != 2 {
		t.Fatalf("夹具该有两段改动，实际 %d 段", len(f.Hunks))
	}
	btn, ok := hoverDiffBody(t, tt, "暂存这段")
	if !ok {
		t.Fatal("改动上该出现「暂存这段」按钮")
	}
	clickHunkAction(t, tt, btn)
	settle(t, a)
	tt.Frame()

	const path = "internal/server/handler.go"
	cached := gitOut(t, a.repoPath, "diff", "--cached", "--", path)
	if n := countHunks(cached); n != 1 {
		t.Errorf("index 里该只有一段改动，实际 %d 段：\n%s", n, cached)
	}
	work := gitOut(t, a.repoPath, "diff", "--", path)
	if n := countHunks(work); n != 2 {
		t.Errorf("这个文件在工作区该还剩两段，实际 %d 段：\n%s", n, work)
	}
	// 三段加起来还是原来那三段，没被按段暂存弄丢或重复。
	all := cached + work
	for _, want := range []string{"line 3: CHANGED AGAIN", "新加的一行", "也改了"} {
		if !strings.Contains(all, want) {
			t.Errorf("改动 %q 该还在 index 或工作区里：\n%s", want, all)
		}
	}
}

// 按段取消暂存：反向把 index 里那一段拿回工作区，别的那段不受连坐。
func TestReviewHunkButtonUnstagesOnlyThatHunk(t *testing.T) {
	a := richApp(t)
	runGit(t, a.repoPath, "add", "internal/server/handler.go")
	a.reload()
	settle(t, a)
	a.fileFilter = "handler"
	a.applyFilter()
	tt := settleReview(t, a)

	if f := listedFile(t, a, "internal/server/handler.go"); f.Section != gitrepo.SectionStaged || len(f.Hunks) != 2 {
		t.Fatalf("夹具该把两段都暂存起来，实际 %v 共 %d 段", f.Section, len(f.Hunks))
	}
	btn, ok := hoverDiffBody(t, tt, "取消暂存这段")
	if !ok {
		t.Fatal("已暂存段的改动上该出现「取消暂存这段」按钮")
	}
	clickHunkAction(t, tt, btn)
	settle(t, a)
	tt.Frame()

	const path = "internal/server/handler.go"
	cached := gitOut(t, a.repoPath, "diff", "--cached", "--", path)
	if n := countHunks(cached); n != 2 {
		t.Errorf("index 里该还剩两段，实际 %d 段：\n%s", n, cached)
	}
	work := gitOut(t, a.repoPath, "diff", "--", path)
	if n := countHunks(work); n != 1 {
		t.Errorf("撤出来的那一段该回到工作区，实际 %d 段：\n%s", n, work)
	}
}

// 冲突文件：右侧是带着标记的工作区文本，左侧是「当前分支」那一份，并且能改。
func TestReviewShowsConflictInline(t *testing.T) {
	a, _ := mergeConflictApp(t)
	a.selectFileKey(fileKey(listedFile(t, a, "a.txt")))
	tt := settleReview(t, a)

	s := &a.review
	if !s.conflict || s.diff == nil {
		t.Fatalf("冲突文件该按冲突画：%q", s.err)
	}
	if !s.diff.Editable {
		t.Error("冲突的解法就是改右侧那一份，它必须可编辑")
	}
	if len(s.regions) == 0 {
		t.Error("该把工作区里的冲突块解析出来")
	}
	// 顶部那条说清处境，并把整文件的三条出路摆出来。
	if !tt.HasText("标记为已解决") {
		t.Errorf("冲突文件顶上该有整文件的挑边按钮：%q", tt.Texts())
	}
}

// 保存并标记已解决：写盘 + git add，冲突就算解了。
func TestReviewConflictSaveResolves(t *testing.T) {
	a, dir := mergeConflictApp(t)
	a.selectFileKey(fileKey(listedFile(t, a, "a.txt")))
	settleReview(t, a)

	a.review.new.Set("手工写好的一行\n")
	a.saveReview()
	settle(t, a)

	if got := readDisk(t, dir+"/a.txt"); got != "手工写好的一行\n" {
		t.Errorf("磁盘上是 %q", got)
	}
	// 直接问 git：解决的定义就是「ls-files -u 里没有它了」。
	if got := gitOut(t, dir, "ls-files", "-u", "--", "a.txt"); got != "" {
		t.Errorf("保存之后不该还有未合并条目：%q", got)
	}
}

// 在改动区里改过的字，切到另一个文件再切回来还在（草稿按 fileKey + 指纹存）。
//
// 改动区是个真的编辑器，顺手改两笔是很自然的事；切文件换缓冲区时把刚打的
// 字默默扔掉，是那种「一次就够让人不敢再用」的体验。
func TestReviewKeepsUnsavedEditsAcrossFileSwitch(t *testing.T) {
	a := richApp(t)
	first := listedFile(t, a, "README.md")
	second := listedFile(t, a, "internal/server/handler.go")

	a.selectFileKey(fileKey(first))
	tt := settleReview(t, a)
	a.review.new.Set("改了一半\n")
	tt.Frame()

	a.selectFileKey(fileKey(second))
	waitReview(t, a, tt)
	if a.review.key != fileKey(second) {
		t.Fatalf("该换到第二个文件了：%s", a.review.key)
	}
	if got := a.review.new.String(); strings.Contains(got, "改了一半") {
		t.Errorf("第二个文件不该看到第一个文件的草稿：\n%s", got)
	}

	a.selectFileKey(fileKey(first))
	waitReview(t, a, tt)
	if got := a.review.new.String(); !strings.Contains(got, "改了一半") {
		t.Errorf("切回来该接着改：\n%s", got)
	}
}

// 内容变了的文件不接旧草稿：指纹对不上说明磁盘上那份已经不是当初那一版，
// 拿旧草稿顶上去是骗人（保存下去会覆盖别人的改动）。
func TestReviewDraftDroppedWhenContentChanges(t *testing.T) {
	a := richApp(t)
	f := listedFile(t, a, "README.md")
	a.selectFileKey(fileKey(f))
	tt := settleReview(t, a)
	a.review.new.Set("改了一半\n")

	// 界面之外把文件改掉（指纹随即变化）
	writeFile(t, filepath.Join(a.repoPath, "README.md"), "# 别人写的\n")
	a.reload()
	settle(t, a)
	a.selectFileKey(fileKey(listedFile(t, a, "README.md")))
	waitReview(t, a, tt)

	if got := a.review.new.String(); strings.Contains(got, "改了一半") {
		t.Errorf("内容变了之后不该再摆旧草稿：\n%s", got)
	}
	if got := a.review.new.String(); !strings.Contains(got, "别人写的") {
		t.Errorf("该显示磁盘上那一份：\n%s", got)
	}
}

// 改动区里的 ⌘S：右侧改过就存下来（和底栏那颗按钮走同一条路）。
func TestReviewSaveShortcut(t *testing.T) {
	a := richApp(t)
	path := filepath.Join(a.repoPath, "README.md")
	a.selectFileKey(fileKey(listedFile(t, a, "README.md")))
	tt := settleReview(t, a)

	focusDiffBody(t, tt, a) // 焦点进改动区
	a.review.new.Set("⌘S 存的\n")
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyS)
	settle(t, a)

	if got := readDisk(t, path); got != "⌘S 存的\n" {
		t.Errorf("⌘S 该把这一份写回工作区，磁盘上是 %q", got)
	}
}

// 超大文件不摆进编辑器：缓冲区、词法切分、排版都按整份文本算，几 MB 的文件
// 摆上去只会拖死界面。拦下来并说清能怎么办，比画到一半卡住好。
func TestReviewSkipsHugeFile(t *testing.T) {
	dir := newRepoDir(t)
	big := strings.Repeat("x\n", 1_200_000) // 2.4 MB，超过 maxReviewBytes
	writeFile(t, filepath.Join(dir, "big.txt"), big)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "big")
	writeFile(t, filepath.Join(dir, "big.txt"), big+"最后加的一行\n")

	a := loadApp(t, dir)
	settleReview(t, a)

	if !strings.Contains(a.review.err, "超过") {
		t.Fatalf("超大文件该被拦下并说清原因，实际 err = %q", a.review.err)
	}
	if a.review.diff != nil {
		t.Error("拦下来之后不该还给这么大的文件建 DiffView")
	}
}

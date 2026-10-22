package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 图片预览：改动区和内置编辑器都按文本行画，二进制文件到那里只剩一句
// 「画不了」。这几条测试盯的是「换成图片之后，前后两侧和尺寸说明真的
// 摆上去了」—— PNG 是代码里生成的，不依赖任何外部素材。

// imageBytes 生成一张 w×h 的 PNG。用两种颜色让体积/内容不像空白图，
// 解码出来也确实是想要的那个尺寸。
func imageBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			if (x/8+y/8)%2 == 0 {
				img.Set(x, y, color.RGBA{R: 80, G: 140, B: 220, A: 255})
			} else {
				img.Set(x, y, color.RGBA{R: 30, G: 30, B: 36, A: 255})
			}
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成 PNG: %v", err)
	}
	return buf.Bytes()
}

// writeImage 把一张 PNG 写进仓库（目录不存在就建）。
func writeImage(t *testing.T, repoDir, rel string, w, h int) {
	t.Helper()
	full := filepath.Join(repoDir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("建目录: %v", err)
	}
	if err := os.WriteFile(full, imageBytes(t, w, h), 0o644); err != nil {
		t.Fatalf("写图片: %v", err)
	}
}

// reloadTestApp 重读仓库模型 —— 相当于文件系统变了之后界面刷新一次。
func reloadTestApp(t *testing.T, a *app) {
	t.Helper()
	repo, err := gitrepo.Open(context.Background(), a.repoPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	m, err := repo.Load(context.Background())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	a.applyModel(m)
	settle(t, a)
}

// screenText 把这一帧屏幕上的文字拼起来，用来断言「看得见什么」。
func screenText(tt *ui.Tester) string {
	return strings.Join(tt.Texts(), "\n")
}

// 未跟踪的图片：文件模型里是二进制（git 不认识它），改动区要摆出「新文件」
// 那张图，而不是「二进制内容画不了」。
func TestReviewPreviewsUntrackedImage(t *testing.T) {
	a := testApp(t)
	writeImage(t, a.repoPath, "assets/logo.png", 24, 16)
	reloadTestApp(t, a)

	f := listedFile(t, a, "assets/logo.png")
	if !f.Binary {
		t.Errorf("未跟踪的 PNG 应该标成二进制（没有可读的行），实际 Binary=false")
	}
	a.selectFileKey(fileKey(f))
	tt := settleReview(t, a)
	waitReview(t, a, tt)

	s := a.review
	if !s.image {
		t.Fatalf("改动区该按图片预览画，实际 image=false，err=%q", s.err)
	}
	if s.imgNew == nil {
		t.Fatalf("新侧图片没解出来：%q", s.imgNewErr)
	}
	if s.imgOld != nil {
		t.Errorf("未跟踪文件没有旧的一侧，不该解出图来")
	}
	if w, h := s.imgNew.Size(); w != 24 || h != 16 {
		t.Errorf("解出来的尺寸是 %d×%d，want 24×16", w, h)
	}
	if !strings.Contains(s.imgNewMeta, "PNG") || !strings.Contains(s.imgNewMeta, "24 × 16") {
		t.Errorf("说明里该有格式和尺寸，实际 %q", s.imgNewMeta)
	}
	text := screenText(tt)
	if !strings.Contains(text, "assets/logo.png") {
		t.Errorf("栏头该写清在看哪张图：%q", text)
	}
	if !strings.Contains(text, T("新文件", "New file")) {
		t.Errorf("新增的图该标「新文件」：%q", text)
	}
	if strings.Contains(text, T("画不了", "can't show")) {
		t.Errorf("图片不该再落到「画不了」那句：%q", text)
	}
}

// 已跟踪的图片被改：两侧都要摆出来，尺寸各按各的版本。
func TestReviewPreviewsBothSidesOfImage(t *testing.T) {
	a := testApp(t)
	writeImage(t, a.repoPath, "assets/logo.png", 20, 10)
	runGit(t, a.repoPath, "add", "assets/logo.png")
	runGit(t, a.repoPath, "commit", "-m", "add logo")

	// 换成另一个尺寸，前后两侧一眼能分出来。
	writeImage(t, a.repoPath, "assets/logo.png", 30, 12)
	reloadTestApp(t, a)

	f := listedFile(t, a, "assets/logo.png")
	if !f.Binary {
		t.Errorf("git 的二进制 diff 该被识别出来，实际 Binary=false")
	}
	a.selectFileKey(fileKey(f))
	tt := settleReview(t, a)
	waitReview(t, a, tt)

	s := a.review
	if s.imgOld == nil || s.imgNew == nil {
		t.Fatalf("两侧都该有图：oldErr=%q newErr=%q err=%q", s.imgOldErr, s.imgNewErr, s.err)
	}
	if w, h := s.imgOld.Size(); w != 20 || h != 10 {
		t.Errorf("旧的一侧是 %d×%d，want 20×10（HEAD 里的那一版）", w, h)
	}
	if w, h := s.imgNew.Size(); w != 30 || h != 12 {
		t.Errorf("新的一侧是 %d×%d，want 30×12（工作区那一版）", w, h)
	}
	text := screenText(tt)
	for _, want := range []string{"20 × 10", "30 × 12", T("改动前", "Before"), T("改动后", "After")} {
		if !strings.Contains(text, want) {
			t.Errorf("屏幕上该有 %q：%q", want, text)
		}
	}
}

// 非图片的二进制文件仍然是「画不了」，但不要再说成「二进制内容」那么含糊 ——
// 这条只确认它没有走图片预览。
func TestReviewKeepsBinaryFileMessage(t *testing.T) {
	a := testApp(t)
	blob := append([]byte{0x00, 0x01, 0x02, 0x03}, bytes.Repeat([]byte{0x7f}, 64)...)
	if err := os.WriteFile(filepath.Join(a.repoPath, "blob.bin"), blob, 0o644); err != nil {
		t.Fatal(err)
	}
	reloadTestApp(t, a)

	f := listedFile(t, a, "blob.bin")
	a.selectFileKey(fileKey(f))
	tt := settleReview(t, a)
	waitReview(t, a, tt)

	s := a.review
	if s.image {
		t.Fatalf("blob.bin 不是图片，不该走图片预览")
	}
	if !strings.Contains(s.err, "二进制") {
		t.Errorf("该说明这是二进制内容画不了，实际 %q", s.err)
	}
}

// 编辑器视图：左边选到图片时右边摆画面，不建文本缓冲区（保存/还原因此
// 一直是灰的，见 editorToolbar 的 ready）。
func TestEditorPanePreviewsImage(t *testing.T) {
	a := testApp(t)
	writeImage(t, a.repoPath, "assets/logo.png", 40, 20)
	reloadTestApp(t, a)

	a.setViewMode(viewModeEditor)
	settle(t, a)
	selectEditorFile(t, a, "assets/logo.png")
	settle(t, a)

	s := a.editorPane
	if s.img == nil {
		t.Fatalf("编辑器视图该把图片解出来，实际 err=%q", s.err)
	}
	if s.code != nil || s.buffer != nil {
		t.Errorf("图片不该建文本缓冲区（code=%v buffer=%v）", s.code != nil, s.buffer != nil)
	}
	if w, h := s.img.Size(); w != 40 || h != 20 {
		t.Errorf("尺寸是 %d×%d，want 40×20", w, h)
	}

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	tt.Frame()
	text := screenText(tt)
	if !strings.Contains(text, "assets/logo.png") {
		t.Errorf("编辑器栏头该写清文件：%q", text)
	}
	if !strings.Contains(text, T("图片", "Image")) {
		t.Errorf("图片该挂一个「图片」标记，和可编辑的文本区分开：%q", text)
	}
}

// 右键「编辑代码」点开一张图：弹的是预览，不是「不是可编辑的文本文件」。
func TestCodeEditorDialogPreviewsImage(t *testing.T) {
	a := testApp(t)
	writeImage(t, a.repoPath, "assets/logo.png", 12, 8)
	reloadTestApp(t, a)

	a.openCodeEditor(listedFile(t, a, "assets/logo.png"))
	settle(t, a)

	s := a.codeEdit
	if !s.on {
		t.Fatalf("对话框该打开")
	}
	if s.img == nil {
		t.Fatalf("对话框该摆出图片，实际 err=%q", s.err)
	}
	if s.code != nil {
		t.Errorf("图片不该有文本缓冲区")
	}
	if w, h := s.img.Size(); w != 12 || h != 8 {
		t.Errorf("尺寸是 %d×%d，want 12×8", w, h)
	}

	tt := ui.NewTester(a.view, 1280, 860)
	for range 3 {
		tt.Frame()
	}
	if text := screenText(tt); !strings.Contains(text, "图片预览") {
		t.Errorf("对话框标题该是「图片预览」：%q", text)
	}
}

// 路径识别只认栅格图；SVG 是文本，走 diff，不抢它的路。
func TestIsPreviewImage(t *testing.T) {
	yes := []string{"a.png", "A.PNG", "dir/b.jpg", "c.jpeg", "d.gif", "e.webp", "f.bmp"}
	for _, p := range yes {
		if !isImagePreview(gitrepo.File{FileDiff: diff.FileDiff{NewPath: p}}) {
			t.Errorf("%s 该认成图片", p)
		}
	}
	no := []string{"a.svg", "a.txt", "a.go", "a.png.bak", "png"}
	for _, p := range no {
		if isImagePreview(gitrepo.File{FileDiff: diff.FileDiff{NewPath: p}}) {
			t.Errorf("%s 不该认成图片", p)
		}
	}
}

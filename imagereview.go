package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/diff"
	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 图片预览。
//
// 改动区和内置编辑器都是按「文本行」画的：一张 PNG 进去只会得到一句
// 「二进制文件画不了」。可图片的改动最该看的恰恰是画面本身，别的 Git GUI
// 也都是直接把前后两张摆出来。这里补上这一步 —— 认出图片、把两侧的字节
// 解成位图，剩下摆在哪由 review / 编辑器各自决定。
//
// 只认栅格图（PNG / JPEG / GIF / WebP / BMP）。SVG 是文本，diff 本身就能读，
// mygo 的 DecodeBitmap 也不收它。

const (
	// maxPreviewImageBytes 是单张图片的字节上限。解码是整份进内存的
	// （mygo 的 Bitmap 一份 RGBA、解码器还有一份中间结果），几十 MB
	// 的图已经很容易把一次切文件拖成一次卡顿。
	maxPreviewImageBytes = 32 << 20

	// maxPreviewImagePixels 是像素数上限。压缩率高的图可以很小却解出几亿
	// 像素（纯色的大 PNG 就是），字节数拦不住它，得单独按面积拦。
	maxPreviewImagePixels = 24_000_000
)

// errImageTooLarge 说这一侧超过预览上限。读之前先查大小，避免为了报一句
// 「太大」把几十 MB 读进内存。
var errImageTooLarge = errors.New("image is over the preview limit")

// imageSide 是图片预览的一侧：解好的位图 + 一行说明（格式、尺寸、体积）。
//
// err 非空表示「这一侧有内容，但画不出来」——文件损坏、格式不认识、超过
// 上限。一侧解不开不该把另一侧也藏起来，所以错误跟着侧走。
type imageSide struct {
	bitmap *ui.Bitmap
	meta   string
	err    string
}

// decodeImageSide 把原始字节解成能摆上屏幕的一侧。
func decodeImageSide(data []byte) imageSide {
	if len(data) == 0 {
		return imageSide{}
	}
	if len(data) > maxPreviewImageBytes {
		return imageSide{err: Tf("图片有 %s，超过 %s 的预览上限",
			"The image is %s, over the %s preview limit",
			humanBytes(len(data)), humanBytes(maxPreviewImageBytes))}
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return imageSide{err: T("这个文件解不出图片（可能已损坏，或不是支持的格式）",
			"The file doesn't decode as an image (it may be damaged, or in an unsupported format)")}
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return imageSide{err: T("图片尺寸读不出来", "The image's size can't be read")}
	}
	if cfg.Width*cfg.Height > maxPreviewImagePixels {
		return imageSide{err: Tf("%d × %d 像素，超过 %d 万像素的预览上限",
			"%d × %d pixels, over the %d-megapixel preview limit",
			cfg.Width, cfg.Height, maxPreviewImagePixels/1_000_000)}
	}
	bm, err := ui.DecodeBitmap(data)
	if err != nil {
		return imageSide{err: T("图片解码失败", "The image failed to decode")}
	}
	w, h := bm.Size()
	meta := fmt.Sprintf("%s · %d × %d · %s", strings.ToUpper(format), w, h, humanBytes(len(data)))
	if format == "gif" {
		// 动图只画第一帧：预览不是播放器，把整段解码进内存换不来
		// 多少信息。说清楚是首帧，免得有人以为图「没动」。
		meta += T(" · 首帧", " · first frame")
	}
	return imageSide{bitmap: bm, meta: meta}
}

// readReviewImageSide 读某一侧的原始字节。disk 为 true 时读工作区。
//
// 读之前先问大小（工作区用 stat，git 里用 cat-file -s）：超过上限的图
// 连读都不读，直接交给界面上那句「太大」。
func readReviewImageSide(ctx context.Context, repo *gitrepo.Repo, rev, path string, disk bool) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	if disk {
		full := filepath.Join(repo.Root, path)
		if info, err := os.Stat(full); err == nil && info.Size() > maxPreviewImageBytes {
			return nil, errImageTooLarge
		}
		return repo.Contents(path)
	}
	if rev == "" {
		return nil, nil
	}
	if size, err := repo.SizeAt(ctx, rev, path); err == nil && size > maxPreviewImageBytes {
		return nil, errImageTooLarge
	}
	return repo.ContentsAt(ctx, rev, path)
}

// humanBytes 把字节数写成一眼看得懂的一行。
func humanBytes(n int) string {
	f := float64(n)
	switch {
	case f >= 1<<20:
		return trimFraction(fmt.Sprintf("%.1f", f/(1<<20))) + " MB"
	case f >= 1<<10:
		return trimFraction(fmt.Sprintf("%.1f", f/(1<<10))) + " KB"
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// trimFraction 去掉「12.0」那个没意义的 .0。
func trimFraction(s string) string {
	return strings.TrimSuffix(s, ".0")
}

// reviewImageLabels 给两侧配标题。新增/删除只有一侧，标题就写「新文件」
// 「已删除」；冲突的图片摆的是我们和对方两份。
func reviewImageLabels(f *gitrepo.File) (oldLabel, newLabel string) {
	oldLabel, newLabel = T("改动前", "Before"), T("改动后", "After")
	if f == nil {
		return
	}
	switch {
	case f.Status == gitrepo.StatusConflict:
		return T("我们这边", "Ours"), T("对方那边", "Theirs")
	case f.Status == gitrepo.StatusAdded, f.Status == gitrepo.StatusUntracked:
		return "", T("新文件", "New file")
	case f.Status == gitrepo.StatusDeleted:
		return T("已删除", "Deleted"), ""
	}
	return
}

// reviewImageBody 画改动区的图片预览：两侧各一栏，各自带格式、尺寸和体积。
func (a *app) reviewImageBody(c *ui.Context, pal palette, f *gitrepo.File) {
	s := &a.review
	oldLabel, newLabel := reviewImageLabels(f)

	ui.Row(c).Grow(1).FillWidth().MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		showOld := s.imgOld != nil || s.imgOldErr != ""
		showNew := s.imgNew != nil || s.imgNewErr != ""
		if showOld {
			imagePanel(c, pal, oldLabel, imageSide{bitmap: s.imgOld, meta: s.imgOldMeta, err: s.imgOldErr})
		}
		if showOld && showNew {
			ui.Box(c).Width(1).FillHeight().Shrink(0).Background(pal.hairline)
		}
		if showNew {
			imagePanel(c, pal, newLabel, imageSide{bitmap: s.imgNew, meta: s.imgNewMeta, err: s.imgNewErr})
		}
		if !showOld && !showNew {
			ui.Column(c).Grow(1).Fill().Center().Padding(24).Children(func() {
				ui.Text(c, T("这个版本里没有可以预览的图片", "There's no image to preview in this version")).
					FontSize(textBodySM).TextColor(pal.inkMuted)
			})
		}
	})
}

// imagePanel 是一栏图片：顶上一条标题（标题 + 格式尺寸），下面一块
// 棋盘格底（透明区域能看出来）的预览。
func imagePanel(c *ui.Context, pal palette, label string, side imageSide) {
	ui.Column(c).Grow(1).MinWidth(0).MinHeight(0).Children(func() {
		if label != "" || side.meta != "" {
			ui.Row(c).Height(30).Shrink(0).FillWidth().Padding(0, 12).Gap(8).
				AlignItems(ui.Center).Children(func() {
				if label != "" {
					// 单行：栏窄的时候让它自己截断，别折成两行把 30px 的
					// 标题条撑破（截图里那半个「改」字就是这么来的）。
					ui.Text(c, label).FontSize(textCaption).FontWeight(weightSemibold).
						SingleLine().Shrink(0).TextColor(pal.inkMuted)
				}
				if side.meta != "" {
					ui.Text(c, side.meta).FontSize(textCaption).TextColor(pal.inkSubtle).
						SingleLine().Shrink(2)
				}
				ui.Spacer(c)
			})
		}
		ui.Box(c).Grow(1).MinHeight(0).FillWidth().Padding(10).Children(func() {
			if side.bitmap == nil {
				ui.Column(c).Fill().Center().Padding(16).Children(func() {
					text := side.err
					if text == "" {
						text = T("这一侧没有图片", "There's no image on this side")
					}
					ui.Text(c, text).FontSize(textBodySM).TextColor(pal.inkMuted).
						TextAlign(ui.Center)
				})
				return
			}
			ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
				p.Clip(r, radiusMD, func() {
					drawCheckerboard(p, r, pal)
					const pad = 10
					inner := ui.Rect{
						X: r.X + pad, Y: r.Y + pad,
						W: max(r.W-2*pad, 1), H: max(r.H-2*pad, 1),
					}
					p.Image(side.bitmap, inner, ui.Contain)
				})
				p.Stroke(r, pal.hairline, radiusMD, 1)
			})
		})
	})
}

// drawCheckerboard 画透明图的标准棋盘格底。
//
// 底色取代码区的底色、隔格取「底色掺一点正文色」：深色下抬高一档、浅色下
// 压深一档，同一套公式两边都成立，不用按明暗各写一组常量。
func drawCheckerboard(p *ui.Painter, r ui.Rect, pal palette) {
	const cell = 12
	base := pal.codeBg
	alt := base.Mix(pal.ink, 0.055)
	p.Fill(r, base, 0)
	for iy := 0; ; iy++ {
		y := r.Y + float32(iy*cell)
		if y >= r.Y+r.H {
			break
		}
		h := min(float32(cell), r.Y+r.H-y)
		for ix := iy % 2; ; ix += 2 {
			x := r.X + float32(ix*cell)
			if x >= r.X+r.W {
				break
			}
			w := min(float32(cell), r.X+r.W-x)
			p.Fill(ui.Rect{X: x, Y: y, W: w, H: h}, alt, 0)
		}
	}
}

// isImagePreview 说这个文件该不该按图片画。
//
// 只看扩展名，不看 f.Binary：未跟踪的图片在文件模型里也是二进制（没有
// hunk），而已跟踪的图片由 git 自己标成二进制 —— 两种都要画。PR 检视只有
// patch 文本、拿不到字节，那条路在 setupPatchReview 里单独说明。
func isImagePreview(f gitrepo.File) bool {
	return diff.IsImagePath(pathOf(f))
}

// looksBinary 说这份内容不该进编辑器：git 的二进制判据（NUL），或者不是
// 合法 UTF-8。后者连 GBK 之类的文本也会拦下来 —— 编辑器按 UTF-8 解，
// 摆上去是一屏替换字符，不如直接说「不是文本文件」。
func looksBinary(data []byte) bool {
	return diff.IsBinaryContent(data) || !utf8.Valid(data)
}

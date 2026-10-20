package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// 提交详情卡：点了一个提交之后，文件列表顶上那张「这个提交是谁、什么时候、
// 哪一串哈希、信息写了什么」。
//
// 摆在文件列表那一栏顶上，不另开一个面板：用户点提交的那一刻想确认的
// 就是「这是我要的那个吗」，而答案就在眼皮底下那份文件列表旁边。
//
// 卡里每行都是**文本**，而且整张卡是一个 Selectable 容器 —— 哈希、邮箱
// 这些要复制的东西拖一下选中、⌘C 就进剪贴板，不必为每一项单开一个
// 「复制」菜单。

// commitAvatar 是作者头像的直径。
const commitAvatar = 34

// commitMsgMaxH 是提交信息正文那一块的最高度。
//
// 一封写得长的提交信息能把整个文件列表顶出屏幕，所以封顶并允许内部滚动：
// 想看全文的滚一下就行，不想看的也不会被它挤没。
const commitMsgMaxH = 72

// commitDetailVisible 问现在该不该画这张卡。
//
// 条件一个都不能少：在历史 tab、选中了一个提交、没在看储藏。储藏那一屏
// 下半区是另一个东西（储藏的改动），摆一张提交的详情在那儿是张冠李戴。
//
// 对比分支和 PR 检视两屏同理 —— 它们霸占着下半区，那时文件列表里列的不是
// 这个提交的改动，卡上写着「这个提交」就是错的。
func (a *app) commitDetailVisible() bool {
	if a.compare.on || a.prReview.on {
		return false
	}
	return a.tab == 1 && a.stashRev == "" &&
		a.commitSel >= 0 && a.commitSel < len(a.commits)
}

// commitDetailPane 画这张卡。cm 是选中的那个提交。
func (a *app) commitDetailPane(c *ui.Context, pal palette, cm gitrepo.Commit) {
	t := c.Theme()

	// 整张卡一个选区：拖动选中时可以连着邮箱、哈希一起选，⌘C 出去是三行。
	ui.Column(c).Shrink(0).Selectable().
		Background(pal.surface1).Children(func() {
		// 头像 + 名字 + 时间，居中。作者名是这一屏里最该被认出来的东西，
		// 摆中间比塞在行首和一行元信息混在一起好认。
		ui.Box(c).Shrink(0).Padding(12, 12, 10, 12).Children(func() {
			ui.Column(c).FillWidth().Gap(3).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(commitAvatar, commitAvatar).Shrink(0).Radius(commitAvatar/2).
					Background(pal.surface2).Border(1, pal.hairline).Center().
					Children(func() {
						drawIcon(c, iconUser, commitAvatar/2, pal.inkSubtle)
					})
				ui.Text(c, cm.Author).FontSize(13).FontWeight(weightSemibold).
					SingleLine().MaxWidth(240).TextColor(pal.ink)
				ui.Text(c, commitWhen(cm.When)).FontSize(11).
					SingleLine().TextColor(t.TextMuted)
			})
		})

		// 元信息三行：邮箱、哈希、网页链接。图标 12，和 11px 的字排在一行
		// 不抢宽度 —— 这一栏只有 300（split 下 220）宽。
		ui.Column(c).Shrink(0).Padding(0, 12).Gap(4).Children(func() {
			if cm.Email != "" {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					drawIcon(c, iconMail, 12, pal.inkSubtle)
					ui.Text(c, cm.Email).FontSize(11).SingleLine().
						Grow(1).MinWidth(0).TextColor(pal.inkMuted)
				})
			}
			// 哈希行可点：点一下复制完整那一串。短哈希摆在这儿是给人认的，
			// 要粘到终端里的是完整那个（cherry-pick、revert 都吃长的）。
			hashRow := ui.Row(c).Gap(6).AlignItems(ui.Center).
				Cursor(ui.CursorPointer)
			hashRow.Children(func() {
				ui.Text(c, "#").FontSize(11).FontWeight(weightSemibold).
					TextColor(pal.inkSubtle)
				ui.Text(c, cm.Short).Font("monospace").FontSize(11).
					Grow(1).MinWidth(0).TextColor(pal.inkMuted)
				ui.Text(c, T("点击复制", "Click to copy")).FontSize(10).
					Shrink(0).TextColor(pal.inkSubtle)
			})
			hashRow.Tooltip(T("复制完整提交哈希", "Copy full commit hash")).
				Label(T("复制完整提交哈希", "Copy full commit hash"))
			if hashRow.Clicked() && cm.Hash != "" {
				a.copyText = cm.Hash
			}

			// 网页链接。走 app 的 openURL 钩子（和「在编辑器打开」「在访达
			// 中显示」一个套路：main 接上 Shell.OpenExternal，测试里是 nil），
			// 不用 ui.Link —— 那一个自己找 host 开浏览器，绕过这个钩子，
			// 于是同一个 App 有两条开网页的路。
			if url, label := a.commitWebURL(cm); url != "" {
				link := ui.Row(c).Gap(6).AlignItems(ui.Center).
					Cursor(ui.CursorPointer).Role(ui.RoleLink)
				link.Children(func() {
					drawIcon(c, iconLink, 12, pal.inkSubtle)
					txt := ui.Text(c, label).FontSize(11).SingleLine().
						Grow(1).MinWidth(0).TextColor(pal.inkMuted)
					if link.Hovered() {
						txt.Underline()
					}
				})
				link.Tooltip(url).Label(label)
				if link.Clicked() && a.openURL != nil {
					a.openURL(url)
				}
			}
		})
		ui.Box(c).Shrink(0).Height(10)

		// 提交信息。默认摊开（那是用户点这个提交时最想读的东西），收起状态
		// 记在 commitMsgClosed 里 —— 和文件树的 filesClosed 一个套路：
		// 记的是「偏离默认的那一次」，来回切提交不会越攒越多。
		ui.Box(c).Shrink(0).Height(1).FillWidth().
			Background(pal.hairline)
		ui.Box(c).Shrink(0).Padding(0, 12, 8, 12).Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				head := ui.Row(c).Shrink(0).Gap(4).AlignItems(ui.Center).
					Cursor(ui.CursorPointer)
				head.Children(func() {
					drawIcon(c, iconChevronDown, 12, pal.inkSubtle)
					ui.Text(c, T("提交信息", "Message")).FontSize(11).
						FontWeight(weightSemibold).TextColor(pal.inkSubtle)
				})
				if head.Clicked() {
					a.commitMsgClosed = !a.commitMsgClosed
				}
				if !a.commitMsgClosed {
					ui.Scroll(c).MaxHeight(commitMsgMaxH).FillWidth().Children(func() {
						ui.Column(c).FillWidth().Gap(4).Children(func() {
							ui.Text(c, cm.Subject).FontSize(12).TextColor(pal.ink)
							if cm.Body != "" {
								ui.Text(c, cm.Body).FontSize(11).TextColor(t.TextMuted)
							}
						})
					})
				}
			})
		})
	})
}

// commitWhen 是详情卡里那个绝对时间。
//
// 列表行给的是「3 小时前」（找的是「今天早上那个提交」），卡里反过来给绝对
// 时间：已经点开一个提交看了，要的是「它到底发生在哪一刻」，相对时间回答
// 不了这个。
func commitWhen(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	if IsEnglish() {
		return t.Format("2 Jan 2006 15:04")
	}
	return t.Format("2006 年 1 月 2 日 15:04")
}

// commitWebURL 是「在网页上看这个提交」的地址和链接上的字。
//
// 认不出来的远程（本地路径、file://、自建 GitLab 子域名之外的怪 host）返回
// 空串 —— 那一行不画，比画一个点进去 404 的链接强。
func (a *app) commitWebURL(cm gitrepo.Commit) (string, string) {
	if cm.Hash == "" {
		return "", ""
	}
	for _, name := range a.remotes {
		repo, ok := parseWebRepo(a.remoteURLs[name])
		if !ok {
			continue
		}
		url := repo.commitURL(cm.Hash)
		return url, Tf("在 %s 上查看", "View on %s", repo.display)
	}
	return "", ""
}

// webRepo 是一个远程仓库在网页上的地址。
type webRepo struct {
	host    string // "gitee.com"
	path    string // "owner/repo"
	display string // 链接上写的名字："Gitee" / "GitHub" / host 本身
}

// parseWebRepo 把 git remote 里配的 URL 拆成网页上的仓库地址。
//
// 三种写法都要认：
//
//	git@github.com:owner/repo.git            （ssh 的 scp 简写，最常见）
//	ssh://git@github.com/owner/repo.git
//	https://github.com/owner/repo.git
//
// 认不出来（本地路径、file://、git-daemon）就返回 false。
func parseWebRepo(raw string) (webRepo, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return webRepo{}, false
	}
	var host, path string
	switch {
	case strings.Contains(s, "://"):
		s = s[strings.Index(s, "://")+3:]
		// 去掉 user@ 和可能的端口
		if at := strings.Index(s, "@"); at >= 0 {
			s = s[at+1:]
		}
		host, path, _ = strings.Cut(s, "/")
	case strings.Contains(s, "@") && strings.Contains(s, ":"):
		// scp 简写：user@host:path。这里没有 "://"，冒号是分隔符
		s = s[strings.Index(s, "@")+1:]
		host, path, _ = strings.Cut(s, ":")
	default:
		return webRepo{}, false
	}
	host = strings.TrimSpace(host)
	if i := strings.Index(host, ":"); i >= 0 { // 端口
		host = host[:i]
	}
	path = strings.Trim(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" || !strings.Contains(path, "/") {
		return webRepo{}, false
	}
	// 路径里再出现冒号说明这一行不是 URL（scp 简写的端口写在 host 后面，
	// 形如 host:2222:owner/repo 的写法 git 自己不认）
	if strings.Contains(path, ":") {
		return webRepo{}, false
	}
	return webRepo{host: host, path: path, display: hostName(host)}, true
}

// commitURL 是这个仓库里一个提交的网页地址。
//
// GitLab 的提交路径中间多一段 /-/（它把仓库底下的页面都挂在 /-/ 底下），
// GitHub 和 Gitee 都是 /owner/repo/commit/<hash>。
func (r webRepo) commitURL(hash string) string {
	if strings.Contains(r.host, "gitlab") {
		return "https://" + r.host + "/" + r.path + "/-/commit/" + hash
	}
	return "https://" + r.host + "/" + r.path + "/commit/" + hash
}

// hostName 把 host 换成链接上那个名字：常见的几个写好，其余原样用 host。
func hostName(host string) string {
	switch host {
	case "github.com":
		return "GitHub"
	case "gitee.com":
		return "Gitee"
	case "gitlab.com":
		return "GitLab"
	case "bitbucket.org":
		return "Bitbucket"
	}
	return host
}

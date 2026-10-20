package main

import (
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 「在网页上看这个提交」那个链接：从 git remote 里配的 URL 拼出来。
//
// 只测这个纯函数 —— 链接点在界面上会真去开浏览器，那是 ui.Link 的事。

func TestParseWebRepo(t *testing.T) {
	cases := []struct {
		raw  string
		host string
		path string
		ok   bool
	}{
		// ssh 的 scp 简写：私有仓库最常见的一种
		{"git@gitee.com:carl/gloss.git", "gitee.com", "carl/gloss", true},
		{"git@github.com:owner/repo.git", "github.com", "owner/repo", true},
		{"ssh://git@github.com/owner/repo.git", "github.com", "owner/repo", true},
		{"https://github.com/owner/repo.git", "github.com", "owner/repo", true},
		{"http://github.com/owner/repo", "github.com", "owner/repo", true},
		{"https://gitlab.com/group/sub/repo.git", "gitlab.com", "group/sub/repo", true},
		{"git@git.example.com:2222:owner/repo.git", "git.example.com", "2222/owner/repo", false},
		{"ssh://git@host.example.com:2222/owner/repo.git", "host.example.com", "owner/repo", true},
		// 认不出来的一律 false：那一行不画，比画一个点进去 404 的链接强
		{"", "", "", false},
		{"/Users/carl/repos/gloss", "", "", false},
		{"file:///Users/carl/repos/gloss", "", "", false},
		{"git://github.com/owner/repo.git", "github.com", "owner/repo", true},
	}
	for _, c := range cases {
		got, ok := parseWebRepo(c.raw)
		if ok != c.ok {
			t.Errorf("parseWebRepo(%q) ok = %v, want %v", c.raw, ok, c.ok)
			continue
		}
		if !c.ok {
			continue
		}
		if got.host != c.host || got.path != c.path {
			t.Errorf("parseWebRepo(%q) = %s / %s, want %s / %s", c.raw, got.host, got.path, c.host, c.path)
		}
	}
}

func TestWebRepoCommitURL(t *testing.T) {
	gh, _ := parseWebRepo("git@github.com:owner/repo.git")
	if got, want := gh.commitURL("abc123"), "https://github.com/owner/repo/commit/abc123"; got != want {
		t.Errorf("GitHub: %q, want %q", got, want)
	}
	ge, _ := parseWebRepo("git@gitee.com:carl/gloss.git")
	if got, want := ge.commitURL("abc123"), "https://gitee.com/carl/gloss/commit/abc123"; got != want {
		t.Errorf("Gitee: %q, want %q", got, want)
	}
	// GitLab 的提交路径中间多一段 /-/
	gl, _ := parseWebRepo("https://gitlab.com/group/repo.git")
	if got, want := gl.commitURL("abc123"), "https://gitlab.com/group/repo/-/commit/abc123"; got != want {
		t.Errorf("GitLab: %q, want %q", got, want)
	}
}

func TestHostName(t *testing.T) {
	for host, want := range map[string]string{
		"github.com":      "GitHub",
		"gitee.com":       "Gitee",
		"gitlab.com":      "GitLab",
		"bitbucket.org":   "Bitbucket",
		"git.example.com": "git.example.com", // 自建的：原样用 host
	} {
		if got := hostName(host); got != want {
			t.Errorf("hostName(%q) = %q, want %q", host, got, want)
		}
	}
}

// 详情卡该出现的时候出现，不该出现的时候不出现。
func TestCommitDetailVisible(t *testing.T) {
	a := graphApp(t)
	if a.commitDetailVisible() {
		t.Fatalf("还没切到历史 tab 就不该有详情卡")
	}
	a.setTab(1)
	settle(t, a)
	if !a.commitDetailVisible() {
		t.Fatalf("历史 tab 选中了提交，该有详情卡（commitSel=%d, commits=%d）",
			a.commitSel, len(a.commits))
	}
	// 看储藏时下半区是储藏的改动，摆一张提交的详情在那儿是张冠李戴
	a.stashRev = "refs/stash"
	if a.commitDetailVisible() {
		t.Errorf("看储藏时不该有提交详情卡")
	}
}

// 卡里的字真的画在了屏幕上，而且提交信息默认是摊开的。
func TestCommitDetailPaneRenders(t *testing.T) {
	a := graphApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	a.setTab(1)
	settle(t, a)
	tt.Frame()
	tt.Frame()

	cm := a.commits[a.commitSel]
	for _, want := range []string{cm.Author, cm.Email, cm.Short, "提交信息"} {
		if !tt.HasText(want) {
			t.Errorf("详情卡里找不到 %q：%q", want, tt.Texts())
		}
	}
}

// detailApp 是一个带远程（Gitee）、带提交正文的仓库。
func detailApp(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	runGit(t, dir, "init", "-b", "main")
	runGit(t, dir, "config", "user.email", "seulbi@example.com")
	runGit(t, dir, "config", "user.name", "Seulbi")
	runGit(t, dir, "remote", "add", "origin", "git@gitee.com:carl/gloss.git")
	writeFile(t, filepath.Join(dir, "README.md"), "# hi\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	writeFile(t, filepath.Join(dir, "README.md"), "# hi\n\n第二段\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "给 README 补一段说明\n\n这一段是正文：解释为什么补、补的是什么。\n\n第二段正文。")

	a := loadApp(t, dir)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	tt.Frame()
	a.setTab(1)
	settle(t, a)
	tt.Frame()
	tt.Frame()
	return a, tt
}

// 正文要照着作者排的版画出来，不能只给第一行。
func TestCommitDetailShowsBody(t *testing.T) {
	a, tt := detailApp(t)
	cm := a.commits[a.commitSel]
	if cm.Body == "" {
		t.Fatalf("这个仓库的提交该有正文")
	}
	for _, want := range []string{"这一段是正文：解释为什么补、补的是什么。", "第二段正文。"} {
		if !tt.HasText(want) {
			t.Errorf("正文里找不到 %q：%q", want, tt.Texts())
		}
	}
}

// 认得出的远程给一条「在网页上看」，点它把地址交给 openURL 钩子。
func TestCommitDetailWebLink(t *testing.T) {
	a, tt := detailApp(t)
	if !tt.HasText("在 Gitee 上查看") {
		t.Fatalf("配了 gitee 远程，该有那条链接：%q", tt.Texts())
	}

	var opened string
	a.openURL = func(u string) { opened = u }
	if err := tt.Click("在 Gitee 上查看"); err != nil {
		t.Fatalf("点链接：%v", err)
	}
	settle(t, a)
	tt.Frame()
	if want := "https://gitee.com/carl/gloss/commit/" + a.commits[a.commitSel].Hash; opened != want {
		t.Errorf("打开的是 %q，want %q", opened, want)
	}

	// 换成一个本地路径远程：链接不该出现，也不能画出一个点进去 404 的
	a2 := graphApp(t)
	a2.remotes = []string{"origin"}
	a2.remoteURLs = map[string]string{"origin": "/Users/carl/repos/gloss"}
	tt2 := ui.NewTester(a2.view, 1280, 900)
	a2.setTab(1)
	settle(t, a2)
	tt2.Frame()
	tt2.Frame()
	if tt2.HasText("上查看") {
		t.Errorf("本地路径远程不该给出网页链接：%q", tt2.Texts())
	}
}

// 点哈希那一行 = 复制完整哈希。界面上摆的是短哈希，要粘去终端的是长的。
func TestCommitDetailHashRowCopies(t *testing.T) {
	a, tt := detailApp(t)
	cm := a.commits[a.commitSel]
	// 按标签点，不按哈希文本点：提交列表行尾也有一列同样的短哈希，按文本
	// 点会落到列表行上去（那一列的点击是「选中这个提交」）。
	if err := tt.Click("复制完整提交哈希"); err != nil {
		t.Fatalf("点哈希行：%v", err)
	}
	settle(t, a)
	tt.Frame()

	// 剪贴板里是完整那一串：界面上摆的短哈希是给人认的，要粘去终端的是长的
	if got := tt.Clipboard(); got != cm.Hash {
		t.Errorf("剪贴板里是 %q，want 完整哈希 %q", got, cm.Hash)
	}
}

// 提交信息那一块能收起来：一封写得长的提交信息不该把文件列表顶出屏幕。
func TestCommitDetailMessageCollapses(t *testing.T) {
	a, tt := detailApp(t)
	if a.commitMsgClosed {
		t.Fatalf("默认该是摊开的")
	}
	if err := tt.Click("提交信息"); err != nil {
		t.Fatalf("点「提交信息」：%v", err)
	}
	settle(t, a)
	tt.Frame()
	if !a.commitMsgClosed {
		t.Errorf("点了该收起")
	}
	if tt.HasText("这一段是正文：解释为什么补、补的是什么。") {
		t.Errorf("收起来了正文不该还在")
	}
	// 再点摊开
	if err := tt.Click("提交信息"); err != nil {
		t.Fatalf("再点「提交信息」：%v", err)
	}
	settle(t, a)
	tt.Frame()
	if a.commitMsgClosed {
		t.Errorf("再点该摊开")
	}
}

// 改动 tab 不画这张卡：那儿没有「选中的提交」可讲。
func TestCommitDetailHiddenOnChangesTab(t *testing.T) {
	a, _ := detailApp(t)
	a.setTab(0)
	settle(t, a)
	if a.commitDetailVisible() {
		t.Errorf("改动 tab 不该有提交详情卡")
	}
}

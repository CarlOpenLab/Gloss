package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/gitrepo"
)

// gitConfigApp 搭好「Git 配置」这一页的场景：一个开着仓库的 app、一个
// 渲染器，外加一条干净的全局配置路径。
//
// 三层配置全指到临时文件上：这一页会**写**全局配置，不指走就是改跑测试
// 这台机器的 ~/.gitconfig。runGit 护不住这里 —— 它只管自己起的那个进程，
// 而写入是 Gloss 自己跑的 git。
func gitConfigApp(t *testing.T) (*app, *ui.Tester, string) {
	t.Helper()
	globalPath := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", globalPath)
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(t.TempDir(), "gitconfig"))

	a := richApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	tt.Frame()
	settle(t, a)
	return a, tt, globalPath
}

// openGitConfigTab 打开设置面板并切到「Git 配置」页，等那一趟读跑完。
func openGitConfigTab(t *testing.T, a *app, tt *ui.Tester) {
	t.Helper()
	a.openSettings()
	tt.Frame()
	settle(t, a)
	tt.Frame()
	if err := tt.Click("Git 配置"); err != nil {
		t.Fatalf("点侧栏的 Git 配置：%v", err)
	}
	tt.Frame()
	// 读配置是异步的：点进这一页才发起，名单要到下一帧才成形。
	settle(t, a)
	tt.Frame()
}

// groupOf 取某一栏（顺序即显示顺序：全局、当前项目、系统级）。
func groupOf(t *testing.T, a *app, scope string) *gitConfigGroup {
	t.Helper()
	for _, g := range a.gitCfg.groups {
		if g.Scope == scope {
			return g
		}
	}
	t.Fatalf("没有 %s 这一栏", scope)
	return nil
}

// gitConfigValue 从文件里读一条配置 —— 走 git 自己读，不信我们这边的解析。
func gitConfigValue(t *testing.T, dir, scope, key string) (string, bool) {
	t.Helper()
	args := []string{"config", "--" + scope, "--get", key}
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// typeInto 把框里的内容换成 text：先点中、全选、再输入。
//
// 两个都不能省：tt.Type 只往当前焦点送字（不点中就没焦点），而且是往光标处
// **插入**（不全选就成了追加）。
func typeInto(t *testing.T, tt *ui.Tester, label, text string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("点 %s：%v", label, err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type(text)
	tt.Frame()
}

// saveGitConfig 点「保存」并等落盘跑完。
//
// 保存会把设置面板关掉（applySettings 的收尾），所以之后再要点这一页的东西，
// 得重新打开 —— 这里顺手代劳，测试里就能连着写「改一项、存、再改一项」。
func saveGitConfig(t *testing.T, a *app, tt *ui.Tester) {
	t.Helper()
	if err := tt.Click("保存"); err != nil {
		t.Fatalf("点保存：%v", err)
	}
	settle(t, a)
	tt.Frame()
	if !a.settingsOn {
		openGitConfigTab(t, a, tt)
	}
}

// 三栏都摆出来，而且只有身份那两项：分支、远程这类结构性配置不上这一页 ——
// 它们在别处有正经入口，摊在这儿当普通键值改最容易出事。
func TestGitConfigPaneListsIdentityOnly(t *testing.T) {
	a, tt, _ := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	for _, want := range []string{
		"Git 配置",
		"全局配置",
		"当前项目",
		"系统级",
		"只读",
		"身份来源",
		"跟随全局",
		"单独设置",
		"姓名",
		"邮箱",
	} {
		if !tt.HasText(want) {
			t.Errorf("页面上没有 %q", want)
		}
	}

	// git init 写下的结构性配置在这儿：读得到（说明读的时候站在仓库里），
	// 但不该出现在界面上。
	local := groupOf(t, a, gitrepo.ScopeLocal)
	if !strings.Contains(local.Files[0], filepath.Join(".git", "config")) {
		t.Errorf("项目级的来源文件不对：%q", local.Files[0])
	}
	for _, unwanted := range []string{"core.repositoryformatversion", "core.bare", "core.filemode", "commit.gpgsign"} {
		if tt.HasText(unwanted) {
			t.Errorf("结构性配置 %q 不该出现在这一页", unwanted)
		}
	}
	if _, ok := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, "core.repositoryformatversion"); !ok {
		t.Fatal("仓库里本来有 core.repositoryformatversion，测试场景不对")
	}

	for _, scope := range []string{gitrepo.ScopeGlobal, gitrepo.ScopeLocal} {
		g := groupOf(t, a, scope)
		if len(g.Rows) != 2 {
			t.Fatalf("%s 这一栏该只有姓名和邮箱两项，实际 %d 项：%+v", scope, len(g.Rows), g.Rows)
		}
		for _, r := range g.Rows {
			if !isIdentityKey(r.Key) {
				t.Errorf("%s 栏里混进了 %s", scope, r.Key)
			}
		}
	}
}

// 项目级那份本来设了自己的身份 ⇒ 这一栏默认是「单独设置」，值就是文件里那份。
func TestGitConfigProjectOwnIdentityIsDefault(t *testing.T) {
	a, tt, _ := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	local := groupOf(t, a, gitrepo.ScopeLocal)
	if local.InheritGlobal {
		t.Fatal("仓库里本来有 user.name / user.email，不该默认成跟随全局")
	}
	if a.identityRowLocal(t, "user.name").Value != "Test" {
		t.Errorf("项目级 user.name 该是文件里那份，实际 %q", a.identityRowLocal(t, "user.name").Value)
	}
}

// 全局身份直接在框里改，保存落到全局配置文件上。
func TestGitConfigEditsGlobalIdentity(t *testing.T) {
	a, tt, globalPath := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	typeInto(t, tt, "全局配置 user.name", "Carl")
	typeInto(t, tt, "全局配置 user.email", "carl@example.com")
	saveGitConfig(t, a, tt)

	if v, ok := gitConfigValue(t, "", gitrepo.ScopeGlobal, "user.name"); !ok || v != "Carl" {
		t.Errorf("全局 user.name 不对：%q（存在 %v）", v, ok)
	}
	if v, ok := gitConfigValue(t, "", gitrepo.ScopeGlobal, "user.email"); !ok || v != "carl@example.com" {
		t.Errorf("全局 user.email 不对：%q（存在 %v）", v, ok)
	}
	if _, err := os.Stat(globalPath); err != nil {
		t.Errorf("全局配置文件没建起来：%v", err)
	}
	// 项目级那份没被顺手改掉。
	if v, _ := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, "user.name"); v != "Test" {
		t.Errorf("只是改全局，项目级不该跟着变：%q", v)
	}
}

// 项目级切到「跟随全局」再保存：.git/config 里那两项被删掉，提交改用全局那份。
func TestGitConfigProjectInheritsGlobal(t *testing.T) {
	a, tt, _ := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	typeInto(t, tt, "全局配置 user.name", "Carl")
	typeInto(t, tt, "全局配置 user.email", "carl@example.com")

	if err := tt.Click("跟随全局"); err != nil {
		t.Fatalf("点「跟随全局」：%v", err)
	}
	tt.Frame()
	// 还没保存：文件里的项目级身份得原封不动。
	if v, _ := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, "user.name"); v != "Test" {
		t.Fatalf("还没保存就把项目级身份删了：%q", v)
	}
	// 跟随状态下摆的是生效的那份 —— 也就是刚在全局栏填的。
	if !tt.HasText("Carl") {
		t.Error("跟随全局时该摆出生效的全局身份")
	}
	// 切回「单独设置」：原来那份要原样回来（这条只是来回切，不保存）。
	if err := tt.Click("单独设置"); err != nil {
		t.Fatalf("点「单独设置」：%v", err)
	}
	tt.Frame()
	if got := a.identityRowLocal(t, "user.name").Value; got != "Test" {
		t.Errorf("切回单独设置该恢复原来的值，实际 %q", got)
	}
	if err := tt.Click("跟随全局"); err != nil {
		t.Fatalf("再点「跟随全局」：%v", err)
	}
	tt.Frame()

	saveGitConfig(t, a, tt)

	if v, ok := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, "user.name"); ok {
		t.Errorf("跟随全局之后项目级不该还有 user.name：%q", v)
	}
	if v, ok := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, "user.email"); ok {
		t.Errorf("跟随全局之后项目级不该还有 user.email：%q", v)
	}
	if v, _ := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, "commit.gpgsign"); v != "false" {
		t.Errorf("别的项目级配置不该被带走：%q", v)
	}
	if v, ok := gitConfigValue(t, "", gitrepo.ScopeGlobal, "user.name"); !ok || v != "Carl" {
		t.Errorf("全局身份该还在：%q（存在 %v）", v, ok)
	}

	// 保存之后重新读回来：.git/config 里已经没有身份项了，这一栏就该是
	// 「跟随全局」，值由全局那份顶着。
	if got := groupOf(t, a, gitrepo.ScopeLocal); !got.InheritGlobal {
		t.Error("项目级身份已经删掉了，重新读进来该是跟随全局")
	}
	if v, _ := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, "user.name"); v != "" {
		t.Errorf("文件里不该再有项目级身份：%q", v)
	}
}

// 项目级「单独设置」：填的值只落到这个仓库的 .git/config。
func TestGitConfigProjectOwnIdentity(t *testing.T) {
	a, tt, _ := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	// 先跟随全局，让项目级那两项变成待删；再切回单独设置改成别的人。
	if err := tt.Click("跟随全局"); err != nil {
		t.Fatalf("点「跟随全局」：%v", err)
	}
	tt.Frame()
	if err := tt.Click("单独设置"); err != nil {
		t.Fatalf("点「单独设置」：%v", err)
	}
	tt.Frame()

	typeInto(t, tt, "项目配置 user.name", "Repo Bot")
	typeInto(t, tt, "项目配置 user.email", "bot@example.com")
	saveGitConfig(t, a, tt)

	if v, ok := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, "user.name"); !ok || v != "Repo Bot" {
		t.Errorf("项目级 user.name 不对：%q（存在 %v）", v, ok)
	}
	if v, ok := gitConfigValue(t, "", gitrepo.ScopeGlobal, "user.name"); ok {
		t.Errorf("只改项目级，全局那份不该被写：%q", v)
	}
}

// 把一项清空再保存 = 从这一层删掉它（不是写成空字符串，那会让 git 拿空名字
// 去提交）。留空之后接着生效的是下面那层。
func TestGitConfigClearingFieldRemovesKey(t *testing.T) {
	a, tt, _ := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	typeInto(t, tt, "全局配置 user.name", "Carl")
	saveGitConfig(t, a, tt)
	if _, ok := gitConfigValue(t, "", gitrepo.ScopeGlobal, "user.name"); !ok {
		t.Fatal("准备阶段没写进全局 user.name")
	}

	// 清空：点中框、全选、删掉。
	if err := tt.Click("全局配置 user.name"); err != nil {
		t.Fatalf("点全局 user.name：%v", err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(0, ui.KeyBackspace)
	tt.Frame()
	saveGitConfig(t, a, tt)

	if v, ok := gitConfigValue(t, "", gitrepo.ScopeGlobal, "user.name"); ok {
		t.Errorf("清空之后这一项该从文件里删掉，实际还剩 %q", v)
	}
}

// 值以 - 开头照样能存、能读回来：写盘走 `--` 分隔，不会把它当成选项吃掉。
func TestGitConfigKeepsDashLedValue(t *testing.T) {
	a, tt, _ := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	typeInto(t, tt, "全局配置 user.name", "-weird")
	saveGitConfig(t, a, tt)

	if a.opErr != nil {
		t.Fatalf("以 - 开头的值不该报错：%v", a.opErr)
	}
	if v, ok := gitConfigValue(t, "", gitrepo.ScopeGlobal, "user.name"); !ok || v != "-weird" {
		t.Errorf("值没原样存下来：%q（存在 %v）", v, ok)
	}
}

// 加载层的规矩：只收身份项；能改的两栏一定摆出姓名与邮箱；系统级只摆文件里
// 真有的；项目级默认跟不跟随全局看文件里有没有那两项。
func TestBuildGitConfigGroupsKeepsIdentityOnly(t *testing.T) {
	groups := buildGitConfigGroups([]gitrepo.ConfigEntry{
		{Key: "core.repositoryformatversion", Value: "0", Scope: gitrepo.ScopeLocal, File: "/repo/.git/config"},
		{Key: "branch.main.remote", Value: "origin", Scope: gitrepo.ScopeLocal, File: "/repo/.git/config"},
		{Key: "user.name", Value: "Repo Bot", Scope: gitrepo.ScopeLocal, File: "/repo/.git/config"},
		{Key: "user.email", Value: "bot@example.com", Scope: gitrepo.ScopeLocal, File: "/repo/.git/config"},
		{Key: "core.editor", Value: "vim", Scope: gitrepo.ScopeGlobal, File: "/home/u/.gitconfig"},
		{Key: "user.name", Value: "Sys", Scope: gitrepo.ScopeSystem, File: "/etc/gitconfig"},
	})

	byScope := map[string]*gitConfigGroup{}
	for _, g := range groups {
		byScope[g.Scope] = g
	}
	if got := len(byScope); got != 3 {
		t.Fatalf("该有三栏，实际 %d 栏", got)
	}

	local := byScope[gitrepo.ScopeLocal]
	if local.InheritGlobal {
		t.Error("文件里有 user.name / user.email，不该默认跟随全局")
	}
	if len(local.Rows) != 2 {
		t.Fatalf("项目级该只有两项身份，实际 %+v", local.Rows)
	}
	// 来源文件的清单照收：要知道保存会落到哪个文件上。
	if len(local.Files) != 1 || local.Files[0] != "/repo/.git/config" {
		t.Errorf("项目级的来源文件不对：%+v", local.Files)
	}

	global := byScope[gitrepo.ScopeGlobal]
	if len(global.Rows) != 2 || global.Rows[0].Key != "user.email" || global.Rows[1].Key != "user.name" {
		t.Fatalf("全局栏该按序摆出这两项（文件里只有 core.editor 也得摆），实际 %+v", global.Rows)
	}
	for _, r := range global.Rows {
		if !r.Fresh || r.Value != "" {
			t.Errorf("文件里没有的身份项该是空的待填行：%+v", r)
		}
	}

	// 系统级只读而且只摆真有的那一项。
	system := byScope[gitrepo.ScopeSystem]
	if !system.ReadOnly {
		t.Error("系统级该是只读的")
	}
	if len(system.Rows) != 1 || system.Rows[0].Key != "user.name" || system.Rows[0].Value != "Sys" {
		t.Fatalf("系统级该只摆文件里真有的身份项：%+v", system.Rows)
	}

	// 文件里没有身份项 ⇒ 项目级默认跟随全局。
	fresh := buildGitConfigGroups([]gitrepo.ConfigEntry{
		{Key: "core.bare", Value: "false", Scope: gitrepo.ScopeLocal, File: "/repo/.git/config"},
	})
	for _, g := range fresh {
		if g.Scope == gitrepo.ScopeLocal && !g.InheritGlobal {
			t.Error(".git/config 里没有身份项时，项目级该默认跟随全局")
		}
	}
}

// 同一个键在文件里有多条：界面上只占一行（取最后一条，git 取值也是最后一条
// 赢），并记下「原有多条」。
func TestBuildGitConfigGroupsMergesDuplicateKey(t *testing.T) {
	groups := buildGitConfigGroups([]gitrepo.ConfigEntry{
		{Key: "user.email", Value: "旧@example.com", Scope: gitrepo.ScopeGlobal, File: "/g"},
		{Key: "user.email", Value: "新@example.com", Scope: gitrepo.ScopeGlobal, File: "/g"},
	})
	var global *gitConfigGroup
	for _, g := range groups {
		if g.Scope == gitrepo.ScopeGlobal {
			global = g
		}
	}
	var email *gitConfigRow
	for _, r := range global.Rows {
		if r.Key == "user.email" {
			email = r
		}
	}
	if email == nil {
		t.Fatal("没有 user.email 这一行")
	}
	if !email.Multi {
		t.Error("原有多条时该标出来")
	}
	if email.Value != "新@example.com" || email.Original != "新@example.com" {
		t.Errorf("该留最后一条：%+v", email)
	}
	if email.changed() {
		t.Error("没改过的行不该算动过")
	}
}

// 界面上新加又清空的行（Fresh 且空）不该产生任何写操作：它从没进过文件。
func TestFreshEmptyRowWritesNothing(t *testing.T) {
	a := &app{}
	a.gitCfg.groups = buildGitConfigGroups(nil)
	if got := a.pendingGitConfigChanges(); len(got) != 0 {
		t.Errorf("空的待填行不该产生写操作：%+v", got)
	}

	a.gitCfg.groups[0].Rows = append(a.gitCfg.groups[0].Rows, &gitConfigRow{
		Key: "user.name", Value: "填了又清空", Fresh: true,
	})
	a.gitCfg.groups[0].Rows[len(a.gitCfg.groups[0].Rows)-1].Value = ""
	if got := a.pendingGitConfigChanges(); len(got) != 0 {
		t.Errorf("填了又清空的待填行不该产生写操作：%+v", got)
	}
}

// identityRowLocal 取项目级那一栏里的某一行。
func (a *app) identityRowLocal(t *testing.T, key string) *gitConfigRow {
	t.Helper()
	for _, r := range a.gitCfg.group(gitrepo.ScopeLocal).Rows {
		if r.Key == key {
			return r
		}
	}
	t.Fatalf("项目级没有 %s", key)
	return nil
}

func (s *gitConfigState) group(scope string) *gitConfigGroup {
	for _, g := range s.groups {
		if g.Scope == scope {
			return g
		}
	}
	return &gitConfigGroup{Scope: scope}
}

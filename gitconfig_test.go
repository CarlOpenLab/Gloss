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

// 三栏都摆出来：全局、当前项目、只读的系统级，项目级还得有 git init 写下的
// core.* —— 读不到它，说明那一趟是站在仓库外面读的。
func TestGitConfigPaneListsThreeScopes(t *testing.T) {
	a, tt, _ := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	for _, want := range []string{
		"Git 配置",
		"全局配置",
		"当前项目",
		"系统级",
		"只读",
		"core.repositoryformatversion",
		"user.email",
	} {
		if !tt.HasText(want) {
			t.Errorf("页面上没有 %q", want)
		}
	}

	local := groupOf(t, a, gitrepo.ScopeLocal)
	if len(local.Rows) == 0 {
		t.Fatal("当前项目那一栏是空的：说明读的时候没站在仓库里")
	}
	if !strings.Contains(local.Files[0], filepath.Join(".git", "config")) {
		t.Errorf("项目级的来源文件不对：%q", local.Files[0])
	}
	// 全局那份是空文件：这一栏该说清「还没读到」，而不是让用户对着
	// 一张空表猜是不是坏了。
	if got := groupOf(t, a, gitrepo.ScopeGlobal); len(got.Rows) != 0 {
		t.Errorf("临时的全局配置应当是空的，实际 %d 行", len(got.Rows))
	}
	if !tt.HasText("还没读到全局配置") {
		t.Error("全局栏空着时缺少说明")
	}
}

// 在全局栏里加一项，点保存，落到全局配置文件上。
func TestGitConfigAddsGlobalEntry(t *testing.T) {
	a, tt, globalPath := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	if err := tt.Click("全局配置项名"); err != nil {
		t.Fatalf("点新键名输入框：%v", err)
	}
	tt.Type("pull.rebase")
	tt.Frame()
	if err := tt.Click("全局配置项值"); err != nil {
		t.Fatalf("点新值输入框：%v", err)
	}
	tt.Type("false")
	tt.Frame()
	if err := tt.Click("添加"); err != nil {
		t.Fatalf("点添加：%v", err)
	}
	tt.Frame()

	global := groupOf(t, a, gitrepo.ScopeGlobal)
	if len(global.Rows) != 1 || global.Rows[0].Key != "pull.rebase" || global.Rows[0].Value != "false" {
		t.Fatalf("新加的行没进全局栏：%+v", global.Rows)
	}
	// 加完就把两格清空，不然下一项会搭在上一项的残字上。
	if global.New.Key != "" || global.New.Value != "" {
		t.Errorf("添加后输入框没清空：%+v", global.New)
	}

	if err := tt.Click("保存"); err != nil {
		t.Fatalf("点保存：%v", err)
	}
	settle(t, a)

	if v, ok := gitConfigValue(t, "", gitrepo.ScopeGlobal, "pull.rebase"); !ok || v != "false" {
		t.Fatalf("全局配置里没有 pull.rebase=false（读到 %q，存在 %v）", v, ok)
	}
	if _, err := os.Stat(globalPath); err != nil {
		t.Errorf("全局配置文件没建起来：%v", err)
	}
}

// 删一行，点保存，文件里就没了；没动过的那几行还在。
func TestGitConfigDeletesRow(t *testing.T) {
	a, tt, _ := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	local := groupOf(t, a, gitrepo.ScopeLocal)
	if len(local.Rows) == 0 {
		t.Fatal("当前项目那一栏是空的")
	}
	// 「删除」是按钮上的字，页面上第一个属于项目栏排在最前的那一行 ——
	// 先把它认下来，免得哪天排序变了，这个用例悄悄删掉别的东西。
	victim := local.Rows[0].Key
	if _, ok := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, victim); !ok {
		t.Fatalf("准备删的 %s 本来就不在文件里", victim)
	}

	if err := tt.Click("删除"); err != nil {
		t.Fatalf("点删除：%v", err)
	}
	tt.Frame()
	// 删除只是记下意图：保存之前文件不许动，中途还能点回来。
	if !tt.HasText("保存时删除") {
		t.Error("删掉的行没有「保存时删除」的标记")
	}
	if _, ok := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, victim); !ok {
		t.Error("还没保存就把文件里的配置删掉了")
	}

	if err := tt.Click("保存"); err != nil {
		t.Fatalf("点保存：%v", err)
	}
	settle(t, a)

	if v, ok := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, victim); ok {
		t.Errorf("保存后 %s 还在文件里（值 %q）", victim, v)
	}
	if v, ok := gitConfigValue(t, a.repoPath, gitrepo.ScopeLocal, "user.email"); !ok || v != "test@example.com" {
		t.Errorf("没动过的 user.email 被带走了：%q，存在 %v", v, ok)
	}
}

// 键名不合法时写入要失败得有话说，而且不能把配置文件写脏。
func TestGitConfigRejectsInvalidKey(t *testing.T) {
	a, tt, globalPath := gitConfigApp(t)
	openGitConfigTab(t, a, tt)

	global := groupOf(t, a, gitrepo.ScopeGlobal)
	global.New.Key = "core.editor --wait"
	global.New.Value = "vim"
	a.addGitConfigRow(global)

	if err := tt.Click("保存"); err != nil {
		t.Fatalf("点保存：%v", err)
	}
	settle(t, a)

	if a.opErr == nil {
		t.Fatal("坏键名应当报错")
	}
	if !strings.Contains(a.opErr.Hint, "节.名") {
		t.Errorf("报错没给出键名该长什么样的提示：%q", a.opErr.Hint)
	}
	if _, err := os.Stat(globalPath); err == nil {
		t.Error("坏键名不该写出全局配置文件")
	}
}

// 键名已经在表里时不另起一行：一个键在界面上只占一行，写回去也只该有一条。
func TestAddGitConfigRowMergesDuplicateKey(t *testing.T) {
	a := &app{}
	a.gitCfg.groups = buildGitConfigGroups([]gitrepo.ConfigEntry{
		{Key: "user.name", Value: "旧名字", Scope: gitrepo.ScopeGlobal},
	})
	global := a.gitCfg.groups[0]

	global.New.Key = " user.name "
	global.New.Value = "新名字"
	a.addGitConfigRow(global)

	if len(global.Rows) != 1 {
		t.Fatalf("同一个键应当还是一条，实际 %d 条：%+v", len(global.Rows), global.Rows)
	}
	row := global.Rows[0]
	if row.Value != "新名字" || row.Original != "旧名字" {
		t.Errorf("改的值/基线不对：%+v", row)
	}
	if row.Fresh {
		t.Error("合并已有的键不该标成新加的")
	}
	if !row.changed() {
		t.Error("改过值的行应当算动过")
	}
}

// 界面上新加的行删掉就是反悔：它从没进过文件，保存时不该去删（删也别报错）。
func TestFreshRowDeletedWritesNothing(t *testing.T) {
	a := &app{}
	a.gitCfg.groups = buildGitConfigGroups(nil)
	a.gitCfg.groups[0].Rows = append(a.gitCfg.groups[0].Rows, &gitConfigRow{
		Key: "core.pager", Value: "-R", Fresh: true, Deleted: true,
	})
	if got := a.pendingGitConfigChanges(); len(got) != 0 {
		t.Errorf("新加又反悔的行不该产生写操作：%+v", got)
	}
}

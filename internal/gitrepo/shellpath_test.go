//go:build !windows

package gitrepo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeExecutable 在临时目录里写一个可执行文件，返回它的绝对路径。
func writeExecutable(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("写 %s 失败: %v", name, err)
	}
	return path
}

// fakeLoginShell 造一个假登录 shell：它不理会 -ilc 这些参数，只把给的那份
// PATH 打出来，模拟「用户 rc 文件往 PATH 里加了 nvm / mise / Homebrew 那些
// 目录」的登录 shell。用它而不是真 shell，测试才不依赖机器上装了什么、
// 也不依赖用户怎么写的 rc。
func fakeLoginShell(t *testing.T, path string) string {
	t.Helper()
	return writeExecutable(t, "fake-shell", "#!/bin/sh\nprintf '%s\\n' '"+path+"'\n")
}

// envValue 取环境切片里某个变量的值（多个同名时以最后一个为准）。
func envValue(env []string, key string) string {
	prefix := key + "="
	val := ""
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			val = strings.TrimPrefix(kv, prefix)
		}
	}
	return val
}

func countEnvKey(env []string, key string) int {
	prefix := key + "="
	n := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			n++
		}
	}
	return n
}

// 登录 shell 的输出里常混着 rc 的欢迎语、插件提示（实测 zsh 用户十有八九有），
// 所以 PATH 只能从里面挑出来；挑完还得和进程自己的 PATH 合并去重。
func TestChildPATHMergesLoginShellPATH(t *testing.T) {
	noisy := writeExecutable(t, "noisy-shell", "#!/bin/sh\n"+
		"echo 'welcome to my shell'\n"+
		"echo 'nvm initialized: ok'\n"+ // 带冒号但也不是 PATH
		"printf '%s\\n' '/opt/brew/bin:/usr/bin'\n"+
		"echo 'bye'\n")

	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SHELL", noisy)
	resetLoginPATHCache()
	t.Cleanup(resetLoginPATHCache)

	if got := loginShellPATH(); got != "/opt/brew/bin:/usr/bin" {
		t.Fatalf("loginShellPATH() = %q, want 最后一行像 PATH 的输出", got)
	}

	// 登录 shell 的在前（那才是用户实际在用的 PATH），进程自己的在后；
	// /usr/bin 重复，只保留第一次出现的位置。
	const want = "/opt/brew/bin:/usr/bin:/bin"
	if got := childPATH(); got != want {
		t.Fatalf("childPATH() = %q, want %q", got, want)
	}

	env := gitEnv()
	if got := envValue(env, "PATH"); got != want {
		t.Fatalf("gitEnv() 里的 PATH = %q, want %q", got, want)
	}
	// 只改了 PATH，其它保证（绝不挂起/绝不分页）一个都不能丢。
	if got := envValue(env, "GIT_EDITOR"); got != "true" {
		t.Fatalf("gitEnv() 丢了 GIT_EDITOR: %q", got)
	}
	if got := envValue(env, "GIT_TERMINAL_PROMPT"); got != "0" {
		t.Fatalf("gitEnv() 丢了 GIT_TERMINAL_PROMPT: %q", got)
	}
	// PATH 只能出现一次：环境里重复的 KEY 在 getenv 那边是没谱的。
	if n := countEnvKey(env, "PATH"); n != 1 {
		t.Fatalf("gitEnv() 里 PATH 出现了 %d 次", n)
	}
	if got := envValue(ghEnv(), "PATH"); got != want {
		t.Fatalf("ghEnv() 里的 PATH = %q, want %q", got, want)
	}
}

// 问不到 PATH 时必须原样保留进程自己的 PATH：宁可用不了钩子里的 node，
// 也不能连 git 本身（/usr/bin/git）都找不到 —— 那才是界面彻底不能用。
func TestChildPATHFallsBackWhenLoginShellFails(t *testing.T) {
	cases := []struct {
		name   string
		script string // 空串表示 SHELL 指向一个不存在的文件
	}{
		{"退出码非零", "#!/bin/sh\nexit 3\n"},
		{"输出为空", "#!/bin/sh\nexit 0\n"},
		{"输出里没有一行像 PATH", "#!/bin/sh\necho hello\nexit 0\n"},
		{"shell 根本不存在", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shell := filepath.Join(t.TempDir(), "no-such-shell")
			if tc.script != "" {
				shell = writeExecutable(t, "fake-shell", tc.script)
			}
			t.Setenv("PATH", "/usr/bin:/bin")
			t.Setenv("SHELL", shell)
			resetLoginPATHCache()
			t.Cleanup(resetLoginPATHCache)

			if got := loginShellPATH(); got != "" {
				t.Fatalf("loginShellPATH() = %q, 这种 shell 该当成问不到", got)
			}
			if got := childPATH(); got != "/usr/bin:/bin" {
				t.Fatalf("childPATH() = %q, want 原封不动的进程 PATH", got)
			}
		})
	}
}

// 用户 rc 里挂住一个交互式命令是完全可能的（read、问密码、等网络）。
// 没有超时的话 GUI 会在这一步无限期卡住 —— 而这还是每 2 秒刷新都要走的路。
func TestLoginShellPATHTimeoutFallsBack(t *testing.T) {
	// exec 换成 sleep：超时杀掉的就是 sleep 本身，不留孤儿进程在后台躺着。
	shell := writeExecutable(t, "hanging-shell", "#!/bin/sh\nexec sleep 30\n")

	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("SHELL", shell)
	resetLoginPATHCache()
	t.Cleanup(resetLoginPATHCache)

	start := time.Now()
	if got := loginShellPATH(); got != "" {
		t.Fatalf("loginShellPATH() = %q, want 空串（超时降级）", got)
	}
	elapsed := time.Since(start)
	if elapsed > shellPathTimeout+3*time.Second {
		t.Fatalf("等了 %v 才放弃，超时没起作用（上限 %v）", elapsed, shellPathTimeout)
	}
}

// gh 的查找必须看「算出来的 PATH」：只在登录 shell 那份 PATH 里的目录也得找到，
// 而且顺序按 PATH 来（前面那个不可执行就继续往后找）。
func TestLookPathInShellPATH(t *testing.T) {
	dirA, dirB := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dirA, "gh"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dirB, "gh")
	if err := os.WriteFile(want, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dirA+string(os.PathListSeparator)+dirB)
	t.Setenv("SHELL", "/usr/bin/false") // 登录 shell 那份不掺和，只测进程 PATH
	resetLoginPATHCache()
	t.Cleanup(resetLoginPATHCache)

	got, err := lookPathInShellPATH("gh")
	if err != nil {
		t.Fatalf("lookPathInShellPATH(gh) = %v", err)
	}
	if got != want {
		t.Fatalf("找到的是 %q, want %q（PATH 里第一个能执行的）", got, want)
	}
	if _, err := lookPathInShellPATH("definitely-not-here"); err == nil {
		t.Fatal("不存在的命令应该报错")
	}

	t.Setenv("PATH", "")
	if _, err := lookPathInShellPATH("gh"); err == nil {
		t.Fatal("空 PATH 下应该找不到")
	}
}

func TestCheckGhUsesShellPATH(t *testing.T) {
	t.Run("gh 只在登录 shell 的 PATH 里也能找到", func(t *testing.T) {
		ghDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(ghDir, "gh"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", "/usr/bin:/bin")
		t.Setenv("SHELL", fakeLoginShell(t, ghDir+":/usr/bin:/bin"))
		resetLoginPATHCache()
		t.Cleanup(resetLoginPATHCache)

		if err := checkGh(); err != nil {
			t.Fatalf("checkGh() = %v, want nil（gh 就在登录 shell 的 PATH 里）", err)
		}
	})

	t.Run("拿不到登录 shell 的 PATH 就找不到 gh", func(t *testing.T) {
		// 只有极简 PATH 里确实没有 gh，这条断言才说明问题。
		if _, err := os.Stat("/usr/bin/gh"); err == nil {
			t.Skip("这台机器把 gh 装在 /usr/bin，没法断言「找不到」")
		}
		t.Setenv("PATH", "/usr/bin:/bin")
		t.Setenv("SHELL", "/usr/bin/false")
		resetLoginPATHCache()
		t.Cleanup(resetLoginPATHCache)

		if err := checkGh(); !errors.Is(err, ErrGhNotFound) {
			t.Fatalf("checkGh() = %v, want ErrGhNotFound", err)
		}
	})
}

// requireSystemGit 确认极简 PATH 下还能跑 git —— 这个测试的前提是「/usr/bin
// 里只有系统自带的东西」，git 不在这儿的话场景造不出来。
func requireSystemGit(t *testing.T) {
	t.Helper()
	if _, err := os.Stat("/usr/bin/git"); err != nil {
		t.Skip("这台机器的 git 不在 /usr/bin，造不出 launchd 那种极简 PATH")
	}
}

// installPreCommitHook 装一个 pre-commit 钩子。husky / lefthook / vite+ 的钩子
// 剥开看就是「调 node」这一件事，钩子里写什么不影响这个结论。
func installPreCommitHook(t *testing.T, root, body string) {
	t.Helper()
	hooks := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre-commit"), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// stageEditedFile 改一个已跟踪文件并 stage 起来，让 Commit 有东西可提交。
func stageEditedFile(t *testing.T, r *Repo) {
	t.Helper()
	write(t, filepath.Join(r.Root, "a.txt"), "changed\n")
	git(t, r.Root, "add", "a.txt")
}

// 这是整个改动存在的理由，端到端跑真 git：
// 进程 PATH 只有 launchd 给的那几个系统目录（模拟从 Finder/Dock 启动的 .app），
// node 只存在于登录 shell 的 PATH 里。仓库里装一个 pre-commit 钩子，钩子只调
// node。Commit() 必须成功 —— 说明登录 shell 的 PATH 真的传给了 git 子进程，
// 又从 git 传给了钩子。
func TestCommitExposesLoginShellPATHToHooks(t *testing.T) {
	t.Run("钩子用登录 shell 的 PATH 找得到 node", func(t *testing.T) {
		requireSystemGit(t)

		nodeDir := t.TempDir()
		marker := filepath.Join(t.TempDir(), "node-ran")
		if err := os.WriteFile(
			filepath.Join(nodeDir, "node"),
			[]byte("#!/bin/sh\n: > '"+marker+"'\nexit 0\n"),
			0o755,
		); err != nil {
			t.Fatal(err)
		}

		// 极简 PATH：没有 node，也没有任何用户装的东西；git/sh 这些系统命令还在。
		t.Setenv("PATH", "/usr/bin:/bin")
		// 假登录 shell 的 PATH 里才有那个 node 替身。
		t.Setenv("SHELL", fakeLoginShell(t, nodeDir+":/usr/bin:/bin"))
		resetLoginPATHCache()
		t.Cleanup(resetLoginPATHCache)

		r := tempRepo(t)
		installPreCommitHook(t, r.Root, "node\n")
		stageEditedFile(t, r)

		if err := r.Commit(context.Background(), "hook test\n"); err != nil {
			t.Fatalf("Commit() = %v —— 登录 shell 的 PATH 没传到钩子", err)
		}
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("钩子没跑到我们那个 node 上: %v", err)
		}
	})

	t.Run("没有登录 shell 的 PATH 时同一场景失败（复现原 bug）", func(t *testing.T) {
		requireSystemGit(t)

		t.Setenv("PATH", "/usr/bin:/bin")
		// 问不出 PATH 的 shell：降级成进程自己的 PATH，node 就不在里头了。
		t.Setenv("SHELL", "/usr/bin/false")
		resetLoginPATHCache()
		t.Cleanup(resetLoginPATHCache)

		r := tempRepo(t)
		installPreCommitHook(t, r.Root, "node\n")
		stageEditedFile(t, r)

		before := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "HEAD"))
		err := r.Commit(context.Background(), "hook test\n")
		if err == nil {
			t.Fatal("Commit() 成功了，但这个场景本来就该失败（钩子找不到 node）")
		}
		if !strings.Contains(err.Error(), "node") {
			t.Fatalf("失败原因看着不像「找不到 node」: %v", err)
		}
		// 失败要失败在钩子上：提交没进去，HEAD 不动。
		if after := strings.TrimSpace(gitOut(t, r.Root, "rev-parse", "HEAD")); after != before {
			t.Fatalf("提交虽然报错但 HEAD 变了：%s → %s", before, after)
		}
	})
}

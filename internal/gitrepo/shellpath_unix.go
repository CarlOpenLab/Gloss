//go:build !windows

package gitrepo

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// shellPathTimeout 是「问登录 shell 要 PATH」的等待上限。
//
// 必须有超时：-i 会把用户的 ~/.zshrc / ~/.bashrc 整个跑一遍，那里面什么都
// 可能发生 —— 条件里带网络请求、nvm/conda 初始化要几百毫秒到几秒、甚至有
// read 之类在等输入的命令。这是界面一打开就要走的路径（每 2 秒的自动刷新都
// 会用 gitEnv），不能因为用户 rc 写得慢就把整个界面拖住。超时就降级：保持
// 进程原来的 PATH 不变 —— git 自己还在 /usr/bin 或 /usr/local/bin 里，界面
// 照常能用，比挂住强。
const shellPathTimeout = 3 * time.Second

var (
	// 登录 shell 那份 PATH 只问一次，按进程缓存。
	//
	// 为什么要缓存：git status 是每 2 秒一拍的后台刷新，每拍都 fork 一个交互式
	// shell、再跑完 nvm/mise 那一串初始化，纯属白烧 CPU 和磁盘（实测登录 shell
	// 起步就是几百毫秒）。用户改 rc 文件后重开 App 才会重新问，这个代价可以接受。
	loginPathOnce sync.Once
	loginPathVal  string
)

// resetLoginPATHCache 扔掉缓存，下次再问一次登录 shell。
//
// 只有测试会调：生产代码里 once 的语义正好是我们要的「进程里只问一次」，
// 而测试需要模拟不同用户、不同 SHELL 的 PATH。
func resetLoginPATHCache() {
	loginPathOnce = sync.Once{}
	loginPathVal = ""
}

// loginShellPATH 返回登录 shell 里的 PATH（拿不到时返回空串）。
func loginShellPATH() string {
	loginPathOnce.Do(func() {
		loginPathVal = fetchLoginShellPATH()
	})
	return loginPathVal
}

// childPATH 返回子进程该用的 PATH：登录 shell 里那份放前面，进程自己这份
// 接在后面，去重。
//
// 为什么要去问登录 shell：从 Finder / Dock 启动的 .app 拿到的是 launchd 给的
// 环境，PATH 通常只有 /usr/bin:/bin:/usr/sbin:/sbin。用户在终端里能跑 node，
// 靠的是 rc 文件里 nvm / volta / Homebrew / mise 那几行 —— GUI 进程从来不读
// 这些文件。于是 GUI 里一点「提交」，git 跑的 pre-commit 钩子（husky /
// lefthook / vite+ 都是 node 写的）就报 `env: node: No such file or
// directory`（退出码 127），同一个仓库在终端里提交却一切正常。gh 同理：
// Homebrew 装在 /opt/homebrew/bin 的 gh 在 GUI 里就是「未检测到」。
//
// 顺序是「登录 shell 的在前」：那份才是用户实际在用的 PATH，装的东西以它
// 为准；进程自己那份接在后面兜底（launchd 给的那几个系统目录总要有）。
//
// 只有登录 shell 那一段走缓存，合并每次都现算：这样进程 PATH 被改了
// （测试里的 t.Setenv，或将来谁在运行时调整环境）立刻生效，不会撞上陈旧缓存。
func childPATH() string {
	return mergePATH(loginShellPATH(), os.Getenv("PATH"))
}

// shellPATHEnv 返回子进程该用的完整环境：进程环境的副本，PATH 换成 childPATH()
// 算出来的那份。
//
// 每次调用都新建一份切片（os.Environ 本身就是新的），调用方可以随手 append
// 自己的变量 —— gitEnv / ghEnv 就是这么干的。
func shellPATHEnv() []string {
	return withEnv(os.Environ(), "PATH", childPATH())
}

// fetchLoginShellPATH 真的去跑一次登录 shell 问它的 PATH；任何失败都返回空串
// （调用方据此保持原 PATH 不变）。
//
// 用 -i（交互式）而不是只 -l：nvm / volta / Homebrew / mise 的 PATH 补全基本
// 都写在 ~/.zshrc、~/.bashrc 这类交互式配置里，非交互的登录 shell 并不读它们，
// 只带 -l 等于白问。代价是 rc 里什么都可能跑，所以超时、/dev/null 的 stdin、
// 丢掉 stderr 一个都不能少。
func fetchLoginShellPATH() string {
	ctx, cancel := context.WithTimeout(context.Background(), shellPathTimeout)
	defer cancel()

	// stdin 默认就是 /dev/null：rc 里若有等输入的 read，会立刻拿到 EOF，而不是
	// 一直耗到超时。stderr 丢掉：rc 的欢迎语、警告、报错都不是 PATH。
	cmd := exec.CommandContext(ctx, loginShell(), "-ilc", `printf %s "$PATH"`)
	// 覆盖式地给 TERM=dumb：让 rc 里的彩色输出、分页器、花哨提示都闭嘴，
	// 少一样东西拖时间。（直接 append 是改不掉的，见 withEnv。）
	cmd.Env = withEnv(os.Environ(), "TERM", "dumb")
	// rc 里起的后台进程可能揪着 stdout 管道不放。没有 WaitDelay 的话，超时杀掉的
	// 只是 shell 自己，Wait 仍会一直等管道关闭 —— 「超时」就变成了「永远挂住」。
	cmd.WaitDelay = time.Second

	out, err := cmd.Output()
	if err != nil || ctx.Err() != nil {
		// 超时、退出码非 0（用户 shell 是 tcsh/fish 之类不认 -ilc 的写法也算）、
		// 执行不起来：一律当成「问不到」，由 childPATH 退回进程自己的 PATH。
		return ""
	}
	return lastPathLine(string(out))
}

// lastPathLine 从登录 shell 的输出里挑出 PATH 那一行。
//
// rc 文件很可能在 stdout 上留下别的东西（欢迎语、版本号、插件提示），所以不能
// 整个信：取最后一行非空、且含 os.PathListSeparator 的行 —— PATH 是「目录:
// 目录」的列表，不带分隔符的一行不可能是它。取最后一行是因为 rc 输出在前、
// 我们那句 printf 在最后。
func lastPathLine(out string) string {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(strings.TrimSuffix(lines[i], "\r"))
		if line == "" || !strings.ContainsRune(line, os.PathListSeparator) {
			continue
		}
		return line
	}
	return ""
}

// loginShell 决定问哪个 shell。
func loginShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return defaultLoginShell()
}

// defaultLoginShell 在 $SHELL 缺席时给出一个登录 shell。
//
// launchd 启动的 GUI 进程不一定有 SHELL（尤其从 Dock 启动）。os/user 只给
// 用户名和 uid，拿不到登录 shell 这个字段，所以按 uid 去 /etc/passwd 里翻
// 第七个字段；再找不到就用 /bin/sh —— 它一定在。
func defaultLoginShell() string {
	u, err := user.Current()
	if err != nil {
		return "/bin/sh"
	}
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return "/bin/sh"
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Split(line, ":")
		if len(f) < 7 {
			continue
		}
		if f[2] != u.Uid && f[0] != u.Username {
			continue
		}
		if shell := strings.TrimSpace(f[6]); shell != "" {
			return shell
		}
	}
	return "/bin/sh"
}

// mergePATH 把两份 PATH 拼成一份：primary 在前、fallback 在后，重复的目录只留
// 第一次出现的位置。顺序就是优先级，重复项没有任何意义，只会把 PATH 撑长。
func mergePATH(primary, fallback string) string {
	seen := make(map[string]bool)
	dirs := make([]string, 0, 8)
	for _, list := range [2]string{primary, fallback} {
		for _, dir := range filepath.SplitList(list) {
			// POSIX 里 PATH 的空项表示「当前目录」。GUI 进程的 PATH 里冒出
			// 一个当前目录只会让人意外，直接丢掉。
			if dir == "" || seen[dir] {
				continue
			}
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	return strings.Join(dirs, string(os.PathListSeparator))
}

// withEnv 覆盖式地设置一个环境变量：同名的旧值扔掉。
//
// 直接 append 是改不掉的 —— 环境里出现重复的 KEY 时 getenv 通常返回先出现的
// 那个，`TERM=xterm-256color` 后面再跟一个 `TERM=dumb` 什么也拦不住。
func withEnv(env []string, key, value string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			continue
		}
		out = append(out, kv)
	}
	return append(out, prefix+value)
}

// lookPathInShellPATH 等价于 exec.LookPath，但搜的是 childPATH() 那份 PATH。
//
// exec.LookPath 只认进程自己的 PATH，没有「把 PATH 递进去」的入口；而从 Finder
// 启动的 .app 那份 PATH 里没有 /opt/homebrew/bin，用户明明装了 gh 也会被判成
// 「未检测到」。所以这里按同样的规则自己找：PATH 逐个目录试，第一个可执行的
// 命中为止。
func lookPathInShellPATH(file string) (string, error) {
	if strings.ContainsRune(file, '/') {
		// 带路径的调用不做 PATH 查找，和 exec.LookPath 一致。
		if err := statExecutable(file); err != nil {
			return "", &exec.Error{Name: file, Err: err}
		}
		return file, nil
	}

	var firstErr error
	for _, dir := range filepath.SplitList(childPATH()) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, file)
		err := statExecutable(candidate)
		if err == nil {
			return candidate, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = exec.ErrNotFound
	}
	return "", &exec.Error{Name: file, Err: firstErr}
}

// statExecutable 判断 path 是不是「存在且可执行的文件」。目录不算：PATH 里
// 同名目录挡在前面时不能当成命中。
func statExecutable(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() || fi.Mode().Perm()&0o111 == 0 {
		return fs.ErrPermission
	}
	return nil
}

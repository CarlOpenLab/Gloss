// Package gitrepo 通过调用 git 命令行读取仓库状态、diff 与历史。
//
// 用 CLI 而不是 go-git 这类库是刻意的：用户在命令行里真实生效的 git 行为
// （配置、.gitattributes、.gitignore 规则、全局 gitignore、hooks）能原样复现，
// 不会出现「GUI 里看到的和命令行不一致」。代价是每次调用要起进程，
// 所以内容读取走长驻的 cat-file --batch（见 contents.go）。
package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo 是一个已打开的 git 仓库。
type Repo struct {
	Root string // 工作区根目录的绝对路径
	Name string // 目录名，用于显示
	// GitDir 是 .git 目录的绝对路径。判断「正卡在合并/变基中间」要看它
	// 底下的标记文件（pending.go），所以打开时顺手量好。
	//
	// 不能想当然写成 Root/.git：linked worktree 和子模块的 .git 是一个
	// **文件**（里面一行 gitdir: …），GIT_DIR 环境变量也常见 ——
	// `--absolute-git-dir` 正是为这些情况存在的。
	GitDir string
}

// emptyTree 是 git 的「空树」对象哈希。仓库还没有任何提交时（HEAD 不存在），
// 拿它当比较基准，就能把 index 里的内容当成全是新增。
const emptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// ErrNotRepo 表示这个目录（以及它的各级父目录）不是 git 仓库。
//
// 它和「仓库在读的时候出错了」不是一回事：那个重试有意义，这个只说明
// 文件夹选错了 —— 重试多少次结果都一样，该做的是换一个。界面上要分开
// 对待，所以这里得是个能认出来的错，而不是一句话。
var ErrNotRepo = errors.New("不是 git 仓库")

// Open 在 dir 里定位仓库根并打开。dir 可以指向仓库的任意子目录。
func Open(ctx context.Context, dir string) (*Repo, error) {
	// 一次进程问两件事：仓库根 + .git 目录。两条输出按参数顺序来。
	// （分开跑两条就是把每次打开仓库的开销翻倍，而它们回答的是同一个问题。）
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotRepo, err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	root := strings.TrimSpace(lines[0])
	gitDir := filepath.Join(root, ".git")
	if len(lines) > 1 {
		gitDir = strings.TrimSpace(lines[1])
	}
	return &Repo{Root: root, Name: filepath.Base(root), GitDir: gitDir}, nil
}

// Branch 返回当前分支名。处于 detached HEAD 时返回短哈希。
func (r *Repo) Branch(ctx context.Context) (string, error) {
	name, _, err := r.branchInfo(ctx)
	return name, err
}

// branchInfo 在 Branch 的基础上多带一个「是不是 detached HEAD」。
//
// 界面靠它把分支芯片换成警示样式：分离头指针上的提交没人接得住，
// 不说清楚的话用户会以为自己在分支上正常干活。
func (r *Repo) branchInfo(ctx context.Context) (string, bool, error) {
	out, err := run(ctx, r.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		return strings.TrimSpace(string(out)), false, nil
	}
	// detached HEAD：symbolic-ref 会失败，退回短哈希
	out, err = run(ctx, r.Root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", false, nil // 空仓库还没有 HEAD，不算错误
	}
	return strings.TrimSpace(string(out)), true, nil
}

// Remotes 返回配置的远程名。工具栏拿它决定 抓取/拉取/推送 能不能点 ——
// `git fetch --all` 在没有远程时是静默成功的，等错误等不到。
func (r *Repo) Remotes(ctx context.Context) ([]string, error) {
	out, err := run(ctx, r.Root, "remote")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			names = append(names, l)
		}
	}
	return names, nil
}

// RemoteURLs 一次读出这些远程各自配的 URL。
//
// 用 `git remote -v` 而不是每个远程一条 get-url：一条命令拿全，且读出来
// 的 fetch/push 两行取 fetch 那一行（用户写的是哪个就认哪个）。
//
// 读不到就返回 nil、不报错 —— 「这个仓库没有远程」不是错误，是「没有网页
// 可看」，调用方拿空 map 就行，不该为它弹一句错。
func (r *Repo) RemoteURLs(ctx context.Context, names []string) map[string]string {
	if len(names) == 0 {
		return nil
	}
	out, err := run(ctx, r.Root, "remote", "-v")
	if err != nil {
		return nil
	}
	urls := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		// 形如 "origin\tgit@github.com:o/r.git (fetch)"
		tab := strings.IndexByte(line, '\t')
		if tab < 0 {
			continue
		}
		name := strings.TrimSpace(line[:tab])
		rest := strings.TrimSpace(line[tab+1:])
		i := strings.Index(rest, " (")
		if i < 0 || !strings.HasSuffix(rest, "(fetch)") {
			continue
		}
		if _, dup := urls[name]; !dup {
			urls[name] = strings.TrimSpace(rest[:i])
		}
	}
	return urls
}

// baseRev 返回应该拿来做 diff 基准的 revision：有提交时是 HEAD，
// 空仓库时是空树。
func (r *Repo) baseRev(ctx context.Context) string {
	if _, err := run(ctx, r.Root, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		return emptyTree
	}
	return "HEAD"
}

// diffArgs 是所有 diff 调用的公共参数。
//
//   - core.quotePath=false 让非 ASCII 路径原样输出而不是转义成 \344\270\255
//   - --no-ext-diff/--no-textconv 避免用户配置的外部 diff 工具和文本转换器
//     污染输出（GUI 需要的是原始 patch）
//   - -M 打开重命名检测，重命名才能显示成 rename 而不是「删一个加一个」
//   - -U3 是 git 默认的上下文行数，显式写出来以免受 diff.context 配置影响
func diffArgs(extra ...string) []string {
	return append([]string{
		"-c", "core.quotePath=false",
		"diff", "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/",
		"-M", "-U3",
	}, extra...)
}

// gitEnv 是给所有 git 调用设的环境。
//
//   - GIT_OPTIONAL_LOCKS=0：读操作不去做「顺便刷新 index」这类可选写入，
//     避免和用户正在跑的 git 命令互相阻塞。注意它挡不住写操作（apply、
//     commit 该拿 index 锁还是会拿），和外部 git 撞上锁由上层翻译
//     index.lock 错误。
//   - LC_ALL=C：锁定输出语言，错误信息和日期格式不会随用户 locale 变化
//   - GIT_PAGER=cat / GIT_TERMINAL_PROMPT=0：绝不分页、绝不弹交互提示
//     （GUI 进程没有终端，弹提示会直接挂住）
//   - GIT_EDITOR=true / GIT_SEQUENCE_EDITOR=true：绝不弹编辑器。这条是
//     实测逼出来的：`git merge --continue` 会把提交信息交给 $GIT_EDITOR
//     过目 —— 终端里跑就是 vim，从这里跑就是「Vim: Warning: Output is
//     not to a terminal」之后的一个没人能回答的挂起。给 true 让「编辑器」
//     立刻退 0，git 就按现有的 MERGE_MSG 往下走。设成全局环境变量而不是
//     每条命令 -c core.editor=true：这是对整个进程的约束（我们永远不要
//     编辑器），漏掉一处就是一次挂起。（cherry-pick/revert 的 --continue
//     实测不拉编辑器，但按「永远不要」来要求自己更省心。）
//   - PATH 用的是 shellPATHEnv() 算出来的那份（登录 shell 的 PATH + 进程
//     自己的）：钩子是用户写的，钩子里调的 node/等工具只存在于用户终端
//     的 PATH 里。从 Finder 启动的 GUI 进程只有 launchd 那几个系统目录，
//     不加这一层，husky/lefthook 这类钩子会在「提交」（还有「继续合并」
//     这种会真提交的操作）时报 `env: node: No such file or directory`。
func gitEnv() []string {
	// PATH 由 shellPATHEnv 处理，其余变量按老办法追加。
	return append(shellPATHEnv(),
		"GIT_OPTIONAL_LOCKS=0",
		"LC_ALL=C",
		"GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_EDITOR=true",
		"GIT_SEQUENCE_EDITOR=true",
	)
}

// run 执行一条 git 命令并返回 stdout。失败时把 stderr（和 stdout 里那些
// 话）作为错误返回，因为 git 的错误信息（比如「not a git repository」）比
// exit status 有用得多。
func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return runIn(ctx, dir, nil, args...)
}

// runIn 是 run 带上 stdin 的版本：`git apply -` 和 `git commit -F -` 要从
// 管道里读 patch 和提交信息。
//
// stdin 为 nil 时子进程拿到的是 /dev/null（exec 的默认），和以前完全一样 ——
// 任何试图读输入的命令都会立刻拿到 EOF，而不是挂住。写操作同样不给 tty：
// GIT_TERMINAL_PROMPT=0 保证绝不会卡在一个没人能回答的提示上。
//
// 失败时 stdout 也要收进错误里：git 不是把坏消息一律打在 stderr 上的 ——
// 拉取撞冲突时 "CONFLICT (content): Merge conflict in a.txt" 和
// "Automatic merge failed" 都在 stdout，stderr 只有抓取的那两行进度。以前
// 只看 stderr，等于把「为什么失败」丢了，界面上翻出来的是一句光秃秃的
// 「拉取没成功」，Detail 里挂着抓取摘要，谁也看不出是冲突。
// newGitCmd 建好一条 git 子进程：工作目录、环境，以及在 Windows 上藏掉那个
// 会一闪一闪的控制台窗口（hideConsole）。仓库里所有 git 调用都从这里出发，
// 所以「子进程不带窗口」这件事只有这一个维护点 —— 见 console_windows.go 里
// 为什么这件事非做不可（Gloss 是 -H=windowsgui 构建，autorefresh 每 2 秒跑
// 一次 status/show-ref）。
func newGitCmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	hideConsole(cmd)
	return cmd
}

func runIn(ctx context.Context, dir string, stdin []byte, args ...string) ([]byte, error) {
	cmd := newGitCmd(ctx, dir, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if out := strings.TrimSpace(stdout.String()); out != "" {
			if msg == "" {
				msg = out
			} else {
				msg += "\n" + out
			}
		}
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return stdout.Bytes(), nil
}

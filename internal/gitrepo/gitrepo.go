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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo 是一个已打开的 git 仓库。
type Repo struct {
	Root string // 工作区根目录的绝对路径
	Name string // 目录名，用于显示
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
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotRepo, err)
	}
	root := strings.TrimSpace(string(out))
	return &Repo{Root: root, Name: filepath.Base(root)}, nil
}

// Branch 返回当前分支名。处于 detached HEAD 时返回短哈希。
func (r *Repo) Branch(ctx context.Context) (string, error) {
	out, err := run(ctx, r.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		return strings.TrimSpace(string(out)), nil
	}
	// detached HEAD：symbolic-ref 会失败，退回短哈希
	out, err = run(ctx, r.Root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", nil // 空仓库还没有 HEAD，不算错误
	}
	return strings.TrimSpace(string(out)), nil
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
//   - GIT_OPTIONAL_LOCKS=0：后台读操作不去抢 index 锁，避免和用户正在跑的
//     git 命令互相阻塞
//   - LC_ALL=C：锁定输出语言，错误信息和日期格式不会随用户 locale 变化
//   - GIT_PAGER=cat / GIT_TERMINAL_PROMPT=0：绝不分页、绝不弹交互提示
//     （GUI 进程没有终端，弹提示会直接挂住）
func gitEnv() []string {
	return append(os.Environ(),
		"GIT_OPTIONAL_LOCKS=0",
		"LC_ALL=C",
		"GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0",
	)
}

// run 执行一条 git 命令并返回 stdout。失败时把 stderr 的内容作为错误返回，
// 因为 git 的错误信息（比如「not a git repository」）比 exit status 有用得多。
func run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return stdout.Bytes(), nil
}

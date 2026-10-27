package gitrepo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// ErrHunkGone 表示点的那段改动在重新读出来的 patch 里已经不存在了 ——
// 用户点下去和 git 真正执行之间文件被改过（或前一次暂存让行号挪了位），
// 是正常竞态，不是 bug。界面翻译成一句人话 + 自动刷新。
var ErrHunkGone = errors.New("这个改动已经变了")

// StageFile 把整个文件放进 index。
//
// -A 让它同时覆盖删除：对未跟踪、二进制、重命名、纯 mode 变化、未跟踪
// 转已跟踪这些都是唯一的路径；冲突时它等价于「标记为已解决」。
// 未跟踪文件不走合成的 patch —— 那种 patch 没有 index 行、没有 new file
// mode，遇上 core.autocrlf、LFS 过滤器或真实文件权限就会被咬。
//
// 路径是变参：重命名要新旧两个路径一起给。只给新路径的话，旧路径在 index
// 里还是「已跟踪」，暂存完的 index 就成了「新文件是新增、旧文件还在」，
// 得再点一次才真的变成重命名 —— 用户看到的是一次莫名其妙的中间态。
func cleanPaths(paths []string) []string {
	var clean []string
	for _, p := range paths {
		if p != "" {
			clean = append(clean, p)
		}
	}
	return clean
}

func (r *Repo) StageFile(ctx context.Context, paths ...string) error {
	clean := cleanPaths(paths)
	if len(clean) == 0 {
		return nil
	}
	args := append([]string{"add", "-A", "--"}, clean...)
	_, err := run(ctx, r.Root, args...)
	return err
}

// UnstageFile 把整个文件撤出 index。
//
// 用 `git reset` 而不是新版的 `git restore --staged`：语义一样（包括
// 「已暂存的新增」撤销后回到未跟踪），但 reset 从 git 1.x 就在，不挑版本。
// 变参的理由和 StageFile 一样：撤销一个已暂存的重命名，得把新旧两个路径
// 都从 index 里退回去，只退一个会留下半截删除。
func (r *Repo) UnstageFile(ctx context.Context, paths ...string) error {
	clean := cleanPaths(paths)
	if len(clean) == 0 {
		return nil
	}
	args := append([]string{"reset", "-q", "--"}, clean...)
	_, err := run(ctx, r.Root, args...)
	return err
}

// RestoreFile 丢弃工作区里这些路径上的未暂存改动，把它们恢复成 index 里的
// 样子（`git checkout -- <paths>`）。
//
// 路径是变参、和 StageFile 一个口径：未暂存的重命名要新旧两个路径一起给，
// 否则只把新路径恢复出来，旧路径仍然是「被删掉」的状态，看着像丢弃没生效。
//
// 只对 git 已经认识的文件有效 —— 未跟踪的文件走 RemoveFiles：checkout 对
// 它是 "pathspec did not match any file(s) known to git"，翻译过来是一句
// 和用户做的事毫不相干的「引用不在了」。
func (r *Repo) RestoreFile(ctx context.Context, paths ...string) error {
	clean := cleanPaths(paths)
	if len(clean) == 0 {
		return nil
	}
	args := append([]string{"checkout", "--"}, clean...)
	_, err := run(ctx, r.Root, args...)
	return err
}

// RemoveFiles 把工作区里这些路径上的文件删掉，用来丢弃未跟踪的改动 ——
// git 还不认识它们，「恢复原状」就是从磁盘上拿走。
//
// 文件已经不在（用户刚在别处删了）不算失败：要的结果已经达成了。路径先
// 夹进仓库根、确认没跑出去（`..` 和绝对路径在这一步挡下 —— 拼接是纯字面
// 的，`a/../../x` 这种会在 Clean 之后被 Rel 认出来），因为这个函数真的会
// 删文件，是这一层里唯一不可逆的那把刀。仓库里指向外面的符号链接不受影响：
// os.Remove 摘的是链接本身，不是它指向的东西。
func (r *Repo) RemoveFiles(ctx context.Context, paths ...string) error {
	for _, p := range cleanPaths(paths) {
		full, err := r.worktreePath(p)
		if err != nil {
			return err
		}
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// worktreePath 把一个仓库内的相对路径拼成磁盘上的绝对路径，跑出仓库的
// 一律拒绝。
func (r *Repo) worktreePath(p string) (string, error) {
	// Windows 的 IsAbs 不认 "/etc/hosts" 这种没盘符的路径，不挡的话它会被
	// 字面拼进仓库里（Root\etc\hosts）—— 真撞上同名文件就删错了。以分隔符
	// 开头一律按仓库外的路径挡掉，和 Unix 上「绝对路径，拒绝」一个语义。
	if p == "" || filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return "", fmt.Errorf("路径不在仓库里: %q", p)
	}
	full := filepath.Join(r.Root, filepath.FromSlash(p))
	rel, err := filepath.Rel(r.Root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("路径跑到仓库外面去了: %q", p)
	}
	return full, nil
}

// AddIgnore 把一条路径写进仓库根的 .gitignore，一行一条（`/<path>`）。
//
// 带上前导斜杠：不带的话 `build` 会连子目录里的同名文件一起忽略，用户点
// 的是「忽略这一个」。同一行已经在了就什么都不做 —— .gitignore 是用户
// 自己的文件，重复追加是最容易把别人的文件写脏的一种做法。
func (r *Repo) AddIgnore(ctx context.Context, p string) error {
	if p == "" {
		return nil
	}
	line := "/" + strings.TrimPrefix(filepath.ToSlash(p), "/")
	path := filepath.Join(r.Root, ".gitignore")
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, l := range strings.Split(string(old), "\n") {
		if strings.TrimSpace(l) == line {
			return nil
		}
	}
	var b strings.Builder
	b.Write(old)
	if len(old) > 0 && !strings.HasSuffix(string(old), "\n") {
		b.WriteString("\n")
	}
	b.WriteString(line + "\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// StageHunk 把未暂存段里的某一段改动放进 index。
func (r *Repo) StageHunk(ctx context.Context, path, hunkID string) error {
	return r.applyHunk(ctx, path, hunkID, false)
}

// UnstageHunk 把已暂存段里的某一段改动撤出 index。
//
// 调用方必须保证目标不是「已暂存的新增文件」：反转 `@@ -0,0 +1,N @@`
// 会在 index 里留下一个空文件，而不是把条目删掉（和整文件 reset 不同）。
// 界面那边由 hunkStageable 挡着，只给整文件操作。
func (r *Repo) UnstageHunk(ctx context.Context, path, hunkID string) error {
	return r.applyHunk(ctx, path, hunkID, true)
}

// StageAll 把工作区和 index 的差异全部放进 index（含未跟踪和删除）。
//
// 不给 pathspec：`git add -A` 一条命令覆盖全仓库。界面上的「全部暂存」不能
// 靠遍历文件列表逐条 add —— 那个列表有未跟踪文件的条数上限，会漏。
func (r *Repo) StageAll(ctx context.Context) error {
	_, err := run(ctx, r.Root, "add", "-A")
	return err
}

// UnstageAll 把 index 里的全部改动撤出（重置暂存区到 HEAD，不影响工作区）。
func (r *Repo) UnstageAll(ctx context.Context) error {
	if r.baseRev(ctx) == emptyTree {
		_, err := run(ctx, r.Root, "rm", "--cached", "-r", "-q", ".")
		return err
	}
	_, err := run(ctx, r.Root, "reset", "-q")
	return err
}

// applyHunk 是 hunk 级暂存/取消暂存的共同实现：重新跑一条和界面展示
// 「同基线」的 diff，找出 hunkID 对应的那一段，原样切出来喂给
// `git apply --cached [--reverse]`。
//
// 同基线是硬要求：暂存方向的展示来自 worktree vs index，这里就重跑
// `git diff`；取消暂存方向的展示来自 index vs HEAD，这里就重跑
// `git diff --cached <base>`。hunk 的 ID 含 @@ 行号，基线一换就对不上。
//
// --whitespace=nowarn：用户配了 apply.whitespace=error/fix 时，git 会拒收
// 甚至改写内容，两条都不能要。
// --recount：上下文或行数头对不上时的兜底（git add -p 也是这么干的），
// 只在第一次失败后试一次，不掩盖真正的冲突。
// 绝不用 --3way：它会动工作区，还能静默按「我们的/他们的」选边。
func (r *Repo) applyHunk(ctx context.Context, path, hunkID string, reverse bool) error {
	var extra []string
	if reverse {
		extra = append(extra, "--cached", r.baseRev(ctx))
	}
	raw, err := run(ctx, r.Root, diffArgs(append(extra, "--", path)...)...)
	if err != nil {
		return err
	}
	patch, ok := diff.SliceHunk(string(raw), path, hunkID)
	if !ok {
		return ErrHunkGone
	}
	apply := []string{"apply", "--cached", "--whitespace=nowarn", "-"}
	if reverse {
		apply = append(apply, "--reverse")
	}
	if _, err := runIn(ctx, r.Root, []byte(patch), apply...); err != nil {
		if _, err2 := runIn(ctx, r.Root, []byte(patch), append(apply, "--recount")...); err2 != nil {
			return err
		}
	}
	return nil
}

// Commit 把 index 里的内容提交成一次提交，信息从管道喂进去。
//
// 用 `-F -` 而不是 `-m`：消息里可以有换行、引号、以 - 开头的行，走参数会
// 被命令行解析咬；管道是字节原样的（git 不认的编码也不会有机会损坏）。git
// 会忽略纯空白消息、给末尾补换行，这些都交给它自己。
func (r *Repo) Commit(ctx context.Context, message string) error {
	_, err := runIn(ctx, r.Root, []byte(message), "commit", "-F", "-")
	return err
}

// CommitAmend 修改上一次提交（git commit --amend -F -）。
func (r *Repo) CommitAmend(ctx context.Context, message string) error {
	_, err := runIn(ctx, r.Root, []byte(message), "commit", "--amend", "-F", "-")
	return err
}

// LastCommitMessage 返回最近一次提交的完整提交信息（修改上次提交时预填）。
func (r *Repo) LastCommitMessage(ctx context.Context) (string, error) {
	out, err := run(ctx, r.Root, "log", "-1", "--format=%B")
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// HasHead 检查仓库当前是否至少存在一次提交（HEAD 有效）。
func (r *Repo) HasHead(ctx context.Context) bool {
	_, err := run(ctx, r.Root, "rev-parse", "--verify", "--quiet", "HEAD")
	return err == nil
}

// ShortHead 返回 HEAD 的短哈希（提交成功后的提示里报一串给用户认）。
func (r *Repo) ShortHead(ctx context.Context) string {
	out, err := run(ctx, r.Root, "rev-parse", "--short", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

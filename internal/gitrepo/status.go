package gitrepo

import (
	"context"
	"strings"
)

// Status 是文件相对 HEAD 的变更类型。
type Status string

const (
	StatusModified  Status = "modified"
	StatusAdded     Status = "added"
	StatusDeleted   Status = "deleted"
	StatusRenamed   Status = "renamed"
	StatusCopied    Status = "copied"
	StatusUntracked Status = "untracked"
	StatusConflict  Status = "conflict"
)

// Label 返回状态的中文说明，用于 tooltip。
func (s Status) Label() string {
	switch s {
	case StatusModified:
		return "已修改"
	case StatusAdded:
		return "新增"
	case StatusDeleted:
		return "已删除"
	case StatusRenamed:
		return "重命名"
	case StatusCopied:
		return "已复制"
	case StatusUntracked:
		return "未跟踪"
	case StatusConflict:
		return "冲突"
	}
	return string(s)
}

// Letter 返回文件树里显示的单字母状态标记。
func (s Status) Letter() byte {
	switch s {
	case StatusAdded, StatusUntracked:
		return 'A'
	case StatusDeleted:
		return 'D'
	case StatusRenamed:
		return 'R'
	case StatusCopied:
		return 'C'
	case StatusConflict:
		return '!'
	}
	return 'M'
}

// Change 是 git status 报告的一个变更文件。
//
// Staged / Unstaged 分别对应 index 相对 HEAD 的变更（XY 里的 X）
// 和工作区相对 index 的变更（Y）。两个都为真表示「暂存了一部分，
// 之后又改了一些」，这在做暂存/提交功能时是必须区分的状态。
type Change struct {
	Path     string
	OldPath  string // 重命名/复制时的原路径
	Status   Status
	Staged   bool
	Unstaged bool
}

// Status 读取仓库的变更列表。
//
// 用 porcelain=v2 而不是 v1：v2 用 . 明确表示「这一侧没有变更」，
// 能把「已暂存的修改」和「未暂存的修改」分开，这是后面做暂存/提交的前提；
// v1 的 XY 在重命名等场景下有歧义。
func (r *Repo) Status(ctx context.Context) ([]Change, error) {
	out, err := run(ctx, r.Root,
		"-c", "core.quotePath=false",
		"status", "--porcelain=v2", "--branch", "-z",
	)
	if err != nil {
		return nil, err
	}
	return parseStatus(out), nil
}

// parseStatus 解析 `git status --porcelain=v2 --branch -z` 的输出。
//
// -z 用 NUL 分隔记录，路径里的空格和换行都不会造成歧义。重命名（类型 2）
// 记录的原路径是紧跟其后的另一个 NUL 分隔字段，所以循环里要额外吃掉一个。
func parseStatus(out []byte) []Change {
	fields := strings.Split(string(out), "\x00")
	var changes []Change

	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if f == "" || f[0] == '#' { // 空字段和 # 开头的分支头都跳过
			continue
		}

		switch f[0] {
		case '1': // 普通变更：1 <XY> <sub> <mH> <mI> <mW> <hH> <hI> <path>
			parts := strings.SplitN(f, " ", 9)
			if len(parts) < 9 {
				continue
			}
			x, y := parts[1][0], parts[1][1]
			changes = append(changes, Change{
				Path:     parts[8],
				Status:   statusOf(x, y, false, false),
				Staged:   x != '.',
				Unstaged: y != '.',
			})

		case '2': // 重命名/复制：2 <XY> ... <X><score> <path> NUL <origPath>
			parts := strings.SplitN(f, " ", 10)
			if len(parts) < 10 {
				continue
			}
			var orig string
			if i+1 < len(fields) {
				orig = fields[i+1]
				i++ // 原路径占了下一个字段
			}
			x, y := parts[1][0], parts[1][1]
			changes = append(changes, Change{
				Path:     parts[9],
				OldPath:  orig,
				Status:   statusOf(x, y, false, false),
				Staged:   x != '.',
				Unstaged: y != '.',
			})

		case 'u': // 冲突：u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
			parts := strings.SplitN(f, " ", 11)
			if len(parts) < 11 {
				continue
			}
			changes = append(changes, Change{
				Path:     parts[10],
				Status:   StatusConflict,
				Staged:   true,
				Unstaged: true,
			})

		case '?': // 未跟踪：? <path>
			parts := strings.SplitN(f, " ", 2)
			if len(parts) < 2 {
				continue
			}
			changes = append(changes, Change{
				Path:     parts[1],
				Status:   StatusUntracked,
				Unstaged: true,
			})

		case '!': // 被忽略，不显示
			continue
		}
	}
	return changes
}

// statusOf 把 porcelain v2 的 XY 两个状态字符合成一个显示用的状态。
//
// 一侧是 '.' 表示那一侧没有变更。两字符含义相同时取其一；不同时按
// 「重命名 > 复制 > 新增 > 删除 > 修改」取更显著的那个——比如
// 「已暂存重命名 + 工作区又改了内容」应该显示重命名而不是修改。
func statusOf(x, y byte, untracked, unmerged bool) Status {
	if unmerged {
		return StatusConflict
	}
	if untracked || x == '?' || y == '?' {
		return StatusUntracked
	}
	switch {
	case x == 'R' || y == 'R':
		return StatusRenamed
	case x == 'C' || y == 'C':
		return StatusCopied
	case x == 'A' || y == 'A':
		return StatusAdded
	case x == 'D' || y == 'D':
		return StatusDeleted
	}
	return StatusModified
}

package gitrepo

import (
	"context"
	"strconv"
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
// StagedStatus / UnstagedStatus 是两侧各自的变更类型（空串 = 这一侧没变），
// Status 是合成出来的显示用类型。
type Change struct {
	Path     string
	OldPath  string // 重命名/复制时的原路径
	Status   Status
	Staged   bool
	Unstaged bool

	StagedStatus   Status // index vs HEAD 那一侧
	UnstagedStatus Status // 工作区 vs index 那一侧
}

// Status 读取仓库的变更列表。
//
// 用 porcelain=v2 而不是 v1：v2 用 . 明确表示「这一侧没有变更」，
// 能把「已暂存的修改」和「未暂存的修改」分开，这是后面做暂存/提交的前提；
// v1 的 XY 在重命名等场景下有歧义。
//
// --untracked-files=all 是必须的：默认模式下 git 会把未跟踪的目录折叠成
// 一条记录（只给 "internal/"，不列里面的文件），那样整个目录的改动都看不见。
func (r *Repo) statusRaw(ctx context.Context) ([]byte, error) {
	return run(ctx, r.Root,
		"-c", "core.quotePath=false",
		"status", "--porcelain=v2", "--branch", "--untracked-files=all", "-z",
	)
}

func (r *Repo) Status(ctx context.Context) ([]Change, error) {
	out, err := r.statusRaw(ctx)
	if err != nil {
		return nil, err
	}
	return parseStatus(out), nil
}

// parseStatusBranch 解析 `git status --porcelain=v2 --branch -z` 中的上游分支与超前/落后数。
// 当未设置上游或处于分离 HEAD 时，返回 0, 0, ""。
func parseStatusBranch(out []byte) (ahead, behind int, upstream string) {
	fields := strings.Split(string(out), "\x00")
	for _, f := range fields {
		if !strings.HasPrefix(f, "# branch.") {
			continue
		}
		if strings.HasPrefix(f, "# branch.ab ") {
			// 格式：# branch.ab +<ahead> -<behind>
			parts := strings.Fields(f)
			for _, p := range parts[2:] {
				if strings.HasPrefix(p, "+") {
					ahead, _ = strconv.Atoi(p[1:])
				} else if strings.HasPrefix(p, "-") {
					behind, _ = strconv.Atoi(p[1:])
				}
			}
		} else if strings.HasPrefix(f, "# branch.upstream ") {
			upstream = strings.TrimSpace(strings.TrimPrefix(f, "# branch.upstream "))
		}
	}
	return
}

// UpstreamCounts 查询当前分支相对其配置的上游分支的超前（ahead）与落后（behind）提交数。
// 若无上游分支、处于分离 HEAD 或无提交，返回 0, 0, nil。
func (r *Repo) UpstreamCounts(ctx context.Context) (ahead, behind int, err error) {
	out, err := run(ctx, r.Root, "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
	if err != nil {
		return 0, 0, nil
	}
	fields := strings.Fields(string(out))
	if len(fields) >= 2 {
		ahead, _ = strconv.Atoi(fields[0])
		behind, _ = strconv.Atoi(fields[1])
		return ahead, behind, nil
	}
	return 0, 0, nil
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
				Path:           parts[8],
				Status:         statusOf(x, y, false, false),
				Staged:         x != '.',
				Unstaged:       y != '.',
				StagedStatus:   sideStatus(x),
				UnstagedStatus: sideStatus(y),
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
				Path:           parts[9],
				OldPath:        orig,
				Status:         statusOf(x, y, false, false),
				Staged:         x != '.',
				Unstaged:       y != '.',
				StagedStatus:   sideStatus(x),
				UnstagedStatus: sideStatus(y),
			})

		case 'u': // 冲突：u <XY> <sub> <m1> <m2> <m3> <mW> <h1> <h2> <h3> <path>
			parts := strings.SplitN(f, " ", 11)
			if len(parts) < 11 {
				continue
			}
			// 冲突在界面上只占「未暂存」那一段一张卡片（两边都没有可
			// 直接提交的内容），侧标也按这个口径给。
			changes = append(changes, Change{
				Path:           parts[10],
				Status:         StatusConflict,
				Staged:         true,
				Unstaged:       true,
				UnstagedStatus: StatusConflict,
			})

		case '?': // 未跟踪：? <path>
			parts := strings.SplitN(f, " ", 2)
			if len(parts) < 2 {
				continue
			}
			changes = append(changes, Change{
				Path:           parts[1],
				Status:         StatusUntracked,
				Unstaged:       true,
				UnstagedStatus: StatusUntracked,
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

// sideStatus 把 XY 里的单个字符翻译成状态；'.'（这一侧没有变更）返回空串。
//
// 分段展示后，每一段显示的是「这一侧自己的类型」：一个文件可以「已暂存
// 新增 + 工作区又改了」，两段的侧标就该一个是 A、一个是 M。
// 'T' 是类型变了（文件↔符号链接），显示上归入已修改。
func sideStatus(c byte) Status {
	switch c {
	case '.':
		return ""
	case 'M', 'T':
		return StatusModified
	case 'A':
		return StatusAdded
	case 'D':
		return StatusDeleted
	case 'R':
		return StatusRenamed
	case 'C':
		return StatusCopied
	case 'U':
		return StatusConflict
	}
	return StatusModified
}

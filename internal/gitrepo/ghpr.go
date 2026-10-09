package gitrepo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

var (
	// ErrGhNotFound 表示系统未安装 gh 命令行工具。
	ErrGhNotFound = errors.New("未检测到 gh (GitHub CLI) 工具，请先安装：brew install gh")

	// ErrGhNotAuth 表示 gh 尚未完成 GitHub 登录认证或 Token 已失效。
	ErrGhNotAuth = errors.New("gh 未登录或认证已失效，请在终端运行 gh auth login")
)

// 可被测试替换的函数钩子
var (
	lookPath           = exec.LookPath
	execCommandContext = exec.CommandContext
)

// PRItem 代表一个 GitHub Pull Request 的基本摘要信息。
type PRItem struct {
	Number     int       `json:"number"`
	Title      string    `json:"title"`
	Author     string    `json:"author"`
	HeadBranch string    `json:"headBranch"`
	BaseBranch string    `json:"baseBranch"`
	UpdatedAt  time.Time `json:"updatedAt"`
	URL        string    `json:"url"`
}

type ghPRItemJSON struct {
	Number      int             `json:"number"`
	Title       string          `json:"title"`
	Author      json.RawMessage `json:"author"`
	HeadRefName string          `json:"headRefName"`
	BaseRefName string          `json:"baseRefName"`
	UpdatedAt   time.Time       `json:"updatedAt"`
	URL         string          `json:"url"`
}

func parseAuthor(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		if obj.Login != "" {
			return obj.Login
		}
		return obj.Name
	}
	return ""
}

func ghEnv() []string {
	return append(os.Environ(),
		"GH_PROMPT_DISABLED=1",
		"NO_COLOR=1",
	)
}

func checkGh() error {
	if _, err := lookPath("gh"); err != nil {
		return ErrGhNotFound
	}
	return nil
}

func runGh(ctx context.Context, repoDir string, args ...string) ([]byte, error) {
	if err := checkGh(); err != nil {
		return nil, err
	}
	cmd := execCommandContext(ctx, "gh", args...)
	cmd.Dir = repoDir
	cmd.Env = ghEnv()

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
		return nil, parseGhError(err, msg)
	}
	return stdout.Bytes(), nil
}

func parseGhError(origErr error, msg string) error {
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "to re-authenticate") ||
		strings.Contains(low, "gh auth login") ||
		strings.Contains(low, "gh auth refresh") ||
		strings.Contains(low, "not logged into") ||
		strings.Contains(low, "authentication token") ||
		strings.Contains(low, "token in keyring is invalid") ||
		strings.Contains(low, "authentication required") ||
		strings.Contains(low, "failed to log in"):
		return fmt.Errorf("%w (%s)", ErrGhNotAuth, msg)
	case strings.Contains(low, "none of the git remotes point to a valid code host") ||
		strings.Contains(low, "could not determine repository") ||
		strings.Contains(low, "no github remote"):
		return fmt.Errorf("当前仓库未关联 GitHub 远程仓库: %s", msg)
	default:
		return fmt.Errorf("gh 执行失败: %s", msg)
	}
}

// ListPRs 通过 gh CLI 查询指定仓库下的打开 PR 列表。
func ListPRs(ctx context.Context, repoDir string) ([]PRItem, error) {
	if strings.TrimSpace(repoDir) == "" {
		return nil, errors.New("仓库路径不能为空")
	}
	out, err := runGh(ctx, repoDir, "pr", "list", "--json", "number,title,author,headRefName,baseRefName,updatedAt,url")
	if err != nil {
		return nil, err
	}
	var rawItems []ghPRItemJSON
	if err := json.Unmarshal(out, &rawItems); err != nil {
		return nil, fmt.Errorf("解析 PR 列表数据失败: %w", err)
	}
	items := make([]PRItem, len(rawItems))
	for i, r := range rawItems {
		items[i] = PRItem{
			Number:     r.Number,
			Title:      r.Title,
			Author:     parseAuthor(r.Author),
			HeadBranch: r.HeadRefName,
			BaseBranch: r.BaseRefName,
			UpdatedAt:  r.UpdatedAt,
			URL:        r.URL,
		}
	}
	return items, nil
}

// GetPRDiff 获取指定 PR 的 diff 并解析为 []diff.FileDiff。
func GetPRDiff(ctx context.Context, repoDir string, prNumber int) ([]diff.FileDiff, error) {
	if strings.TrimSpace(repoDir) == "" {
		return nil, errors.New("仓库路径不能为空")
	}
	if prNumber <= 0 {
		return nil, fmt.Errorf("无效的 PR 编号: %d", prNumber)
	}
	out, err := runGh(ctx, repoDir, "pr", "diff", strconv.Itoa(prNumber))
	if err != nil {
		return nil, err
	}
	parsed := diff.Parse(string(out))
	for i := range parsed {
		parsed[i].AssignHunkIDs()
	}
	return parsed, nil
}

// PostPRComment 为指定的 GitHub PR 发表评论。
func PostPRComment(ctx context.Context, repoDir string, prNumber int, body string) error {
	if strings.TrimSpace(repoDir) == "" {
		return errors.New("仓库路径不能为空")
	}
	if prNumber <= 0 {
		return fmt.Errorf("无效的 PR 编号: %d", prNumber)
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("评论内容不能为空")
	}
	_, err := runGh(ctx, repoDir, "pr", "comment", strconv.Itoa(prNumber), "--body", body)
	return err
}

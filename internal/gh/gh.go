// Package gh 封装 GitHub API：解析 PR 链接、拉取 PR 元数据和 diff、按需取文件全文。
package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// PRRef 定位一个 Pull Request。
type PRRef struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

var prURLRe = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/pull/(\d+)`)

// ParsePRURL 解析 https://github.com/owner/repo/pull/123（允许带 /files 等后缀）。
func ParsePRURL(raw string) (PRRef, error) {
	m := prURLRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return PRRef{}, fmt.Errorf("无法识别的 PR 链接，格式应为 https://github.com/owner/repo/pull/123")
	}
	n, _ := strconv.Atoi(m[3])
	return PRRef{Owner: m[1], Repo: strings.TrimSuffix(m[2], ".git"), Number: n}, nil
}

// PRInfo 是 PR 的元数据 + 完整 diff 文本。
type PRInfo struct {
	Ref     PRRef  `json:"ref"`
	Title   string `json:"title"`
	Body    string `json:"body"`
	Author  string `json:"author"`
	HeadSHA string `json:"headSha"`
	BaseSHA string `json:"baseSha"`
	HTMLURL string `json:"htmlUrl"`
	Diff    string `json:"-"`
}

// FetchPR 拉取 PR 元数据和 unified diff。token 为空时只能访问公开仓库（有限流）。
func FetchPR(ctx context.Context, ref PRRef, token string) (*PRInfo, error) {
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls/%d", ref.Owner, ref.Repo, ref.Number)

	metaBody, err := get(ctx, api, token, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var meta struct {
		Title   string `json:"title"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
		Head struct {
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			SHA string `json:"sha"`
		} `json:"base"`
	}
	if err := json.Unmarshal(metaBody, &meta); err != nil {
		return nil, fmt.Errorf("解析 PR 元数据失败: %w", err)
	}

	diffText, err := get(ctx, api, token, "application/vnd.github.v3.diff")
	if err != nil {
		return nil, err
	}

	return &PRInfo{
		Ref:     ref,
		Title:   meta.Title,
		Body:    meta.Body,
		Author:  meta.User.Login,
		HeadSHA: meta.Head.SHA,
		BaseSHA: meta.Base.SHA,
		HTMLURL: meta.HTMLURL,
		Diff:    string(diffText),
	}, nil
}

// FetchFile 取某个 git ref 下文件的全文，给 AI 解释提供上下文。
func FetchFile(ctx context.Context, ref PRRef, path, gitRef, token string) (string, error) {
	segs := strings.Split(path, "/")
	for i := range segs {
		segs[i] = url.PathEscape(segs[i])
	}
	u := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		ref.Owner, ref.Repo, strings.Join(segs, "/"), url.QueryEscape(gitRef))
	b, err := get(ctx, u, token, "application/vnd.github.raw")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func get(ctx context.Context, u, token, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub 失败: %w", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	switch resp.StatusCode {
	case http.StatusOK:
		return b, nil
	case http.StatusNotFound:
		return nil, fmt.Errorf("仓库或 PR 不存在（如果是私有仓库，请在设置里配置 GitHub Token）")
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf("GitHub API 限流或权限不足，建议在设置里配置 GitHub Token")
	default:
		return nil, fmt.Errorf("GitHub API 返回 %s", resp.Status)
	}
}

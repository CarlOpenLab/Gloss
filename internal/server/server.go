// Package server 提供 HTTP API：创建会话（PR / 本地 diff）、按 hunk 流式解释、SPA 静态服务。
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"diff-explainer/internal/diff"
	"diff-explainer/internal/gh"
	"diff-explainer/internal/llm"
)

// Session 是一次解析结果（PR 或本地 diff），存在内存里供解释接口引用。
type Session struct {
	ID      string
	Source  string // "pr" | "diff"
	Title   string
	PR      *gh.PRInfo
	GHToken string
	Files   []diff.FileDiff
	Created time.Time
}

type Server struct {
	static fs.FS

	mu       sync.Mutex
	sessions map[string]*Session
	cache    map[string]string // hunkID -> 解释结果
}

func New(static fs.FS) *Server {
	return &Server{
		static:   static,
		sessions: map[string]*Session{},
		cache:    map[string]string{},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/session/pr", s.handlePRSession)
	mux.HandleFunc("POST /api/session/diff", s.handleDiffSession)
	mux.HandleFunc("POST /api/explain", s.handleExplain)
	mux.HandleFunc("POST /api/test-provider", s.handleTestProvider)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "model": (*llm.Provider)(nil).EffectiveModel()})
	})
	mux.Handle("/", s.spaHandler())
	return mux
}

// ---------- 会话创建 ----------

func (s *Server) handlePRSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if !decode(w, r, &req) {
		return
	}
	ref, err := gh.ParsePRURL(req.URL)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	token := headerOrEnv(r, "X-GitHub-Token", "GITHUB_TOKEN")
	info, err := gh.FetchPR(r.Context(), ref, token)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	files := diff.Parse(info.Diff)
	if len(files) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, errors.New("该 PR 没有可展示的文本变更（可能只有二进制文件）"))
		return
	}
	sess := s.store(&Session{
		ID: randID(), Source: "pr", Title: info.Title,
		PR: info, GHToken: token, Files: files,
	})
	writeJSON(w, http.StatusOK, sessionResp(sess, info.HTMLURL))
}

func (s *Server) handleDiffSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Raw   string `json:"raw"`
		Title string `json:"title"`
	}
	if !decode(w, r, &req) {
		return
	}
	files := diff.Parse(req.Raw)
	if len(files) == 0 {
		writeErr(w, http.StatusUnprocessableEntity, errors.New("没有从输入中解析出 diff，请确认粘贴的是 git diff 的输出"))
		return
	}
	if req.Title == "" {
		req.Title = "本地 Diff"
	}
	sess := s.store(&Session{ID: randID(), Source: "diff", Title: req.Title, Files: files})
	writeJSON(w, http.StatusOK, sessionResp(sess, ""))
}

func sessionResp(sess *Session, htmlURL string) map[string]any {
	adds, dels := 0, 0
	for _, f := range sess.Files {
		adds += f.AddCount
		dels += f.DelCount
	}
	return map[string]any{
		"sessionId": sess.ID,
		"source":    sess.Source,
		"title":     sess.Title,
		"htmlUrl":   htmlURL,
		"files":     sess.Files,
		"totalAdd":  adds,
		"totalDel":  dels,
	}
}

// ---------- AI 解释（SSE） ----------

func (s *Server) handleExplain(w http.ResponseWriter, r *http.Request) {
	var req struct {
		SessionID string        `json:"sessionId"`
		FileIndex int           `json:"fileIndex"`
		HunkID    string        `json:"hunkId"`
		Provider  *llm.Provider `json:"provider"`
	}
	if !decode(w, r, &req) {
		return
	}
	s.mu.Lock()
	sess, ok := s.sessions[req.SessionID]
	s.mu.Unlock()
	if !ok {
		writeErr(w, http.StatusNotFound, errors.New("会话不存在或已过期，请重新加载 diff"))
		return
	}
	if req.FileIndex < 0 || req.FileIndex >= len(sess.Files) {
		writeErr(w, http.StatusBadRequest, errors.New("文件索引越界"))
		return
	}
	file := sess.Files[req.FileIndex]
	hunkIdx := -1
	for i := range file.Hunks {
		if file.Hunks[i].ID == req.HunkID {
			hunkIdx = i
			break
		}
	}
	if hunkIdx < 0 {
		writeErr(w, http.StatusNotFound, errors.New("找不到对应的代码块"))
		return
	}
	hunk := file.Hunks[hunkIdx]

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, errors.New("当前环境不支持流式响应"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(v map[string]any) bool {
		b, _ := json.Marshal(v)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	// 供应商解析：请求体里的 provider 优先；其次 X-Anthropic-Key 头（兼容 curl 调用）；
	// 都没有则传 nil，走 Anthropic 环境变量默认凭证链
	provider := req.Provider
	if provider != nil && provider.Type == "" {
		provider = nil
	}
	if provider == nil {
		if key := strings.TrimSpace(r.Header.Get("X-Anthropic-Key")); key != "" {
			provider = &llm.Provider{Name: "Anthropic", Type: "anthropic", APIKey: key}
		}
	}
	if provider == nil && os.Getenv("ANTHROPIC_API_KEY") == "" && os.Getenv("ANTHROPIC_AUTH_TOKEN") == "" {
		send(map[string]any{"type": "error", "message": "未配置模型供应商：请在右上角「设置」里添加供应商，或设置环境变量 ANTHROPIC_API_KEY 后重启服务"})
		return
	}

	// 缓存命中：整块返回（缓存 key 带模型名，不同模型结果分开缓存）
	cacheKey := hunk.ID + "|" + provider.EffectiveModel()
	s.mu.Lock()
	cached, hit := s.cache[cacheKey]
	s.mu.Unlock()
	if hit {
		send(map[string]any{"type": "delta", "text": cached})
		send(map[string]any{"type": "done", "cached": true})
		return
	}

	// PR 会话：尽量拉取文件全文作为上下文（本地 diff 没有全文可用）
	fileCtx := ""
	if sess.PR != nil {
		gitRef, path := sess.PR.HeadSHA, file.NewPath
		if file.Status == "deleted" {
			gitRef, path = sess.PR.BaseSHA, file.OldPath
		}
		if path != "" {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			if content, err := gh.FetchFile(ctx, sess.PR.Ref, path, gitRef, sess.GHToken); err == nil {
				fileCtx = window(content, hunk, 80, 16000)
			}
			cancel()
		}
	}

	system, user := buildPrompts(sess, file, hunk, hunkIdx, fileCtx)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	var full strings.Builder
	_, err := llm.StreamExplanation(ctx, provider, system, user, func(text string) {
		full.WriteString(text)
		if !send(map[string]any{"type": "delta", "text": text}) {
			cancel() // 前端断开，终止上游流
		}
	})
	if err != nil {
		// 出错或客户端断开时不落缓存，避免缓存到截断的内容
		if !errors.Is(err, context.Canceled) {
			send(map[string]any{"type": "error", "message": "调用模型失败: " + err.Error()})
		}
		return
	}
	if full.Len() > 0 {
		s.mu.Lock()
		s.cache[cacheKey] = full.String()
		s.mu.Unlock()
		send(map[string]any{"type": "done", "cached": false})
	}
}

// handleTestProvider 用极小的请求验证供应商配置可用。
func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider *llm.Provider `json:"provider"`
	}
	if !decode(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	start := time.Now()
	reply, err := llm.StreamExplanation(ctx, req.Provider, "You are a connectivity probe. Reply with exactly: pong", "ping", func(string) {})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if len(reply) > 100 {
		reply = reply[:100]
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ms": time.Since(start).Milliseconds(), "reply": reply})
}

// ---------- prompt 构建 ----------

const systemPrompt = `你是一位资深的 Code Review 专家，擅长用简洁清晰的中文解释代码变更。
用户会给你一个 Pull Request（或本地 diff）中某个代码块（hunk）的内容，以及它所在文件的上下文。

请解释：
1. **这段变更做了什么**：新增的代码实现了什么；删除的代码原本是干什么的、为什么可能被删掉
2. **变更目的**：结合 PR 描述和文件上下文推断作者意图
3. **值得注意的点**（可选）：潜在风险、副作用或需要 reviewer 重点关注的地方，没有就省略

要求：
- 先给一句话总结，再用要点展开，总长度控制在 250 字以内
- 不要逐行翻译代码，讲清楚"为什么"比"是什么"更重要
- **只解释给你的这一个变更块**：引用它里面真实出现的函数名、变量名、字符串；上下文里带行号，需要指位置时用「第 N 行」这种说法。不要泛泛介绍整个文件，也不要解释同一个文件里的其它变更块
- 如果从文件上下文能看出这个改动的调用方、被调用方或影响面，用一句话点明；看不出来就不要猜
- 不要复述用户给你的 diff 文本，也不要用"如下"之类的开场白
- 用中文回答`

func buildPrompts(sess *Session, file diff.FileDiff, hunk diff.Hunk, hunkIdx int, fileCtx string) (string, string) {
	var b strings.Builder
	if sess.PR != nil {
		b.WriteString("## PR 信息\n")
		fmt.Fprintf(&b, "标题：%s\n作者：%s\n", sess.PR.Title, sess.PR.Author)
		if body := sess.PR.Body; body != "" {
			if len(body) > 2000 {
				body = body[:2000] + "…（已截断）"
			}
			fmt.Fprintf(&b, "描述：%s\n", body)
		}
		b.WriteByte('\n')
	}

	statusText := map[string]string{"added": "新增", "deleted": "删除", "modified": "修改", "renamed": "重命名"}[file.Status]
	path := file.NewPath
	if path == "" {
		path = file.OldPath
	}
	fmt.Fprintf(&b, "## 文件\n路径：%s（%s，+%d -%d）\n", displayPath(file), statusText, file.AddCount, file.DelCount)

	// 一个文件常有多个变更块：点名当前是哪一个，并列出其它块的位置，
	// 避免模型把同文件其它改动也当成这次的解释对象。
	if len(file.Hunks) > 1 {
		fmt.Fprintf(&b, "该文件共有 %d 个变更块，下面要给解释的是第 %d 个。其它变更块的位置（不要解释它们）：\n", len(file.Hunks), hunkIdx+1)
		for i, h := range file.Hunks {
			if i == hunkIdx {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", h.Header)
		}
	}
	b.WriteByte('\n')

	if fileCtx != "" {
		fmt.Fprintf(&b, "以下是该文件的相关内容（每行行首的 N| 是文件里的真实行号）：\n```%s\n%s\n```\n\n", langOf(path), fileCtx)
	}

	fmt.Fprintf(&b, "## 需要解释的变更块（老文件第 %d 行起 %d 行 → 新文件第 %d 行起 %d 行）\n```diff\n",
		hunk.OldStart, hunk.OldLines, hunk.NewStart, hunk.NewLines)
	b.WriteString(hunk.Raw())
	b.WriteString("```\n")
	return systemPrompt, b.String()
}

func displayPath(f diff.FileDiff) string {
	if f.Status == "renamed" {
		return f.OldPath + " → " + f.NewPath
	}
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// window 截取文件中 hunk 前后 around 行的内容，行首带上真实行号，
// 这样模型能精确指认「第几行」，也更容易把改动和上下文对上。
func window(content string, h diff.Hunk, around, maxChars int) string {
	lines := strings.Split(content, "\n")
	start, end := 0, 0
	if h.NewLines > 0 || h.NewStart > 0 {
		start = max(0, h.NewStart-1-around)
		end = min(len(lines), h.NewStart+h.NewLines+around)
	} else {
		start = max(0, h.OldStart-1-around)
		end = min(len(lines), h.OldStart+h.OldLines+around)
	}
	var b strings.Builder
	if start > 0 {
		fmt.Fprintf(&b, "…（前面省略 %d 行）\n", start)
	}
	for i := start; i < end; i++ {
		fmt.Fprintf(&b, "%d| %s\n", i+1, lines[i])
	}
	if end < len(lines) {
		fmt.Fprintf(&b, "\n…（后面省略 %d 行）", len(lines)-end)
	}
	out := b.String()
	if len(out) > maxChars {
		// 按行边界截断，避免把某一行切一半
		cut := strings.LastIndex(out[:maxChars], "\n")
		if cut < 0 {
			cut = maxChars
		}
		out = out[:cut] + "\n…（文件过长，已截断）"
	}
	return out
}

func langOf(path string) string {
	i := strings.LastIndex(path, ".")
	if i < 0 {
		return ""
	}
	m := map[string]string{
		"go": "go", "ts": "typescript", "tsx": "typescript", "js": "javascript", "jsx": "javascript",
		"vue": "vue", "py": "python", "java": "java", "rs": "rust", "rb": "ruby", "php": "php",
		"c": "c", "h": "c", "cpp": "cpp", "hpp": "cpp", "cc": "cpp", "cs": "csharp",
		"css": "css", "scss": "scss", "html": "html", "json": "json", "md": "markdown",
		"yml": "yaml", "yaml": "yaml", "toml": "toml", "sh": "bash", "sql": "sql", "swift": "swift", "kt": "kotlin",
	}
	return m[strings.ToLower(path[i+1:])]
}

// ---------- 静态资源（SPA） ----------

func (s *Server) spaHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p != "" {
			if f, err := s.static.Open(p); err == nil {
				_ = f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}

// ---------- 工具 ----------

func (s *Server) store(sess *Session) *Session {
	sess.Created = time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	// 简单清理：会话数超 200 时删掉最老的
	if len(s.sessions) >= 200 {
		var oldestID string
		var oldest time.Time
		for id, v := range s.sessions {
			if oldestID == "" || v.Created.Before(oldest) {
				oldestID, oldest = id, v.Created
			}
		}
		delete(s.sessions, oldestID)
	}
	s.sessions[sess.ID] = sess
	return sess
}

func headerOrEnv(r *http.Request, header, env string) string {
	if v := strings.TrimSpace(r.Header.Get(header)); v != "" {
		return v
	}
	return os.Getenv(env)
}

func randID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("请求体解析失败: %v", err))
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}

package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"
)

// streamOpenAI 调 OpenAI 兼容的 /chat/completions 流式接口。
// 覆盖 DeepSeek / Kimi(Moonshot) / 通义(DashScope 兼容模式) / 智谱 GLM / OpenRouter / Ollama 等。
func streamOpenAI(ctx context.Context, p *Provider, system, user string, onDelta func(text string)) (string, error) {
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		return "", ErrNoBaseURL
	}
	if p.Model == "" {
		return "", ErrNoModel
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model":      p.Model,
		"stream":     true,
		"max_tokens": 2048,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", strings.NewReader(string(reqBody)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := openAIClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求供应商失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			Message string `json:"message"`
		}
		msg := ""
		if json.Unmarshal(body, &apiErr) == nil {
			if apiErr.Error.Message != "" {
				msg = apiErr.Error.Message
			} else if apiErr.Message != "" {
				msg = apiErr.Message
			}
		}
		if msg == "" {
			msg = truncate(string(body), 300)
		}
		return "", &statusError{Code: resp.StatusCode, Message: msg}
	}

	return readOpenAIStream(resp.Body, onDelta)
}

// 流式请求不设整体超时，生命周期由 ctx（客户端断开）控制
var openAIClient = &http.Client{}

func readOpenAIStream(body io.Reader, onDelta func(string)) (string, error) {
	var full strings.Builder
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil || len(chunk.Choices) == 0 {
			continue
		}
		if text := chunk.Choices[0].Delta.Content; text != "" {
			full.WriteString(text)
			onDelta(text)
		}
	}
	if err := sc.Err(); err != nil {
		return full.String(), err
	}
	return full.String(), nil
}

// truncate 按字节截断，但不把一个字符切成两半 —— 切面落在字符中间时，
// 末尾会冒出一个乱码方块，而供应商的中文报错（自己搭的网关照抄一句
// 中文说明）正好会踩上。往前退到最后一个完整的字符再切。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

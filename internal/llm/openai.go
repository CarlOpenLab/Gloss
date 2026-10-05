package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// streamOpenAI 调 OpenAI 兼容的 /chat/completions 流式接口。
// 覆盖 DeepSeek / Kimi(Moonshot) / 通义(DashScope 兼容模式) / 智谱 GLM / OpenRouter / Ollama 等。
func streamOpenAI(ctx context.Context, p *Provider, system, user string, onDelta func(text string)) (string, error) {
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		return "", errors.New("OpenAI 兼容供应商需要填写 Base URL")
	}
	if p.Model == "" {
		return "", errors.New("OpenAI 兼容供应商需要填写模型名称")
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
		if json.Unmarshal(body, &apiErr) == nil {
			if m := apiErr.Error.Message; m != "" {
				return "", fmt.Errorf("供应商返回 %d: %s", resp.StatusCode, m)
			}
			if apiErr.Message != "" {
				return "", fmt.Errorf("供应商返回 %d: %s", resp.StatusCode, apiErr.Message)
			}
		}
		return "", fmt.Errorf("供应商返回 %d: %s", resp.StatusCode, truncate(string(body), 300))
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

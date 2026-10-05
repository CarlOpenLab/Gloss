// Package llm 封装多家模型供应商的流式调用。
package llm

import (
	"context"
	"fmt"
	"os"
)

// Provider 描述一个模型供应商配置。
type Provider struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // "anthropic" | "openai"（OpenAI 兼容接口）
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`
}

// DefaultModel 解释代码变更用 Sonnet 级别即可，性价比最高；可用 ANTHROPIC_MODEL 覆盖。
const DefaultModel = "claude-sonnet-5-5"

// StreamExplanation 按供应商类型分发到对应实现。
// p 为 nil 时使用 Anthropic 环境变量默认凭证链（ANTHROPIC_API_KEY / ANTHROPIC_AUTH_TOKEN 等）。
func StreamExplanation(ctx context.Context, p *Provider, system, user string, onDelta func(text string)) (string, error) {
	switch {
	case p == nil || p.Type == "" || p.Type == "anthropic":
		return streamAnthropic(ctx, p, system, user, onDelta)
	case p.Type == "openai":
		return streamOpenAI(ctx, p, system, user, onDelta)
	default:
		return "", fmt.Errorf("不支持的供应商类型: %s", p.Type)
	}
}

// EffectiveModel 返回生效的模型名（用于缓存 key 区分不同模型的解释结果）。
func (p *Provider) EffectiveModel() string {
	if p != nil && p.Model != "" {
		return p.Model
	}
	if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
		return m
	}
	return DefaultModel
}

// ErrRefusal 表示模型拒绝回答（极少见，一般说明内容触发了安全策略）。
var ErrRefusal = errString("模型拒绝了解释这段代码")

type errString string

func (e errString) Error() string { return string(e) }

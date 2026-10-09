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
	if onDelta == nil {
		onDelta = func(string) {}
	}
	switch {
	case p == nil || p.Type == "" || p.Type == "anthropic":
		return streamAnthropic(ctx, p, system, user, onDelta)
	case p.Type == "openai":
		return streamOpenAI(ctx, p, system, user, onDelta)
	default:
		return "", fmt.Errorf("不支持的供应商类型: %s", p.Type)
	}
}

// Ready 报告这个供应商有没有可以用的凭证。false 只覆盖一种情况：走的是
// Anthropic 那条路（没填类型也算它），但哪里都没有 key —— 自己配的没有，
// SDK 会去读的那几个环境变量也没有。
//
// 判据宁可宽：说「没有」就是把一次本来可能成功的请求挡在门外。所以
// OpenAI 兼容那一路一律放行 —— 本机跑的 Ollama 本来就不需要 key，
// 用户自定义的网关也可能不校验。Anthropic 的显式 profile 和联邦令牌
// 一并认：它们都是 SDK 真会去读的来源，轮不到我们替它说不行。
func (p *Provider) Ready() bool {
	if p != nil && p.APIKey != "" {
		return true
	}
	if p != nil && p.Type != "" && p.Type != "anthropic" {
		return true
	}
	for _, env := range []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_PROFILE", "ANTHROPIC_FEDERATION_RULE_ID",
	} {
		if os.Getenv(env) != "" {
			return true
		}
	}
	return false
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

// 这两种是配置没填全，不是请求失败 —— 界面上要指去设置，不该说「失败了」。
// 见 Describe。
var (
	ErrNoBaseURL = errString("OpenAI 兼容供应商需要填写 Base URL")
	ErrNoModel   = errString("OpenAI 兼容供应商需要填写模型名称")
)

type errString string

func (e errString) Error() string { return string(e) }

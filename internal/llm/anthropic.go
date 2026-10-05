package llm

import (
	"context"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// streamAnthropic 调 Anthropic Messages API（流式）。
// p 为 nil 时走 SDK 默认凭证链（ANTHROPIC_API_KEY → ANTHROPIC_AUTH_TOKEN → …）；
// p 中的 BaseURL/APIKey 非空时覆盖对应配置；Model 为空时用 ANTHROPIC_MODEL 或 DefaultModel。
func streamAnthropic(ctx context.Context, p *Provider, system, user string, onDelta func(text string)) (string, error) {
	var opts []option.RequestOption
	if p != nil && p.APIKey != "" {
		opts = append(opts, option.WithAPIKey(p.APIKey))
	}
	base := os.Getenv("ANTHROPIC_BASE_URL")
	if p != nil && p.BaseURL != "" {
		base = p.BaseURL
	}
	if base != "" {
		opts = append(opts, option.WithBaseURL(base))
	}
	client := anthropic.NewClient(opts...)

	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.EffectiveModel()),
		MaxTokens: 2048, // 提示词明确要求简洁，不需要长输出
		System:    []anthropic.TextBlockParam{{Text: system}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(user)),
		},
	})

	var full strings.Builder
	message := anthropic.Message{}
	for stream.Next() {
		event := stream.Current()
		_ = message.Accumulate(event)
		if ev, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
			if d, ok := ev.Delta.AsAny().(anthropic.TextDelta); ok {
				full.WriteString(d.Text)
				onDelta(d.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return full.String(), err
	}
	if message.StopReason == anthropic.StopReasonRefusal {
		return full.String(), ErrRefusal
	}
	return full.String(), nil
}

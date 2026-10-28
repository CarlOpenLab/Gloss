package llm

import (
	"context"
	"fmt"
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
		// 累加失败说明事件序已经乱了（协议异常）：再走下去 StopReason 会
		// 缺失，Refusal 检测就静默失效，不如直接报错。
		if err := message.Accumulate(event); err != nil {
			_ = stream.Close()
			return full.String(), fmt.Errorf("解析流式事件失败: %w", err)
		}
		if ev, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
			if d, ok := ev.Delta.AsAny().(anthropic.TextDelta); ok {
				full.WriteString(d.Text)
				onDelta(d.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return full.String(), wrapStatus(err)
	}
	if message.StopReason == anthropic.StopReasonRefusal {
		return full.String(), ErrRefusal
	}
	return full.String(), nil
}

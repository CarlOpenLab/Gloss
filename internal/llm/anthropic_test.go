package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// sse 写一条 SSE 事件。SDK 的解码器按 event 行分发（不读 JSON 里的 type），
// 所以 event 名不能省。
func sse(w http.ResponseWriter, event, data string) {
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

// fakesse 起一个回 Anthropic Messages SSE 流的假服务，收到的请求头和体
// 记下来供断言。status 非 0 时直接回那个错误状态。
func fakesse(t *testing.T, status int, events ...[2]string) (*httptest.Server, *[]string) {
	t.Helper()
	var got []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("x-api-key"))
		if status != 0 {
			w.WriteHeader(status)
			fmt.Fprint(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, e := range events {
			sse(w, e[0], e[1])
		}
	}))
	t.Cleanup(ts.Close)
	return ts, &got
}

func TestStreamAnthropic(t *testing.T) {
	ts, got := fakesse(t, 0,
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_0","type":"message","role":"assistant","model":"m","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"你好"}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"，世界"}}`},
		[2]string{"content_block_stop", `{"type":"content_block_stop","index":0}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":5}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)
	p := &Provider{Type: "anthropic", BaseURL: ts.URL, APIKey: "test-key", Model: "test-model"}
	var deltas []string
	full, err := streamAnthropic(context.Background(), p, "sys", "usr", func(s string) {
		deltas = append(deltas, s)
	})
	if err != nil {
		t.Fatal(err)
	}
	if full != "你好，世界" {
		t.Errorf("full: %q", full)
	}
	if len(deltas) != 2 {
		t.Errorf("deltas: %v", deltas)
	}
	if len(*got) != 1 || (*got)[0] != "test-key" {
		t.Errorf("请求头里没有带上 key: %v", *got)
	}
}

func TestStreamAnthropicRefusal(t *testing.T) {
	ts, _ := fakesse(t, 0,
		[2]string{"message_start", `{"type":"message_start","message":{"id":"msg_0","type":"message","role":"assistant","model":"m","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":1,"output_tokens":0}}}`},
		[2]string{"content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`},
		[2]string{"content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"我不能"}}`},
		[2]string{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"refusal","stop_sequence":null},"usage":{"output_tokens":2}}`},
		[2]string{"message_stop", `{"type":"message_stop"}`},
	)
	p := &Provider{Type: "anthropic", BaseURL: ts.URL, APIKey: "k", Model: "m"}
	_, err := streamAnthropic(context.Background(), p, "s", "u", func(string) {})
	if !errors.Is(err, ErrRefusal) {
		t.Errorf("stop_reason=refusal 应该报 ErrRefusal，实际: %v", err)
	}
}

func TestStreamAnthropicStatusError(t *testing.T) {
	ts, _ := fakesse(t, http.StatusUnauthorized)
	p := &Provider{Type: "anthropic", BaseURL: ts.URL, APIKey: "bad", Model: "m"}
	_, err := streamAnthropic(context.Background(), p, "s", "u", func(string) {})
	var se *statusError
	if !errors.As(err, &se) {
		t.Fatalf("SDK 错误该收成 statusError，实际: %v", err)
	}
	if se.Code != http.StatusUnauthorized || se.Type != "authentication_error" {
		t.Errorf("statusError = %+v", se)
	}
	if !strings.Contains(se.Message, "invalid x-api-key") {
		t.Errorf("Message = %q", se.Message)
	}
}

func TestEffectiveModel(t *testing.T) {
	t.Setenv("ANTHROPIC_MODEL", "")
	if got := (&Provider{Model: "mine"}).EffectiveModel(); got != "mine" {
		t.Errorf("显式模型被覆盖: %q", got)
	}
	t.Setenv("ANTHROPIC_MODEL", "from-env")
	if got := (&Provider{}).EffectiveModel(); got != "from-env" {
		t.Errorf("环境变量没生效: %q", got)
	}
	t.Setenv("ANTHROPIC_MODEL", "")
	if got := (&Provider{}).EffectiveModel(); got != DefaultModel {
		t.Errorf("默认模型 = %q, want %q", got, DefaultModel)
	}
	if got := (*Provider)(nil).EffectiveModel(); got != DefaultModel {
		t.Errorf("nil provider = %q, want %q", got, DefaultModel)
	}
}

func TestStreamExplanationDispatch(t *testing.T) {
	// openai 类型走 OpenAI 兼容那一路
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"好\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()
	p := &Provider{Type: "openai", BaseURL: ts.URL, Model: "m"}
	full, err := StreamExplanation(context.Background(), p, "s", "u", nil)
	if err != nil || full != "好" {
		t.Errorf("openai 分发失败: %q %v", full, err)
	}

	// 认不出来的类型直接报错，不该静默落空
	if _, err := StreamExplanation(context.Background(), &Provider{Type: "gemini"}, "s", "u", nil); err == nil ||
		!strings.Contains(err.Error(), "gemini") {
		t.Errorf("未知类型应报错，实际: %v", err)
	}
}

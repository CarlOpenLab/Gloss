package llm

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamOpenAI(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing auth header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"，世界\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{}}]}\n\n") // 空 delta 应被跳过
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer ts.Close()

	p := &Provider{Type: "openai", BaseURL: ts.URL + "/v1", APIKey: "test-key", Model: "test-model"}
	var deltas []string
	full, err := streamOpenAI(context.Background(), p, "sys", "usr", func(s string) {
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
}

func TestStreamOpenAIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"error":{"message":"Invalid API key"}}`)
	}))
	defer ts.Close()

	p := &Provider{Type: "openai", BaseURL: ts.URL, APIKey: "bad", Model: "m"}
	_, err := streamOpenAI(context.Background(), p, "s", "u", func(string) {})
	if err == nil || !strings.Contains(err.Error(), "Invalid API key") {
		t.Errorf("expect api error message, got: %v", err)
	}
}

func TestStreamOpenAIValidation(t *testing.T) {
	if _, err := streamOpenAI(context.Background(), &Provider{Type: "openai"}, "s", "u", func(string) {}); err == nil {
		t.Error("expect error for empty baseURL")
	}
	if _, err := streamOpenAI(context.Background(), &Provider{Type: "openai", BaseURL: "http://x"}, "s", "u", func(string) {}); err == nil {
		t.Error("expect error for empty model")
	}
}

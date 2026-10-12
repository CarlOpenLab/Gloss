package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/llm"
	"github.com/CarlOpenLab/Gloss/internal/store"
)

// fakeLLM 起一个假的 OpenAI 兼容接口，按片返回一段回复。
// 返回的 *int 记录被调用了几次 —— 用来验证缓存真的生效了。
func fakeLLM(t *testing.T, chunks ...string) (*httptest.Server, *int) {
	t.Helper()
	calls := new(int)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		*calls++
		w.Header().Set("Content-Type", "text/event-stream")
		for _, c := range chunks {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", c)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

// withProvider 给 app 换上一个指向假接口的 store。
func withProvider(t *testing.T, a *app, srv *httptest.Server) {
	t.Helper()
	s := store.Open(t.TempDir())
	s.SetConfig(store.Config{Providers: []llm.Provider{{
		Name: "假接口", Type: "openai", BaseURL: srv.URL, APIKey: "k", Model: "test-model",
	}}})
	a.attachStore(s)
}

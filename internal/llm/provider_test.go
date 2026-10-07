package llm

import "testing"

// Ready 管的是「要不要提前把这一趟拦下来」：只有在能确定没有凭证的时候
// 才说不。判错的方向不一样代价差很多 —— 说「有」顶多发一次必然失败的请求，
// 说「没有」会把一个本来能用的配置挡在门外。
func TestProviderReady(t *testing.T) {
	// 这些是本机的环境变量，测试里先清干净
	for _, env := range []string{
		"ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_PROFILE", "ANTHROPIC_FEDERATION_RULE_ID",
	} {
		t.Setenv(env, "")
	}

	cases := []struct {
		why  string
		p    *Provider
		want bool
	}{
		{"什么都没配", nil, false},
		{"配了 Anthropic 但 key 是空的", &Provider{Type: "anthropic", Model: "m"}, false},
		{"没填类型 —— 按分派规则它走 Anthropic 那条路", &Provider{Model: "m"}, false},
		{"填了 key", &Provider{Type: "anthropic", APIKey: "sk-x"}, true},
		{"OpenAI 兼容不填 key 也放行：本机 Ollama、自建网关都可能不校验", &Provider{Type: "openai", BaseURL: "http://localhost:11434/v1"}, true},
	}
	for _, c := range cases {
		if got := c.p.Ready(); got != c.want {
			t.Errorf("%s：Ready() = %v, want %v", c.why, got, c.want)
		}
	}

	// 环境变量里的凭证也算数 —— 不在设置里填、靠环境变量跑是最常见的用法
	t.Setenv("ANTHROPIC_API_KEY", "sk-env")
	if !(&Provider{Type: "anthropic"}).Ready() {
		t.Error("环境里有 ANTHROPIC_API_KEY，就该放行")
	}
	if !(*Provider)(nil).Ready() {
		t.Error("环境里有 key 时，连供应商都没配也该放行 —— 走的正是这条默认链")
	}
}

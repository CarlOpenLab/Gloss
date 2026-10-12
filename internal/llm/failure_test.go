package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
)

// Describe 管的是「这一趟失败该怎么说给用户听」。这里钉住的是每一种失败
// 各自的关键判断：说什么事、要不要给一条通向设置的路、原话留着没有。
func TestDescribe(t *testing.T) {
	cases := []struct {
		why      string
		err      error
		title    string // 期望的 Title（不填表示只断言 Auth/Detail）
		auth     bool
		hasTitle bool // Title 非空
	}{
		{why: "钥匙不对", err: &statusError{Code: 401, Message: "invalid x-api-key"}, title: "模型没认这个密钥", auth: true},
		{why: "密钥没权限", err: &statusError{Code: 403}, title: "模型没认这个密钥", auth: true},
		{why: "类型说是认证错，码却给了别的", err: &statusError{Code: 400, Type: "authentication_error"}, title: "模型没认这个密钥", auth: true},
		{why: "限流：能自己好，别指去设置", err: &statusError{Code: 429}, title: "请求太频繁了"},
		{why: "对端 5xx", err: &statusError{Code: 503}, title: "模型服务出错了"},
		{why: "余额", err: &statusError{Code: 402}, title: "供应商那边说账单有问题", auth: true},
		{why: "模型名不对", err: &statusError{Code: 404}, title: "找不到这个模型", auth: true},
		{why: "请求本身有问题", err: &statusError{Code: 400}, title: "这次请求模型没收"},
		{why: "没见过的码也得有个说法", err: &statusError{Code: 418}, title: "模型返回了 418"},
		{why: "配没配全", err: ErrNoModel, title: "这个供应商还没配全", auth: true},
		{why: "模型拒绝回答", err: ErrRefusal, title: "模型拒绝了这次请求"},
		{why: "认不出来的错误", err: errors.New("某种没见过的失败"), title: "生成失败了"},
	}
	for _, c := range cases {
		f := Describe(c.err)
		if f.Title != c.title {
			t.Errorf("%s：Title = %q, want %q", c.why, f.Title, c.title)
		}
		if f.Auth != c.auth {
			t.Errorf("%s：Auth = %v, want %v", c.why, f.Auth, c.auth)
		}
	}
}

// 原话要留着 —— 翻译不准的时候，证据就在这一行。
func TestDescribeKeepsDetail(t *testing.T) {
	f := Describe(&statusError{Code: 401, Message: "invalid x-api-key"})
	if f.Detail != "invalid x-api-key" {
		t.Errorf("Detail = %q", f.Detail)
	}
	if h := Describe(&statusError{Code: 401}).Hint; !strings.Contains(h, "API key") {
		t.Errorf("说密钥不对，就该说清去哪儿改：%q", h)
	}
	// 供应商连一句话都没给：至少把状态码摆出来，别是空白
	if d := Describe(&statusError{Code: 502}).Detail; !strings.Contains(d, "502") {
		t.Errorf("没原话时 Detail = %q，该退到状态码", d)
	}
}

// 连不上：DNS、连接被拒、超时都算。这类错的原文（方法 + URL + 拨号细节）
// 又长又没用，所以主体得是「连不上」和「去查什么」。
func TestDescribeOffline(t *testing.T) {
	refused := &url.Error{Op: "Post", URL: "http://localhost:11434/v1/chat/completions",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connect: connection refused")}}
	for _, err := range []error{refused, context.DeadlineExceeded} {
		f := Describe(err)
		if f.Title != "连不上模型服务" {
			t.Errorf("%v：Title = %q", err, f.Title)
		}
		if f.Auth {
			t.Error("连不上不是钥匙的问题，不该指去设置")
		}
		if f.Hint == "" {
			t.Error("连不上要给一句「查什么」")
		}
	}
}

// 原话截断不能把一个字符切成两半：供应商自己搭的网关照抄一句中文说明，
// 切面正好落在字符中间的话，末尾会冒出一个乱码方块。
func TestTruncateKeepsRunesWhole(t *testing.T) {
	s := strings.Repeat("模", 120) // 每个 3 字节，远超上限
	got := truncate(s, 200)
	if !utf8.ValidString(got) {
		t.Fatalf("截断之后不是合法的 UTF-8：%q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("截断了就该看得出来：%q", got)
	}
	if n := len(strings.TrimSuffix(got, "…")); n != 198 {
		t.Errorf("截到 %d 字节，该退到最近的字符边界（198）", n)
	}
	// 短的原样返回，不加省略号
	if got := truncate("短", 200); got != "短" {
		t.Errorf("没超上限就别动它：%q", got)
	}
}

// SDK 的错误要收成我们自己的形状 —— 状态码、错误类型、它那句话，三样都
// 得从 SDK 那坨东西里捞出来，而 Error() 里那串 URL/Request-ID/JSON 不进
// 界面。
func TestWrapStatus(t *testing.T) {
	var apiErr anthropic.Error
	raw := `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`
	if err := json.Unmarshal([]byte(raw), &apiErr); err != nil {
		t.Fatalf("构造 SDK 错误：%v", err)
	}
	apiErr.StatusCode = 401

	err := wrapStatus(&apiErr)
	se, ok := err.(*statusError)
	if !ok {
		t.Fatalf("wrapStatus 没换成 statusError：%T", err)
	}
	if se.Code != 401 || se.Type != "authentication_error" || se.Message != "invalid x-api-key" {
		t.Errorf("捞出来的是 %+v", se)
	}
	if f := Describe(err); !f.Auth {
		t.Errorf("换成 statusError 之后该能认出是钥匙问题：%+v", f)
	}
}

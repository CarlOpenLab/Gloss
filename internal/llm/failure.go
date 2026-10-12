package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"

	"github.com/anthropics/anthropic-sdk-go"
)

// Failure 是一次失败整理成界面能用的说法。
//
// 分三层，因为它们回答的是不同的问题：Title 说「出了什么事」，Hint 说
// 「能怎么办」，Detail 是供应商自己的原话 —— 前两句是我们替它翻译的，
// 翻译可能不准，原话留着，遇到怪问题才有得对。
//
// Title 为空表示没有失败。
type Failure struct {
	Title  string
	Hint   string
	Detail string
	// Auth 表示「钥匙不对」这一类 —— 这几种错里，用户唯一能自己动手改的
	// 就是那把钥匙，界面上给一条通向模型设置的路。
	Auth bool
}

// Describe 把一次失败整理成界面上能读的话。
//
// 不把 err.Error() 直接摆出去，是因为它生来不是给人看的：Anthropic 的 SDK
// 会把请求方法、完整 URL、Request-ID 和一整坨 JSON 拼成一行，OpenAI 兼容
// 那一路也差不多。对着那样一屏字，用户只知道「我点了一下，冒出来一大段
// 看不懂的话」—— 出了什么事、下一步该干什么，一个字都没有。
func Describe(err error) Failure {
	switch {
	case err == nil:
		return Failure{}

	case errors.Is(err, ErrRefusal):
		return Failure{
			Title: "模型拒绝了这次请求",
			Hint:  "换一段代码或者换一个模型再试。",
		}

	case errors.Is(err, ErrNoBaseURL), errors.Is(err, ErrNoModel):
		return Failure{
			Title: "这个供应商还没配全",
			Hint:  "在模型设置里把 Base URL 和模型名补上。",
			Auth:  true,
		}
	}

	var se *statusError
	if errors.As(err, &se) {
		return describeStatus(se)
	}

	// 根本没连上：DNS 没解析出来、连接被拒、超时。*url.Error 也在这里面 ——
	// 它实现了 net.Error，而那正是传输层失败的形状。
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) {
		return Failure{
			Title:  "连不上模型服务",
			Hint:   "检查一下网络，或者模型设置里的 Base URL。",
			Detail: truncate(err.Error(), detailMax),
		}
	}

	return Failure{Title: "生成失败了", Detail: truncate(err.Error(), detailMax)}
}

// detailMax 是原话保留的长度上限。
//
// 原话是为了「出怪问题时对得上」，前面那句「出了什么事」才是主体。整段
// 抄下来，卡片就又被一大坨字淹了。
const detailMax = 200

// describeStatus 把「对端回了非 2xx」翻译成人话。
func describeStatus(e *statusError) Failure {
	f := Failure{Detail: truncate(e.Message, detailMax)}
	if f.Detail == "" {
		f.Detail = truncate(e.Error(), detailMax)
	}
	switch {
	case e.Code == 401 || e.Code == 403 || e.Type == "authentication_error" || e.Type == "permission_error":
		f.Title = "模型没认这个密钥"
		f.Hint = "检查一下模型设置里的 API key。"
		f.Auth = true
	case e.Code == 429 || e.Type == "rate_limit_error":
		f.Title = "请求太频繁了"
		f.Hint = "等一会儿再试。"
	case e.Code == 402 || e.Type == "billing_error":
		f.Title = "供应商那边说账单有问题"
		f.Hint = "去它的控制台看看余额或者付款方式。"
		f.Auth = true
	case e.Code >= 500 || e.Type == "api_error" || e.Type == "overloaded_error":
		f.Title = "模型服务出错了"
		f.Hint = "多半是临时的，稍后重试。"
	case e.Code == 404 || e.Type == "not_found_error":
		f.Title = "找不到这个模型"
		f.Hint = "检查一下模型名，或者 Base URL 是不是填成了别的服务的。"
		f.Auth = true
	case e.Code == 400 || e.Type == "invalid_request_error":
		f.Title = "这次请求模型没收"
		f.Hint = "换一段改动再试 —— 多半是它太长或者格式特殊。"
	default:
		f.Title = fmt.Sprintf("模型返回了 %d", e.Code)
	}
	return f
}

// statusError 是「对端回了非 2xx」。两条路都会产生它：OpenAI 兼容那路
// 自己造，Anthropic 那路把 SDK 的错误收成这个形状（见 wrapStatus）。
//
// 收成一种，是为了 Describe 只需要认识一种形状 —— 否则每加一个供应商，
// 判断「这是不是密钥问题」的代码就得多抄一遍。
type statusError struct {
	Code int
	// Type 是供应商自己的错误类型（"authentication_error" 之类）。OpenAI
	// 兼容那路没有，所以判断时要连状态码一起看。
	Type    string
	Message string
}

func (e *statusError) Error() string {
	s := fmt.Sprintf("HTTP %d", e.Code)
	if e.Type != "" {
		s += " " + e.Type
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	return s
}

// wrapStatus 把 SDK 的错误收成 statusError。
func wrapStatus(err error) error {
	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	return &statusError{
		Code:    apiErr.StatusCode,
		Type:    string(apiErr.Type()),
		Message: apiMessage(apiErr.RawJSON()),
	}
}

// apiMessage 从 {"error":{"message":"…"}} 里取出那句话。
func apiMessage(raw string) string {
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &body) == nil {
		return body.Error.Message
	}
	return ""
}

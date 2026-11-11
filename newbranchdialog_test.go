package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestAIBranchRerollDoesNotFeedBackSuggestion 钉住「再点一次 AI 起名」的行为。
//
// 名字框和意图输入共用一个输入位：第一次生成的答案留在框里。第二次点击时
// 如果把它当成用户写的需求再喂给模型，模型看见一个现成的合法分支名会原样
// 抄回来 —— 于是再点多少次都是同一个名字。
func TestAIBranchRerollDoesNotFeedBackSuggestion(t *testing.T) {
	const suggestion = "chore/add-vue-tsconfig"

	var mu sync.Mutex
	var userMessages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解码请求失败: %v", err)
		}
		mu.Lock()
		for _, m := range req.Messages {
			if m.Role == "user" {
				userMessages = append(userMessages, m.Content)
			}
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", suggestion)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	a := richApp(t)
	withProvider(t, a, srv)
	a.openNewBranchDialog("", "HEAD")

	a.startAIBranch()
	settle(t, a)
	if a.newBranch.name != suggestion {
		t.Fatalf("第一次生成的推荐分支名 = %q, want %q", a.newBranch.name, suggestion)
	}

	a.startAIBranch()
	settle(t, a)

	mu.Lock()
	defer mu.Unlock()
	if len(userMessages) != 2 {
		t.Fatalf("模型被调用了 %d 次，want 2", len(userMessages))
	}
	if strings.Contains(userMessages[1], suggestion) {
		t.Errorf("再点一次 AI 起名时，把上一次的建议当成需求喂回模型了：\n%s", userMessages[1])
	}
}

// TestAIBranchSendsTypedIntent 验证名字框里打的字真的进了请求。
// 用户报的是「框里明明有输入，AI 却始终给同一个名字」——先钉住这条链路。
func TestAIBranchSendsTypedIntent(t *testing.T) {
	var mu sync.Mutex
	var userMessage string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("解码请求失败: %v", err)
		}
		mu.Lock()
		for _, m := range req.Messages {
			if m.Role == "user" {
				userMessage = m.Content
			}
		}
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"fix/login-timeout\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	a := richApp(t)
	withProvider(t, a, srv)
	a.openNewBranchDialog("", "HEAD")

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()
	tt.Type("修复登录超时")
	tt.Frame()
	if err := tt.Click("AI 建议分支名"); err != nil {
		t.Fatalf("点击 AI 建议分支名按钮失败: %v", err)
	}
	settle(t, a)

	mu.Lock()
	defer mu.Unlock()
	if !strings.Contains(userMessage, "修复登录超时") {
		t.Errorf("名字框里打的意图没有进请求：\n%s", userMessage)
	}
}

// TestAIBranchStaleResponseDoesNotClobberNewer 覆盖「取消后马上重发」的时序：
// 旧请求的响应要等一会儿才回来，不能让它在回来时清掉新请求的状态、
// 把新请求的答案丢掉。
func TestAIBranchStaleResponseDoesNotClobberNewer(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		user := ""
		if len(req.Messages) > 0 {
			user = req.Messages[len(req.Messages)-1].Content
		}

		name := "first/stale"
		if strings.Contains(user, "second-intent") {
			name = "second/fresh"
			time.Sleep(50 * time.Millisecond)
		} else {
			// 旧请求先挂住，等测试确认它已经在路上再取消。
			once.Do(func() { close(started) })
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", name)
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	a := richApp(t)
	withProvider(t, a, srv)
	a.openNewBranchDialog("", "HEAD")

	a.startAIBranch()  // 旧请求
	<-started          // 确认旧请求已经发出去
	a.cancelAIBranch() // 取消它，但它的 dispatch 还没跑
	a.newBranch.name = "second-intent"
	a.startAIBranch() // 新请求
	close(release)
	settle(t, a)

	if a.newBranch.name != "second/fresh" {
		t.Fatalf("新请求的结果被旧响应挤掉了：name = %q, want %q", a.newBranch.name, "second/fresh")
	}
}

// TestAIBranchKeepsEditMadeWhileWaiting 覆盖「等结果时用户又改了名字框」：
// 迟到的那份结果不能覆盖用户后来写的东西。
func TestAIBranchKeepsEditMadeWhileWaiting(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"chore/ai-name\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)

	a := richApp(t)
	withProvider(t, a, srv)
	a.openNewBranchDialog("", "HEAD")

	a.startAIBranch()
	a.newBranch.name = "我自己写的名字"
	close(release)
	settle(t, a)

	if a.newBranch.name != "我自己写的名字" {
		t.Fatalf("等结果时改的名字被 AI 结果覆盖了：name = %q", a.newBranch.name)
	}
}

// 侧栏「分支」标题右边的 + 直接打开新建分支对话框，不再先弹一次菜单。
func TestSidebarBranchHeaderHasNewBranchButton(t *testing.T) {
	a := branchApp(t)
	tt := ui.NewTester(a.view, 1280, 900)
	settle(t, a)
	tt.Frame()

	if _, ok := tt.Find("新建分支…"); !ok {
		t.Fatalf("侧栏分支标题旁该有「新建分支…」按钮：%q", tt.Texts())
	}
	// + 贴在标题行右端，和「项目」行那颗收在同一处：挨着标题放既像标题
	// 的一部分，侧栏右端也对不齐。
	branchPlus, _ := tt.Find("新建分支…")
	projectsPlus, ok := tt.Find(T("添加项目", "Add Project"))
	if !ok {
		t.Fatalf("项目行找不到 + 按钮：%q", tt.Texts())
	}
	if d := (branchPlus.X + branchPlus.W/2) - (projectsPlus.X + projectsPlus.W/2); d > 1 || d < -1 {
		t.Errorf("「分支」+ 中心 x=%.1f 和「项目」+ 中心 x=%.1f 没对齐（差 %.1f）",
			branchPlus.X+branchPlus.W/2, projectsPlus.X+projectsPlus.W/2, d)
	}
	if err := tt.Click("新建分支…"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()

	if !a.newBranch.on {
		t.Fatal("点 + 该直接打开新建分支对话框")
	}
	if !tt.HasText("新建分支") {
		t.Errorf("对话框标题该出现：%q", tt.Texts())
	}
}

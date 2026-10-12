package main

import (
	"strings"
	"sync"
	"time"
)

// aiFlush 是流式输出的节流间隔。
//
// 每个 token 都回主线程重绘会把界面拖垮：一次回答几百个 delta，
// 每个都触发一整帧的布局和绘制。攒够 50ms 再刷一次，肉眼仍是逐字出现。
const aiFlush = 50 * time.Millisecond

// aiMail 是流式片段的中转站：后台只写，主线程按节流取。
//
// AI 智能提交和 PR 评论生成共用它 —— 两者的收尾都发生在后台 goroutine
// 回到主线程之后，那时元素早就没了，片段得先在这里攒着。
type aiMail struct {
	mu   sync.Mutex
	buf  strings.Builder
	last time.Time
}

// fill 收下一个片段，并报告这次是否到了该刷新界面的时间。
func (m *aiMail) fill(chunk string) (due bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf.WriteString(chunk)
	if time.Since(m.last) < aiFlush {
		return false
	}
	m.last = time.Now()
	return true
}

// take 取走攒下的片段。
func (m *aiMail) take() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.buf.String()
	m.buf.Reset()
	return s
}

package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/CarlOpenLab/Gloss/internal/explain"
)

const fakeVulnerableReportJSON = `{
	"summary": "发现 2 处潜在安全风险：1 处高危硬编码凭证与 1 处并发数据竞争隐患。",
	"passed": false,
	"findings": [
		{
			"title": "硬编码生产环境 API Token",
			"severity": "CRITICAL",
			"file_path": "server/auth.go",
			"line": 28,
			"description": "代码中包含明文密钥 token_live_abcdef123456",
			"suggestion": "移除硬编码凭据，改用环境变量或密钥管理服务注入"
		},
		{
			"title": "并发无锁读写共享 map",
			"severity": "MEDIUM",
			"file_path": "server/session.go",
			"line": 74,
			"description": "多 goroutine 直接访问 sessionCache 可能导致 fatal error: concurrent map writes",
			"suggestion": "使用 sync.Map 或 sync.RWMutex 保护并发读写"
		}
	]
}`

const fakeSafeReportJSON = `{
	"summary": "代码变更安全预检通过，未发现敏感信息泄露与已知高危漏洞。",
	"passed": true,
	"findings": []
}`

func TestRiskScanSuccessWithFindings(t *testing.T) {
	srv, calls := fakeLLM(t, fakeVulnerableReportJSON)
	a := richApp(t)
	withProvider(t, a, srv)

	a.stageAll()
	settle(t, a)

	if a.stagedCount() == 0 {
		t.Fatal("没有暂存的文件")
	}

	a.startRiskScan()
	settle(t, a)

	if *calls != 1 {
		t.Fatalf("LLM 被调用了 %d 次，want 1", *calls)
	}
	if a.riskScan.loading {
		t.Error("预检完成后 loading 应当为 false")
	}
	if !a.riskScan.dialog {
		t.Error("预检完成后 dialog 应当处于打开状态")
	}
	if a.riskScan.report.Passed {
		t.Error("含 CRITICAL 风险时 report.Passed 应当为 false")
	}
	if len(a.riskScan.report.Findings) != 2 {
		t.Fatalf("预期 2 处风险项，实际为 %d", len(a.riskScan.report.Findings))
	}

	f1 := a.riskScan.report.Findings[0]
	if f1.Severity != explain.RiskCritical || f1.Title != "硬编码生产环境 API Token" || f1.FilePath != "server/auth.go" || f1.Line != 28 {
		t.Errorf("第一项风险解析不匹配: %+v", f1)
	}

	badge, label := a.riskScan.report.StatusBadge()
	if badge != "🔴" || label != "高危发现" {
		t.Errorf("StatusBadge 预期 🔴 高危发现，实际为 %s %s", badge, label)
	}
}

func TestRiskScanSafePass(t *testing.T) {
	srv, calls := fakeLLM(t, fakeSafeReportJSON)
	a := richApp(t)
	withProvider(t, a, srv)

	a.stageAll()
	settle(t, a)

	a.startRiskScan()
	settle(t, a)

	if *calls != 1 {
		t.Fatalf("LLM 被调用了 %d 次，want 1", *calls)
	}
	if !a.riskScan.report.Passed {
		t.Error("安全报告 report.Passed 应当为 true")
	}
	if len(a.riskScan.report.Findings) != 0 {
		t.Errorf("预期无风险项，实际得到 %d 项", len(a.riskScan.report.Findings))
	}
	badge, label := a.riskScan.report.StatusBadge()
	if badge != "🟢" || label != "安全通过" {
		t.Errorf("StatusBadge 预期 🟢 安全通过，实际为 %s %s", badge, label)
	}
}

func TestRiskScanFromUI(t *testing.T) {
	srv, _ := fakeLLM(t, fakeVulnerableReportJSON)
	a := richApp(t)
	withProvider(t, a, srv)

	a.stageAll()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	// 点击预检按钮
	if err := tt.Click("🛡️ 变更预检"); err != nil {
		t.Fatalf("点击 🛡️ 变更预检 按钮失败: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if !a.riskScan.dialog {
		t.Fatal("点击后预检对话框未打开")
	}

	// 对话框中点击 关闭 按钮
	if err := tt.Click("关闭"); err != nil {
		t.Fatalf("点击预检对话框 关闭 按钮失败: %v", err)
	}
	tt.Frame()

	if a.riskScan.dialog {
		t.Error("点击关闭后预检对话框应当关闭")
	}
}

func TestRiskScanProceedToCommitFromUI(t *testing.T) {
	srv, _ := fakeLLM(t, fakeSafeReportJSON)
	a := richApp(t)
	withProvider(t, a, srv)

	a.stageAll()
	a.commitMsg = "feat: safe changes ready to commit"
	settle(t, a)

	a.startRiskScan()
	settle(t, a)

	tt := ui.NewTester(a.view, 1280, 860)
	tt.Frame()

	if !a.riskScan.dialog {
		t.Fatal("预检完成后对话框应当处于打开状态")
	}

	// 点击 继续提交
	if err := tt.Click("继续提交"); err != nil {
		t.Fatalf("点击 继续提交 失败: %v", err)
	}
	settle(t, a)
	tt.Frame()

	if a.riskScan.dialog {
		t.Error("点击继续提交后对话框应当关闭")
	}
}

func TestRiskScanWithoutProviderOpensSettings(t *testing.T) {
	a := richApp(t)
	clearProviderEnv(t)
	a.stageAll()
	settle(t, a)

	a.startRiskScan()
	settle(t, a)

	if !a.settingsOn {
		t.Error("未配置模型供应商时，发起安全预检应当自动打开模型设置窗口")
	}
}

func TestRiskScanCancel(t *testing.T) {
	srv, _ := fakeLLM(t, "slow chunk data")
	a := richApp(t)
	withProvider(t, a, srv)

	a.stageAll()
	settle(t, a)

	a.startRiskScan()
	if !a.riskScan.loading {
		t.Fatal("发起预检后应当处于 loading 状态")
	}

	a.cancelRiskScan()
	if a.riskScan.loading {
		t.Error("取消后 loading 应当为 false")
	}
	settle(t, a)
}

func TestRiskScanPaletteCommand(t *testing.T) {
	srv, calls := fakeLLM(t, fakeSafeReportJSON)
	a := richApp(t)
	withProvider(t, a, srv)

	a.stageAll()
	settle(t, a)

	// 查找命令栏中是否有预检命令
	found := false
	for _, cmd := range a.commands() {
		if cmd.label == "预检暂存区代码风险…" {
			found = true
			cmd.run(a)
			break
		}
	}

	if !found {
		t.Fatal("命令栏中未找到 预检暂存区代码风险… 命令")
	}
	settle(t, a)

	if *calls != 1 {
		t.Errorf("执行命令栏预检后 LLM 应当被调用 1 次，实际为 %d", *calls)
	}
	if !a.riskScan.dialog {
		t.Error("执行命令后预检对话框应当处于打开状态")
	}
}

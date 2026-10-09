package explain

import (
	"strings"
	"testing"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

func TestBuildRiskScanPromptEmpty(t *testing.T) {
	sys, user := BuildRiskScanPrompt(nil)
	if !strings.Contains(sys, "Senior Security Auditor") {
		t.Errorf("系统提示词未包含角色定义: %s", sys)
	}
	if !strings.Contains(user, "暂存区无文件变更") {
		t.Errorf("空文件列表时提示不正确: %s", user)
	}
}

func TestBuildRiskScanPromptWithFiles(t *testing.T) {
	files := []diff.FileDiff{
		parseOne(t, twoHunks),
	}
	sys, user := BuildRiskScanPrompt(files)

	for _, kw := range []string{"Senior Security Auditor", "Leaked Secrets", "Security Vulnerabilities", "Concurrency / Panic Bugs"} {
		if !strings.Contains(sys, kw) {
			t.Errorf("系统提示词缺少审查要点: %q", kw)
		}
	}
	if !strings.Contains(user, "a.go") {
		t.Errorf("用户提示词中缺少文件路径: %s", user)
	}
	if !strings.Contains(user, "+inserted") {
		t.Errorf("用户提示词中缺少 diff 代码: %s", user)
	}
}

func TestBuildRiskScanPromptBinaryFile(t *testing.T) {
	files := []diff.FileDiff{
		{
			OldPath: "token.bin",
			NewPath: "token.bin",
			Status:  "modified",
			Binary:  true,
		},
	}
	_, user := BuildRiskScanPrompt(files)
	if !strings.Contains(user, "二进制文件已变更") {
		t.Errorf("未正确标注二进制文件: %s", user)
	}
}

func TestBuildRiskScanPromptTruncation(t *testing.T) {
	var bigDiff strings.Builder
	bigDiff.WriteString("diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n@@ -1,1 +1,2000 @@\n")
	for i := 0; i < 600; i++ {
		bigDiff.WriteString("+this is a line exceeding character limit for risk scan 1234567890\n")
	}
	f := parseOne(t, bigDiff.String())
	files := []diff.FileDiff{f, f, f}

	_, user := BuildRiskScanPrompt(files)
	if !strings.Contains(user, "截断") {
		t.Errorf("超大变更未截断: 长度=%d", len(user))
	}
}

func TestParseRiskReportJSON(t *testing.T) {
	jsonPayload := `{
		"summary": "发现 2 处安全风险，包含 1 处硬编码密钥与 1 处空指针隐患。",
		"passed": false,
		"findings": [
			{
				"title": "硬编码 AWS Access Key",
				"severity": "CRITICAL",
				"file_path": "config/cloud.go",
				"line": 15,
				"description": "源码中存在显式 AKIA 格式凭据",
				"suggestion": "移除硬编码，改用环境变量读取"
			},
			{
				"title": "潜在空指针解引用",
				"severity": "MEDIUM",
				"file_path": "handler/user.go",
				"line": 42,
				"description": "未对 user.Profile 进行判空即访问 Avatar",
				"suggestion": "增加 if user.Profile != nil 防御判断"
			}
		]
	}`

	report := ParseRiskReport(jsonPayload)
	if report.Passed {
		t.Error("存在 CRITICAL 漏洞，Passed 应当为 false")
	}
	if len(report.Findings) != 2 {
		t.Fatalf("预期 2 处风险发现，实际得到 %d", len(report.Findings))
	}

	f1 := report.Findings[0]
	if f1.Severity != RiskCritical || f1.Title != "硬编码 AWS Access Key" || f1.FilePath != "config/cloud.go" || f1.Line != 15 {
		t.Errorf("第一条发现解析不正确: %+v", f1)
	}

	f2 := report.Findings[1]
	if f2.Severity != RiskMedium || f2.Line != 42 {
		t.Errorf("第二条发现解析不正确: %+v", f2)
	}

	maxSev := report.MaxSeverity()
	if maxSev != RiskCritical {
		t.Errorf("MaxSeverity 预期 CRITICAL，实际为 %v", maxSev)
	}

	badge, label := report.StatusBadge()
	if badge != "🔴" || label != "高危发现" {
		t.Errorf("StatusBadge 预期 🔴 高危发现，实际为 %s %s", badge, label)
	}
}

func TestParseRiskReportFencedJSON(t *testing.T) {
	input := "```json\n" + `{
		"summary": "代码变更无明显安全漏洞",
		"passed": true,
		"findings": []
	}` + "\n```"

	report := ParseRiskReport(input)
	if !report.Passed {
		t.Error("Passed 应当为 true")
	}
	if len(report.Findings) != 0 {
		t.Errorf("预期无发现，实际得到 %d", len(report.Findings))
	}
	badge, label := report.StatusBadge()
	if badge != "🟢" || label != "安全通过" {
		t.Errorf("StatusBadge 预期 🟢 安全通过，实际为 %s %s", badge, label)
	}
}

func TestParseRiskReportMediumOnlyBadge(t *testing.T) {
	input := `{
		"summary": "发现部分规范与并发安全提示",
		"passed": true,
		"findings": [
			{
				"title": "缺乏互斥锁保护共享变量",
				"severity": "MEDIUM",
				"file_path": "cache.go",
				"line": 88,
				"description": "并发写 cache 可能导致数据竞争",
				"suggestion": "使用 sync.RWMutex 保护"
			}
		]
	}`

	report := ParseRiskReport(input)
	badge, label := report.StatusBadge()
	if badge != "🟡" || label != "需注意" {
		t.Errorf("仅有中危时 StatusBadge 预期 🟡 需注意，实际为 %s %s", badge, label)
	}
	if report.MaxSeverity() != RiskMedium {
		t.Errorf("MaxSeverity 预期 MEDIUM，实际为 %v", report.MaxSeverity())
	}
}

func TestParseRiskReportMarkdownFallback(t *testing.T) {
	md := `
预检完成，识别到以下漏洞：

### [HIGH] SQL 注入漏洞
文件: internal/db/query.go:77
描述: 直接使用 fmt.Sprintf 拼接 SQL 查询参数
建议: 使用 database/sql 的参数化查询

### [LOW] 未使用的 TODO 占位
文件: utils.go:12
描述: 遗留的临时测试分支
建议: 提交前清理临时逻辑
`

	report := ParseRiskReport(md)
	if report.Passed {
		t.Error("含 HIGH 漏洞时 Passed 应当为 false")
	}
	if len(report.Findings) != 2 {
		t.Fatalf("预期解析出 2 条发现，实际得到 %d", len(report.Findings))
	}
	if report.Findings[0].Severity != RiskHigh || report.Findings[0].Line != 77 {
		t.Errorf("第一条发现解析有误: %+v", report.Findings[0])
	}
	if report.Findings[1].Severity != RiskLow || report.Findings[1].Line != 12 {
		t.Errorf("第二条发现解析有误: %+v", report.Findings[1])
	}
}

func TestParseRiskReportEmpty(t *testing.T) {
	report := ParseRiskReport("")
	if report.Passed {
		t.Error("空输出时 Passed 不应为 true")
	}
	if report.Summary == "" {
		t.Error("空输出时应有默认提示 summary")
	}
}

func TestParseRiskSeverity(t *testing.T) {
	cases := []struct {
		input string
		want  RiskSeverity
	}{
		{"CRITICAL", RiskCritical},
		{"critical", RiskCritical},
		{"严重", RiskCritical},
		{"HIGH", RiskHigh},
		{"高危", RiskHigh},
		{"MEDIUM", RiskMedium},
		{"中危", RiskMedium},
		{"warn", RiskMedium},
		{"LOW", RiskLow},
		{"低危", RiskLow},
		{"info", RiskLow},
		{"unknown", RiskMedium},
	}

	for _, tc := range cases {
		got := ParseRiskSeverity(tc.input)
		if got != tc.want {
			t.Errorf("ParseRiskSeverity(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

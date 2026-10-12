package explain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/CarlOpenLab/Gloss/internal/diff"
)

// RiskSeverity 表示风险严重级别。
type RiskSeverity string

const (
	RiskLow      RiskSeverity = "LOW"
	RiskMedium   RiskSeverity = "MEDIUM"
	RiskHigh     RiskSeverity = "HIGH"
	RiskCritical RiskSeverity = "CRITICAL"
)

// RiskFinding 描述一条具体的安全或代码风险发现。
type RiskFinding struct {
	Title       string       `json:"title"`
	Severity    RiskSeverity `json:"severity"`
	FilePath    string       `json:"file_path"`
	Line        int          `json:"line"`
	Description string       `json:"description"`
	Suggestion  string       `json:"suggestion"`
}

// RiskReport 是对代码变更执行安全与风险预检后的完整报告。
type RiskReport struct {
	Summary  string        `json:"summary"`
	Passed   bool          `json:"passed"`
	Findings []RiskFinding `json:"findings"`
}

// MaxSeverity 返回当前报告中最高的风险级别。若无发现，返回空。
func (r RiskReport) MaxSeverity() RiskSeverity {
	if len(r.Findings) == 0 {
		return ""
	}
	rank := map[RiskSeverity]int{
		RiskLow:      1,
		RiskMedium:   2,
		RiskHigh:     3,
		RiskCritical: 4,
	}
	maxSev := RiskLow
	for _, f := range r.Findings {
		if rank[f.Severity] > rank[maxSev] {
			maxSev = f.Severity
		}
	}
	return maxSev
}

// StatusBadge 返回整体安全徽标图标与文案：
// 🟢 安全通过 / 🟡 需注意 / 🔴 高危发现
func (r RiskReport) StatusBadge() (icon string, text string) {
	if len(r.Findings) == 0 && r.Passed {
		return "🟢", "安全通过"
	}
	for _, f := range r.Findings {
		if f.Severity == RiskCritical || f.Severity == RiskHigh {
			return "🔴", "高危发现"
		}
	}
	if len(r.Findings) > 0 || !r.Passed {
		return "🟡", "需注意"
	}
	return "🟢", "安全通过"
}

// ParseRiskSeverity 解析字符串为标准的 RiskSeverity。
func ParseRiskSeverity(s string) RiskSeverity {
	s = strings.ToUpper(strings.TrimSpace(s))
	switch {
	case strings.Contains(s, "CRIT") || strings.Contains(s, "严重") || strings.Contains(s, "重大"):
		return RiskCritical
	case strings.Contains(s, "HIGH") || strings.Contains(s, "高危") || strings.Contains(s, "高"):
		return RiskHigh
	case strings.Contains(s, "MED") || strings.Contains(s, "中危") || strings.Contains(s, "中") || strings.Contains(s, "WARN"):
		return RiskMedium
	case strings.Contains(s, "LOW") || strings.Contains(s, "低危") || strings.Contains(s, "低") || strings.Contains(s, "INFO") || strings.Contains(s, "提示"):
		return RiskLow
	default:
		return RiskMedium
	}
}

// RiskScanSystemPrompt 是安全与代码风险预检的系统提示词。
const RiskScanSystemPrompt = `You are a senior security auditor and code safety reviewer (Senior Security Auditor & Code Safety Reviewer).
Your task is to perform a thorough security pre-check and code quality review of the user's complete staged code changes (diff), accurately identifying potential vulnerabilities, risks, and credential leaks.

Review focus includes but is not limited to:
1. Leaked Secrets: API keys, private keys, OAuth tokens, hardcoded passwords, database credentials, JWT secrets, etc.
2. Security Vulnerabilities: SQL injection, command injection, path traversal, insecure deserialization, SSRF, XSS, insecure encryption/random number generation, etc.
3. Concurrency / Panic Bugs: nil dereference, data races, deadlocks, uncaught errors or panics on critical paths.
4. Breaking changes or destructive mutations: breaking public interface compatibility, dangerous unconditional batch deletion/updates of databases, irreversible data erasure, etc.

Output format:
Output strictly valid JSON (either directly or wrapped in a ` + "```json" + ` code block). No filler or greetings.
Schema:
{
  "summary": "Concise audit conclusion. If safe, state that it passed; if risks found, summarize the count and severity of findings.",
  "passed": true, // Must be false if any HIGH or CRITICAL severity risk exists; true if no high-severity risks and overall security is acceptable.
  "findings": [
    {
      "title": "Risk title (short and clear, e.g.: Hardcoded sensitive API key)",
      "severity": "CRITICAL" | "HIGH" | "MEDIUM" | "LOW",
      "file_path": "Relative path of the file with the risk",
      "line": 42, // Corresponding line number (use 0 if exact location cannot be determined)
      "description": "Detailed description of the vulnerability's cause and potential impact",
      "suggestion": "Specific fix recommendations and security hardening measures"
    }
  ]
}

Requirements:
- If no risks are found, "passed" must be true, "findings" must be an empty array, and "summary" should state the pre-check passed.
- Never fabricate or invent findings. Ensure alerts are accurate; maintain zero tolerance for real vulnerabilities and leaks.`

// maxRiskScanDiffChars 限制输入模型 diff 的字符预算。
const maxRiskScanDiffChars = 16000

// BuildRiskScanPrompt 构建预检安全提示词。
func BuildRiskScanPrompt(files []diff.FileDiff) (systemPrompt, userPrompt string) {
	var b strings.Builder

	if len(files) == 0 {
		b.WriteString("（暂存区无文件变更）\n")
		return RiskScanSystemPrompt, b.String()
	}

	fmt.Fprintf(&b, "## 待审查暂存区文件列表（共 %d 个文件）：\n", len(files))

	truncated := false
	for _, f := range files {
		if truncated {
			break
		}
		fmt.Fprintf(&b, "\n### %s（%s，+%d -%d）\n", DisplayPath(f), statusText(f.Status), f.AddCount, f.DelCount)
		if f.Binary {
			b.WriteString("（二进制文件已变更）\n")
			continue
		}

		if len(f.Hunks) == 0 {
			continue
		}

		b.WriteString("```diff\n")
		for _, h := range f.Hunks {
			raw := h.Raw()
			if b.Len()+len(raw) > maxRiskScanDiffChars {
				rem := maxRiskScanDiffChars - b.Len()
				if rem > 0 && rem < len(raw) {
					cut := strings.LastIndex(raw[:rem], "\n")
					if cut > 0 {
						b.WriteString(raw[:cut])
					}
				}
				b.WriteString("\n…（diff 过长，后续变更已截断）\n")
				truncated = true
				break
			}
			b.WriteString(raw)
		}
		b.WriteString("```\n")
	}

	if truncated {
		b.WriteString("\n注意：部分文件的 diff 过长已做截断，请结合已有改动与文件列表分析潜在安全风险。\n")
	}

	b.WriteString("\n请对以上全部变更进行安全性与代码风险扫描，并严格按指定的 JSON 结构返回预检报告。")

	return RiskScanSystemPrompt, b.String()
}

// rawFinding 内部解析辅助结构体。
type rawFinding struct {
	Title       string      `json:"title"`
	Severity    string      `json:"severity"`
	FilePath    string      `json:"file_path"`
	File        string      `json:"file"` // 容错兼容
	Line        interface{} `json:"line"`
	Description string      `json:"description"`
	Desc        string      `json:"desc"` // 容错兼容
	Suggestion  string      `json:"suggestion"`
	Fix         string      `json:"fix"` // 容错兼容
}

type rawReport struct {
	Summary  string       `json:"summary"`
	Passed   *bool        `json:"passed"`
	Findings []rawFinding `json:"findings"`
}

// ParseRiskReport 将模型输出解析为结构化的 RiskReport。
// 优先提取 JSON 结构；若模型返回纯 Markdown 或格式异常，做降级解析。
func ParseRiskReport(output string) RiskReport {
	output = strings.TrimSpace(output)
	if output == "" {
		return RiskReport{
			Summary: "未收到预检分析结果。",
			Passed:  false,
		}
	}

	// 1. 尝试提取 JSON 内容
	jsonStr := extractJSON(output)
	if jsonStr != "" {
		var raw rawReport
		if err := json.Unmarshal([]byte(jsonStr), &raw); err == nil {
			report := RiskReport{
				Summary: strings.TrimSpace(raw.Summary),
			}
			for _, rf := range raw.Findings {
				sev := ParseRiskSeverity(rf.Severity)
				filePath := strings.TrimSpace(rf.FilePath)
				if filePath == "" {
					filePath = strings.TrimSpace(rf.File)
				}
				desc := strings.TrimSpace(rf.Description)
				if desc == "" {
					desc = strings.TrimSpace(rf.Desc)
				}
				sug := strings.TrimSpace(rf.Suggestion)
				if sug == "" {
					sug = strings.TrimSpace(rf.Fix)
				}
				lineNo := parseLineNumber(rf.Line)

				report.Findings = append(report.Findings, RiskFinding{
					Title:       strings.TrimSpace(rf.Title),
					Severity:    sev,
					FilePath:    filePath,
					Line:        lineNo,
					Description: desc,
					Suggestion:  sug,
				})
			}

			// 计算 passed
			if raw.Passed != nil {
				report.Passed = *raw.Passed
			} else {
				report.Passed = len(report.Findings) == 0
			}

			// 如果存在高危/严重风险，强制 passed = false
			for _, f := range report.Findings {
				if f.Severity == RiskCritical || f.Severity == RiskHigh {
					report.Passed = false
					break
				}
			}

			if report.Summary == "" {
				if len(report.Findings) == 0 {
					report.Summary = "安全预检通过，未发现明显安全漏洞或敏感信息泄露。"
				} else {
					report.Summary = fmt.Sprintf("安全预检完成，共发现 %d 处风险隐患。", len(report.Findings))
				}
			}

			return report
		}
	}

	// 2. 降级：从 Markdown 文本中解析
	return parseMarkdownFallback(output)
}

func extractJSON(s string) string {
	// 如果含有 ```json ... ```，取代码块内部
	if idx := strings.Index(s, "```json"); idx >= 0 {
		end := strings.Index(s[idx+7:], "```")
		if end >= 0 {
			return strings.TrimSpace(s[idx+7 : idx+7+end])
		}
		return strings.TrimSpace(s[idx+7:])
	}
	if idx := strings.Index(s, "```"); idx >= 0 {
		end := strings.Index(s[idx+3:], "```")
		if end >= 0 {
			block := strings.TrimSpace(s[idx+3 : idx+3+end])
			if strings.HasPrefix(block, "{") {
				return block
			}
		}
	}

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}

func parseLineNumber(v interface{}) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

var (
	headingRe = regexp.MustCompile(`(?i)^#{1,4}\s*(?:\[?(CRITICAL|HIGH|MEDIUM|LOW|严重|高危|中危|低危)\]?[\s:：-]*)?(.*)$`)
	fileLineRe = regexp.MustCompile(`(?i)(?:文件|file|path)[：:]\s*([^\s:]+)(?::(\d+))?`)
)

func parseMarkdownFallback(text string) RiskReport {
	lines := strings.Split(text, "\n")
	var findings []RiskFinding
	var current *RiskFinding
	var summaryLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if m := headingRe.FindStringSubmatch(trimmed); len(m) > 0 && (m[1] != "" || strings.Contains(trimmed, "风险") || strings.Contains(trimmed, "漏洞")) {
			if current != nil && current.Title != "" {
				findings = append(findings, *current)
			}
			sev := ParseRiskSeverity(m[1])
			title := strings.TrimSpace(m[2])
			if title == "" {
				title = trimmed
			}
			current = &RiskFinding{
				Title:    title,
				Severity: sev,
			}
			continue
		}

		if current != nil {
			if fm := fileLineRe.FindStringSubmatch(trimmed); len(fm) > 0 {
				current.FilePath = fm[1]
				if len(fm) > 2 && fm[2] != "" {
					current.Line, _ = strconv.Atoi(fm[2])
				}
				continue
			}
			lower := strings.ToLower(trimmed)
			switch {
			case strings.HasPrefix(lower, "描述：") || strings.HasPrefix(lower, "描述:") || strings.HasPrefix(lower, "description:"):
				idx := strings.Index(trimmed, ":")
				if idx < 0 {
					idx = strings.Index(trimmed, "：")
				}
				current.Description = strings.TrimSpace(trimmed[idx+1:])
			case strings.HasPrefix(lower, "建议：") || strings.HasPrefix(lower, "建议:") || strings.HasPrefix(lower, "suggestion:") || strings.HasPrefix(lower, "fix:"):
				idx := strings.Index(trimmed, ":")
				if idx < 0 {
					idx = strings.Index(trimmed, "：")
				}
				current.Suggestion = strings.TrimSpace(trimmed[idx+1:])
			default:
				if current.Description == "" {
					current.Description = trimmed
				} else if current.Suggestion == "" {
					current.Suggestion = trimmed
				}
			}
		} else {
			summaryLines = append(summaryLines, trimmed)
		}
	}
	if current != nil && current.Title != "" {
		findings = append(findings, *current)
	}

	summary := strings.Join(summaryLines, "\n")
	if summary == "" {
		if len(findings) == 0 {
			summary = "预检完成，未发现明显安全风险。"
		} else {
			summary = fmt.Sprintf("预检完成，发现 %d 项潜在风险。", len(findings))
		}
	}

	passed := true
	if len(findings) > 0 {
		for _, f := range findings {
			if f.Severity == RiskCritical || f.Severity == RiskHigh {
				passed = false
				break
			}
		}
	} else {
		// 检查摘要中是否包含明显的安全通过或报警词
		if strings.Contains(summary, "高危") || strings.Contains(summary, "漏洞") || strings.Contains(summary, "泄露") {
			passed = false
		}
	}

	return RiskReport{
		Summary:  summary,
		Passed:   passed,
		Findings: findings,
	}
}

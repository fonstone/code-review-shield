package engines

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ParseFindings 从LLM输出解析缺陷JSON数组。
// 兼容：纯JSON数组、Markdown代码块包裹、前后有说明文字的格式。
// 解析失败返回可读错误（供重试与诊断）。
func ParseFindings(content string) ([]Finding, error) {
	arr := extractJSONArray(content)
	if arr == "" {
		return nil, fmt.Errorf("LLM输出中未找到JSON数组: %s", truncate(content, 200))
	}
	var findings []Finding
	dec := json.NewDecoder(strings.NewReader(arr))
	dec.UseNumber()
	if err := dec.Decode(&findings); err != nil {
		// 容错：字段名大小写不一致时尝试宽松解析
		var raw []map[string]any
		if err2 := json.Unmarshal([]byte(arr), &raw); err2 != nil {
			return nil, fmt.Errorf("缺陷JSON解析失败: %v; 原文: %s", err, truncate(arr, 200))
		}
		for _, m := range raw {
			f := Finding{}
			f.Severity = strOf(m, "severity", "level", "Severity")
			f.Category = strOf(m, "category", "type")
			f.FilePath = strOf(m, "file_path", "file", "path", "FilePath")
			f.LineNumber = strOf(m, "line_number", "line", "LineNumber")
			f.CodeSnippet = strOf(m, "code_snippet", "code", "snippet")
			f.Title = strOf(m, "title", "Title")
			f.Detail = strOf(m, "detail", "description")
			f.Suggestion = strOf(m, "suggestion", "fix")
			f.Confidence = floatOf(m, "confidence")
			findings = append(findings, f)
		}
	}
	// 清洗：严重度归一化、路径转相对格式
	for i := range findings {
		f := &findings[i]
		f.Severity = normalizeSeverity(f.Severity)
		f.FilePath = strings.TrimPrefix(strings.TrimSpace(f.FilePath), "./")
		f.FilePath = strings.ReplaceAll(f.FilePath, "\\", "/")
		if f.LineNumber == "" {
			f.LineNumber = "0"
		}
		if f.Title == "" {
			f.Title = fmt.Sprintf("%s缺陷(%s)", f.Category, f.FilePath)
		}
		if f.Confidence <= 0 {
			f.Confidence = 0.8
		}
	}
	// 去重（同文件同行同标题）
	findings = dedupe(findings)
	return findings, nil
}

var (
	reJSONArray = regexp.MustCompile(`(?s)\[\s*\{.*\}\s*\]`)
	reFence     = regexp.MustCompile("(?s)```(?:json)?\\s*(\\[.*?\\])\\s*```")
)

func extractJSONArray(s string) string {
	if m := reFence.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	if m := reJSONArray.FindString(s); m != "" {
		return m
	}
	// 最后兜底：从第一个'['到最后一个']'
	start := strings.Index(s, "[")
	end := strings.LastIndex(s, "]")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return ""
}

func normalizeSeverity(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "CRITICAL", "BLOCKER", "严重", "致命":
		return "CRITICAL"
	case "HIGH", "MAJOR", "高危", "高":
		return "HIGH"
	case "MEDIUM", "MINOR", "中危", "中":
		return "MEDIUM"
	case "LOW", "INFO", "低危", "低", "":
		return "LOW"
	default:
		return strings.ToUpper(strings.TrimSpace(s))
	}
}

func strOf(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func floatOf(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case float64:
				return t
			case json.Number:
				f, _ := t.Float64()
				return f
			case string:
				var f float64
				_ = json.Unmarshal([]byte(t), &f)
				return f
			}
		}
	}
	return 0
}

func dedupe(fs []Finding) []Finding {
	seen := map[string]bool{}
	var out []Finding
	for _, f := range fs {
		key := fmt.Sprintf("%s|%s|%s", f.FilePath, f.LineNumber, f.Title)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
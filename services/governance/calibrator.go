// Package governance 缺陷治理域：确定性校准、专项管理、误报记忆、分类白名单。
package governance

import "strings"

// Calibration 校准结果
type Calibration struct {
	Severity   string `json:"severity"`
	Confidence float64 `json:"confidence"`
	Adjusted   bool   `json:"adjusted"`
	Reason     string `json:"reason"`
}

// Calibrator 严重度决策树校准器（确定性，无LLM参与）
type Calibrator struct {
	// DowngradeBelowConfidence 低于该置信度降一级
	DowngradeBelowConfidence float64
	// UpgradeAtOrAboveConfidence 高置信度+高危关键词可升一级
	UpgradeAtOrAboveConfidence float64
}

// NewCalibrator 创建校准器
func NewCalibrator() *Calibrator {
	return &Calibrator{
		DowngradeBelowConfidence:   0.5,
		UpgradeAtOrAboveConfidence: 0.9,
	}
}

// severityRank 严重度等级数值
func severityRank(s string) int {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return 4
	case "HIGH":
		return 3
	case "MEDIUM":
		return 2
	case "LOW":
		return 1
	default:
		return 2
	}
}

func rankSeverity(r int) string {
	switch {
	case r >= 4:
		return "CRITICAL"
	case r == 3:
		return "HIGH"
	case r == 2:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

// criticalPatterns 决定性的高危代码模式（命中直接升到CRITICAL）
var criticalPatterns = []string{
	"double free", "double-free", "use after free", "use-after-free",
	"缓冲区溢出", "buffer overflow", "空指针解引用", "null dereference",
	"内存泄漏且无释放路径", "未检查malloc", "unchecked malloc",
}

// highPatterns 高危模式
var highPatterns = []string{
	"内存泄漏", "memory leak", "越界", "out of bounds", "未初始化",
	"uninitialized", "dangling", "野指针", "integer overflow",
}

// Calibrate 决策树：
//  1. 代码模式命中critical → CRITICAL
//  2. 代码模式命中high → 至少HIGH
//  3. confidence < 阈值 → 降一级
//  4. confidence ≥ 阈值且模式命中high → 升一级（上限CRITICAL）
//  5. 其他保持原级
func (c *Calibrator) Calibrate(severity, title, detail string, confidence float64) Calibration {
	text := strings.ToLower(title + " " + detail)
	rank := severityRank(severity)
	cal := Calibration{Severity: rankSeverity(rank), Confidence: confidence}

	critHit := containsAny(text, criticalPatterns)
	highHit := containsAny(text, highPatterns)

	switch {
	case critHit:
		if rank != 4 {
			cal.Adjusted = true
			cal.Reason = "命中决定性高危模式，升级为CRITICAL"
		}
		rank = 4
	case highHit && rank < 3:
		rank = 3
		cal.Adjusted = true
		cal.Reason = "命中高危模式，最低为HIGH"
	}

	if confidence > 0 && confidence < c.DowngradeBelowConfidence && rank > 1 {
		rank--
		cal.Adjusted = true
		cal.Reason = appendReason(cal.Reason, "置信度过低，降一级")
	} else if confidence >= c.UpgradeAtOrAboveConfidence && highHit && rank < 4 {
		rank++
		cal.Adjusted = true
		cal.Reason = appendReason(cal.Reason, "高置信度+高危模式，升一级")
	}
	cal.Severity = rankSeverity(rank)
	if cal.Reason == "" {
		cal.Reason = "无需调整"
	}
	return cal
}

func containsAny(text string, pats []string) bool {
	for _, p := range pats {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

func appendReason(base, add string) string {
	if base == "" {
		return add
	}
	return base + "；" + add
}
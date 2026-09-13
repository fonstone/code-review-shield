package defects

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// L1指纹：物理强指纹。基于"相对路径 + 清洗后的逐行代码 + 符号锚点"的确定性哈希。
// 同一段物理代码（含注释/空行差异）必然产生相同L1。
// L2指纹：语义弱指纹。对代码做标识符归一化（数字→N、字符串→S、大小写不敏感），
// 允许变量重命名、常量变化等语义等价差异。

var (
	reNumber = regexp.MustCompile(`\b\d[\d_]*\.?[\d_]*([eE][+-]?\d+)?\b`)
	reString = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	reIdent  = regexp.MustCompile(`\b[a-zA-Z_][a-zA-Z0-9_]*\b`)
)

// Fingerprint L1/L2指纹对
type Fingerprint struct {
	L1 string `json:"l1"`
	L2 string `json:"l2"`
}

// FingerprintSource 计算代码片段的L1/L2指纹
// relPath: 仓库内相对路径；src: 原始代码片段；anchorName: 可选锚点符号名（如函数名）
func FingerprintSource(relPath, src, anchorName string) Fingerprint {
	cleaned := CleanSource(src)
	l1Input := []string{strings.ToLower(NormalizeRelPath(relPath))}
	l2Input := []string{strings.ToLower(NormalizeRelPath(relPath))}
	if anchorName != "" {
		l1Input = append(l1Input, anchorName)
		l2Input = append(l2Input, strings.ToLower(anchorName))
	}
	cleanLines := cleaned
	if len(cleanLines) == 0 {
		cleanLines = []string{"<empty>"}
	}
	sort.Strings(cleanLines) // 行序无关，容忍行内微小重排
	l1Input = append(l1Input, cleanLines...)
	l2Input = append(l2Input, normalizeSemantic(cleaned)...)

	l1 := sha256.Sum256([]byte(strings.Join(l1Input, "\n")))
	l2 := sha256.Sum256([]byte(strings.Join(l2Input, "\n")))
	return Fingerprint{
		L1: "L1:" + hex.EncodeToString(l1[:16]),
		L2: "L2:" + hex.EncodeToString(l2[:16]),
	}
}

// FingerprintFinding 基于缺陷字段计算指纹（file+line上下文+title+code snippet）
func FingerprintFinding(relPath, title, codeSnippet, lineNumber string) Fingerprint {
	anchor := extractAnchorName(title)
	return FingerprintSource(relPath, codeSnippet+"\n// "+title+" @"+lineNumber, anchor)
}

func extractAnchorName(title string) string {
	// 尝试从标题提取函数名（如 "malloc未释放: func_name" 或 "func_name: xxx"）
	for _, pat := range []*regexp.Regexp{
		regexp.MustCompile(`[:\s]([a-zA-Z_]\w*)\s*$`),
		regexp.MustCompile(`^([a-zA-Z_]\w*)\s*:`),
	} {
		if m := pat.FindStringSubmatch(title); len(m) > 1 {
			if !isKeyword(m[1]) {
				return m[1]
			}
		}
	}
	return ""
}

var keywords = map[string]bool{
	"if": true, "for": true, "while": true, "return": true, "switch": true,
	"case": true, "break": true, "continue": true, "goto": true, "else": true,
	"int": true, "char": true, "void": true, "struct": true, "enum": true,
	"static": true, "const": true, "sizeof": true, "new": true, "delete": true,
}

func isKeyword(s string) bool { return keywords[strings.ToLower(s)] }

// normalizeSemantic 语义归一化：数字→N、字符串→S、标识符→ID（关键字保留）
func normalizeSemantic(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		s := reString.ReplaceAllString(line, "\"S\"")
		s = reNumber.ReplaceAllString(s, "N")
		s = reIdent.ReplaceAllStringFunc(s, func(w string) string {
			lw := strings.ToLower(w)
			if isKeyword(lw) {
				return lw
			}
			return "ID"
		})
		out = append(out, s)
	}
	return out
}

// Similarity 计算两段清洗代码的相似度(0-1)，用于四级漏斗第3级
func Similarity(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	// 使用bigram Dice系数
	gramsA := bigrams(a)
	gramsB := bigrams(b)
	if len(gramsA) == 0 || len(gramsB) == 0 {
		return 0
	}
	inter := 0
	for g := range gramsA {
		if gramsB[g] {
			inter++
		}
	}
	return 2.0 * float64(inter) / float64(len(gramsA)+len(gramsB))
}

func bigrams(lines []string) map[string]bool {
	joined := strings.Join(lines, "\n")
	out := make(map[string]bool)
	runes := []rune(joined)
	for i := 0; i+1 < len(runes); i++ {
		if unicode.IsSpace(runes[i]) || unicode.IsSpace(runes[i+1]) {
			continue
		}
		out[string(runes[i])+string(runes[i+1])] = true
	}
	return out
}
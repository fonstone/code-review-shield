// Package defects 缺陷指纹域：AST源码锚点、L1/L2指纹、四级漏斗增量比对引擎、Scope范围守卫。
package defects

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ASTSymbol 源码锚点符号（轻量AST：通过正则提取函数/结构体/宏等符号边界）
type ASTSymbol struct {
	File      string `json:"file"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // function / struct / macro / variable
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Signature string `json:"signature"`
}

// CleanedLines 去除注释与空白后的规范化代码行
type CleanedLines struct {
	File    string   `json:"file"`
	Lines   []string `json:"lines"`
	RawLine []int    `json:"raw_line"` // 清洗后行对应原始行号
}

var (
	reBlockComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reLineComment   = regexp.MustCompile(`(?m)//.*$`)
	reFuncDef       = regexp.MustCompile(`(?m)^\s*((?:static\s+|inline\s+|extern\s+)*[\w\*&\s:<>]+?)\s+(\w+)\s*\(([^)]*)\)\s*\{`)
	reStructDef     = regexp.MustCompile(`(?m)^\s*(?:typedef\s+)?struct\s+(\w+)\s*\{`)
	reEnumDef       = regexp.MustCompile(`(?m)^\s*(?:typedef\s+)?enum\s+(\w+)\s*\{`)
	reMacroDef      = regexp.MustCompile(`(?m)^\s*#\s*define\s+(\w+)`)
	reVarDecl       = regexp.MustCompile(`(?m)^\s*(?:static\s+|const\s+|volatile\s+)*(?:unsigned\s+|signed\s+)?(?:int|char|long|short|float|double|void|size_t|bool|[\w_]+_t|\w+)\s+(\w+)\s*(?:=|;)`)
)

// CleanSource 清洗源码：删除注释、压缩空行
func CleanSource(src string) []string {
	s := reBlockComment.ReplaceAllString(src, "")
	s = reLineComment.ReplaceAllString(s, "")
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// ExtractSymbols 从源码提取AST符号锚点
func ExtractSymbols(file string, src string) []ASTSymbol {
	cleaned := CleanSource(src)
	joined := strings.Join(cleaned, "\n")
	var syms []ASTSymbol

	for _, m := range reFuncDef.FindAllStringSubmatchIndex(joined, -1) {
		name := joined[m[4]:m[5]]
		sig := joined[m[2]:m[7]]
		start := lineIndexOf(joined, m[0])
		syms = append(syms, ASTSymbol{File: file, Name: name, Kind: "function", StartLine: start, Signature: sig})
	}
	for _, m := range reStructDef.FindAllStringSubmatchIndex(joined, -1) {
		name := joined[m[2]:m[3]]
		start := lineIndexOf(joined, m[0])
		syms = append(syms, ASTSymbol{File: file, Name: name, Kind: "struct", StartLine: start})
	}
	for _, m := range reEnumDef.FindAllStringSubmatchIndex(joined, -1) {
		name := joined[m[2]:m[3]]
		start := lineIndexOf(joined, m[0])
		syms = append(syms, ASTSymbol{File: file, Name: name, Kind: "enum", StartLine: start})
	}
	for _, m := range reMacroDef.FindAllStringSubmatchIndex(joined, -1) {
		name := joined[m[2]:m[3]]
		start := lineIndexOf(joined, m[0])
		syms = append(syms, ASTSymbol{File: file, Name: name, Kind: "macro", StartLine: start})
	}
	return syms
}

// ExtractVariables 提取变量声明锚点
func ExtractVariables(file string, src string) []ASTSymbol {
	cleaned := CleanSource(src)
	joined := strings.Join(cleaned, "\n")
	var syms []ASTSymbol
	for _, m := range reVarDecl.FindAllStringSubmatchIndex(joined, -1) {
		name := joined[m[2]:m[3]]
		start := lineIndexOf(joined, m[0])
		syms = append(syms, ASTSymbol{File: file, Name: name, Kind: "variable", StartLine: start})
	}
	return syms
}

// ExtractAnchors 提取文件全部锚点（符号+变量）
func ExtractAnchors(file string, src string) []ASTSymbol {
	return append(ExtractSymbols(file, src), ExtractVariables(file, src)...)
}

// CleanedLinesFor 生成行级清洗数据（保留原始行号映射）
func CleanedLinesFor(file string, src string) CleanedLines {
	s := reBlockComment.ReplaceAllString(src, "")
	s = reLineComment.ReplaceAllString(s, "")
	cl := CleanedLines{File: file}
	raw := 0
	for _, line := range strings.Split(s, "\n") {
		raw++
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		cl.Lines = append(cl.Lines, t)
		cl.RawLine = append(cl.RawLine, raw)
	}
	return cl
}

// FileExistsInScope 检查文件是否仍在仓库范围内（Scope守卫）
func FileExistsInScope(repoRoot, relPath string) bool {
	if repoRoot == "" || relPath == "" {
		return true
	}
	full := filepath.Join(repoRoot, filepath.Clean(relPath))
	info, err := os.Stat(full)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// NormalizeRelPath 统一相对路径分隔符
func NormalizeRelPath(p string) string {
	p = filepath.Clean(p)
	return strings.ReplaceAll(p, "\\", "/")
}

func lineIndexOf(s string, idx int) int {
	if idx > len(s) {
		idx = len(s)
	}
	return strings.Count(s[:idx], "\n") + 1
}
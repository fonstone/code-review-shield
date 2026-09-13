package governance

import (
	"strings"
)

// Taxonomy 缺陷分类白名单与标准化
type Taxonomy struct {
	// canonical 白名单分类（canonical名 → 别名集合）
	canonical map[string][]string
}

// NewTaxonomy 创建分类白名单
func NewTaxonomy() *Taxonomy {
	return &Taxonomy{canonical: map[string][]string{
		"memory_leak":        {"memory_leak", "memory leak", "内存泄漏", "leak", "泄漏"},
		"coredump_risk":      {"coredump_risk", "coredump", "crash", "崩溃", "core dump", "segfault"},
		"float_comparison":   {"float_comparison", "float compare", "浮点比较", "浮点", "float equality"},
		"cjson_scan":         {"cjson_scan", "cjson", "json misuse", "json"},
		"unused_var":         {"unused_var", "unused variable", "未使用变量", "unused"},
		"null_deref":         {"null_deref", "null pointer", "空指针", "null dereference", "nullptr"},
		"buffer_overflow":    {"buffer_overflow", "buffer overflow", "缓冲区溢出", "overflow", "越界"},
		"resource_leak":      {"resource_leak", "fd leak", "文件描述符泄漏", "handle leak", "资源泄漏"},
		"concurrency":        {"concurrency", "race", "race condition", "并发", "竞态", "死锁", "deadlock"},
		"error_handling":     {"error_handling", "error handling", "错误处理", "返回值未检查", "unchecked"},
		"type_safety":        {"type_safety", "类型安全", "type mismatch", "cast"},
		"initialization":     {"initialization", "uninitialized", "未初始化", "未初始化变量"},
		"other":              {},
	}}
}

// Normalize 将任意分类名标准化为白名单内canonical名；未知分类归入other
func (t *Taxonomy) Normalize(category string) string {
	c := strings.ToLower(strings.TrimSpace(category))
	if c == "" {
		return "other"
	}
	for canon, aliases := range t.canonical {
		if c == canon {
			return canon
		}
		for _, a := range aliases {
			if c == strings.ToLower(a) || strings.Contains(c, strings.ToLower(a)) {
				return canon
			}
		}
	}
	return "other"
}

// IsKnown 是否白名单分类
func (t *Taxonomy) IsKnown(category string) bool {
	return t.Normalize(category) != "other" || strings.EqualFold(strings.TrimSpace(category), "other")
}

// Categories 全部白名单分类
func (t *Taxonomy) Categories() []string {
	out := make([]string, 0, len(t.canonical))
	for k := range t.canonical {
		out = append(out, k)
	}
	return out
}

// DisplayName 分类中文名
func (t *Taxonomy) DisplayName(category string) string {
	names := map[string]string{
		"memory_leak":      "内存泄漏",
		"coredump_risk":    "Coredump风险",
		"float_comparison": "浮点比较",
		"cjson_scan":       "cJSON使用",
		"unused_var":       "未使用变量",
		"null_deref":       "空指针解引用",
		"buffer_overflow":  "缓冲区溢出",
		"resource_leak":    "资源泄漏",
		"concurrency":      "并发安全",
		"error_handling":   "错误处理",
		"type_safety":      "类型安全",
		"initialization":   "初始化问题",
		"other":            "其他",
	}
	if n, ok := names[t.Normalize(category)]; ok {
		return n
	}
	return category
}
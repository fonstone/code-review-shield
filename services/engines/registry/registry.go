// Package registry 引擎注册工厂：按模式创建引擎实例。
// 独立子包用于打破 engines ↔ engines/<sub> 的父子循环依赖。
package registry

import (
	"code-shield/services/engines"
	"code-shield/services/engines/chunked"
	"code-shield/services/engines/debate"
	"code-shield/services/engines/single"
)

// NewEngine 按模式创建引擎实例
//   - single → SingleEngine
//   - chunked / chunked_fast → ChunkedEngine（fast为轻量分片）
//   - debate_full / debate_selective → DebateEngine（selective仅高风险分片辩论）
func NewEngine(mode string) engines.TaskEngine {
	switch mode {
	case "single":
		return &single.Engine{}
	case "chunked", "chunked_fast":
		return &chunked.Engine{}
	case "debate_full", "debate_selective":
		return &debate.Engine{}
	default:
		return &chunked.Engine{}
	}
}

// SupportedModes 全部支持的引擎模式
func SupportedModes() []string {
	return []string{"single", "chunked", "chunked_fast", "debate_full", "debate_selective"}
}
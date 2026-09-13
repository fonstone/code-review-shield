// Package engines 扫描引擎域：纯内存规约。
// 引擎只接收内存结构体输入、返回内存EngineResult，禁止任何DB/gorm访问（lint-arch门禁校验）。
// 依赖方向：runner → engines → invoker。
package engines

import (
	"time"

	"code-shield/services/invoker"
)

// EngineContext 引擎运行上下文（纯内存，由runner层填充）
type EngineContext struct {
	TaskTypeName string
	TaskTypeID   uint
	RepoName     string
	RepoPath     string // 仓库本地路径（chunker使用）
	DiffBase     string // 对比基线（git ref）
	SinceDays    int    // 增量天数

	Mode         string // single / chunked / chunked_fast / debate_full / debate_selective
	Config       ChunkConfig
	Debate       DebateTiers
	FastPass     bool

	// Invokers 按资源ID索引的调用器（辩论分层调用不同资源）
	Invokers map[string]invoker.AIInvoker
	// DefaultInvoker 默认调用器
	DefaultInvoker invoker.AIInvoker

	AnalysisPrompt  string // 任务插件analysis_prompt.md内容
	SynthesisPrompt string // 任务插件synthesis_prompt.md内容

	TimeoutSeconds int
	CancelCh       <-chan struct{}

	// ResumeChunkIDs 断点续跑：仅重跑指定分片（空=全量）
	ResumeChunkIDs []string
}

// Finding 引擎输出的缺陷（内存结构体，DB写入由runner层负责）
type Finding struct {
	Severity    string  `json:"severity"`
	Category    string  `json:"category"`
	FilePath    string  `json:"file_path"`
	LineNumber  string  `json:"line_number"`
	CodeSnippet string  `json:"code_snippet"`
	Title       string  `json:"title"`
	Detail      string  `json:"detail"`
	Suggestion  string  `json:"suggestion"`
	Confidence  float64 `json:"confidence"`
}

// ChunkStatus 分片执行状态
type ChunkStatus struct {
	ID     string `json:"id"`
	File   string `json:"file"`
	Status string `json:"status"` // success / failed
	Error  string `json:"error,omitempty"`
}

// EngineResult 引擎输出（内存结构体）
type EngineResult struct {
	Findings       []Finding      `json:"findings"`
	Summary        string         `json:"summary"`
	Metrics        map[string]any `json:"metrics"`
	TotalChunks    int            `json:"total_chunks"`
	ProcessedChunks int           `json:"processed_chunks"`
	SuccessChunks  int            `json:"success_chunks"`
	FailedChunks   int            `json:"failed_chunks"`
	FailedChunkIDs []string       `json:"failed_chunk_ids"`
	Chunks         []ChunkStatus  `json:"chunks"`
	DurationMs     int64          `json:"duration_ms"`
}

// TaskEngine 引擎顶层接口
type TaskEngine interface {
	// Name 引擎名
	Name() string
	// Modes 支持的引擎模式
	Modes() []string
	// Run 执行扫描
	Run(ctx EngineContext) (*EngineResult, error)
}

// isCancelled 检查取消信号
func isCancelled(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// IsCancelled 供子包复用的取消检查
func IsCancelled(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	return isCancelled(ch)
}

// elapsed 计时辅助
func elapsed(start time.Time) int64 { return time.Since(start).Milliseconds() }

// invariant 防御性校验：确保纯内存规约不被破坏
func invariant(cond bool, msg string) {
	if !cond {
		panic("[engines] 规约违规: " + msg)
	}
}

var _ = elapsed
var _ = invariant
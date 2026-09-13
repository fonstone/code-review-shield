// Package single SingleEngine：single模式，一次调用扫描整个仓库（小仓库/演示场景）。
package single

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"code-shield/services/engines"
	"code-shield/services/engines/chunker"
	"code-shield/services/invoker"
)

// Engine single模式引擎
type Engine struct{}

// Name 引擎名
func (e *Engine) Name() string { return "single" }

// Modes 支持的模式
func (e *Engine) Modes() []string { return []string{"single"} }

// Run 单次全仓扫描（不拆分代码分片）
func (e *Engine) Run(ctx engines.EngineContext) (*engines.EngineResult, error) {
	start := time.Now()
	inv := ctx.DefaultInvoker
	if inv == nil {
		return nil, fmt.Errorf("single引擎缺少默认Invoker")
	}
	cfg := ctx.Config
	cfg.Normalize()
	cfg.FastMode = true

	// 复用chunker收集文件（只保留文件元数据）
	chunks, err := chunker.ChunkRepository(ctx.RepoPath, chunkerOpts(cfg))
	if err != nil {
		return nil, fmt.Errorf("扫描仓库文件失败: %w", err)
	}
	metas := collectMetas(chunks)
	if len(metas) == 0 {
		return &engines.EngineResult{
			Metrics:    map[string]any{"mode": "single", "files": 0, "note": "无匹配文件"},
			DurationMs: time.Since(start).Milliseconds(),
		}, nil
	}

	// 拼接文件索引
	var sb strings.Builder
	sb.WriteString("仓库文件索引：\n")
	for _, m := range metas {
		sb.WriteString(fmt.Sprintf("- %s (%d行)\n", m.FilePath, m.EndLine))
	}
	idx := sb.String()
	if len(idx) > 12000 {
		idx = idx[:12000] + "...(截断)"
	}

	prompt := fmt.Sprintf("%s\n\n## 待扫描仓库\n%s\n## 输出要求\n严格输出JSON数组，元素字段: severity/category/file_path/line_number/code_snippet/title/detail/suggestion/confidence。file_path必须使用仓库内相对路径。",
		ctx.AnalysisPrompt, idx)

	callCtx, cancel := withCancel(ctx)
	defer cancel()

	resp, err := inv.Invoke(callCtx, invoker.AIRequest{
		Model:       inv.Model(),
		Prompt:      prompt,
		MaxTokens:   4096,
		Temperature: 0.2,
	})
	if err != nil {
		return nil, err
	}
	findings, err := engines.ParseFindings(resp.Content)
	if err != nil {
		return nil, err
	}
	findings = filterInScope(findings, metas)
	return &engines.EngineResult{
		Findings:      findings,
		Summary:       fmt.Sprintf("single模式完成，分析 %d 个文件，发现 %d 个缺陷", len(metas), len(findings)),
		Metrics:       map[string]any{"mode": "single", "files": len(metas), "findings": len(findings)},
		TotalChunks:   len(metas),
		SuccessChunks: len(metas),
		DurationMs:    time.Since(start).Milliseconds(),
	}, nil
}

type fileMeta struct {
	FilePath string
	EndLine  int
}

func collectMetas(chunks []chunker.Chunk) []fileMeta {
	seen := map[string]bool{}
	var metas []fileMeta
	for _, c := range chunks {
		if seen[c.FilePath] {
			continue
		}
		seen[c.FilePath] = true
		metas = append(metas, fileMeta{FilePath: c.FilePath, EndLine: c.EndLine})
	}
	sort.SliceStable(metas, func(i, j int) bool { return metas[i].FilePath < metas[j].FilePath })
	return metas
}

func chunkerOpts(cfg engines.ChunkConfig) chunker.Options {
	return chunker.Options{
		FileExtensions:   cfg.FileExtensions,
		ContentKeywords:  cfg.ContentKeywords,
		ExcludePaths:     cfg.ExcludePaths,
		MaxFiles:         cfg.MaxFiles,
		Depth:            cfg.Depth,
		MaxLinesPerChunk: cfg.MaxLinesPerChunk,
		OverlapLines:     cfg.OverlapLines,
		FastMode:         cfg.FastMode,
	}
}

func filterInScope(fs []engines.Finding, metas []fileMeta) []engines.Finding {
	inScope := map[string]bool{}
	for _, m := range metas {
		inScope[m.FilePath] = true
	}
	var out []engines.Finding
	for _, f := range fs {
		if f.FilePath == "" {
			continue
		}
		p := filepath.Clean(f.FilePath)
		if inScope[p] {
			out = append(out, f)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].FilePath != out[j].FilePath {
			return out[i].FilePath < out[j].FilePath
		}
		return out[i].LineNumber < out[j].LineNumber
	})
	return out
}

func withCancel(ctx engines.EngineContext) (context.Context, context.CancelFunc) {
	cancelCtx, cancel := context.WithCancel(context.Background())
	if ctx.CancelCh != nil {
		go func() {
			select {
			case <-ctx.CancelCh:
				cancel()
			case <-cancelCtx.Done():
			}
		}()
	}
	return cancelCtx, cancel
}
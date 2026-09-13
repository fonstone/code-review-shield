// Package chunked ChunkedEngine：chunked / chunked_fast 模式，语义分片并行扫描。
// chunked_fast：轻量分片（更小分片、更少调用轮次），用于快速扫描。
package chunked

import (
	"context"
	"fmt"
	"sync"
	"time"

	"code-shield/services/engines"
	"code-shield/services/engines/chunker"
	"code-shield/services/invoker"
)

// Engine 分片并行引擎
type Engine struct{}

// Name 引擎名
func (e *Engine) Name() string { return "chunked" }

// Modes 支持的模式
func (e *Engine) Modes() []string { return []string{"chunked", "chunked_fast"} }

// Run 分片扫描：仓库→分片→并发调用→合并
func (e *Engine) Run(ctx engines.EngineContext) (*engines.EngineResult, error) {
	start := time.Now()
	inv := ctx.DefaultInvoker
	if inv == nil {
		return nil, fmt.Errorf("chunked引擎缺少默认Invoker")
	}
	cfg := ctx.Config
	cfg.Normalize()
	if ctx.Mode == "chunked_fast" {
		cfg.FastMode = true
	}

	chunks, err := chunker.ChunkRepository(ctx.RepoPath, chunkerOpts(cfg))
	if err != nil {
		return nil, fmt.Errorf("仓库分片失败: %w", err)
	}
	if ctx.ResumeChunkIDs != nil {
		chunks = chunker.FilterByIDs(chunks, ctx.ResumeChunkIDs)
	}
	result := &engines.EngineResult{
		Metrics:   map[string]any{"mode": ctx.Mode, "engine": "chunked"},
		TotalChunks: len(chunks),
	}
	if len(chunks) == 0 {
		result.DurationMs = time.Since(start).Milliseconds()
		return result, nil
	}

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	sem := make(chan struct{}, concurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := range chunks {
		if engines.IsCancelled(ctx.CancelCh) {
			result.FailedChunkIDs = append(result.FailedChunkIDs, chunks[i].ID)
			result.FailedChunks++
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(c chunker.Chunk) {
			defer wg.Done()
			defer func() { <-sem }()
			findings, err := e.analyzeChunk(ctx, inv, c, cfg.FastMode)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				result.FailedChunks++
				result.FailedChunkIDs = append(result.FailedChunkIDs, c.ID)
				result.Chunks = append(result.Chunks, engines.ChunkStatus{ID: c.ID, File: c.FilePath, Status: "failed", Error: err.Error()})
				return
			}
			result.SuccessChunks++
			result.Findings = append(result.Findings, findings...)
			result.Chunks = append(result.Chunks, engines.ChunkStatus{ID: c.ID, File: c.FilePath, Status: "success"})
		}(chunks[i])
	}
	wg.Wait()
	result.ProcessedChunks = result.SuccessChunks + result.FailedChunks
	result.DurationMs = time.Since(start).Milliseconds()
	result.Metrics["total_chunks"] = result.TotalChunks
	result.Metrics["success_chunks"] = result.SuccessChunks
	result.Metrics["failed_chunks"] = result.FailedChunks
	result.Summary = fmt.Sprintf("%s模式完成：%d/%d 分片成功，发现 %d 个缺陷",
		ctx.Mode, result.SuccessChunks, result.TotalChunks, len(result.Findings))
	return result, nil
}

// analyzeChunk 单分片分析
func (e *Engine) analyzeChunk(ctx engines.EngineContext, inv invoker.AIInvoker, c chunker.Chunk, fast bool) ([]engines.Finding, error) {
	prompt := fmt.Sprintf("%s\n\n## 待分析代码片段（%s 第%d-%d行）\n```\n%s\n```\n## 输出要求\n严格输出JSON数组，元素字段: severity/category/file_path/line_number/code_snippet/title/detail/suggestion/confidence。file_path必须等于\"%s\"。line_number使用片段内相对行号。",
		ctx.AnalysisPrompt, c.FilePath, c.StartLine, c.EndLine, c.Content, c.FilePath)

	callCtx, cancel := withCancel(ctx)
	defer cancel()

	resp, err := inv.Invoke(callCtx, invoker.AIRequest{
		Model:       inv.Model(),
		Prompt:      prompt,
		MaxTokens:   2048,
		Temperature: 0.2,
	})
	if err != nil {
		return nil, err
	}
	findings, err := engines.ParseFindings(resp.Content)
	if err != nil {
		return nil, err
	}
	// 行号转全局行号
	for i := range findings {
		f := &findings[i]
		f.FilePath = c.FilePath
		if n := parseLine(f.LineNumber); n > 0 {
			f.LineNumber = fmt.Sprintf("%d", c.StartLine+n-1)
		}
	}
	return findings, nil
}

func parseLine(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
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
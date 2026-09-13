// Package debate DebateEngine：debate_full / debate_selective 多智能体辩论引擎。
// debate_full：全量多轮质询（猎手→质询者→裁判→汇总），高风险专项扫描。
// debate_selective：仅高风险分片启动辩论，普通分片单模型快速通过（fast pass）。
package debate

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"code-shield/services/engines"
	"code-shield/services/engines/chunker"
	"code-shield/services/invoker"
)

// Engine 辩论引擎
type Engine struct{}

// Name 引擎名
func (e *Engine) Name() string { return "debate" }

// Modes 支持的模式
func (e *Engine) Modes() []string { return []string{"debate_full", "debate_selective"} }

// Run 辩论扫描
func (e *Engine) Run(ctx engines.EngineContext) (*engines.EngineResult, error) {
	start := time.Now()
	cfg := ctx.Config
	cfg.Normalize()
	cfg.FastMode = false

	chunks, err := chunker.ChunkRepository(ctx.RepoPath, chunkerOpts(cfg))
	if err != nil {
		return nil, fmt.Errorf("仓库分片失败: %w", err)
	}
	if ctx.ResumeChunkIDs != nil {
		chunks = chunker.FilterByIDs(chunks, ctx.ResumeChunkIDs)
	}
	result := &engines.EngineResult{
		Metrics:     map[string]any{"mode": ctx.Mode, "engine": "debate", "debated": 0, "fast_passed": 0},
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
			// debate_selective：低风险分片走fast pass（单模型）
			useDebate := ctx.Mode == "debate_full" || c.RiskScore >= 2 || len(chunks) <= 4
			findings, debated, err := e.analyzeChunk(ctx, c, useDebate)
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
			if debated {
				result.Metrics["debated"] = result.Metrics["debated"].(int) + 1
			} else {
				result.Metrics["fast_passed"] = result.Metrics["fast_passed"].(int) + 1
			}
		}(chunks[i])
	}
	wg.Wait()
	result.ProcessedChunks = result.SuccessChunks + result.FailedChunks
	result.DurationMs = time.Since(start).Milliseconds()
	result.Metrics["total_chunks"] = result.TotalChunks
	result.Metrics["success_chunks"] = result.SuccessChunks
	result.Metrics["failed_chunks"] = result.FailedChunks
	result.Summary = fmt.Sprintf("%s模式完成：%d 分片成功（辩论%d/快速通过%d），发现 %d 个缺陷",
		ctx.Mode, result.SuccessChunks, result.Metrics["debated"], result.Metrics["fast_passed"], len(result.Findings))
	return result, nil
}

// analyzeChunk 单分片：辩论或快速通过
func (e *Engine) analyzeChunk(ctx engines.EngineContext, c chunker.Chunk, debate bool) ([]engines.Finding, bool, error) {
	if !debate {
		findings, err := e.hunt(ctx, c)
		return findings, false, err
	}
	// 完整辩论：猎手 → 质询者 → 裁判 → 汇总
	hunterFindings, err := e.hunt(ctx, c)
	if err != nil {
		return nil, true, err
	}
	if len(hunterFindings) == 0 {
		return nil, true, nil
	}
	challenged, err := e.challenge(ctx, c, hunterFindings)
	if err != nil {
		return nil, true, err
	}
	judged, err := e.judge(ctx, c, challenged)
	if err != nil {
		return nil, true, err
	}
	return judged, true, nil
}

// hunt 猎手：定位候选缺陷
func (e *Engine) hunt(ctx engines.EngineContext, c chunker.Chunk) ([]engines.Finding, error) {
	inv, ok := selectInvoker(ctx, ctx.Debate.Tier1Hunter)
	if !ok {
		return nil, fmt.Errorf("辩论猎手层缺少Invoker")
	}
	prompt := fmt.Sprintf("%s\n\n## 待分析代码（%s 第%d-%d行）\n```\n%s\n```\n## 输出要求\n严格输出JSON数组（仅候选缺陷，宁缺毋滥），元素字段: severity/category/file_path/line_number/code_snippet/title/detail/suggestion/confidence。file_path=\"%s\"。",
		ctx.AnalysisPrompt, c.FilePath, c.StartLine, c.EndLine, c.Content, c.FilePath)
	callCtx, cancel := withCancel(ctx)
	defer cancel()
	resp, err := inv.Invoke(callCtx, invoker.AIRequest{Model: inv.Model(), Prompt: prompt, MaxTokens: 2048, Temperature: 0.3})
	if err != nil {
		return nil, err
	}
	findings, err := engines.ParseFindings(resp.Content)
	if err != nil {
		return nil, err
	}
	// 行号转全局
	for i := range findings {
		f := &findings[i]
		f.FilePath = c.FilePath
		if n := parseLine(f.LineNumber); n > 0 {
			f.LineNumber = fmt.Sprintf("%d", c.StartLine+n-1)
		}
	}
	return findings, nil
}

// challenge 质询者：对候选缺陷交叉校验，剔除可疑项
func (e *Engine) challenge(ctx engines.EngineContext, c chunker.Chunk, findings []engines.Finding) ([]engines.Finding, error) {
	inv, ok := selectInvoker(ctx, ctx.Debate.Tier2Challenger)
	if !ok {
		return findings, nil
	}
	jsonF, _ := marshalFindings(findings)
	prompt := fmt.Sprintf(
		"你是严格的代码质询者。以下是从代码中提取的候选缺陷清单，请逐条交叉校验：\n"+
			"1) 是否存在误报（例如：释放后有重赋值、错误路径已处理、编译器语义正确）\n"+
			"2) 严重度是否虚高\n"+
			"3) 是否漏掉关键上下文导致结论错误\n\n"+
			"## 原始代码（%s 第%d-%d行）\n```\n%s\n```\n\n## 候选缺陷JSON\n%s\n\n"+
			"## 输出要求\n输出审查后的JSON数组：保留你认为真实有效的缺陷，剔除误报，可修正severity/confidence。必须仍为JSON数组格式。",
		c.FilePath, c.StartLine, c.EndLine, c.Content, jsonF)
	callCtx, cancel := withCancel(ctx)
	defer cancel()
	resp, err := inv.Invoke(callCtx, invoker.AIRequest{Model: inv.Model(), Prompt: prompt, MaxTokens: 2048, Temperature: 0.2})
	if err != nil {
		return findings, nil // 质询失败不阻塞，保留猎手结果
	}
	reviewed, err := engines.ParseFindings(resp.Content)
	if err != nil || len(reviewed) == 0 {
		return findings, nil
	}
	return reviewed, nil
}

// judge 裁判：最终裁决
func (e *Engine) judge(ctx engines.EngineContext, c chunker.Chunk, findings []engines.Finding) ([]engines.Finding, error) {
	inv, ok := selectInvoker(ctx, ctx.Debate.Tier3Judge)
	if !ok {
		return findings, nil
	}
	jsonF, _ := marshalFindings(findings)
	prompt := fmt.Sprintf(
		"你是终审裁判。猎手提出候选缺陷，质询者已审查，请做最终裁决：保留真实缺陷、合并重复项、统一严重度分级(CRITICAL/HIGH/MEDIUM/LOW)、为每项输出修正后的confidence(0-1)。\n\n"+
			"## 原始代码（%s 第%d-%d行）\n```\n%s\n```\n\n## 候选缺陷JSON\n%s\n\n## 输出要求\n严格输出最终JSON数组。",
		c.FilePath, c.StartLine, c.EndLine, c.Content, jsonF)
	callCtx, cancel := withCancel(ctx)
	defer cancel()
	resp, err := inv.Invoke(callCtx, invoker.AIRequest{Model: inv.Model(), Prompt: prompt, MaxTokens: 2048, Temperature: 0.1})
	if err != nil {
		return findings, nil
	}
	final, err := engines.ParseFindings(resp.Content)
	if err != nil || len(final) == 0 {
		return findings, nil
	}
	return final, nil
}

func marshalFindings(fs []engines.Finding) (string, error) {
	b, err := json.Marshal(fs)
	return string(b), err
}

// selectInvoker 层级调用器选择
func selectInvoker(ctx engines.EngineContext, tier engines.DebateTier) (invoker.AIInvoker, bool) {
	if tier.Resource != "" {
		if inv, ok := ctx.Invokers[tier.Resource]; ok && inv != nil {
			return inv, true
		}
	}
	if ctx.DefaultInvoker != nil {
		return ctx.DefaultInvoker, true
	}
	return nil, false
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
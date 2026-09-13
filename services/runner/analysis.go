package runner

import (
	"fmt"
	"time"

	"code-shield/models"
)

// STAGE3 AI分析：调用TaskEngine，失败重试（指数退避），结果清洗。
func (r *Runner) stageAnalysis(tc *TaskContext) error {
	engine, ectx, err := r.buildEngine(tc)
	if err != nil {
		return err
	}

	maxRetries := r.Cfg.Scanner.Analysis.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	backoff := r.Cfg.Scanner.Analysis.RetryBackoffMs
	if backoff <= 0 {
		backoff = 2000
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if isCancelled(tc.CancelCh) {
			return fmt.Errorf("任务被取消")
		}
		result, err := engine.Run(ectx)
		if err == nil {
			tc.EngineResult = result
			r.updateProgress(tc)
			return nil
		}
		lastErr = err
		if attempt < maxRetries {
			delay := time.Duration(backoff*(1<<attempt)) * time.Millisecond
			if delay > time.Minute {
				delay = time.Minute
			}
			select {
			case <-time.After(delay):
			case <-tc.CancelCh:
				return fmt.Errorf("任务被取消")
			}
		}
	}
	return fmt.Errorf("引擎执行%d次尝试均失败: %w", maxRetries+1, lastErr)
}

// updateProgress 分片进度持久化（分析完成后）
func (r *Runner) updateProgress(tc *TaskContext) {
	res := tc.EngineResult
	if res == nil {
		return
	}
	r.DB.Model(&models.TaskReport{}).Where("id = ?", tc.Report.ID).Updates(map[string]any{
		"total_chunks":     res.TotalChunks,
		"processed_chunks": res.ProcessedChunks,
		"success_chunks":   res.SuccessChunks,
		"failed_chunks":    res.FailedChunks,
		"has_failed_chunks": res.FailedChunks > 0,
	})
}
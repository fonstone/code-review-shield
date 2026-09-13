package runner

import (
	"context"
	"fmt"

	"code-shield/models"
)

// 断点续跑：恢复失败分片逻辑。
// Resume 重新执行失败分片：读取上次报告的失败分片ID，仅重跑这些分片，
// 然后重新走 汇总→后处理→持久化 阶段。

// Resume 恢复失败分片（由 /api/tasks/:id/resume 调用）
func (r *Runner) Resume(ctx context.Context, reportID uint) error {
	var report models.TaskReport
	if err := r.DB.First(&report, reportID).Error; err != nil {
		return fmt.Errorf("任务报告不存在: %w", err)
	}
	if !report.HasFailedChunks {
		return fmt.Errorf("任务没有失败分片，无需恢复")
	}
	ids := metricsFailedChunkIDs(report.Metrics)
	if len(ids) == 0 {
		return fmt.Errorf("未找到失败分片记录")
	}
	if report.Status == models.StatusAnalyzing {
		return fmt.Errorf("任务正在执行中，无法恢复")
	}

	// 重置状态后重跑流水线（ResumeChunkIDs由RunPipeline自动读取）
	if err := r.DB.Model(&models.TaskReport{}).Where("id = ?", reportID).
		Updates(map[string]any{"status": models.StatusPending, "error_message": ""}).Error; err != nil {
		return err
	}
	return r.RunPipeline(ctx, reportID)
}

// CanResume 判断任务是否可恢复
func (r *Runner) CanResume(reportID uint) bool {
	var report models.TaskReport
	if err := r.DB.First(&report, reportID).Error; err != nil {
		return false
	}
	return report.HasFailedChunks && report.Status != models.StatusAnalyzing
}
package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"code-shield/models"
)

// STAGE6 持久化：单一事务写入 task_reports / analysis_findings。
func (r *Runner) stageFinalize(tc *TaskContext) error {
	now := time.Now()
	return r.DB.Transaction(func(tx *gorm.DB) error {
		// 1. 更新旧缺陷状态（RESOLVED/REOPENED）
		for _, old := range tc.OldFindings {
			if old.DiffStatus != "" && (old.Status == models.FindingResolved || old.DiffStatus != models.DiffExisted) {
				if err := tx.Model(&models.AnalysisFinding{}).Where("id = ?", old.ID).
					Updates(map[string]any{"status": old.Status, "diff_status": old.DiffStatus, "updated_at": now}).Error; err != nil {
					return err
				}
			}
		}

		// 2. 写入新缺陷
		for i := range tc.NewFindings {
			tc.NewFindings[i].ID = 0
			if err := tx.Create(&tc.NewFindings[i]).Error; err != nil {
				return err
			}
		}

		// 3. 报告路径与Markdown产物
		reportPath := r.writeReportFile(tc)

		// 4. 更新报告
		metrics := marshalMetrics(tc)
		tc.Report.Metrics = metrics
		return tx.Model(&models.TaskReport{}).Where("id = ?", tc.Report.ID).Updates(map[string]any{
			"status":          models.StatusSuccess,
			"report_path":     reportPath,
			"ai_summary":      tc.AISummary,
			"score":           tc.Score,
			"metrics":         metrics,
			"clone_status":    tc.CloneStatus,
			"clone_message":   tc.CloneMsg,
			"total_chunks":    tc.EngineResult.TotalChunks,
			"processed_chunks": tc.EngineResult.ProcessedChunks,
			"success_chunks":  tc.EngineResult.SuccessChunks,
			"failed_chunks":   tc.EngineResult.FailedChunks,
			"has_failed_chunks": tc.EngineResult.FailedChunks > 0,
			"error_message":   "",
			"finished_at":     now,
			"updated_at":      now,
		}).Error
	})
}

// marshalMetrics 序列化metrics（阶段日志+引擎统计+配置）
func marshalMetrics(tc *TaskContext) datatypes.JSON {
	m := map[string]any{
		"stages":       tc.stageLogsJSON(),
		"engine":       tc.EngineResultMetrics(),
		"since_days":   tc.SinceDays,
		"diff_base":    tc.DiffBase,
		"diff_summary": tc.DiffSummary,
		"calibrated":   tc.CalibratedN,
		"suppressed":   tc.SuppressedN,
	}
	if len(tc.CampaignGroups) > 0 {
		m["campaign_groups"] = tc.CampaignGroups
	}
	b, _ := json.Marshal(m)
	return datatypes.JSON(b)
}

// EngineResultMetrics 引擎统计（防御nil）
func (tc *TaskContext) EngineResultMetrics() map[string]any {
	if tc.EngineResult == nil {
		return map[string]any{"mode": "none"}
	}
	return map[string]any{
		"mode":             tc.EngineResult.Metrics["mode"],
		"total_chunks":     tc.EngineResult.TotalChunks,
		"success_chunks":   tc.EngineResult.SuccessChunks,
		"failed_chunks":    tc.EngineResult.FailedChunks,
		"duration_ms":      tc.EngineResult.DurationMs,
		"failed_chunk_ids": tc.EngineResult.FailedChunkIDs,
	}
}

// writeReportFile 生成Markdown报告文件
func (r *Runner) writeReportFile(tc *TaskContext) string {
	dir := filepath.Join(r.Cfg.Storage.Root, r.Cfg.Storage.ReportDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	name := fmt.Sprintf("%s-%s-%d.md", safeName(tc.Repo.Name), tc.TaskType.Name, tc.Report.ID)
	path := filepath.Join(dir, name)
	content := tc.AISummary
	if content == "" {
		content = fmt.Sprintf("# %s 扫描报告\n\n仓库: %s\n任务: %s\n", tc.TaskType.DisplayName, tc.Repo.Name, tc.TaskType.Name)
	}
	var sb strings.Builder
	sb.WriteString(content)
	sb.WriteString("\n\n## 缺陷明细\n")
	for _, f := range tc.NewFindings {
		sb.WriteString(fmt.Sprintf("- [%s][%s] %s | %s:%s | %s\n", f.Severity, f.DiffStatus, f.Title, f.FilePath, f.LineNumber, f.Category))
	}
	if len(tc.NewFindings) == 0 {
		sb.WriteString("- 无缺陷\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return ""
	}
	return path
}
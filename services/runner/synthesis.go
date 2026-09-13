package runner

import (
	"encoding/json"
	"fmt"
	"strings"

	"code-shield/models"
	"code-shield/services/defects"
	"code-shield/services/invoker"
)

// STAGE4 汇总：调用defects.DiffEngine做四级漏斗增量比对 + Scope守卫 + AI总结。
func (r *Runner) stageSynthesis(tc *TaskContext) error {
	// 1. 计算新缺陷指纹
	newFindings := make([]models.AnalysisFinding, 0, len(tc.EngineResult.Findings))
	for _, f := range tc.EngineResult.Findings {
		fp := defects.FingerprintFinding(f.FilePath, f.Title, f.CodeSnippet, f.LineNumber)
		newFindings = append(newFindings, models.AnalysisFinding{
			TaskReportID:  tc.Report.ID,
			TaskTypeID:    tc.TaskType.ID,
			RepoID:        tc.Repo.ID,
			Severity:      f.Severity,
			Category:      f.Category,
			FilePath:      defects.NormalizeRelPath(f.FilePath),
			LineNumber:    f.LineNumber,
			CodeSnippet:   f.CodeSnippet,
			Title:         f.Title,
			Detail:        f.Detail,
			Suggestion:    f.Suggestion,
			Status:        models.FindingOpen,
			Confidence:    f.Confidence,
			L1Fingerprint: fp.L1,
			L2Fingerprint: fp.L2,
		})
	}

	// 2. 加载历史缺陷（同repo+task_type上次成功报告）
	oldFindings := r.loadPreviousFindings(tc)

	// 3. 四级漏斗增量比对
	if r.Cfg.Governance.Fingerprint.Enabled {
		in := defects.DiffInput{
			RepoRoot:        tc.RepoRoot,
			NewFindings:     toFindingLike(newFindings),
			Previous:        toFindingLike(oldFindings),
			SimilarityThres: r.Cfg.Governance.Fingerprint.SimilarityThreshold,
			ScopeGuard:      r.Cfg.Governance.Lifecycle.ScopeGuardEnabled,
			AutoResolve:     r.Cfg.Governance.Lifecycle.AutoResolveMissing,
		}
		out := r.defectsEngine().Compare(in)
		tc.DiffOutput = out
		tc.DiffSummary = out.Summary()
		// 新缺陷diff_status
		for i := range newFindings {
			if st, ok := out.NewStatus[newFindings[i].ID]; ok {
				newFindings[i].DiffStatus = st
			} else {
				newFindings[i].DiffStatus = models.DiffNew
			}
		}
		// 旧缺陷状态继承（在finalize中落库）
		applyOldStatus(tc, out)
	} else {
		for i := range newFindings {
			newFindings[i].DiffStatus = models.DiffNew
		}
		tc.DiffSummary = "指纹比对已禁用，全部标记为NEW"
	}
	tc.NewFindings = newFindings
	tc.OldFindings = oldFindings

	// 4. AI总结（synthesis prompt）
	tc.AISummary = r.generateSummary(tc)
	return nil
}

// loadPreviousFindings 加载历史缺陷
func (r *Runner) loadPreviousFindings(tc *TaskContext) []models.AnalysisFinding {
	var report models.TaskReport
	err := r.DB.Where("repo_id = ? AND task_type_id = ? AND status = ? AND id <> ?",
		tc.Repo.ID, tc.TaskType.ID, models.StatusSuccess, tc.Report.ID).
		Order("id DESC").First(&report).Error
	if err != nil {
		return nil
	}
	var findings []models.AnalysisFinding
	r.DB.Where("task_report_id = ? AND status <> ?", report.ID, models.FindingSuppressed).
		Find(&findings)
	return findings
}

// applyOldStatus 记录旧缺陷的RESOLVED/REOPENED状态（finalize中落库）
func applyOldStatus(tc *TaskContext, out *defects.DiffOutput) {
	for id, st := range out.OldStatus {
		for i := range tc.OldFindings {
			if tc.OldFindings[i].ID == id {
				tc.OldFindings[i].DiffStatus = st
				if st == defects.DiffResolved {
					tc.OldFindings[i].Status = models.FindingResolved
				}
			}
		}
	}
}

// generateSummary 生成AI汇总报告（synthesis prompt），失败时降级为模板总结
func (r *Runner) generateSummary(tc *TaskContext) string {
	sevs := map[string]int{}
	for _, f := range tc.NewFindings {
		sevs[f.Severity]++
	}
	tpl := fmt.Sprintf(
		"# %s 扫描报告\n\n- 仓库: %s\n- 任务类型: %s (%s)\n- 执行引擎: %s\n- 新增缺陷: %d | EXISTED: %d | 总计: %d\n- 严重度: CRITICAL=%d HIGH=%d MEDIUM=%d LOW=%d\n- 评分: %d\n\n## 汇总\n%s\n\n%s",
		tc.TaskType.DisplayName, tc.Repo.Name, tc.TaskType.DisplayName, tc.TaskType.EngineMode,
		"六阶段流水线", countDiff(tc, models.DiffNew), countDiff(tc, models.DiffExisted), len(tc.NewFindings),
		sevs[models.SeverityCritical], sevs[models.SeverityHigh], sevs[models.SeverityMedium], sevs[models.SeverityLow],
		computeScore(tc.NewFindings), tc.DiffSummary, topFindingsMarkdown(tc.NewFindings))

	// 尝试LLM总结
	synthInv := r.synthesisInvoker(tc)
	if synthInv == nil {
		return tpl
	}
	prompt := tc.TaskType.SynthesisPrompt
	if strings.TrimSpace(prompt) == "" {
		return tpl
	}
	payload := buildSummaryPayload(tc)
	resp, err := synthInv.Invoke(backgroundCtx(tc), invoker.AIRequest{
		Model:       synthInv.Model(),
		Prompt:      fmt.Sprintf("%s\n\n## 缺陷数据\n%s", prompt, payload),
		MaxTokens:   2048,
		Temperature: 0.3,
	})
	if err != nil || resp == nil || strings.TrimSpace(resp.Content) == "" {
		return tpl
	}
	return resp.Content
}

func countDiff(tc *TaskContext, st string) int {
	n := 0
	for _, f := range tc.NewFindings {
		if f.DiffStatus == st {
			n++
		}
	}
	return n
}

func topFindingsMarkdown(fs []models.AnalysisFinding) string {
	if len(fs) == 0 {
		return "- 未发现缺陷"
	}
	var sb strings.Builder
	sb.WriteString("## 高风险缺陷\n")
	n := 0
	for _, f := range fs {
		if (f.Severity == models.SeverityCritical || f.Severity == models.SeverityHigh) && n < 10 {
			sb.WriteString(fmt.Sprintf("- [%s] %s (%s:%s) %s\n", f.Severity, f.Title, f.FilePath, f.LineNumber, f.DiffStatus))
			n++
		}
	}
	if n == 0 {
		sb.WriteString("- 无高风险缺陷\n")
	}
	return sb.String()
}

// computeScore 缺陷评分
func computeScore(fs []models.AnalysisFinding) int {
	score := 100
	for _, f := range fs {
		switch f.Severity {
		case models.SeverityCritical:
			score -= 10
		case models.SeverityHigh:
			score -= 5
		case models.SeverityMedium:
			score -= 2
		default:
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	return score
}

func buildSummaryPayload(tc *TaskContext) string {
	items := make([]map[string]any, 0, len(tc.NewFindings))
	for _, f := range tc.NewFindings {
		items = append(items, map[string]any{
			"severity": f.Severity, "category": f.Category, "file_path": f.FilePath,
			"line_number": f.LineNumber, "title": f.Title, "diff_status": f.DiffStatus,
			"confidence": f.Confidence,
		})
	}
	b, _ := json.Marshal(map[string]any{
		"repo": tc.Repo.Name, "task_type": tc.TaskType.Name, "engine_mode": tc.TaskType.EngineMode,
		"diff_summary": tc.DiffSummary, "findings": items,
	})
	return string(b)
}
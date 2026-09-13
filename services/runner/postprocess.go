package runner

import (
	"context"

	"code-shield/models"
	"code-shield/services/defects"
	"code-shield/services/governance"
	"code-shield/services/invoker"
)

// STAGE5 后处理：缺陷打分、治理规则、issue归并、误报抑制。
func (r *Runner) stagePostprocess(tc *TaskContext) error {
	tc.Score = computeScore(tc.NewFindings)

	// 1. 严重度决策树校准
	for i := range tc.NewFindings {
		f := &tc.NewFindings[i]
		cal := r.Calibrator.Calibrate(f.Severity, f.Title, f.Detail, f.Confidence)
		if cal.Adjusted {
			tc.CalibratedN++
			f.Severity = cal.Severity
		}
	}

	// 2. 分类白名单标准化
	for i := range tc.NewFindings {
		f := &tc.NewFindings[i]
		f.Category = r.Taxonomy.Normalize(f.Category)
	}

	// 3. 误报记忆抑制（feedback治理模式）
	if tc.TaskType.GovernanceMode == "feedback" {
		var keep []models.AnalysisFinding
		for i := range tc.NewFindings {
			f := &tc.NewFindings[i]
			if r.Feedback.IsSuppressed(f.L1Fingerprint, f.L2Fingerprint) {
				tc.SuppressedN++
				continue
			}
			keep = append(keep, *f)
		}
		tc.NewFindings = keep
	}

	// 4. 专项扫描归并（campaign）
	if tc.TaskType.IsCampaign {
		items := make([]governance.CampaignFinding, 0, len(tc.NewFindings))
		for _, f := range tc.NewFindings {
			items = append(items, governance.CampaignFinding{
				FindingID:  f.ID,
				RepoID:     tc.Repo.ID,
				RepoName:   tc.Repo.Name,
				Severity:   f.Severity,
				Category:   f.Category,
				FilePath:   f.FilePath,
				Title:      f.Title,
				DiffStatus: f.DiffStatus,
			})
		}
		group := r.Campaign.Group(tc.TaskType.Name, tc.TaskType.DisplayName, items)
		tc.CampaignGroups = append(tc.CampaignGroups, map[string]any{
			"name":        group.Name,
			"display_name": group.DisplayName,
			"total":       group.Total,
			"by_severity": group.BySeverity,
			"by_repo":     group.ByRepo,
			"top_files":   group.TopFiles,
		})
	}
	return nil
}

// toFindingLike 适配到DiffEngine输入
func toFindingLike(fs []models.AnalysisFinding) []defects.FindingLike {
	out := make([]defects.FindingLike, 0, len(fs))
	for _, f := range fs {
		out = append(out, defects.FindingLike{
			ID: f.ID, FilePath: f.FilePath, LineNumber: f.LineNumber, Title: f.Title,
			Severity: f.Severity, L1Fingerprint: f.L1Fingerprint, L2Fingerprint: f.L2Fingerprint,
			Status: f.Status, DiffStatus: f.DiffStatus,
		})
	}
	return out
}

// synthesisInvoker 汇总阶段调用器（debate tier4优先）
func (r *Runner) synthesisInvoker(tc *TaskContext) invoker.AIInvoker {
	_, ectx, err := r.buildEngine(tc)
	if err != nil {
		return nil
	}
	tier := tc.Runner.debateTiers().Tier4Synthesis
	if tier.Resource != "" {
		if inv, ok := ectx.Invokers[tier.Resource]; ok && inv != nil {
			return inv
		}
	}
	return ectx.DefaultInvoker
}

// invReq 简化请求结构（避免循环依赖）
type invReq struct {
	Prompt string
}

// backgroundCtx 任务取消感知的context
func backgroundCtx(tc *TaskContext) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-tc.CancelCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx
}
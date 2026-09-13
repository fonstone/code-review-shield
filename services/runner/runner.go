// Package runner 流水线协调域：6阶段任务生命周期。
// STAGE1 Git同步 → STAGE2 准入 → STAGE3 分析 → STAGE4 汇总 → STAGE5 后处理 → STAGE6 持久化。
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"

	"code-shield/models"
	"code-shield/services/defects"
	"code-shield/services/dispatcher"
	"code-shield/services/engines"
	"code-shield/services/engines/registry"
	"code-shield/services/governance"
	"code-shield/services/invoker"
	"code-shield/services/queue"
)

// Runner 流水线主协调器
type Runner struct {
	DB        *gorm.DB
	Cfg       *models.Config
	Dispatcher *dispatcher.ModelDispatcher

	Feedback  *governance.FeedbackStore
	Taxonomy  *governance.Taxonomy
	Calibrator *governance.Calibrator
	Campaign  *governance.Campaign

	Workers *queue.WorkerPool

	runningMu sync.Mutex
	running   map[uint]context.CancelFunc
}

// NewRunner 创建流水线协调器
func NewRunner(db *gorm.DB, cfg *models.Config, d *dispatcher.ModelDispatcher, workers *queue.WorkerPool) *Runner {
	r := &Runner{
		DB:         db,
		Cfg:        cfg,
		Dispatcher: d,
		Feedback:   governance.NewFeedbackStore(),
		Taxonomy:   governance.NewTaxonomy(),
		Calibrator: governance.NewCalibrator(),
		Campaign:   governance.NewCampaign(),
		Workers:    workers,
		running:    map[uint]context.CancelFunc{},
	}
	if workers != nil {
		workers.SetHandler(r.handleJob)
	}
	return r
}

// Start 启动队列消费
func (r *Runner) Start() {
	if r.Workers != nil {
		r.Workers.Start()
	}
}

// Stop 停止消费
func (r *Runner) Stop() {
	if r.Workers != nil {
		r.Workers.Stop()
	}
}

// handleJob Worker消费回调：执行流水线，失败按重试策略重入队
func (r *Runner) handleJob(ctx context.Context, job queue.Job) error {
	err := r.RunPipeline(ctx, job.TaskReportID)
	if err == nil {
		r.DB.Model(&models.QueueTask{}).Where("id = ?", job.QueueTaskID).
			Updates(map[string]any{"status": models.QueueDone, "attempts": job.Attempt + 1})
		return nil
	}
	r.DB.Model(&models.QueueTask{}).Where("id = ?", job.QueueTaskID).
		Updates(map[string]any{"status": models.QueueFailed, "attempts": job.Attempt + 1})
	return err
}

// RunPipeline 执行6阶段流水线
func (r *Runner) RunPipeline(ctx context.Context, reportID uint) error {
	// 加载任务
	var report models.TaskReport
	if err := r.DB.Preload("Repo").Preload("TaskType").First(&report, reportID).Error; err != nil {
		return fmt.Errorf("加载任务报告失败: %w", err)
	}
	tc := &TaskContext{
		Report:   &report,
		TaskType: report.TaskType,
		Repo:     report.Repo,
		DB:       r.DB,
		Runner:   r,
		StartedAt: time.Now(),
		ResumeChunkIDs: r.resumeChunkIDs(&report),
	}

	// 注册取消
	cancelCtx, cancel := context.WithCancel(ctx)
	tc.CancelCh = cancelCtx.Done()
	r.registerRunning(report.ID, cancel)
	defer func() {
		r.unregisterRunning(report.ID)
		cancel()
	}()

	// 状态流转
	r.updateStatus(report.ID, models.StatusAnalyzing)

	// STAGE1 Git同步
	start := time.Now()
	if err := r.stageGitSync(tc); err != nil {
		tc.addStage(StageGitSync, StageError, start, "Git同步失败", err.Error())
		r.failReport(tc, fmt.Errorf("Git同步失败: %w", err))
		return err
	}
	tc.addStage(StageGitSync, StageOK, start, fmt.Sprintf("同步完成: %s", tc.RepoRoot), "")

	// STAGE2 准入
	start = time.Now()
	skipReason, err := r.stagePrecondition(tc)
	if err != nil {
		tc.addStage(StagePrecondition, StageError, start, "准入校验失败", err.Error())
		r.failReport(tc, fmt.Errorf("准入校验失败: %w", err))
		return err
	}
	if skipReason != "" {
		tc.addStage(StagePrecondition, StageSkipped, start, "快速跳过: "+skipReason, "")
		r.finalizeSkipped(tc, skipReason)
		return nil
	}
	tc.addStage(StagePrecondition, StageOK, start, "准入校验通过", "")

	// STAGE3 分析
	start = time.Now()
	if err := r.stageAnalysis(tc); err != nil {
		tc.addStage(StageAnalysis, StageError, start, "AI分析失败", err.Error())
		r.failReport(tc, fmt.Errorf("AI分析失败: %w", err))
		return err
	}
	tc.addStage(StageAnalysis, StageOK, start, fmt.Sprintf("分析完成: %d/%d分片成功, %d缺陷",
		tc.EngineResult.SuccessChunks, tc.EngineResult.TotalChunks, len(tc.EngineResult.Findings)), "")

	// STAGE4 汇总
	start = time.Now()
	if err := r.stageSynthesis(tc); err != nil {
		tc.addStage(StageSynthesis, StageError, start, "增量比对失败", err.Error())
		r.failReport(tc, fmt.Errorf("汇总失败: %w", err))
		return err
	}
	tc.addStage(StageSynthesis, StageOK, start, tc.DiffSummary, "")

	// STAGE5 后处理
	start = time.Now()
	if err := r.stagePostprocess(tc); err != nil {
		tc.addStage(StagePostprocess, StageError, start, "后处理失败", err.Error())
		r.failReport(tc, fmt.Errorf("后处理失败: %w", err))
		return err
	}
	tc.addStage(StagePostprocess, StageOK, start, fmt.Sprintf("后处理完成: 校准%d条, 抑制%d条", tc.CalibratedN, tc.SuppressedN), "")

	// STAGE6 持久化
	start = time.Now()
	if err := r.stageFinalize(tc); err != nil {
		tc.addStage(StageFinalize, StageError, start, "持久化失败", err.Error())
		r.failReport(tc, fmt.Errorf("持久化失败: %w", err))
		return err
	}
	tc.addStage(StageFinalize, StageOK, start, fmt.Sprintf("持久化完成: 写入%d条缺陷, 评分%d", len(tc.NewFindings), tc.Score), "")
	// 补充写入包含finalize阶段的完整metrics（阶段日志）
	r.DB.Model(&models.TaskReport{}).Where("id = ?", tc.Report.ID).
		Update("metrics", marshalMetrics(tc))

	// 通知（notify_threshold触发）
	r.notify(tc)
	return nil
}

// CancelRunning 批量取消正在执行的任务
func (r *Runner) CancelRunning() int {
	r.runningMu.Lock()
	defer r.runningMu.Unlock()
	n := 0
	for id, cancel := range r.running {
		cancel()
		r.updateStatus(id, models.StatusCancelled)
		n++
	}
	return n
}

// RunningTasks 运行中任务列表
func (r *Runner) RunningTasks() []map[string]any {
	r.runningMu.Lock()
	defer r.runningMu.Unlock()
	var out []map[string]any
	for id := range r.running {
		var report models.TaskReport
		if err := r.DB.Select("id", "status", "task_type_id", "repo_id").First(&report, id).Error; err == nil {
			out = append(out, map[string]any{
				"task_report_id": report.ID,
				"status":         report.Status,
				"stage":          "running",
			})
		}
	}
	return out
}

func (r *Runner) registerRunning(id uint, cancel context.CancelFunc) {
	r.runningMu.Lock()
	defer r.runningMu.Unlock()
	r.running[id] = cancel
}

func (r *Runner) unregisterRunning(id uint) {
	r.runningMu.Lock()
	defer r.runningMu.Unlock()
	delete(r.running, id)
}

func (r *Runner) updateStatus(id uint, status string) {
	now := time.Now()
	updates := map[string]any{"status": status, "updated_at": now}
	if status == models.StatusAnalyzing {
		updates["started_at"] = now
	}
	if status == models.StatusSuccess || status == models.StatusFailed || status == models.StatusCancelled {
		updates["finished_at"] = now
	}
	r.DB.Model(&models.TaskReport{}).Where("id = ?", id).Updates(updates)
}

// failReport 失败报告落库
func (r *Runner) failReport(tc *TaskContext, err error) {
	tc.Report.Status = models.StatusFailed
	tc.Report.ErrorMessage = err.Error()
	tc.Report.Metrics = marshalMetrics(tc)
	r.updateStatus(tc.Report.ID, models.StatusFailed)
	r.DB.Model(&models.TaskReport{}).Where("id = ?", tc.Report.ID).
		Updates(map[string]any{"error_message": err.Error(), "metrics": tc.Report.Metrics})
}

// finalizeSkipped 快速跳过落库
func (r *Runner) finalizeSkipped(tc *TaskContext, reason string) {
	r.DB.Model(&models.TaskReport{}).Where("id = ?", tc.Report.ID).Updates(map[string]any{
		"status":        models.StatusSkipped,
		"clone_status":  models.CloneSkipped,
		"error_message": reason,
		"metrics":       marshalMetrics(tc),
		"finished_at":   time.Now(),
	})
}

func (r *Runner) resumeChunkIDs(report *models.TaskReport) []string {
	if !report.HasFailedChunks {
		return nil
	}
	ids := metricsFailedChunkIDs(report.Metrics)
	if len(ids) == 0 {
		return nil
	}
	return ids
}

// buildEngine 构造引擎上下文（纯内存）
func (r *Runner) buildEngine(tc *TaskContext) (engines.TaskEngine, engines.EngineContext, error) {
	cfg := r.Cfg
	factory := dispatcher.NewDispatchingFactory(r.Dispatcher, tc.Report.ID)

	// 每个资源创建基础invoker → 装饰为调度invoker
	base := map[string]invoker.AIInvoker{}
	for _, res := range cfg.LLM.Resources {
		inv, err := invoker.CreateInvoker(invoker.ResourceConfig{
			ID: res.ID, Driver: res.Driver, Model: res.Model,
			BaseURL: res.BaseURL, APIKey: res.APIKey, CLIPath: res.CLIPath,
			TimeoutSec: res.TimeoutSec, Concurrent: res.Concurrent,
		})
		if err != nil {
			return nil, engines.EngineContext{}, err
		}
		base[res.ID] = inv
	}
	disp := factory.WrapAll(base)

	def := cfg.DefaultResource()
	var defInvoker invoker.AIInvoker
	if def != nil {
		defInvoker = disp[def.ID]
	} else {
		for _, inv := range disp {
			defInvoker = inv
			break
		}
	}
	if defInvoker == nil {
		return nil, engines.EngineContext{}, errors.New("没有可用的LLM资源")
	}

	chunkCfg := engines.ChunkConfig{FastMode: tc.TaskType.EngineMode == "chunked_fast"}
	if len(tc.TaskType.EngineConfig) > 0 {
		_ = json.Unmarshal(tc.TaskType.EngineConfig, &chunkCfg)
	}

	ectx := engines.EngineContext{
		TaskTypeName:    tc.TaskType.Name,
		TaskTypeID:      tc.TaskType.ID,
		RepoName:        tc.Repo.Name,
		RepoPath:        tc.RepoRoot,
		DiffBase:        tc.DiffBase,
		SinceDays:       tc.SinceDays,
		Mode:            tc.TaskType.EngineMode,
		Config:          chunkCfg,
		Debate:          r.debateTiers(),
		FastPass:        r.Cfg.Debate.FastPassEnabled,
		Invokers:        disp,
		DefaultInvoker:  defInvoker,
		AnalysisPrompt:  tc.TaskType.AnalysisPrompt,
		SynthesisPrompt: tc.TaskType.SynthesisPrompt,
		TimeoutSeconds:  tc.TaskType.TimeoutSeconds,
		CancelCh:        tc.CancelCh,
		ResumeChunkIDs:  tc.ResumeChunkIDs,
	}
	return registry.NewEngine(tc.TaskType.EngineMode), ectx, nil
}

func (r *Runner) debateTiers() engines.DebateTiers {
	cfg := r.Cfg.Debate
	get := func(key string) engines.DebateTier {
		if t, ok := cfg.Tiers[key]; ok {
			return engines.DebateTier{Resource: t.Resource, TimeoutSeconds: t.TimeoutSeconds}
		}
		return engines.DebateTier{Resource: r.Cfg.LLM.DefaultResource}
	}
	return engines.DebateTiers{
		Tier1Hunter:     get("tier1_hunter"),
		Tier2Challenger: get("tier2_challenger"),
		Tier3Judge:      get("tier3_judge"),
		Tier4Synthesis:  get("tier4_synthesis"),
	}
}

func (r *Runner) defectsEngine() *defects.DiffEngine {
	return defects.NewDiffEngine(
		r.Cfg.Governance.Fingerprint.SimilarityThreshold,
		r.Cfg.Governance.Lifecycle.ScopeGuardEnabled,
		r.Cfg.Governance.Lifecycle.AutoResolveMissing,
	)
}
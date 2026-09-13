package runner

import (
	"sync"
	"time"

	"gorm.io/gorm"

	"code-shield/models"
	"code-shield/services/defects"
	"code-shield/services/engines"
)

// 流水线阶段
const (
	StageGitSync      = "git_sync"
	StagePrecondition = "precondition"
	StageAnalysis     = "analysis"
	StageSynthesis    = "synthesis"
	StagePostprocess  = "postprocess"
	StageFinalize     = "finalize"
)

// StageStatus 阶段状态
const (
	StageOK      = "success"
	StageError   = "error"
	StageSkipped = "skipped"
)

// StageLog 阶段日志（写入metrics.stages供前端Stepper展示）
type StageLog struct {
	Stage      string `json:"stage"`
	Status     string `json:"status"`
	StartAt    time.Time `json:"start_at"`
	EndAt      time.Time `json:"end_at"`
	DurationMs int64  `json:"duration_ms"`
	Log        string `json:"log"`
	Error      string `json:"error,omitempty"`
}

// TaskContext 流水线全生命周期内存上下文
type TaskContext struct {
	Report   *models.TaskReport
	TaskType *models.TaskType
	Repo     *models.Repository

	DB     *gorm.DB
	Runner *Runner

	CancelCh  <-chan struct{}
	StartedAt time.Time
	StageLogs []StageLog

	// STAGE1 产物
	RepoRoot    string
	CloneStatus string
	CloneMsg    string

	// STAGE2 产物
	SkipReason  string
	SinceDays   int
	DiffBase    string

	// STAGE3 产物
	EngineResult *engines.EngineResult

	// STAGE4 产物
	DiffOutput    *defects.DiffOutput
	DiffSummary   string
	NewFindings   []models.AnalysisFinding
	OldFindings   []models.AnalysisFinding // 历史缺陷（来自上一次成功报告）
	AISummary     string

	// STAGE5 产物
	Score          int
	SuppressedN    int
	CalibratedN    int
	CampaignGroups []map[string]any

	// 断点续跑
	ResumeChunkIDs []string

	mu sync.Mutex
}

// addStage 记录阶段日志
func (tc *TaskContext) addStage(stage string, status string, start time.Time, logMsg, errMsg string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.StageLogs = append(tc.StageLogs, StageLog{
		Stage:      stage,
		Status:     status,
		StartAt:    start,
		EndAt:      time.Now(),
		DurationMs: time.Since(start).Milliseconds(),
		Log:        logMsg,
		Error:      errMsg,
	})
}

// stageLogsJSON metrics序列化用
func (tc *TaskContext) stageLogsJSON() []map[string]any {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	out := make([]map[string]any, 0, len(tc.StageLogs))
	for _, s := range tc.StageLogs {
		out = append(out, map[string]any{
			"stage":       s.Stage,
			"status":      s.Status,
			"start_at":    s.StartAt,
			"end_at":      s.EndAt,
			"duration_ms": s.DurationMs,
			"log":         s.Log,
			"error":       s.Error,
		})
	}
	return out
}

// isCancelled 检查取消信号
func isCancelled(ch <-chan struct{}) bool {
	if ch == nil {
		return false
	}
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
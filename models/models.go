package models

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Repository 代码仓库
type Repository struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:200;uniqueIndex" json:"name"`
	URL       string    `gorm:"size:500" json:"url"`
	Branch    string    `gorm:"size:100" json:"branch"`
	LocalPath string    `gorm:"size:500" json:"local_path"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// User 平台用户（JWT鉴权）
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:100;uniqueIndex" json:"username"`
	PasswordHash string    `gorm:"size:200" json:"-"`
	Role         string    `gorm:"size:20;default:user" json:"role"` // admin / user
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TaskType 任务类型插件（启动时扫描 tasks/ 目录同步）
type TaskType struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	Name            string         `gorm:"size:100;uniqueIndex" json:"name"`
	DisplayName     string         `gorm:"size:200" json:"display_name"`
	Description     string         `json:"description"`
	EngineMode      string         `gorm:"size:50" json:"engine_mode"`
	EngineConfig    datatypes.JSON `json:"engine_config"`
	AIBackend       string         `gorm:"size:50" json:"ai_backend"`
	IsActive        bool           `gorm:"default:true" json:"is_active"`
	IsCampaign      bool           `gorm:"default:false" json:"is_campaign"`
	GovernanceMode  string         `gorm:"size:50" json:"governance_mode"`
	NotifyThreshold int            `json:"notify_threshold"`
	TimeoutSeconds  int            `json:"timeout_seconds"`
	TargetScope     string         `gorm:"size:50" json:"target_scope"`
	AnalysisPrompt  string         `json:"analysis_prompt"`
	SynthesisPrompt string         `json:"synthesis_prompt"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// TaskReport 任务报告（流水线产物）
type TaskReport struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	RepoID          uint           `gorm:"index" json:"repo_id"`
	TaskTypeID      uint           `gorm:"index" json:"task_type_id"`
	Status          string         `gorm:"size:50;index" json:"status"`
	ReportPath      string         `json:"report_path"`
	AISummary       string         `json:"ai_summary"`
	Score           int            `json:"score"`
	Metrics         datatypes.JSON `json:"metrics"`
	CloneStatus     string         `gorm:"size:20" json:"clone_status"`
	CloneMessage    string         `json:"clone_message"`
	TotalChunks     int            `gorm:"default:0" json:"total_chunks"`
	ProcessedChunks int            `gorm:"default:0" json:"processed_chunks"`
	SuccessChunks   int            `gorm:"default:0" json:"success_chunks"`
	FailedChunks    int            `gorm:"default:0" json:"failed_chunks"`
	HasFailedChunks bool           `gorm:"default:false" json:"has_failed_chunks"`
	ErrorMessage    string         `json:"error_message"`
	StartedAt       *time.Time     `json:"started_at"`
	FinishedAt      *time.Time     `json:"finished_at"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`

	Repo     *Repository      `gorm:"foreignKey:RepoID" json:"repo,omitempty"`
	TaskType *TaskType        `gorm:"foreignKey:TaskTypeID" json:"task_type,omitempty"`
	Findings []AnalysisFinding `gorm:"foreignKey:TaskReportID" json:"findings,omitempty"`
}

// AnalysisFinding 缺陷问题（DiffEngine四级漏斗输出diff_status）
type AnalysisFinding struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TaskReportID  uint      `gorm:"index" json:"task_report_id"`
	TaskTypeID    uint      `gorm:"index" json:"task_type_id"`
	RepoID        uint      `gorm:"index" json:"repo_id"`
	Severity      string    `gorm:"size:20" json:"severity"`
	Category      string    `gorm:"size:100" json:"category"`
	FilePath      string    `json:"file_path"`
	LineNumber    string    `json:"line_number"`
	CodeSnippet   string    `json:"code_snippet"`
	Title         string    `gorm:"size:500" json:"title"`
	Detail        string    `json:"detail"`
	Suggestion    string    `json:"suggestion"`
	Status        string    `gorm:"size:20;default:open" json:"status"`
	DiffStatus    string    `gorm:"size:20" json:"diff_status"`
	L1Fingerprint string    `gorm:"size:128;index" json:"l1_fingerprint"`
	L2Fingerprint string    `gorm:"size:128;index" json:"l2_fingerprint"`
	Confidence    float64   `json:"confidence"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	TaskReport *TaskReport `gorm:"foreignKey:TaskReportID" json:"task_report,omitempty"`
}

// QueueTask 持久化队列任务
type QueueTask struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	TaskReportID uint       `gorm:"index" json:"task_report_id"`
	Status       string     `gorm:"size:20;index" json:"status"`
	Priority     int        `json:"priority"`
	Attempts     int        `gorm:"default:0" json:"attempts"`
	MaxAttempts  int        `gorm:"default:3" json:"max_attempts"`
	NextRetryAt  *time.Time `json:"next_retry_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`

	TaskReport *TaskReport `gorm:"foreignKey:TaskReportID" json:"task_report,omitempty"`
}

// BeforeCreate 默认值兜底
func (q *QueueTask) BeforeCreate(tx *gorm.DB) error {
	if q.Status == "" {
		q.Status = QueuePending
	}
	if q.MaxAttempts == 0 {
		q.MaxAttempts = 3
	}
	return nil
}
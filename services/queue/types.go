package queue

// 队列内部任务结构定义

// JobStatus 任务状态
const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// JobMeta 队列任务元数据（持久化到queue_tasks表）
type JobMeta struct {
	ID           uint
	TaskReportID uint
	Status       string
	Priority     int
	Attempts     int
	MaxAttempts  int
}

// ToJob 转换为Worker池任务
func (m JobMeta) ToJob() Job {
	return Job{
		QueueTaskID:  m.ID,
		TaskReportID: m.TaskReportID,
		Priority:     m.Priority,
		Attempt:      m.Attempts,
	}
}
// Package queue 持久化队列域：Worker池、任务消费、重试与限流。
// 队列持久化在 queue_tasks 表，Worker池消费并调用runner流水线。
package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// ErrQueueFull 队列已满
var ErrQueueFull = errors.New("队列已满")

// Job 队列任务
type Job struct {
	QueueTaskID  uint `json:"queue_task_id"`
	TaskReportID uint `json:"task_report_id"`
	Priority     int  `json:"priority"`
	Attempt      int  `json:"attempt"`
}

// Stats Worker池状态
type Stats struct {
	WorkerCount  int `json:"worker_count"`
	BusyWorkers  int `json:"busy_workers"`
	QueueCapacity int `json:"queue_capacity"`
	Queued       int `json:"queued"`
	Processed    int64 `json:"processed"`
	Failed       int64 `json:"failed"`
	Retried      int64 `json:"retried"`
}

// WorkerPool Worker池
type WorkerPool struct {
	ctx      context.Context
	cancel   context.CancelFunc
	jobs     chan Job
	workers  int
	capacity int
	handler  func(ctx context.Context, job Job) error

	busy      atomic.Int64
	processed atomic.Int64
	failed    atomic.Int64
	retried   atomic.Int64
	wg        sync.WaitGroup
}

// NewWorkerPool 创建Worker池；handler返回error则任务判定失败（由调用方决定重试）
func NewWorkerPool(workerCount, capacity int, handler func(ctx context.Context, job Job) error) *WorkerPool {
	if workerCount <= 0 {
		workerCount = 1
	}
	if capacity <= 0 {
		capacity = 100
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		ctx:      ctx,
		cancel:   cancel,
		jobs:     make(chan Job, capacity),
		workers:  workerCount,
		capacity: capacity,
		handler:  handler,
	}
}

// SetHandler 设置任务处理器（Start前调用）
func (p *WorkerPool) SetHandler(h func(ctx context.Context, job Job) error) {
	p.handler = h
}

// Start 启动Worker
func (p *WorkerPool) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.workerLoop()
	}
}

// Stop 优雅停止：不再消费新任务
func (p *WorkerPool) Stop() {
	p.cancel()
	p.wg.Wait()
}

// Enqueue 入队（非阻塞，满则返回ErrQueueFull）
func (p *WorkerPool) Enqueue(job Job) error {
	select {
	case p.jobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

// Stats 池状态快照
func (p *WorkerPool) Stats() Stats {
	return Stats{
		WorkerCount:   p.workers,
		BusyWorkers:   int(p.busy.Load()),
		QueueCapacity: p.capacity,
		Queued:        len(p.jobs),
		Processed:     p.processed.Load(),
		Failed:        p.failed.Load(),
		Retried:       p.retried.Load(),
	}
}

func (p *WorkerPool) workerLoop() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case job := <-p.jobs:
			p.busy.Add(1)
			err := p.handler(p.ctx, job)
			p.busy.Add(-1)
			if err != nil {
				p.failed.Add(1)
			} else {
				p.processed.Add(1)
			}
		}
	}
}

// RetryPolicy 重试策略
type RetryPolicy struct {
	MaxAttempts   int
	BackoffBaseMs int
}

// ShouldRetry 是否应重试
func (r RetryPolicy) ShouldRetry(attempt int) bool {
	return attempt < r.MaxAttempts
}

// NextRetryAt 计算下次重试时间（指数退避）
func (r RetryPolicy) NextRetryAt(attempt int) time.Time {
	backoff := r.BackoffBaseMs
	if backoff <= 0 {
		backoff = 2000
	}
	exp := backoff << (attempt - 1) // 2, 4, 8...倍
	if exp <= 0 || exp > 3600000 {
		exp = 3600000 // 上限1小时
	}
	return time.Now().Add(time.Duration(exp) * time.Millisecond)
}
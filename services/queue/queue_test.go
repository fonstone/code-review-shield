package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPoolProcesses(t *testing.T) {
	var processed atomic.Int64
	pool := NewWorkerPool(3, 100, func(ctx context.Context, job Job) error {
		processed.Add(1)
		return nil
	})
	pool.Start()
	defer pool.Stop()

	for i := 0; i < 20; i++ {
		if err := pool.Enqueue(Job{QueueTaskID: uint(i + 1)}); err != nil {
			t.Fatalf("入队失败: %v", err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for processed.Load() < 20 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if processed.Load() != 20 {
		t.Errorf("应处理20个任务, got %d", processed.Load())
	}
}

func TestWorkerPoolErrorCounts(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	pool := NewWorkerPool(2, 10, func(ctx context.Context, job Job) error {
		wg.Done()
		return errors.New("boom")
	})
	pool.Start()
	defer pool.Stop()
	if err := pool.Enqueue(Job{}); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	deadline := time.Now().Add(2 * time.Second)
	for pool.Stats().Failed == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if pool.Stats().Failed != 1 {
		t.Errorf("失败任务应计数, got %d", pool.Stats().Failed)
	}
}

func TestWorkerPoolQueueFull(t *testing.T) {
	pool := NewWorkerPool(1, 2, func(ctx context.Context, job Job) error { return nil })
	// 不启动worker，直接灌满容量
	pool.Enqueue(Job{})
	pool.Enqueue(Job{})
	if err := pool.Enqueue(Job{}); err != ErrQueueFull {
		t.Errorf("队列满应返回ErrQueueFull, got %v", err)
	}
}

func TestRetryPolicy(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 3, BackoffBaseMs: 10}
	if !p.ShouldRetry(1) || !p.ShouldRetry(2) || p.ShouldRetry(3) {
		t.Error("重试边界错误")
	}
	at := p.NextRetryAt(1)
	if !at.After(time.Now()) {
		t.Error("下次重试时间应在未来")
	}
}

func TestWorkerPoolStop(t *testing.T) {
	pool := NewWorkerPool(2, 10, func(ctx context.Context, job Job) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	})
	pool.Start()
	for i := 0; i < 5; i++ {
		pool.Enqueue(Job{})
	}
	pool.Stop()
	// Stop后入队不应panic
	_ = pool.Enqueue(Job{})
}
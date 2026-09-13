// Package dispatcher 算力调度域：SWRR平滑加权轮询 + LLM槽位租约管理。
package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// ErrNoSlot 无可用槽位
var ErrNoSlot = errors.New("所有LLM资源槽位已满")

// ErrResourceNotFound 资源不存在
var ErrResourceNotFound = errors.New("LLM资源不存在")

// ModelResource 单个LLM算力资源（运行时状态）
type ModelResource struct {
	ID          string `json:"id"`
	Driver      string `json:"driver"`
	Model       string `json:"model"`
	Concurrent  int    `json:"concurrent"` // 最大并发槽位
	ActiveSlots int    `json:"active_slots"`

	TotalCalls   int64   `json:"total_calls"`
	FailedCalls  int64   `json:"failed_calls"`
	TotalTokens  int64   `json:"total_tokens"`
	TotalMs      int64   `json:"total_ms"`

	// weight SWRR权重
	weight  int
	current int

	mu sync.Mutex
}

// LoadRatio 负载比例（0-1）
func (r *ModelResource) LoadRatio() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Concurrent <= 0 {
		return 0
	}
	return float64(r.ActiveSlots) / float64(r.Concurrent)
}

// AvgDurationMs 平均耗时
func (r *ModelResource) AvgDurationMs() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.TotalCalls == 0 {
		return 0
	}
	return float64(r.TotalMs) / float64(r.TotalCalls)
}

// ModelDispatcher SWRR调度器 + 租约存储
type ModelDispatcher struct {
	mu        sync.Mutex
	resources map[string]*ModelResource
	order     []string // 稳定顺序（SWRR选择）
	leases    map[string]*LLMSlotLease
	leaseTTL  time.Duration
}

// NewModelDispatcher 创建调度器；resources: 资源ID→并发数
func NewModelDispatcher(leaseTTL time.Duration) *ModelDispatcher {
	if leaseTTL <= 0 {
		leaseTTL = 30 * time.Minute
	}
	return &ModelDispatcher{
		resources: map[string]*ModelResource{},
		leases:    map[string]*LLMSlotLease{},
		leaseTTL:  leaseTTL,
	}
}

// AddResource 注册资源
func (d *ModelDispatcher) AddResource(id, driver, model string, concurrent, weight int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if weight <= 0 {
		weight = 1
	}
	if concurrent <= 0 {
		concurrent = 1
	}
	if _, ok := d.resources[id]; ok {
		return
	}
	d.resources[id] = &ModelResource{ID: id, Driver: driver, Model: model, Concurrent: concurrent, weight: weight}
	d.order = append(d.order, id)
	sort.Strings(d.order)
}

// Resources 全部资源快照（按ID排序，确定性）
func (d *ModelDispatcher) Resources() []*ModelResource {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]*ModelResource, 0, len(d.order))
	for _, id := range d.order {
		out = append(out, d.resources[id])
	}
	return out
}

// Resource 按ID取资源
func (d *ModelDispatcher) Resource(id string) *ModelResource {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.resources[id]
}

// Select SWRR平滑加权轮询选择资源（仅在有空闲槽位时返回）
func (d *ModelDispatcher) Select() (*ModelResource, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.order) == 0 {
		return nil, ErrResourceNotFound
	}
	total := 0
	var best *ModelResource
	for _, id := range d.order {
		r := d.resources[id]
		r.mu.Lock()
		r.current += r.weight
		total += r.weight
		avail := r.ActiveSlots < r.Concurrent
		r.mu.Unlock()
		if !avail {
			continue
		}
		if best == nil || r.current > best.current {
			best = r
		}
	}
	if best == nil {
		return nil, ErrNoSlot
	}
	best.mu.Lock()
	best.current -= total
	best.mu.Unlock()
	return best, nil
}

// Acquire 申请槽位：SWRR选择资源并创建租约；阻塞等待直到有空闲槽位或ctx取消
func (d *ModelDispatcher) Acquire(ctx context.Context, resourceID string, taskReportID uint) (*LLMSlotLease, error) {
	for {
		d.mu.Lock()
		var target *ModelResource
		if resourceID != "" {
			target = d.resources[resourceID]
			if target == nil {
				d.mu.Unlock()
				return nil, ErrResourceNotFound
			}
		} else {
			var err error
			target, err = d.selectLocked()
			if err != nil && !errors.Is(err, ErrNoSlot) {
				d.mu.Unlock()
				return nil, err
			}
			if err != nil {
				target = nil
			}
		}
		if target != nil && target.tryAcquire() {
			lease := NewLease(target, taskReportID, d.leaseTTL)
			d.leases[lease.ID] = lease
			d.mu.Unlock()
			return lease, nil
		}
		d.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("等待槽位超时或被取消: %w", ctx.Err())
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// TryAcquire 非阻塞申请（测试与健康检查用）
func (d *ModelDispatcher) TryAcquire(resourceID string, taskReportID uint) (*LLMSlotLease, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var target *ModelResource
	if resourceID != "" {
		target = d.resources[resourceID]
		if target == nil {
			return nil, ErrResourceNotFound
		}
	} else {
		var err error
		target, err = d.selectLocked()
		if err != nil {
			return nil, err
		}
	}
	if !target.tryAcquire() {
		return nil, ErrNoSlot
	}
	lease := NewLease(target, taskReportID, d.leaseTTL)
	d.leases[lease.ID] = lease
	return lease, nil
}

// Release 释放租约
func (d *ModelDispatcher) Release(leaseID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	lease, ok := d.leases[leaseID]
	if !ok || lease.Status != LeaseActive {
		return
	}
	lease.release()
	if r := d.resources[lease.ResourceID]; r != nil {
		r.releaseSlot()
	}
}

// RecordCall 记录调用统计
func (d *ModelDispatcher) RecordCall(resourceID string, failed bool, tokens int, ms int64) {
	r := d.Resource(resourceID)
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.TotalCalls++
	r.TotalMs += ms
	r.TotalTokens += int64(tokens)
	if failed {
		r.FailedCalls++
	}
}

// selectLocked 已持锁的SWRR选择
func (d *ModelDispatcher) selectLocked() (*ModelResource, error) {
	if len(d.order) == 0 {
		return nil, ErrResourceNotFound
	}
	total := 0
	var best *ModelResource
	for _, id := range d.order {
		r := d.resources[id]
		r.mu.Lock()
		r.current += r.weight
		total += r.weight
		avail := r.ActiveSlots < r.Concurrent
		r.mu.Unlock()
		if !avail {
			continue
		}
		if best == nil || r.current > best.current {
			best = r
		}
	}
	if best == nil {
		return nil, ErrNoSlot
	}
	best.mu.Lock()
	best.current -= total
	best.mu.Unlock()
	return best, nil
}

// tryAcquire 槽位+1（需已持dispatcher锁）
func (r *ModelResource) tryAcquire() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ActiveSlots >= r.Concurrent {
		return false
	}
	r.ActiveSlots++
	return true
}

// releaseSlot 槽位-1（需已持dispatcher锁）
func (r *ModelResource) releaseSlot() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.ActiveSlots > 0 {
		r.ActiveSlots--
	}
}
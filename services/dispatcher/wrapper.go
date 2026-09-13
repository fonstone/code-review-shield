package dispatcher

import (
	"context"

	"code-shield/services/invoker"
)

// DispatchingInvoker 调度装饰器：自动申请/释放槽位，内置调用统计。
// 包装任意AIInvoker，调用前Acquire槽位租约，调用后Release并记录统计。
type DispatchingInvoker struct {
	dispatcher   *ModelDispatcher
	inner        invoker.AIInvoker
	resourceID   string // 固定资源ID；为空则SWRR自动选择
	taskReportID uint
}

// NewDispatchingInvoker 创建调度装饰器
func NewDispatchingInvoker(d *ModelDispatcher, inner invoker.AIInvoker, resourceID string, taskReportID uint) *DispatchingInvoker {
	return &DispatchingInvoker{dispatcher: d, inner: inner, resourceID: resourceID, taskReportID: taskReportID}
}

// Name 驱动名
func (d *DispatchingInvoker) Name() string { return d.inner.Name() }

// Model 模型名
func (d *DispatchingInvoker) Model() string { return d.inner.Model() }

// ResourceID 当前绑定资源
func (d *DispatchingInvoker) ResourceID() string { return d.resourceID }

// Inner 内部调用器
func (d *DispatchingInvoker) Inner() invoker.AIInvoker { return d.inner }

// Invoke 带槽位租约的调用
func (d *DispatchingInvoker) Invoke(ctx context.Context, req invoker.AIRequest) (*invoker.AIResponse, error) {
	lease, err := d.dispatcher.Acquire(ctx, d.resourceID, d.taskReportID)
	if err != nil {
		return nil, err
	}
	resourceID := lease.ResourceID
	defer d.dispatcher.Release(lease.ID)

	resp, err := d.inner.Invoke(ctx, req)
	if err != nil {
		d.dispatcher.RecordCall(resourceID, true, 0, 0)
		return nil, err
	}
	d.dispatcher.RecordCall(resourceID, false, resp.Usage.TotalTokens, resp.DurationMs)
	return resp, nil
}

// DispatchingFactory 批量创建：为每个资源生成调度装饰器映射（resourceID → DispatchingInvoker）
type DispatchingFactory struct {
	dispatcher *ModelDispatcher
	taskReportID uint
}

// NewDispatchingFactory 创建批量装饰器工厂
func NewDispatchingFactory(d *ModelDispatcher, taskReportID uint) *DispatchingFactory {
	return &DispatchingFactory{dispatcher: d, taskReportID: taskReportID}
}

// Wrap 包装单个调用器（绑定固定资源）
func (f *DispatchingFactory) Wrap(inner invoker.AIInvoker, resourceID string) *DispatchingInvoker {
	return NewDispatchingInvoker(f.dispatcher, inner, resourceID, f.taskReportID)
}

// WrapAll 为全部注册资源生成装饰器映射
func (f *DispatchingFactory) WrapAll(inners map[string]invoker.AIInvoker) map[string]invoker.AIInvoker {
	out := make(map[string]invoker.AIInvoker, len(inners))
	for id, inv := range inners {
		out[id] = NewDispatchingInvoker(f.dispatcher, inv, id, f.taskReportID)
	}
	return out
}
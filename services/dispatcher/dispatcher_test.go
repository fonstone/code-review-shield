package dispatcher

import (
	"context"
	"testing"
	"time"
)

func TestSWRRBalancedSelection(t *testing.T) {
	d := NewModelDispatcher(time.Minute)
	d.AddResource("a", "fake", "m1", 10, 3)
	d.AddResource("b", "fake", "m2", 10, 1)

	counts := map[string]int{}
	for i := 0; i < 400; i++ {
		lease, err := d.TryAcquire("", 1)
		if err != nil {
			t.Fatalf("TryAcquire失败: %v", err)
		}
		counts[lease.ResourceID]++
		d.Release(lease.ID)
	}
	// 权重3:1 → 约300:100
	if counts["a"] < 250 || counts["a"] > 350 {
		t.Errorf("SWRR分布异常: %v", counts)
	}
	if counts["b"] < 50 || counts["b"] > 150 {
		t.Errorf("SWRR分布异常: %v", counts)
	}
}

func TestAcquireSlotCap(t *testing.T) {
	d := NewModelDispatcher(time.Minute)
	d.AddResource("a", "fake", "m1", 2, 1)

	l1, err := d.TryAcquire("a", 1)
	if err != nil {
		t.Fatalf("第一次获取失败: %v", err)
	}
	l2, err := d.TryAcquire("a", 1)
	if err != nil {
		t.Fatalf("第二次获取失败: %v", err)
	}
	if _, err := d.TryAcquire("a", 1); err != ErrNoSlot {
		t.Fatalf("并发上限应返回ErrNoSlot, got %v", err)
	}
	// 释放一个后可再获取
	d.Release(l1.ID)
	l3, err := d.TryAcquire("a", 1)
	if err != nil {
		t.Fatalf("释放后应可获取: %v", err)
	}
	d.Release(l2.ID)
	d.Release(l3.ID)

	res := d.Resource("a")
	if res.ActiveSlots != 0 {
		t.Errorf("全部释放后槽位应归零, got %d", res.ActiveSlots)
	}
}

func TestAcquireWithTimeout(t *testing.T) {
	d := NewModelDispatcher(time.Minute)
	d.AddResource("a", "fake", "m1", 1, 1)
	if _, err := d.TryAcquire("a", 1); err != nil {
		t.Fatalf("占用失败: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := d.Acquire(ctx, "a", 1)
	if err == nil {
		t.Fatal("槽位被占时应等待超时返回错误")
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Errorf("应等待一段时间后超时")
	}
}

func TestLeaseExpiryAndReset(t *testing.T) {
	d := NewModelDispatcher(100 * time.Millisecond)
	d.AddResource("a", "fake", "m1", 5, 1)
	l1, _ := d.TryAcquire("a", 1)
	l2, _ := d.TryAcquire("a", 2)
	d.Release(l1.ID)

	time.Sleep(150 * time.Millisecond)
	hung := d.HealthCheck()
	if hung < 1 {
		t.Errorf("过期租约应被标记为悬挂, hung=%d", hung)
	}
	res := d.ResetActiveSlots()
	if res.ReleasedLeases < 1 {
		t.Errorf("校准应释放悬挂租约, released=%d", res.ReleasedLeases)
	}
	if d.Resource("a").ActiveSlots != 0 {
		t.Errorf("校准后活跃槽位应为0, got %d", d.Resource("a").ActiveSlots)
	}
	// 释放的l2租约状态
	lease := d.Leases()
	found := false
	for _, l := range lease {
		if l.ID == l2.ID {
			found = true
			if l.Status != LeaseReleased {
				t.Errorf("过期租约应被强制释放, got %s", l.Status)
			}
		}
	}
	if !found {
		t.Error("租约应保留在存储中")
	}
}

func TestLeaseStoreMarkHung(t *testing.T) {
	s := NewLeaseStore()
	res := &ModelResource{ID: "a", Driver: "fake", Model: "m", Concurrent: 2}
	lease := NewLease(res, 1, time.Minute)
	s.Put(lease)
	if len(s.All()) != 1 {
		t.Fatal("租约应存入")
	}
	// 未过期不标记
	hung := s.MarkHung()
	if len(hung) != 0 {
		t.Fatalf("未过期租约不应标记悬挂")
	}
	// 手动过期
	lease.ExpiresAt = time.Now().Add(-time.Second)
	hung = s.MarkHung()
	if len(hung) != 1 || !lease.Hung {
		t.Fatalf("过期租约应标记悬挂")
	}
}
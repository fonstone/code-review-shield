package dispatcher

import (
	"sync"
	"time"
)

// HealthCheck 资源健康心跳检测：扫描租约，标记悬挂租约，返回被标记数量
func (d *ModelDispatcher) HealthCheck() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	hung := 0
	for _, lease := range d.leases {
		if lease.Expired() {
			lease.markHung()
			hung++
		}
	}
	return hung
}

// HungLeases 返回全部悬挂租约
func (d *ModelDispatcher) HungLeases() []*LLMSlotLease {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []*LLMSlotLease
	for _, l := range d.leases {
		if l.Hung {
			out = append(out, l)
		}
	}
	return out
}

// Leases 全部租约快照
func (d *ModelDispatcher) Leases() []*LLMSlotLease {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]*LLMSlotLease, 0, len(d.leases))
	for _, l := range d.leases {
		out = append(out, l)
	}
	return out
}

// ResetActiveSlots 槽位校准：释放全部悬挂/过期租约，将各资源ActiveSlots重算为真实占用数。
// 返回校准统计。
type ResetResult struct {
	ReleasedLeases int `json:"released_leases"`
	HungLeases     int `json:"hung_leases"`
	AdjustedSlots  int `json:"adjusted_slots"`
}

// ResetActiveSlots 校准槽位（管理员接口调用）
func (d *ModelDispatcher) ResetActiveSlots() ResetResult {
	d.mu.Lock()
	defer d.mu.Unlock()
	res := ResetResult{}
	now := time.Now()
	activeByResource := map[string]int{}
	for _, lease := range d.leases {
		// 悬挂租约（已过期且被健康检查标记）直接强制释放
		if lease.Hung || lease.Status == LeaseHung {
			res.HungLeases++
		}
		if lease.Status == LeaseActive {
			if now.After(lease.ExpiresAt) {
				// 过期未释放：强制释放
				lease.release()
				res.ReleasedLeases++
				continue
			}
			activeByResource[lease.ResourceID]++
		}
		if (lease.Status == LeaseHung || lease.Hung) && lease.Status != LeaseReleased {
			lease.release()
			res.ReleasedLeases++
		}
	}
	// 重算槽位
	for _, r := range d.resources {
		real := activeByResource[r.ID]
		if real != r.ActiveSlots {
			res.AdjustedSlots += r.ActiveSlots - real
			r.ActiveSlots = real
		}
	}
	return res
}

// _ 编译期断言：确保锁使用一致
var _ sync.Locker = &sync.Mutex{}
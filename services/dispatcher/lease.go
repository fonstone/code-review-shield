package dispatcher

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// 租约状态
const (
	LeaseActive   = "active"   // 占用中
	LeaseReleased = "released" // 已释放
	LeaseExpired  = "expired"  // 到期未释放
	LeaseHung     = "hung"     // 悬挂（过期且被健康检查标记）
)

// LLMSlotLease LLM槽位租约
type LLMSlotLease struct {
	ID           string     `json:"id"`
	ResourceID   string     `json:"resource_id"`
	Driver       string     `json:"driver"`
	Model        string     `json:"model"`
	Status       string     `json:"status"`
	TaskReportID uint       `json:"task_report_id"`
	AcquiredAt   time.Time  `json:"acquired_at"`
	ReleasedAt   *time.Time `json:"released_at,omitempty"`
	ExpiresAt    time.Time  `json:"expires_at"`
	Hung         bool       `json:"hung"` // 悬挂标记

	mu sync.Mutex `json:"-"`
}

// NewLease 创建租约
func NewLease(res *ModelResource, taskReportID uint, ttl time.Duration) *LLMSlotLease {
	now := time.Now()
	return &LLMSlotLease{
		ID:           "lease-" + uuid.NewString(),
		ResourceID:   res.ID,
		Driver:       res.Driver,
		Model:        res.Model,
		Status:       LeaseActive,
		TaskReportID: taskReportID,
		AcquiredAt:   now,
		ExpiresAt:    now.Add(ttl),
	}
}

// Expired 是否已过期
func (l *LLMSlotLease) Expired() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Status == LeaseActive && time.Now().After(l.ExpiresAt)
}

func (l *LLMSlotLease) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	l.Status = LeaseReleased
	l.ReleasedAt = &now
	l.Hung = false
}

func (l *LLMSlotLease) markHung() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.Status == LeaseActive {
		l.Status = LeaseHung
		l.Hung = true
	}
}

// LeaseStore 租约存储与状态查询
type LeaseStore struct {
	mu     sync.Mutex
	leases map[string]*LLMSlotLease
}

// NewLeaseStore 创建租约存储
func NewLeaseStore() *LeaseStore {
	return &LeaseStore{leases: map[string]*LLMSlotLease{}}
}

// Put 存储租约
func (s *LeaseStore) Put(l *LLMSlotLease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leases[l.ID] = l
}

// Get 查询租约
func (s *LeaseStore) Get(id string) *LLMSlotLease {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leases[id]
}

// All 全部租约快照（按创建时间排序）
func (s *LeaseStore) All() []*LLMSlotLease {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*LLMSlotLease, 0, len(s.leases))
	for _, l := range s.leases {
		out = append(out, l)
	}
	return out
}

// MarkHung 将过期未释放租约标记为悬挂
func (s *LeaseStore) MarkHung() []*LLMSlotLease {
	s.mu.Lock()
	defer s.mu.Unlock()
	var hung []*LLMSlotLease
	for _, l := range s.leases {
		if l.Expired() {
			l.markHung()
			hung = append(hung, l)
		}
	}
	return hung
}
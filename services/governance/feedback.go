package governance

import (
	"sync"
	"time"
)

// FeedbackEntry 误报记忆条目
type FeedbackEntry struct {
	L1Fingerprint string    `json:"l1_fingerprint"`
	L2Fingerprint string    `json:"l2_fingerprint"`
	Reason        string    `json:"reason"`
	ReportedBy    string    `json:"reported_by"`
	CreatedAt     time.Time `json:"created_at"`
}

// FeedbackStore 误报记忆库：抑制重复误报（内存实现，进程级持久）
type FeedbackStore struct {
	mu       sync.RWMutex
	byL1     map[string]*FeedbackEntry
	byL2     map[string]*FeedbackEntry
}

// NewFeedbackStore 创建误报记忆库
func NewFeedbackStore() *FeedbackStore {
	return &FeedbackStore{
		byL1: map[string]*FeedbackEntry{},
		byL2: map[string]*FeedbackEntry{},
	}
}

// Suppress 记录一条误报
func (s *FeedbackStore) Suppress(l1, l2, reason, reportedBy string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := &FeedbackEntry{L1Fingerprint: l1, L2Fingerprint: l2, Reason: reason, ReportedBy: reportedBy, CreatedAt: time.Now()}
	if l1 != "" {
		s.byL1[l1] = e
	}
	if l2 != "" {
		s.byL2[l2] = e
	}
}

// IsSuppressed 判断缺陷是否为已知误报（L1或L2任一命中）
func (s *FeedbackStore) IsSuppressed(l1, l2 string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if l1 != "" {
		if _, ok := s.byL1[l1]; ok {
			return true
		}
	}
	if l2 != "" {
		if _, ok := s.byL2[l2]; ok {
			return true
		}
	}
	return false
}

// Reason 返回抑制原因
func (s *FeedbackStore) Reason(l1, l2 string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if e, ok := s.byL1[l1]; ok && l1 != "" {
		return e.Reason
	}
	if e, ok := s.byL2[l2]; ok && l2 != "" {
		return e.Reason
	}
	return ""
}

// List 全部误报条目
func (s *FeedbackStore) List() []*FeedbackEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*FeedbackEntry, 0, len(s.byL1))
	for _, e := range s.byL1 {
		out = append(out, e)
	}
	return out
}

// Remove 移除误报记忆（误报申诉）
func (s *FeedbackStore) Remove(l1, l2 string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l1 != "" {
		delete(s.byL1, l1)
	}
	if l2 != "" {
		delete(s.byL2, l2)
	}
}

// FilterSuppressed 过滤掉已知误报，返回保留项与抑制数
func (s *FeedbackStore) FilterSuppressed(l1s, l2s []string) (keep []bool, suppressed int) {
	keep = make([]bool, len(l1s))
	for i := range l1s {
		if s.IsSuppressed(l1s[i], l2s[i]) {
			suppressed++
			continue
		}
		keep[i] = true
	}
	return keep, suppressed
}
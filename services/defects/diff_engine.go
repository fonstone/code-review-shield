package defects

import (
	"fmt"
	"sort"
	"strings"
)

// diff状态常量（四级漏斗状态机输出）
const (
	DiffNew      = "NEW"
	DiffExisted  = "EXISTED"
	DiffResolved = "RESOLVED"
	DiffReopened = "REOPENED"
)

// FindingLike 参与增量比对的最小缺陷结构（由runner层适配到DB模型）
type FindingLike struct {
	ID            uint
	FilePath      string // 仓库内相对路径
	LineNumber    string
	Title         string
	Severity      string
	L1Fingerprint string
	L2Fingerprint string
	Status        string // open / resolved / suppressed
	DiffStatus    string // 历史diff状态
}

// DiffInput 增量比对输入
type DiffInput struct {
	RepoRoot        string        // 仓库本地根路径（Scope守卫用，可空则跳过文件存在性校验）
	NewFindings     []FindingLike // 本次扫描新缺陷
	Previous        []FindingLike // 历史缺陷（同repo+task_type上次成功报告）
	SimilarityThres float64       // 第3级漏斗相似度阈值，默认0.85
	ScopeGuard      bool          // 是否启用Scope守卫
	AutoResolve     bool          // 超出Scope(文件缺失)的旧缺陷是否自动RESOLVED
}

// DiffOutput 比对输出
type DiffOutput struct {
	// Matched: 新缺陷 → 匹配到的旧缺陷（用于状态继承）
	Matched map[uint]uint // newFindingID -> oldFindingID
	// NewStatus: 新缺陷ID → diff状态
	NewStatus map[uint]string
	// OldStatus: 旧缺陷ID → 最新diff状态（RESOLVED/REOPENED）
	OldStatus map[uint]string
	// Stats 统计
	Stats DiffStats
}

// DiffStats 四级漏斗统计
type DiffStats struct {
	TotalNew      int `json:"total_new"`      // 进入比对的全部新缺陷
	L1Matched     int `json:"l1_matched"`     // 第1级：L1强指纹命中
	L2Matched     int `json:"l2_matched"`     // 第2级：L2语义指纹命中
	SimilarityHit int `json:"similarity_hit"` // 第3级：相似度阈值命中
	AnchorHit     int `json:"anchor_hit"`     // 第4级：锚点兜底命中
	New           int `json:"new"`            // 最终NEW数
	Existed       int `json:"existed"`        // 最终EXISTED数
	Resolved      int `json:"resolved"`       // 旧缺陷RESOLVED数
	Reopened      int `json:"reopened"`       // 旧缺陷REOPENED数
}

// DiffEngine 四级漏斗增量比对引擎（确定性、无DB访问）
type DiffEngine struct {
	Threshold float64
	ScopeGuard bool
	AutoResolve bool
}

// NewDiffEngine 创建比对引擎
func NewDiffEngine(threshold float64, scopeGuard, autoResolve bool) *DiffEngine {
	if threshold <= 0 {
		threshold = 0.85
	}
	return &DiffEngine{Threshold: threshold, ScopeGuard: scopeGuard, AutoResolve: autoResolve}
}

// Compare 执行四级漏斗比对，输出NEW/EXISTED/RESOLVED/REOPENED状态机结果
//
//	漏斗四级：1) L1强指纹精确匹配 → 2) L2语义指纹匹配 → 3) 内容相似度≥阈值 → 4) 文件+锚点兜底
//	状态机：新缺陷命中旧缺陷→EXISTED；未命中→NEW；
//	旧缺陷未被命中→RESOLVED（Scope守卫确认文件仍在范围内则保留EXISTED）
//	新缺陷命中历史已RESOLVED缺陷→REOPENED（旧缺陷被"唤醒"）
func (d *DiffEngine) Compare(in DiffInput) *DiffOutput {
	thres := in.SimilarityThres
	if thres <= 0 {
		thres = d.Threshold
	}
	if thres <= 0 {
		thres = 0.85
	}
	out := &DiffOutput{
		Matched:   map[uint]uint{},
		NewStatus: map[uint]string{},
		OldStatus: map[uint]string{},
	}
	out.Stats.TotalNew = len(in.NewFindings)

	oldByL1 := indexByFingerprint(in.Previous, true)
	oldByL2 := indexByFingerprint(in.Previous, false)
	unmatchedOld := map[uint]*FindingLike{}
	for i := range in.Previous {
		f := &in.Previous[i]
		if f.Status == "suppressed" {
			continue
		}
		unmatchedOld[f.ID] = f
	}

	for i := range in.NewFindings {
		nf := &in.NewFindings[i]
		status := DiffNew
		matched := d.matchNew(nf, oldByL1, oldByL2, in.Previous, thres, &out.Stats)

		if matched != nil {
			// 第4级锚点兜底：文件+行号邻近+标题关键词命中
			status = DiffExisted
			// 命中历史已解决缺陷 → REOPENED
			if matched.Status == "resolved" || matched.DiffStatus == DiffResolved {
				status = DiffReopened
				out.OldStatus[matched.ID] = DiffReopened
			} else {
				out.OldStatus[matched.ID] = DiffExisted
			}
			out.Matched[nf.ID] = matched.ID
			delete(unmatchedOld, matched.ID)
		}
		out.NewStatus[nf.ID] = status
		switch status {
		case DiffNew:
			out.Stats.New++
		case DiffExisted:
			out.Stats.Existed++
		case DiffReopened:
			out.Stats.Reopened++
		}
	}

	// 旧缺陷未被命中 → RESOLVED（Scope守卫）
	for id, old := range unmatchedOld {
		if d.ScopeGuard && !FileExistsInScope(in.RepoRoot, old.FilePath) {
			// 文件已移出范围
			if d.AutoResolve {
				out.OldStatus[id] = DiffResolved
				out.Stats.Resolved++
				continue
			}
			// 守卫开启但不在范围内且不允许自动解决：保持EXISTED，交由人工
			out.OldStatus[id] = DiffExisted
			continue
		}
		out.OldStatus[id] = DiffResolved
		out.Stats.Resolved++
	}
	return out
}

// matchNew 四级漏斗顺序匹配：L1 → L2 → 相似度 → 锚点
func (d *DiffEngine) matchNew(nf *FindingLike, oldByL1, oldByL2 map[string][]*FindingLike, prev []FindingLike, thres float64, stats *DiffStats) *FindingLike {
	// 第1级：L1物理强指纹
	if nf.L1Fingerprint != "" {
		if cands := oldByL1[nf.L1Fingerprint]; len(cands) > 0 {
			stats.L1Matched++
			return cands[0]
		}
	}
	// 第2级：L2语义指纹
	if nf.L2Fingerprint != "" {
		if cands := oldByL2[nf.L2Fingerprint]; len(cands) > 0 {
			stats.L2Matched++
			return cands[0]
		}
	}
	// 第3级：内容相似度（同文件候选）
	var best *FindingLike
	var bestScore float64
	for i := range prev {
		p := &prev[i]
		if p.FilePath != nf.FilePath || p.Status == "suppressed" {
			continue
		}
		score := Similarity([]string{nf.Title, snippetOf(nf)}, []string{p.Title, snippetOf(p)})
		if score > bestScore {
			bestScore = score
			best = p
		}
	}
	if best != nil && bestScore >= thres {
		stats.SimilarityHit++
		return best
	}
	// 第4级：锚点兜底（同文件 + 行号相近 + 标题关键词重叠）
	for i := range prev {
		p := &prev[i]
		if p.FilePath != nf.FilePath || p.Status == "suppressed" {
			continue
		}
		if lineClose(nf.LineNumber, p.LineNumber, 5) && titleOverlap(nf.Title, p.Title) {
			stats.AnchorHit++
			return p
		}
	}
	return nil
}

func snippetOf(f *FindingLike) string { return f.Title }

// indexByFingerprint 按指纹建索引（L1或L2）
func indexByFingerprint(fs []FindingLike, l1 bool) map[string][]*FindingLike {
	m := map[string][]*FindingLike{}
	for i := range fs {
		f := &fs[i]
		if f.Status == "suppressed" {
			continue
		}
		k := f.L2Fingerprint
		if l1 {
			k = f.L1Fingerprint
		}
		if k == "" {
			continue
		}
		m[k] = append(m[k], f)
	}
	return m
}

// lineClose 行号是否接近（解析范围如 "12-15"）
func lineClose(a, b string, tolerance int) bool {
	an, bn := parseLineRange(a), parseLineRange(b)
	if an == 0 || bn == 0 {
		return false
	}
	diff := an - bn
	if diff < 0 {
		diff = -diff
	}
	return diff <= tolerance
}

func parseLineRange(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "-–~"); i > 0 {
		s = s[:i]
	}
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// titleOverlap 标题关键词重叠（分词按非字母数字切分）
func titleOverlap(a, b string) bool {
	wa := tokenize(a)
	wb := tokenize(b)
	if len(wa) == 0 || len(wb) == 0 {
		return false
	}
	hit := 0
	for _, t := range wa {
		if t == "" || len(t) < 3 {
			continue
		}
		for _, u := range wb {
			if t == u {
				hit++
			}
		}
	}
	return hit >= 1
}

func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			cur.WriteRune(r)
		} else {
			if cur.Len() > 0 {
				out = append(out, strings.ToLower(cur.String()))
				cur.Reset()
			}
		}
	}
	if cur.Len() > 0 {
		out = append(out, strings.ToLower(cur.String()))
	}
	return out
}

// SortedIDs 按ID升序输出（确定性）
func SortedIDs(m map[uint]string) []uint {
	ids := make([]uint, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// Summary 人类可读的比对摘要
func (o *DiffOutput) Summary() string {
	return fmt.Sprintf(
		"四级漏斗: L1=%d L2=%d Sim=%d Anchor=%d | 状态机: NEW=%d EXISTED=%d RESOLVED=%d REOPENED=%d",
		o.Stats.L1Matched, o.Stats.L2Matched, o.Stats.SimilarityHit, o.Stats.AnchorHit,
		o.Stats.New, o.Stats.Existed, o.Stats.Resolved, o.Stats.Reopened)
}
package governance

import (
	"sort"
	"strings"
)

// CampaignFinding 专项归并的缺陷条目
type CampaignFinding struct {
	FindingID uint   `json:"finding_id"`
	RepoID    uint   `json:"repo_id"`
	RepoName  string `json:"repo_name"`
	Severity  string `json:"severity"`
	Category  string `json:"category"`
	FilePath  string `json:"file_path"`
	Title     string `json:"title"`
	DiffStatus string `json:"diff_status"`
}

// CampaignGroup 专项分组
type CampaignGroup struct {
	Name       string             `json:"name"`
	DisplayName string            `json:"display_name"`
	Total      int                `json:"total"`
	BySeverity map[string]int     `json:"by_severity"`
	ByRepo     map[string]int     `json:"by_repo"`
	TopFiles   []FileCount        `json:"top_files"`
	Findings   []CampaignFinding  `json:"findings"`
}

// FileCount 文件缺陷计数
type FileCount struct {
	FilePath string `json:"file_path"`
	Count    int    `json:"count"`
}

// Campaign 专项扫描归并器：将同一专项（is_campaign任务类型）跨报告缺陷合并分组
type Campaign struct{}

// NewCampaign 创建专项归并器
func NewCampaign() *Campaign { return &Campaign{} }

// Group 将缺陷归并为专项分组
func (c *Campaign) Group(name, displayName string, findings []CampaignFinding) *CampaignGroup {
	g := &CampaignGroup{
		Name:        name,
		DisplayName: displayName,
		Total:       len(findings),
		BySeverity:  map[string]int{},
		ByRepo:      map[string]int{},
		Findings:    findings,
	}
	fileCount := map[string]int{}
	for _, f := range findings {
		g.BySeverity[normalizeSev(f.Severity)]++
		g.ByRepo[f.RepoName]++
		fileCount[f.FilePath]++
	}
	for file, cnt := range fileCount {
		g.TopFiles = append(g.TopFiles, FileCount{FilePath: file, Count: cnt})
	}
	sort.SliceStable(g.TopFiles, func(i, j int) bool {
		if g.TopFiles[i].Count != g.TopFiles[j].Count {
			return g.TopFiles[i].Count > g.TopFiles[j].Count
		}
		return g.TopFiles[i].FilePath < g.TopFiles[j].FilePath
	})
	if len(g.TopFiles) > 10 {
		g.TopFiles = g.TopFiles[:10]
	}
	return g
}

// Merge 将新一批缺陷归并进已有分组（幂等：按FindingID去重）
func (c *Campaign) Merge(group *CampaignGroup, incoming []CampaignFinding) *CampaignGroup {
	seen := map[uint]bool{}
	for _, f := range group.Findings {
		seen[f.FindingID] = true
	}
	merged := append([]CampaignFinding{}, group.Findings...)
	for _, f := range incoming {
		if !seen[f.FindingID] {
			merged = append(merged, f)
			seen[f.FindingID] = true
		}
	}
	return c.Group(group.Name, group.DisplayName, merged)
}

func normalizeSev(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return "LOW"
	}
	return s
}
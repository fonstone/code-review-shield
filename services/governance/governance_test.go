package governance

import "testing"

func TestCalibratorCriticalPattern(t *testing.T) {
	c := NewCalibrator()
	cal := c.Calibrate("HIGH", "double free风险", "ptr被重复释放", 0.6)
	if cal.Severity != "CRITICAL" {
		t.Errorf("double free应升为CRITICAL, got %s", cal.Severity)
	}
	if !cal.Adjusted {
		t.Error("应标记为已调整")
	}
}

func TestCalibratorHighPattern(t *testing.T) {
	c := NewCalibrator()
	cal := c.Calibrate("LOW", "内存泄漏", "malloc未释放", 0.7)
	if cal.Severity != "HIGH" {
		t.Errorf("内存泄漏最低应为HIGH, got %s", cal.Severity)
	}
}

func TestCalibratorDowngradeLowConfidence(t *testing.T) {
	c := NewCalibrator()
	cal := c.Calibrate("HIGH", "普通问题", "无明显证据", 0.3)
	if cal.Severity != "MEDIUM" {
		t.Errorf("低置信度应降一级, got %s", cal.Severity)
	}
}

func TestCalibratorKeepLow(t *testing.T) {
	c := NewCalibrator()
	cal := c.Calibrate("LOW", "轻微问题", "低风险", 0.3)
	// 置信度0.3 < 0.5应降级，但LOW已是底线
	if cal.Severity != "LOW" {
		t.Errorf("LOW不应再降级, got %s", cal.Severity)
	}
}

func TestFeedbackSuppression(t *testing.T) {
	s := NewFeedbackStore()
	if s.IsSuppressed("L1:x", "L2:y") {
		t.Fatal("初始不应有误报")
	}
	s.Suppress("L1:x", "L2:y", "已知误报: 系统调用模式", "admin")
	if !s.IsSuppressed("L1:x", "L2:y") {
		t.Error("L1命中应被抑制")
	}
	if !s.IsSuppressed("L1:other", "L2:y") {
		t.Error("L2命中应被抑制")
	}
	if s.IsSuppressed("L1:other", "L2:other") {
		t.Error("无关指纹不应被抑制")
	}
	if r := s.Reason("L1:x", "L2:y"); r != "已知误报: 系统调用模式" {
		t.Errorf("原因记录错误: %s", r)
	}
	if len(s.List()) != 1 {
		t.Errorf("应记录1条误报, got %d", len(s.List()))
	}
	s.Remove("L1:x", "L2:y")
	if s.IsSuppressed("L1:x", "L2:y") {
		t.Error("移除后不应再被抑制")
	}
}

func TestTaxonomyNormalize(t *testing.T) {
	tax := NewTaxonomy()
	cases := map[string]string{
		"memory leak":    "memory_leak",
		"MemoryLeak":     "memory_leak",
		"内存泄漏":         "memory_leak",
		"null pointer":   "null_deref",
		"cjson misuse":   "cjson_scan",
		"浮点直接相等比较":     "float_comparison",
		"totally-unknown": "other",
		"":               "other",
	}
	for in, want := range cases {
		if got := tax.Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCampaignGroup(t *testing.T) {
	c := NewCampaign()
	items := []CampaignFinding{
		{FindingID: 1, RepoID: 1, RepoName: "repo-a", Severity: "CRITICAL", FilePath: "x.c", Title: "泄漏"},
		{FindingID: 2, RepoID: 1, RepoName: "repo-a", Severity: "HIGH", FilePath: "x.c", Title: "越界"},
		{FindingID: 3, RepoID: 2, RepoName: "repo-b", Severity: "LOW", FilePath: "y.c", Title: "未使用"},
	}
	g := c.Group("memory_leak", "内存泄漏检测", items)
	if g.Total != 3 {
		t.Errorf("总数错误: %d", g.Total)
	}
	if g.BySeverity["CRITICAL"] != 1 {
		t.Errorf("严重度统计错误: %v", g.BySeverity)
	}
	if len(g.TopFiles) != 2 || g.TopFiles[0].FilePath != "x.c" || g.TopFiles[0].Count != 2 {
		t.Errorf("文件Top错误: %+v", g.TopFiles)
	}
	// Merge幂等
	merged := c.Merge(g, items)
	if merged.Total != 3 {
		t.Errorf("Merge后应去重保持3, got %d", merged.Total)
	}
}
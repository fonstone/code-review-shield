package defects

import (
	"testing"
)

func finding(id uint, file, line, title string, l1, l2, status, diff string) FindingLike {
	return FindingLike{ID: id, FilePath: file, LineNumber: line, Title: title,
		L1Fingerprint: l1, L2Fingerprint: l2, Status: status, DiffStatus: diff}
}

func TestDiffEngineNewAndExisted(t *testing.T) {
	engine := NewDiffEngine(0.85, false, false)
	prev := []FindingLike{
		finding(1, "a.c", "10", "malloc未释放", "L1:same", "L2:same", "open", "EXISTED"),
	}
	in := DiffInput{
		NewFindings: []FindingLike{
			finding(101, "a.c", "10", "malloc未释放", "L1:same", "L2:same", "open", ""),
			finding(102, "b.c", "5", "新的缺陷", "L1:new1", "L2:new2", "open", ""),
		},
		Previous: prev,
	}
	out := engine.Compare(in)
	if out.NewStatus[101] != DiffExisted {
		t.Errorf("L1命中应EXISTED, got %s", out.NewStatus[101])
	}
	if out.NewStatus[102] != DiffNew {
		t.Errorf("未命中应NEW, got %s", out.NewStatus[102])
	}
	if out.Stats.L1Matched != 1 {
		t.Errorf("L1漏斗应命中1次, got %d", out.Stats.L1Matched)
	}
	if out.Stats.New != 1 || out.Stats.Existed != 1 {
		t.Errorf("统计异常: %+v", out.Stats)
	}
}

func TestDiffEngineL2Fallback(t *testing.T) {
	engine := NewDiffEngine(0.85, false, false)
	prev := []FindingLike{
		finding(1, "a.c", "10", "old title", "L1:old-l1", "L2:semantic-same", "open", "EXISTED"),
	}
	out := engine.Compare(DiffInput{
		NewFindings: []FindingLike{
			finding(101, "a.c", "10", "new title", "L1:new-l1", "L2:semantic-same", "open", ""),
		},
		Previous: prev,
	})
	if out.NewStatus[101] != DiffExisted {
		t.Errorf("L1未命中但L2命中应EXISTED, got %s", out.NewStatus[101])
	}
	if out.Stats.L2Matched != 1 {
		t.Errorf("L2漏斗应命中, got %d", out.Stats.L2Matched)
	}
}

func TestDiffEngineResolved(t *testing.T) {
	engine := NewDiffEngine(0.85, false, false)
	prev := []FindingLike{
		finding(1, "a.c", "10", "旧缺陷已被修复", "L1:old", "L2:old", "open", "EXISTED"),
	}
	out := engine.Compare(DiffInput{
		NewFindings: []FindingLike{
			finding(101, "b.c", "1", "无关新缺陷", "L1:other", "L2:other", "open", ""),
		},
		Previous: prev,
	})
	if out.OldStatus[1] != DiffResolved {
		t.Errorf("未匹配的旧缺陷应RESOLVED, got %s", out.OldStatus[1])
	}
	if out.Stats.Resolved != 1 {
		t.Errorf("RESOLVED统计异常: %+v", out.Stats)
	}
}

func TestDiffEngineReopened(t *testing.T) {
	engine := NewDiffEngine(0.85, false, false)
	prev := []FindingLike{
		finding(1, "a.c", "10", "旧缺陷(已解决)", "L1:same", "L2:same", "resolved", "RESOLVED"),
	}
	out := engine.Compare(DiffInput{
		NewFindings: []FindingLike{
			finding(101, "a.c", "10", "旧缺陷重现", "L1:same", "L2:same", "open", ""),
		},
		Previous: prev,
	})
	if out.NewStatus[101] != DiffReopened {
		t.Errorf("命中已解决缺陷应REOPENED, got %s", out.NewStatus[101])
	}
	if out.OldStatus[1] != DiffReopened {
		t.Errorf("旧缺陷应标记REOPENED, got %s", out.OldStatus[1])
	}
	if out.Stats.Reopened != 1 {
		t.Errorf("REOPENED统计异常: %+v", out.Stats)
	}
}

func TestDiffEngineSimilarityFunnel(t *testing.T) {
	engine := NewDiffEngine(0.5, false, false) // 低阈值便于测试
	prev := []FindingLike{
		finding(1, "a.c", "20", "内存泄漏: 在错误路径中未释放 p_buf", "L1:x", "L2:x", "open", "EXISTED"),
	}
	out := engine.Compare(DiffInput{
		NewFindings: []FindingLike{
			finding(101, "a.c", "25", "内存泄漏: 错误路径未释放 p_buf 缓冲", "L1:y", "L2:y", "open", ""),
		},
		Previous: prev,
	})
	if out.NewStatus[101] != DiffExisted {
		t.Errorf("相似度命中应EXISTED, got %s", out.NewStatus[101])
	}
	if out.Stats.SimilarityHit != 1 {
		t.Errorf("相似度漏斗应命中, got %d", out.Stats.SimilarityHit)
	}
}

func TestDiffEngineScopeGuard(t *testing.T) {
	engine := NewDiffEngine(0.85, true, true)
	prev := []FindingLike{
		finding(1, "deleted_file.c", "5", "已删除文件的缺陷", "L1:a", "L2:a", "open", "EXISTED"),
	}
	// 仓库根目录指向一个空目录，deleted_file.c必然不存在
	out := engine.Compare(DiffInput{
		RepoRoot:    t.TempDir(),
		NewFindings: []FindingLike{finding(101, "b.c", "1", "x", "L1:b", "L2:b", "open", "")},
		Previous:    prev,
	})
	if out.OldStatus[1] != DiffResolved {
		t.Errorf("Scope守卫：文件缺失应RESOLVED, got %s", out.OldStatus[1])
	}
	// 关闭自动解决：守卫开启但文件缺失 → 保持EXISTED
	engine2 := NewDiffEngine(0.85, true, false)
	out2 := engine2.Compare(DiffInput{
		RepoRoot:    t.TempDir(),
		NewFindings: []FindingLike{finding(101, "b.c", "1", "x", "L1:b", "L2:b", "open", "")},
		Previous:    prev,
	})
	if out2.OldStatus[1] != DiffExisted {
		t.Errorf("auto_resolve=false时缺失文件缺陷应保持EXISTED, got %s", out2.OldStatus[1])
	}
}

func TestDiffEngineAnchorFallback(t *testing.T) {
	engine := NewDiffEngine(0.95, false, false) // 高相似度阈值，逼退到锚点级
	prev := []FindingLike{
		finding(1, "a.c", "30", "double free风险: buf 被重复释放", "L1:q", "L2:q", "open", "EXISTED"),
	}
	out := engine.Compare(DiffInput{
		NewFindings: []FindingLike{
			finding(101, "a.c", "32", "double free风险", "L1:r", "L2:r", "open", ""),
		},
		Previous: prev,
	})
	if out.NewStatus[101] != DiffExisted {
		t.Errorf("锚点兜底应EXISTED, got %s", out.NewStatus[101])
	}
	if out.Stats.AnchorHit != 1 {
		t.Errorf("锚点漏斗应命中, got %d", out.Stats.AnchorHit)
	}
}

func TestDiffEngineSuppressedExcluded(t *testing.T) {
	engine := NewDiffEngine(0.85, false, false)
	prev := []FindingLike{
		finding(1, "a.c", "10", "已知误报", "L1:fp", "L2:fp", "suppressed", "EXISTED"),
	}
	out := engine.Compare(DiffInput{
		NewFindings: []FindingLike{
			finding(101, "a.c", "10", "已知误报", "L1:fp", "L2:fp", "open", ""),
		},
		Previous: prev,
	})
	// 误报历史不参与匹配 → 新缺陷应为NEW（由误报记忆库另行抑制）
	if out.NewStatus[101] != DiffNew {
		t.Errorf("suppressed历史不应匹配, got %s", out.NewStatus[101])
	}
}

func TestBackfillFingerprints(t *testing.T) {
	res := BackfillFingerprints(MigrateRequest{Items: []MigrateItem{
		{ID: 1, FilePath: "a.c", LineNumber: "5", Title: "泄漏", CodeSnippet: "malloc(10)"},
		{ID: 2, FilePath: "b.c", LineNumber: "9", Title: "越界", CodeSnippet: "arr[10]"},
	}})
	if len(res.Items) != 2 {
		t.Fatal("回填数量错误")
	}
	if res.Items[0].L1Fingerprint == "" || res.Items[0].L2Fingerprint == "" {
		t.Error("回填指纹为空")
	}
}
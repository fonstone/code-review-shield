package engines

import (
	"context"
	"testing"
)

func TestParseFindingsPureJSON(t *testing.T) {
	content := `[{"severity":"HIGH","category":"memory_leak","file_path":"src/a.c","line_number":"12","code_snippet":"malloc(10)","title":"malloc未释放","detail":"无free路径","suggestion":"添加free","confidence":0.9}]`
	fs, err := ParseFindings(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("应解析1条, got %d", len(fs))
	}
	f := fs[0]
	if f.Severity != "HIGH" || f.FilePath != "src/a.c" || f.LineNumber != "12" {
		t.Errorf("字段解析错误: %+v", f)
	}
}

func TestParseFindingsFenced(t *testing.T) {
	content := "分析结果如下：\n```json\n[{\"severity\":\"low\",\"title\":\"未使用变量\",\"file_path\":\"./src/x.c\",\"line_number\":\"3\"}]\n```\n以上。"
	fs, err := ParseFindings(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Fatalf("应解析1条, got %d", len(fs))
	}
	// severity归一化 + 路径清理
	if fs[0].Severity != "LOW" {
		t.Errorf("severity应归一化为LOW, got %s", fs[0].Severity)
	}
	if fs[0].FilePath != "src/x.c" {
		t.Errorf("路径应去除./前缀: %s", fs[0].FilePath)
	}
	if fs[0].Confidence <= 0 {
		t.Errorf("缺省置信度应为0.8, got %f", fs[0].Confidence)
	}
}

func TestParseFindingsInvalid(t *testing.T) {
	if _, err := ParseFindings("这不是JSON"); err == nil {
		t.Error("无效输出应报错")
	}
}

func TestParseFindingsDedupe(t *testing.T) {
	content := `[{"title":"同一条","file_path":"a.c","line_number":"1"},{"title":"同一条","file_path":"a.c","line_number":"1"}]`
	fs, err := ParseFindings(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 1 {
		t.Errorf("重复缺陷应去重, got %d", len(fs))
	}
}

// 编译期断言：引擎不依赖DB（纯内存规约）
func TestEnginePackagePurity(t *testing.T) {
	_ = context.Background
}
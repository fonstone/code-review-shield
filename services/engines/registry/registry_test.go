package registry

import (
	"os"
	"path/filepath"
	"testing"

	"code-shield/services/engines"
	"code-shield/services/invoker"
)

// 单引擎端到端测试（fake invoker注入），放在registry包避免父子循环
func TestSingleEngineWithFakeInvoker(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "main.c"), []byte("int main(){ malloc(10); }"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "util.c"), []byte("int u(){ return 1; }"), 0o644)

	fake, err := invoker.NewFakeInvoker(invoker.ResourceConfig{ID: "f", Driver: "fake", Model: "test"})
	if err != nil {
		t.Fatal(err)
	}
	fk := fake.(*invoker.FakeInvoker)
	fk.SetHandler(func(req invoker.AIRequest) (string, error) {
		return `[{"severity":"MEDIUM","title":"泄漏","file_path":"main.c","line_number":"1","category":"memory_leak","confidence":0.8}]`, nil
	})

	engine := NewEngine("single")
	ctx := engines.EngineContext{
		RepoPath:       dir,
		Mode:           "single",
		DefaultInvoker: fake,
		AnalysisPrompt: "你是代码审计专家。",
		Config:         engines.ChunkConfig{FileExtensions: []string{".c"}},
		CancelCh:       make(chan struct{}),
	}
	res, err := engine.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("应产出1条缺陷, got %d", len(res.Findings))
	}
	if res.Findings[0].FilePath != "main.c" {
		t.Errorf("文件路径错误: %s", res.Findings[0].FilePath)
	}
	if engine.Name() != "single" || len(engine.Modes()) != 1 {
		t.Errorf("引擎元数据错误")
	}
}

// 全部5种模式工厂映射
func TestFactoryAllModes(t *testing.T) {
	cases := map[string]string{
		"single":            "single",
		"chunked":           "chunked",
		"chunked_fast":      "chunked",
		"debate_full":       "debate",
		"debate_selective":  "debate",
	}
	for mode, wantName := range cases {
		e := NewEngine(mode)
		if e.Name() != wantName {
			t.Errorf("mode=%s 应映射到 %s, got %s", mode, wantName, e.Name())
		}
	}
}
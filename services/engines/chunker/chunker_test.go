package chunker

import (
	"os"
	"path/filepath"
	"testing"
)

func setupRepo(t *testing.T) string {
	root := t.TempDir()
	files := map[string]string{
		"src/main.c":     "#include <stdio.h>\nint main() { malloc(10); return 0; }",
		"src/util.cpp":   "int util() { return 42; }",
		"src/header.h":   "#define MAX 100\nvoid helper();",
		"vendor/lib.c":   "int vendor_code() { return 1; }",
		"src/readme.md":  "# 文档不该被扫描",
		"src/big.c":      bigFile(1000),
	}
	for rel, content := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func bigFile(lines int) string {
	var s string
	for i := 0; i < lines; i++ {
		s += "int var_" + itoa(i) + " = " + itoa(i) + ";\n"
	}
	return s
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

func TestChunkRepositoryFiltering(t *testing.T) {
	root := setupRepo(t)
	opts := Options{
		FileExtensions:   []string{".c", ".cpp", ".h"},
		ContentKeywords:  []string{"malloc"},
		ExcludePaths:     []string{"vendor"},
		MaxFiles:         10,
		Depth:            2,
		MaxLinesPerChunk: 200,
	}
	chunks, err := ChunkRepository(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	// 排除vendor与.md；包含main.c/util.cpp/header.h/big.c
	seen := map[string]bool{}
	for _, c := range chunks {
		seen[c.FilePath] = true
	}
	if seen["vendor/lib.c"] {
		t.Error("vendor应被排除")
	}
	if seen["src/readme.md"] {
		t.Error("md应被排除")
	}
	if !seen["src/main.c"] || !seen["src/util.cpp"] || !seen["src/header.h"] {
		t.Errorf("应包含源码文件: %v", seen)
	}
	// 优先级：main.c含malloc关键词，排序时应靠前
	if chunks[0].FilePath != "src/main.c" {
		t.Errorf("关键词命中文件应排最前, got %s", chunks[0].FilePath)
	}
}

func TestChunkSplitting(t *testing.T) {
	root := setupRepo(t)
	opts := Options{
		FileExtensions:   []string{".c"},
		MaxFiles:         10,
		MaxLinesPerChunk: 300,
		OverlapLines:     20,
	}
	chunks, err := ChunkRepository(root, opts)
	if err != nil {
		t.Fatal(err)
	}
	var bigCount int
	for _, c := range chunks {
		if c.FilePath == "src/big.c" {
			bigCount++
		}
	}
	if bigCount < 3 {
		t.Errorf("1000行文件按300行应切分≥3片, got %d", bigCount)
	}
	// 分片ID确定性
	again, _ := ChunkRepository(root, opts)
	if len(again) != len(chunks) {
		t.Fatal("分片数量应确定性")
	}
	for i := range chunks {
		if chunks[i].ID != again[i].ID {
			t.Errorf("分片ID应确定性: %s vs %s", chunks[i].ID, again[i].ID)
		}
	}
}

func TestFilterByIDs(t *testing.T) {
	chunks := []Chunk{
		{ID: "a", FilePath: "x.c"}, {ID: "b", FilePath: "y.c"}, {ID: "c", FilePath: "z.c"},
	}
	got := FilterByIDs(chunks, []string{"b"})
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("过滤错误: %+v", got)
	}
	if len(FilterByIDs(chunks, nil)) != 3 {
		t.Error("空ID列表应返回全部分片")
	}
}

func TestChunkRepositoryEmpty(t *testing.T) {
	root := t.TempDir()
	chunks, err := ChunkRepository(root, Options{FileExtensions: []string{".c"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 0 {
		t.Error("空仓库应无分片")
	}
}
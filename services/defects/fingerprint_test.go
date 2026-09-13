package defects

import (
	"strings"
	"testing"
)

func TestFingerprintDeterministic(t *testing.T) {
	src := "int main() {\n    char *p = malloc(100);\n    return 0;\n}"
	f1 := FingerprintSource("src/main.c", src, "main")
	f2 := FingerprintSource("src/main.c", src, "main")
	if f1.L1 != f2.L1 || f1.L2 != f2.L2 {
		t.Errorf("指纹应确定性: %+v vs %+v", f1, f2)
	}
}

func TestL1DiffersOnCodeChange(t *testing.T) {
	a := FingerprintSource("a.c", "int x = 1;", "foo")
	b := FingerprintSource("a.c", "int x = 2;", "foo")
	if a.L1 == b.L1 {
		t.Error("物理代码变化L1应不同")
	}
}

func TestL2AllowsSemanticEquivalence(t *testing.T) {
	a := FingerprintSource("a.c", "int result = count + 42;", "foo")
	b := FingerprintSource("a.c", "int total = result + 42;", "foo")
	// L1物理指纹应不同（变量重命名）
	if a.L1 == b.L1 {
		t.Error("变量重命名L1应不同")
	}
	// L2语义指纹相同（标识符归一化）
	if a.L2 != b.L2 {
		t.Errorf("语义等价代码L2应相同: %s vs %s", a.L2, b.L2)
	}
}

func TestL2AllowsNumberChange(t *testing.T) {
	a := FingerprintSource("a.c", "buffer[64] = value;", "foo")
	b := FingerprintSource("a.c", "buffer[128] = value;", "foo")
	if a.L2 != b.L2 {
		t.Errorf("数字变化L2应相同（数字归一化）")
	}
}

func TestSimilarity(t *testing.T) {
	a := []string{"int main()", "{", "return 0;", "}"}
	b := []string{"int main()", "{", "return 0;", "}"}
	if Similarity(a, b) < 0.9 {
		t.Errorf("相同代码相似度应接近1: %f", Similarity(a, b))
	}
	c := []string{"totally different content here"}
	if Similarity(a, c) > 0.5 {
		t.Errorf("无关代码相似度应低: %f", Similarity(a, c))
	}
}

func TestExtractSymbols(t *testing.T) {
	src := `
#include <stdio.h>
#define MAX_SIZE 100
struct config { int port; };
static int helper(int a) { return a; }
int main(void) {
    int unused_var = 42;
    return helper(unused_var);
}
`
	syms := ExtractAnchors("test.c", src)
	found := map[string]bool{}
	for _, s := range syms {
		found[s.Name] = true
	}
	for _, want := range []string{"MAX_SIZE", "config", "helper", "main", "unused_var"} {
		if !found[want] {
			t.Errorf("锚点缺失: %s (got %v)", want, found)
		}
	}
}

func TestCleanSource(t *testing.T) {
	src := "// 注释\nint a; /* 块注释 */\n\n\nint b;"
	lines := CleanSource(src)
	if len(lines) != 2 {
		t.Fatalf("清洗后应有2行: %v", lines)
	}
	if !strings.Contains(lines[0], "int a;") {
		t.Errorf("注释应被移除: %s", lines[0])
	}
}

func TestFileExistsInScope(t *testing.T) {
	if !FileExistsInScope("", "x.c") {
		t.Error("空root应视为存在（跳过守卫）")
	}
}
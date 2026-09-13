// Package chunker 语义分片器：将代码仓库拆分为可独立分析的Chunk。
// 分片规则：文件扩展名过滤 → 排除路径过滤 → 关键词风险优先级排序 → 按行数切分（带重叠）。
package chunker

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Chunk 代码分片
type Chunk struct {
	ID        string `json:"id"`
	FilePath  string `json:"file_path"` // 仓库内相对路径
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
	Priority  int    `json:"priority"`   // 越大越优先
	RiskScore int    `json:"risk_score"` // 关键词命中数
}

// Options 分片选项
type Options struct {
	FileExtensions   []string
	ContentKeywords  []string
	ExcludePaths     []string
	MaxFiles         int
	Depth            int
	MaxLinesPerChunk int
	OverlapLines     int
	FastMode         bool
}

// ChunkRepository 扫描仓库并产出分片（确定性顺序：优先级降序→路径升序→行号升序）
func ChunkRepository(root string, opts Options) ([]Chunk, error) {
	if root == "" {
		return nil, fmt.Errorf("仓库路径为空")
	}
	if opts.MaxFiles <= 0 {
		opts.MaxFiles = 20
	}
	if opts.MaxLinesPerChunk <= 0 {
		opts.MaxLinesPerChunk = 400
	}
	if opts.OverlapLines < 0 {
		opts.OverlapLines = 0
	}
	if opts.Depth <= 0 {
		opts.Depth = 3
	}

	type fileInfo struct {
		rel       string
		abs       string
		risk      int
		lineCount int
	}
	var files []fileInfo

	rootClean := filepath.Clean(root)
	err := filepath.Walk(rootClean, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 忽略不可读路径
		}
		rel, rerr := filepath.Rel(rootClean, path)
		if rerr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		depth := strings.Count(rel, "/")
		if info.IsDir() {
			if depth >= opts.Depth {
				return filepath.SkipDir
			}
			if shouldExclude(rel, opts.ExcludePaths) {
				return filepath.SkipDir
			}
			if strings.HasPrefix(filepath.Base(rel), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if depth > opts.Depth {
			return nil
		}
		if shouldExclude(rel, opts.ExcludePaths) {
			return nil
		}
		if !matchExt(rel, opts.FileExtensions) {
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil || len(data) == 0 || len(data) > 2*1024*1024 {
			return nil
		}
		content := string(data)
		files = append(files, fileInfo{
			rel:       rel,
			abs:       path,
			risk:      keywordScore(content, opts.ContentKeywords),
			lineCount: strings.Count(content, "\n") + 1,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// 关键词命中优先，命中数相同按行数降序（大文件优先）
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].risk != files[j].risk {
			return files[i].risk > files[j].risk
		}
		if files[i].lineCount != files[j].lineCount {
			return files[i].lineCount > files[j].lineCount
		}
		return files[i].rel < files[j].rel
	})
	if len(files) > opts.MaxFiles {
		files = files[:opts.MaxFiles]
	}

	var chunks []Chunk
	for _, f := range files {
		data, err := os.ReadFile(f.abs)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, c := range splitFile(f.rel, lines, opts) {
			c.Priority = f.risk
			chunks = append(chunks, c)
		}
	}
	// 确定性排序：优先级降序 → 路径升序 → 起始行升序
	sort.SliceStable(chunks, func(i, j int) bool {
		if chunks[i].Priority != chunks[j].Priority {
			return chunks[i].Priority > chunks[j].Priority
		}
		if chunks[i].FilePath != chunks[j].FilePath {
			return chunks[i].FilePath < chunks[j].FilePath
		}
		return chunks[i].StartLine < chunks[j].StartLine
	})
	return chunks, nil
}

// splitFile 单文件按行切分，带重叠窗口
func splitFile(rel string, lines []string, opts Options) []Chunk {
	maxLines := opts.MaxLinesPerChunk
	if maxLines <= 0 {
		maxLines = 400
	}
	overlap := opts.OverlapLines
	if overlap >= maxLines {
		overlap = maxLines / 5
	}
	if len(lines) <= maxLines {
		content := strings.Join(lines, "\n")
		return []Chunk{newChunk(rel, 1, len(lines), content)}
	}
	var out []Chunk
	start := 0
	for start < len(lines) {
		end := start + maxLines
		if end > len(lines) {
			end = len(lines)
		}
		out = append(out, newChunk(rel, start+1, end, strings.Join(lines[start:end], "\n")))
		if end == len(lines) {
			break
		}
		start = end - overlap
		if start < 0 {
			start = 0
		}
	}
	return out
}

func newChunk(rel string, start, end int, content string) Chunk {
	h := sha1.Sum([]byte(fmt.Sprintf("%s:%d:%d", rel, start, end)))
	return Chunk{
		ID:        "chunk-" + hex.EncodeToString(h[:8]),
		FilePath:  rel,
		StartLine: start,
		EndLine:   end,
		Content:   content,
	}
}

// FilterByIDs 断点续跑：仅保留指定分片
func FilterByIDs(chunks []Chunk, ids []string) []Chunk {
	if len(ids) == 0 {
		return chunks
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []Chunk
	for _, c := range chunks {
		if want[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

func matchExt(rel string, exts []string) bool {
	if len(exts) == 0 {
		return true
	}
	lower := strings.ToLower(rel)
	for _, e := range exts {
		if e == "" {
			continue
		}
		if strings.HasSuffix(lower, strings.ToLower(e)) {
			return true
		}
	}
	return false
}

func shouldExclude(rel string, excludes []string) bool {
	lower := strings.ToLower(rel)
	for _, e := range excludes {
		e = strings.ToLower(strings.Trim(e, "/"))
		if e == "" {
			continue
		}
		if strings.HasPrefix(lower, e+"/") || strings.Contains(lower, "/"+e+"/") || lower == e {
			return true
		}
	}
	return false
}

func keywordScore(content string, keywords []string) int {
	lower := strings.ToLower(content)
	score := 0
	for _, k := range keywords {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		score += strings.Count(lower, k)
	}
	return score
}
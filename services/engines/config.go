package engines

// ChunkConfig 分片参数定义（来自任务插件meta.json engine_config）
type ChunkConfig struct {
	FileExtensions []string `json:"file_extensions"`
	ContentKeywords []string `json:"content_keywords"`
	ExcludePaths   []string `json:"exclude_paths"`
	MaxFiles       int      `json:"max_files"`
	Depth          int      `json:"depth"`
	Concurrency    int      `json:"concurrency"`
	MaxLinesPerChunk int    `json:"max_lines_per_chunk"`
	OverlapLines   int      `json:"overlap_lines"`
	FastMode       bool     `json:"fast_mode"`
}

// Normalize 填充默认值
func (c *ChunkConfig) Normalize() {
	if len(c.FileExtensions) == 0 {
		c.FileExtensions = []string{".c", ".cpp", ".h", ".hpp"}
	}
	if c.MaxFiles <= 0 {
		c.MaxFiles = 20
	}
	if c.Depth <= 0 {
		c.Depth = 3
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 4
	}
	if c.MaxLinesPerChunk <= 0 {
		if c.FastMode {
			c.MaxLinesPerChunk = 150
		} else {
			c.MaxLinesPerChunk = 400
		}
	}
	if c.OverlapLines <= 0 {
		c.OverlapLines = 20
	}
}

// DebateTier 辩论层级（对应config.yaml debate.tiers）
type DebateTier struct {
	Resource       string
	TimeoutSeconds int
}

// DebateTiers 四层辩论结构
type DebateTiers struct {
	Tier1Hunter     DebateTier // 猎手：定位候选缺陷
	Tier2Challenger DebateTier // 质询者：交叉校验
	Tier3Judge      DebateTier // 裁判：裁决保留/驳回
	Tier4Synthesis  DebateTier // 汇总：输出结构化结果
}
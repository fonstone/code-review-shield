package defects

// Migration 指纹迁移工具：历史缺陷指纹回填
//
// 使用场景：升级引入指纹计算后，历史 analysis_findings 记录的
// l1_fingerprint/l2_fingerprint 为空，导致增量比对无法识别历史缺陷。
// 本工具提供确定性回填，保证新旧数据在同一个指纹坐标系内可比。

// MigrateRequest 回填请求
type MigrateRequest struct {
	// 每项：历史缺陷的定位信息（file+line+title+snippet 由调用方从DB取出）
	Items []MigrateItem
}

// MigrateItem 单条历史缺陷
type MigrateItem struct {
	ID          uint   `json:"id"`
	FilePath    string `json:"file_path"`
	LineNumber  string `json:"line_number"`
	Title       string `json:"title"`
	CodeSnippet string `json:"code_snippet"`
}

// MigrateResult 回填结果
type MigrateResult struct {
	Items []MigrateItemResult `json:"items"`
}

// MigrateItemResult 单条回填结果
type MigrateItemResult struct {
	ID            uint   `json:"id"`
	L1Fingerprint string `json:"l1_fingerprint"`
	L2Fingerprint string `json:"l2_fingerprint"`
}

// BackfillFingerprints 为历史缺陷回填L1/L2指纹（确定性，可直接用于DB批量UPDATE）
func BackfillFingerprints(req MigrateRequest) MigrateResult {
	res := MigrateResult{Items: make([]MigrateItemResult, 0, len(req.Items))}
	for _, it := range req.Items {
		fp := FingerprintFinding(it.FilePath, it.Title, it.CodeSnippet, it.LineNumber)
		res.Items = append(res.Items, MigrateItemResult{
			ID:            it.ID,
			L1Fingerprint: fp.L1,
			L2Fingerprint: fp.L2,
		})
	}
	return res
}
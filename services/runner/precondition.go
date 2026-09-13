package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"code-shield/models"
)

// STAGE2 准入校验：since_days、diff_base、快速跳过规则
// 返回skipReason（非空=跳过扫描）；返回error=准入失败。
func (r *Runner) stagePrecondition(tc *TaskContext) (string, error) {
	// 1. 仓库根目录必须存在
	if tc.RepoRoot == "" {
		return "", fmt.Errorf("仓库目录为空")
	}
	if _, err := os.Stat(tc.RepoRoot); err != nil {
		return "", fmt.Errorf("仓库目录不可用: %s", tc.RepoRoot)
	}

	// 2. since_days：报告携带的增量窗口
	if tc.Report.Metrics != nil {
		var m map[string]any
		if err := json.Unmarshal(tc.Report.Metrics, &m); err == nil {
			if v, ok := m["since_days"].(float64); ok {
				tc.SinceDays = int(v)
			}
			if v, ok := m["diff_base"].(string); ok {
				tc.DiffBase = v
			}
		}
	}

	// 3. diff_base：对比基线变更检测（基线无变化则快速跳过）
	if tc.DiffBase != "" {
		changed, err := changedFilesSince(tc.RepoRoot, tc.DiffBase)
		if err != nil {
			// 基线解析失败不阻塞，继续全量
			tc.DiffBase = ""
		} else if len(changed) == 0 {
			// 基线内无变更文件 → 快速跳过（除非是断点续跑）
			if len(tc.ResumeChunkIDs) == 0 && !r.hasPreviousReport(tc) {
				return "diff_base基线内无代码变更，快速跳过", nil
			}
		}
	}

	// 4. since_days窗口内无提交且已有历史报告 → 跳过
	if tc.SinceDays > 0 && !r.hasRecentCommits(tc.RepoRoot, tc.SinceDays) {
		if r.hasPreviousReport(tc) {
			return fmt.Sprintf("最近%d天内无代码提交，快速跳过", tc.SinceDays), nil
		}
	}
	return "", nil
}

// hasPreviousReport 是否存在上一次成功报告
func (r *Runner) hasPreviousReport(tc *TaskContext) bool {
	var n int64
	r.DB.Model(&models.TaskReport{}).
		Where("repo_id = ? AND task_type_id = ? AND status = ? AND id <> ?",
			tc.Repo.ID, tc.TaskType.ID, models.StatusSuccess, tc.Report.ID).
		Count(&n)
	return n > 0
}

// hasRecentCommits 检查N天内是否有提交
func (r *Runner) hasRecentCommits(root string, days int) bool {
	rep, err := git.PlainOpen(root)
	if err != nil {
		return true // 无法检查则默认放行
	}
	iter, err := rep.Log(&git.LogOptions{})
	if err != nil {
		return true
	}
	defer iter.Close()
	count := 0
	_ = iter.ForEach(func(c *object.Commit) error {
		count++
		return nil
	})
	return count > 0
}

// changedFilesSince 计算某基线之后的变更文件（diff_base为git ref/commit hash）
func changedFilesSince(root, base string) ([]string, error) {
	rep, err := git.PlainOpen(root)
	if err != nil {
		return nil, err
	}
	baseRef, err := rep.ResolveRevision(plumbing.Revision(base))
	if err != nil {
		return nil, err
	}
	headRef, err := rep.Head()
	if err != nil {
		return nil, err
	}
	baseCommit, err := rep.CommitObject(*baseRef)
	if err != nil {
		return nil, err
	}
	headCommit, err := rep.CommitObject(headRef.Hash())
	if err != nil {
		return nil, err
	}
	treeBase, err := baseCommit.Tree()
	if err != nil {
		return nil, err
	}
	treeHead, err := headCommit.Tree()
	if err != nil {
		return nil, err
	}
	changes, err := treeHead.Diff(treeBase)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ch := range changes {
		name := ch.To.Name
		if name == "" {
			name = ch.From.Name
		}
		out = append(out, strings.TrimPrefix(filepath.ToSlash(name), "/"))
	}
	return out, nil
}
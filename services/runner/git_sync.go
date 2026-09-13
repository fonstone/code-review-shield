package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"code-shield/models"
)

// STAGE1 Git同步：克隆远程仓库或使用本地目录，拉到storage.repo_dir/<repo_name>
func (r *Runner) stageGitSync(tc *TaskContext) error {
	repo := tc.Repo
	if repo == nil {
		return fmt.Errorf("仓库信息为空")
	}
	cfg := r.Cfg

	// 本地目录仓库：直接用
	if isLocalPath(repo.URL) {
		abs, err := filepath.Abs(repo.URL)
		if err == nil {
			if _, statErr := os.Stat(abs); statErr == nil {
				tc.RepoRoot = abs
				tc.CloneStatus = models.CloneSkipped
				tc.CloneMsg = "本地目录仓库，直接使用"
				r.DB.Model(&models.Repository{}).Where("id = ?", repo.ID).Update("local_path", abs)
				return nil
			}
		}
		return fmt.Errorf("本地仓库路径不可用: %s", repo.URL)
	}

	// 远程仓库：clone/pull
	root := filepath.Join(cfg.Storage.Root, cfg.Storage.RepoDir, safeName(repo.Name))
	tc.RepoRoot = root
	auth := r.gitAuth()

	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		// 已存在：pull
		rep, err := git.PlainOpen(root)
		if err != nil {
			return fmt.Errorf("打开已有仓库失败: %w", err)
		}
		w, err := rep.Worktree()
		if err != nil {
			return err
		}
		opts := &git.PullOptions{RemoteName: "origin", Auth: auth}
		if repo.Branch != "" {
			opts.ReferenceName = plumbing.NewBranchReferenceName(repo.Branch)
		}
		if err := w.Pull(opts); err != nil {
			if err == git.NoErrAlreadyUpToDate {
				tc.CloneStatus = models.CloneSuccess
				tc.CloneMsg = "已是最新"
				r.DB.Model(&models.Repository{}).Where("id = ?", repo.ID).Update("local_path", root)
				return nil
			}
			// pull失败不致命：使用已有工作区
			tc.CloneStatus = models.CloneSuccess
			tc.CloneMsg = fmt.Sprintf("pull警告: %v", err)
			return nil
		}
		tc.CloneStatus = models.CloneSuccess
		tc.CloneMsg = "pull更新完成"
		r.DB.Model(&models.Repository{}).Where("id = ?", repo.ID).Update("local_path", root)
		return nil
	}

	// 全新克隆
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return err
	}
	opts := &git.CloneOptions{URL: repo.URL, Auth: auth, Depth: 1}
	if repo.Branch != "" {
		opts.ReferenceName = plumbing.NewBranchReferenceName(repo.Branch)
		opts.SingleBranch = true
	}
	start := time.Now()
	_, err := git.PlainClone(root, false, opts)
	if err != nil {
		tc.CloneStatus = models.CloneFailed
		tc.CloneMsg = err.Error()
		return fmt.Errorf("克隆仓库失败(%s): %w", time.Since(start).Round(time.Second), err)
	}
	tc.CloneStatus = models.CloneSuccess
	tc.CloneMsg = "克隆完成"
	r.DB.Model(&models.Repository{}).Where("id = ?", repo.ID).Update("local_path", root)
	return nil
}

// gitAuth 从配置环境变量构造认证（GIT_USERNAME/GIT_PASSWORD/GIT_SSH_KEY）
func (r *Runner) gitAuth() transport.AuthMethod {
	user := os.Getenv("GIT_USERNAME")
	pass := os.Getenv("GIT_PASSWORD")
	if user != "" || pass != "" {
		return &http.BasicAuth{Username: user, Password: pass}
	}
	keyPath := os.Getenv("GIT_SSH_KEY")
	if keyPath != "" {
		if a, err := ssh.NewPublicKeysFromFile("git", keyPath, os.Getenv("GIT_SSH_PASSPHRASE")); err == nil {
			return a
		}
	}
	return nil
}

func isLocalPath(u string) bool {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") ||
		strings.HasPrefix(u, "git@") || strings.HasPrefix(u, "ssh://") || strings.HasPrefix(u, "git://") {
		return false
	}
	return true
}

func safeName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "..", "_")
	return name
}

// metricsFailedChunkIDs 从metrics读取失败分片ID
func metricsFailedChunkIDs(m []byte) []string {
	if len(m) == 0 {
		return nil
	}
	var mm map[string]any
	if err := json.Unmarshal(m, &mm); err != nil {
		return nil
	}
	engine, ok := mm["engine"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := engine["failed_chunk_ids"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, id := range raw {
		if s, ok := id.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
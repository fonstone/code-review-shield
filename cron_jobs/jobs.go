// Package cron_jobs 定时任务领域：周期执行的系统任务。
// 包括：槽位健康心跳、队列补投、缺陷生命周期巡检（Scope守卫自动解决）。
package cron_jobs

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"code-shield/models"
	"code-shield/services"
)

// Jobs 定时任务集合
type Jobs struct {
	svc    *services.Services
	stopCh chan struct{}
}

// New 创建定时任务
func New(svc *services.Services) *Jobs {
	return &Jobs{svc: svc, stopCh: make(chan struct{})}
}

// Start 启动全部定时任务
func (j *Jobs) Start() {
	// 槽位健康心跳：30秒
	go j.loop("slot-health", 30*time.Second, j.slotHealth)
	// 队列补投：30秒
	go j.loop("queue-drain", 30*time.Second, j.queueDrain)
	// 缺陷生命周期巡检（Scope守卫自动解决缺失文件缺陷）：6小时
	go j.loop("lifecycle-audit", 6*time.Hour, j.lifecycleAudit)
}

// Stop 停止
func (j *Jobs) Stop() {
	close(j.stopCh)
}

func (j *Jobs) loop(name string, interval time.Duration, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[CRON:%s] panic: %v", name, r)
		}
	}()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// 启动即执行一次
	fn()
	for {
		select {
		case <-j.stopCh:
			return
		case <-ticker.C:
			fn()
		}
	}
}

// slotHealth 槽位健康检测：标记悬挂租约
func (j *Jobs) slotHealth() {
	hung := j.svc.Dispatcher.HealthCheck()
	if hung > 0 {
		log.Printf("[CRON:slot-health] 发现 %d 个悬挂租约", hung)
	}
}

// queueDrain 队列补投：将pending且到期的queue_tasks重新入队
func (j *Jobs) queueDrain() {
	n := j.svc.Tasks.DrainQueue()
	if n > 0 {
		log.Printf("[CRON:queue-drain] 补投 %d 个任务", n)
	}
}

// lifecycleAudit 缺陷生命周期巡检：Scope守卫，自动解决文件已不在仓库范围的open缺陷
func (j *Jobs) lifecycleAudit() {
	if !j.svc.Cfg.Governance.Lifecycle.ScopeGuardEnabled ||
		!j.svc.Cfg.Governance.Lifecycle.AutoResolveMissing {
		return
	}
	var findings []models.AnalysisFinding
	j.svc.DB.Where("status = ? AND diff_status <> ?", models.FindingOpen, models.DiffResolved).
		Limit(500).Find(&findings)
	resolved := 0
	for _, f := range findings {
		// 定位仓库根目录
		var repo models.Repository
		if err := j.svc.DB.First(&repo, f.RepoID).Error; err != nil {
			continue
		}
		if repo.LocalPath == "" {
			continue
		}
		if !fileExists(repo.LocalPath, f.FilePath) {
			now := time.Now()
			j.svc.DB.Model(&models.AnalysisFinding{}).Where("id = ?", f.ID).Updates(map[string]any{
				"status": models.FindingResolved, "diff_status": models.DiffResolved, "updated_at": now,
			})
			resolved++
		}
	}
	if resolved > 0 {
		log.Printf("[CRON:lifecycle-audit] Scope守卫自动解决 %d 个缺失文件缺陷", resolved)
	}
}

func fileExists(root, rel string) bool {
	full := filepath.Join(root, filepath.Clean(rel))
	info, err := os.Stat(full)
	return err == nil && !info.IsDir()
}
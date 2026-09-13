package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"code-shield/models"
)

// 通知：邮件/Webhook告警，notify_threshold触发。
// 当本次扫描NEW缺陷数 ≥ 任务插件notify_threshold时触发。
func (r *Runner) notify(tc *TaskContext) {
	threshold := tc.TaskType.NotifyThreshold
	if threshold <= 0 {
		threshold = 10
	}
	newCount := countDiff(tc, models.DiffNew)
	if newCount < threshold {
		return
	}
	title := fmt.Sprintf("[Code-Shield] %s 扫描告警：%d 个新增缺陷", tc.Repo.Name, newCount)
	body := fmt.Sprintf("仓库: %s\n任务类型: %s\n新增缺陷: %d (阈值%d)\n评分: %d\n严重度: %s\n",
		tc.Repo.Name, tc.TaskType.DisplayName, newCount, threshold, tc.Score, severityBrief(tc))

	// 1. 日志告警（默认通道）
	log.Printf("[NOTIFY] %s\n%s", title, body)

	// 2. Webhook告警（server.webhook_url配置）
	webhook := r.Cfg.Server.WebhookURL
	if webhook == "" {
		return
	}
	payload, _ := json.Marshal(map[string]any{
		"title":          title,
		"text":           body,
		"repo":           tc.Repo.Name,
		"task_type":      tc.TaskType.Name,
		"new_findings":   newCount,
		"threshold":      threshold,
		"score":          tc.Score,
		"task_report_id": tc.Report.ID,
		"timestamp":      time.Now().Format(time.RFC3339),
	})
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest(http.MethodPost, webhook, bytes.NewReader(payload))
	if err != nil {
		log.Printf("[NOTIFY] webhook请求构造失败: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[NOTIFY] webhook发送失败: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("[NOTIFY] webhook返回异常状态: %d", resp.StatusCode)
	}
}

func severityBrief(tc *TaskContext) string {
	sevs := map[string]int{}
	for _, f := range tc.NewFindings {
		sevs[f.Severity]++
	}
	return fmt.Sprintf("CRITICAL=%d HIGH=%d MEDIUM=%d LOW=%d",
		sevs[models.SeverityCritical], sevs[models.SeverityHigh], sevs[models.SeverityMedium], sevs[models.SeverityLow])
}
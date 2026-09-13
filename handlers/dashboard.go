package handlers

import (
	"github.com/gin-gonic/gin"
)

// DashboardHandler 数据看板接口
type DashboardHandler struct {
	*Handler
}

// Summary 全局指标
// GET /api/dashboard/summary
func (h *DashboardHandler) Summary(c *gin.Context) {
	OK(c, h.Svc.Dashboard.Summary())
}

// Trends 缺陷趋势
// GET /api/dashboard/trends?days=14
func (h *DashboardHandler) Trends(c *gin.Context) {
	days := queryInt(c, "days", 14)
	if days > 90 {
		days = 90
	}
	OK(c, h.Svc.Dashboard.Trends(days))
}

// RepoRank 仓库质量排行
// GET /api/dashboard/repo-rank
func (h *DashboardHandler) RepoRank(c *gin.Context) {
	OK(c, h.Svc.Dashboard.RepoRank())
}

// ModelStats AI模型调用统计
// GET /api/dashboard/model-stats
func (h *DashboardHandler) ModelStats(c *gin.Context) {
	OK(c, h.Svc.Dashboard.ModelStats())
}

// RegisterDashboardRoutes 注册看板路由
func (h *DashboardHandler) RegisterDashboardRoutes(rg *gin.RouterGroup) {
	rg.GET("/dashboard/summary", h.Summary)
	rg.GET("/dashboard/trends", h.Trends)
	rg.GET("/dashboard/repo-rank", h.RepoRank)
	rg.GET("/dashboard/model-stats", h.ModelStats)
}
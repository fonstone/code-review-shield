package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// ReportHandler 报告导出/预览接口
type ReportHandler struct {
	*Handler
}

// Preview 报告预览（Markdown）
// GET /api/reports/:id/preview
func (h *ReportHandler) Preview(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法报告ID")
		return
	}
	md, err := h.Svc.Reports.Preview(uint(id))
	if err != nil {
		Fail(c, 404, "报告不存在")
		return
	}
	OK(c, gin.H{"markdown": md})
}

// RegisterReportRoutes 注册报告路由
func (h *ReportHandler) RegisterReportRoutes(rg *gin.RouterGroup) {
	rg.GET("/reports/:id/preview", h.Preview)
}
package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"code-shield/models"
	"code-shield/services"
)

// IssueHandler 缺陷追踪接口：/api/issues/*
type IssueHandler struct {
	*Handler
}

// List 缺陷列表（分页+筛选）
// GET /api/issues
func (h *IssueHandler) List(c *gin.Context) {
	q := services.IssueQuery{
		Page:       queryInt(c, "page", 1),
		PageSize:   queryInt(c, "pageSize", 20),
		Severity:   c.Query("severity"),
		Status:     c.Query("status"),
		DiffStatus: c.Query("diff_status"),
		RepoID:     uint(queryInt(c, "repo_id", 0)),
		TaskTypeID: uint(queryInt(c, "task_type_id", 0)),
		Keyword:    c.Query("keyword"),
	}
	list, total, err := h.Svc.Issues.List(q)
	if err != nil {
		Fail(c, 500, err.Error())
		return
	}
	OK(c, PageResult{List: list, Total: total, Page: q.Page, PageSize: q.PageSize})
}

// UpdateStatus 更新缺陷状态
// PATCH /api/issues/:id/status
func (h *IssueHandler) UpdateStatus(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法ID")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "参数错误: "+err.Error())
		return
	}
	if req.Status != models.FindingOpen && req.Status != models.FindingResolved && req.Status != models.FindingSuppressed {
		Fail(c, 400, "status仅支持 open/resolved/suppressed")
		return
	}
	if err := h.Svc.Issues.UpdateStatus(uint(id), req.Status); err != nil {
		Fail(c, 500, err.Error())
		return
	}
	OK(c, gin.H{"updated": true})
}

// Suppress 标记误报（写入误报记忆库）
// POST /api/issues/:id/suppress
func (h *IssueHandler) Suppress(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法ID")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	operator, _ := c.Get("username")
	op := ""
	if s, ok := operator.(string); ok {
		op = s
	}
	if err := h.Svc.Issues.Suppress(uint(id), req.Reason, op); err != nil {
		Fail(c, 500, err.Error())
		return
	}
	OK(c, gin.H{"suppressed": true})
}

// RegisterIssueRoutes 注册缺陷追踪路由
func (h *IssueHandler) RegisterIssueRoutes(rg *gin.RouterGroup) {
	rg.GET("/issues", h.List)
	rg.PATCH("/issues/:id/status", h.UpdateStatus)
	rg.POST("/issues/:id/suppress", h.Suppress)
}
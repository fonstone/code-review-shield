package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"code-shield/models"
	"code-shield/services"
)

// ScheduleHandler 定时调度接口：/api/schedules/*
type ScheduleHandler struct {
	*Handler
}

// List 调度计划列表
// GET /api/schedules
func (h *ScheduleHandler) List(c *gin.Context) {
	OK(c, h.Svc.Schedules.List())
}

// Create 新增调度计划
// POST /api/schedules
func (h *ScheduleHandler) Create(c *gin.Context) {
	var req struct {
		RepoID      uint `json:"repo_id"`
		TaskTypeID  uint `json:"task_type_id"`
		IntervalMin int  `json:"interval_minutes"`
		SinceDays   int  `json:"since_days"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "参数错误: "+err.Error())
		return
	}
	sc, err := h.Svc.Schedules.Add(req.RepoID, req.TaskTypeID, req.IntervalMin, req.SinceDays)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	OK(c, sc)
}

// Delete 删除调度计划
// DELETE /api/schedules/:id
func (h *ScheduleHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法ID")
		return
	}
	h.Svc.Schedules.Delete(uint(id))
	OK(c, gin.H{"deleted": true})
}

// RegisterScheduleRoutes 注册调度路由
func (h *ScheduleHandler) RegisterScheduleRoutes(rg *gin.RouterGroup) {
	rg.GET("/schedules", h.List)
	rg.POST("/schedules", h.Create)
	rg.DELETE("/schedules/:id", h.Delete)
}

// _ 引用
var _ = models.QueuePending
var _ = services.TaskQuery{}
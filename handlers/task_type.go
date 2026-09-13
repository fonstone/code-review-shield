package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"code-shield/services"
)

// TaskTypeHandler 插件管理接口：/api/task-types/*
type TaskTypeHandler struct {
	*Handler
}

// List 任务类型列表（支持active_only）
// GET /api/task-types
func (h *TaskTypeHandler) List(c *gin.Context) {
	activeOnly := c.Query("active_only") == "true"
	list, err := h.Svc.TaskTypes.List(activeOnly)
	if err != nil {
		Fail(c, 500, err.Error())
		return
	}
	OK(c, list)
}

// Create 创建任务插件
// POST /api/task-types
func (h *TaskTypeHandler) Create(c *gin.Context) {
	var req services.CreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "参数错误: "+err.Error())
		return
	}
	tt, err := h.Svc.TaskTypes.Create(req)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	OK(c, tt)
}

// Update 更新插件配置
// PUT /api/task-types/:id
func (h *TaskTypeHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法ID")
		return
	}
	var req services.CreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "参数错误: "+err.Error())
		return
	}
	tt, err := h.Svc.TaskTypes.Update(uint(id), req)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	OK(c, tt)
}

// Delete 删除插件
// DELETE /api/task-types/:id
func (h *TaskTypeHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法ID")
		return
	}
	if err := h.Svc.TaskTypes.Delete(uint(id)); err != nil {
		Fail(c, 400, err.Error())
		return
	}
	OK(c, gin.H{"deleted": true})
}

// Activate 激活插件
// POST /api/task-types/:id/activate
func (h *TaskTypeHandler) Activate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法ID")
		return
	}
	if err := h.Svc.TaskTypes.Activate(uint(id)); err != nil {
		Fail(c, 500, err.Error())
		return
	}
	OK(c, gin.H{"activated": true})
}

// Deactivate 停用插件
// POST /api/task-types/:id/deactivate
func (h *TaskTypeHandler) Deactivate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法ID")
		return
	}
	if err := h.Svc.TaskTypes.Deactivate(uint(id)); err != nil {
		Fail(c, 500, err.Error())
		return
	}
	OK(c, gin.H{"deactivated": true})
}

// RegisterTaskTypeRoutes 注册插件路由
func (h *TaskTypeHandler) RegisterTaskTypeRoutes(rg *gin.RouterGroup) {
	rg.GET("/task-types", h.List)
	rg.POST("/task-types", h.Create)
	rg.PUT("/task-types/:id", h.Update)
	rg.DELETE("/task-types/:id", h.Delete)
	rg.POST("/task-types/:id/activate", h.Activate)
	rg.POST("/task-types/:id/deactivate", h.Deactivate)
}

// _ 引用services包
var _ = services.CreateReq{}
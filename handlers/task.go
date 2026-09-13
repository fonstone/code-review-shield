package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"code-shield/models"
	"code-shield/services"
)

// TaskHandler 任务接口：/api/tasks/*
type TaskHandler struct {
	*Handler
}

// Trigger 手动触发扫描任务
// POST /api/tasks/trigger
func (h *TaskHandler) Trigger(c *gin.Context) {
	var req services.TriggerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "参数错误: "+err.Error())
		return
	}
	if req.TaskTypeID == 0 {
		Fail(c, 400, "task_type_id必填")
		return
	}
	report, err := h.Svc.Tasks.Trigger(req)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	OK(c, report)
}

// Resume 失败分片恢复续跑
// POST /api/tasks/:id/resume
func (h *TaskHandler) Resume(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法任务ID")
		return
	}
	if err := h.Svc.Tasks.Resume(uint(id)); err != nil {
		Fail(c, 400, err.Error())
		return
	}
	OK(c, gin.H{"resumed": true, "task_id": id})
}

// List 任务列表（分页+多状态筛选）
// GET /api/tasks
func (h *TaskHandler) List(c *gin.Context) {
	q := services.TaskQuery{
		Page:       queryInt(c, "page", 1),
		PageSize:   queryInt(c, "pageSize", 20),
		Status:     c.Query("status"),
		RepoID:     uint(queryInt(c, "repo_id", 0)),
		TaskTypeID: uint(queryInt(c, "task_type_id", 0)),
		StartTime:  c.Query("start_time"),
		EndTime:    c.Query("end_time"),
	}
	list, total, err := h.Svc.Tasks.List(q)
	if err != nil {
		Fail(c, 500, err.Error())
		return
	}
	OK(c, PageResult{List: list, Total: total, Page: q.Page, PageSize: q.PageSize})
}

// Get 任务详情
// GET /api/tasks/:id
func (h *TaskHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法任务ID")
		return
	}
	report, err := h.Svc.Tasks.Get(uint(id))
	if err != nil {
		Fail(c, 404, "任务不存在")
		return
	}
	OK(c, report)
}

// Delete 删除任务报告（running任务禁止删除）
// DELETE /api/tasks/:id
func (h *TaskHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法任务ID")
		return
	}
	if err := h.Svc.Tasks.Delete(uint(id)); err != nil {
		Fail(c, 400, err.Error())
		return
	}
	OK(c, gin.H{"deleted": true})
}

// CancelRunning 批量取消正在执行任务
// POST /api/tasks/cancel-running
func (h *TaskHandler) CancelRunning(c *gin.Context) {
	n := h.Svc.Tasks.CancelRunning()
	OK(c, gin.H{"cancelled": n})
}

// Export 导出报告
// GET /api/tasks/:id/report/export?format=markdown|json|excel
func (h *TaskHandler) Export(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		Fail(c, 400, "非法任务ID")
		return
	}
	format := c.DefaultQuery("format", "markdown")
	if format != "markdown" && format != "json" && format != "excel" {
		Fail(c, 400, "format仅支持 markdown/json/excel")
		return
	}
	data, contentType, filename, err := h.Svc.Reports.Export(uint(id), format)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	c.Header("Content-Disposition", "attachment; filename="+filename)
	c.Data(200, contentType, data)
}

// RegisterTaskRoutes 注册任务路由
func (h *TaskHandler) RegisterTaskRoutes(rg *gin.RouterGroup) {
	rg.POST("/tasks/trigger", h.Trigger)
	rg.GET("/tasks", h.List)
	rg.GET("/tasks/:id", h.Get)
	rg.DELETE("/tasks/:id", h.Delete)
	rg.POST("/tasks/:id/resume", h.Resume)
	rg.POST("/tasks/cancel-running", h.CancelRunning)
	rg.GET("/tasks/:id/report/export", h.Export)
}

func queryInt(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// _ 引用models包
var _ = models.StatusAnalyzing
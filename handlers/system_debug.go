package handlers

import (
	"github.com/gin-gonic/gin"
)

// DebugHandler 系统诊断接口：/api/debug/*（JWT管理员权限保护）
type DebugHandler struct {
	*Handler
}

// Info 系统诊断大盘
// GET /api/debug
func (h *DebugHandler) Info(c *gin.Context) {
	OK(c, h.Svc.Debug.Info())
}

// ResetSlots 槽位校准，释放悬挂租约
// POST /api/debug/reset-slots
func (h *DebugHandler) ResetSlots(c *gin.Context) {
	res := h.Svc.Debug.ResetSlots()
	OK(c, res)
}

// Leases LLMSlotLease租约透视
// GET /api/debug/leases
func (h *DebugHandler) Leases(c *gin.Context) {
	OK(c, h.Svc.Debug.Leases())
}

// Queue 任务队列状态透视
// GET /api/debug/queue
func (h *DebugHandler) Queue(c *gin.Context) {
	OK(c, h.Svc.Debug.Queue())
}

// RegisterDebugRoutes 注册调试路由（管理员）
func (h *DebugHandler) RegisterDebugRoutes(rg *gin.RouterGroup) {
	admin := rg.Group("", h.AdminMiddleware())
	admin.GET("/debug", h.Info)
	admin.POST("/debug/reset-slots", h.ResetSlots)
	admin.GET("/debug/leases", h.Leases)
	admin.GET("/debug/queue", h.Queue)
}
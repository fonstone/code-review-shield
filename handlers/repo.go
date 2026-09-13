package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// RepoHandler 代码仓库管理接口：/api/repos/*
type RepoHandler struct {
	*Handler
}

// List 仓库列表
// GET /api/repos
func (h *RepoHandler) List(c *gin.Context) {
	list, err := h.Svc.Repos.List()
	if err != nil {
		Fail(c, 500, err.Error())
		return
	}
	OK(c, list)
}

// Create 创建仓库
// POST /api/repos
func (h *RepoHandler) Create(c *gin.Context) {
	var req struct {
		Name   string `json:"name"`
		URL    string `json:"url"`
		Branch string `json:"branch"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, 400, "参数错误: "+err.Error())
		return
	}
	repo, err := h.Svc.Repos.Create(req.Name, req.URL, req.Branch)
	if err != nil {
		Fail(c, 400, err.Error())
		return
	}
	OK(c, repo)
}

// RegisterRepoRoutes 注册仓库路由
func (h *RepoHandler) RegisterRepoRoutes(rg *gin.RouterGroup) {
	rg.GET("/repos", h.List)
	rg.POST("/repos", h.Create)
}

// _ 引用
var _ = strconv.ParseUint
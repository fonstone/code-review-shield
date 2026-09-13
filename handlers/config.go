package handlers

import (
	"os"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"code-shield/models"
)

// ConfigHandler 系统配置读写接口
type ConfigHandler struct {
	*Handler
}

// Get 读取系统配置（脱敏：隐藏API Key）
// GET /api/config
func (h *ConfigHandler) Get(c *gin.Context) {
	cfg := h.Svc.Cfg
	sanitized := map[string]any{
		"server":   cfg.Server,
		"storage":  cfg.Storage,
		"database": map[string]any{"driver": cfg.Database.Driver, "host": cfg.Database.Host, "port": cfg.Database.Port, "dbname": cfg.Database.DBName},
		"llm": map[string]any{
			"default_resource": cfg.LLM.DefaultResource,
			"resources":        maskAPIKeys(cfg.LLM.Resources),
		},
		"scanner":    cfg.Scanner,
		"debate":     cfg.Debate,
		"governance": cfg.Governance,
	}
	OK(c, sanitized)
}

// Update 更新系统配置（管理员）
// PUT /api/config
func (h *ConfigHandler) Update(c *gin.Context) {
	var raw map[string]any
	if err := c.ShouldBindJSON(&raw); err != nil {
		Fail(c, 400, "参数错误: "+err.Error())
		return
	}
	// 仅支持更新storage/scanner/debate/governance子集（防误改DB/鉴权配置）
	allowed := map[string]bool{"storage": true, "scanner": true, "debate": true, "governance": true}
	for k := range raw {
		if !allowed[k] {
			Fail(c, 400, "字段不允许修改: "+k)
			return
		}
	}
	path := "config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		path = p
	}
	// 读取现有配置合并
	var existing map[string]any
	if b, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(b, &existing)
	}
	if existing == nil {
		existing = map[string]any{}
	}
	for k, v := range raw {
		existing[k] = v
	}
	out, err := yaml.Marshal(existing)
	if err != nil {
		Fail(c, 500, "配置序列化失败")
		return
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		Fail(c, 500, "配置写入失败: "+err.Error())
		return
	}
	OK(c, gin.H{"updated": true})
}

func maskAPIKeys(resources []models.LLMResourceConfig) []map[string]any {
	out := make([]map[string]any, 0, len(resources))
	for _, r := range resources {
		key := ""
		if len(r.APIKey) > 8 {
			key = r.APIKey[:4] + "****" + r.APIKey[len(r.APIKey)-4:]
		} else if r.APIKey != "" {
			key = "****"
		}
		out = append(out, map[string]any{
			"id": r.ID, "driver": r.Driver, "model": r.Model, "concurrent": r.Concurrent,
			"base_url": r.BaseURL, "api_key": key,
		})
	}
	return out
}

// RegisterConfigRoutes 注册配置路由
func (h *ConfigHandler) RegisterConfigRoutes(rg *gin.RouterGroup) {
	rg.GET("/config", h.Get)
	admin := rg.Group("", h.AdminMiddleware())
	admin.PUT("/config", h.Update)
}
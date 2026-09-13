// Package handlers HTTP API控制器：只做参数校验、鉴权、traceId，业务下沉services门面。
package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"code-shield/models"
	"code-shield/services"
)

// Response 统一返回包装
type Response struct {
	Code    int    `json:"code"`
	Msg     string `json:"msg"`
	Data    any    `json:"data"`
	TraceID string `json:"traceId"`
}

// OK 成功响应
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: 0, Msg: "ok", Data: data, TraceID: traceID(c)})
}

// Fail 失败响应
func Fail(c *gin.Context, httpCode int, msg string) {
	c.JSON(httpCode, Response{Code: httpCode, Msg: msg, Data: nil, TraceID: traceID(c)})
}

// FailCode 带业务码的失败响应
func FailCode(c *gin.Context, httpCode, code int, msg string) {
	c.JSON(httpCode, Response{Code: code, Msg: msg, Data: nil, TraceID: traceID(c)})
}

// PageResult 分页包装
type PageResult struct {
	List     any   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

func traceID(c *gin.Context) string {
	if v, ok := c.Get("traceId"); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Handler 处理器集合
type Handler struct {
	Svc *services.Services
}

// New 创建处理器集合
func New(svc *services.Services) *Handler {
	return &Handler{Svc: svc}
}

// TraceMiddleware traceId中间件
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		tid := c.GetHeader("X-Trace-Id")
		if tid == "" {
			tid = uuid.NewString()
		}
		c.Set("traceId", tid)
		c.Header("X-Trace-Id", tid)
		c.Next()
	}
}

// RecoverMiddleware panic恢复中间件
func RecoverMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] %s %s: %v", c.Request.Method, c.Request.URL.Path, r)
				Fail(c, http.StatusInternalServerError, "服务内部错误")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// JWTClaims 自定义claims
type JWTClaims struct {
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Login 登录接口：POST /api/auth/login
func (h *Handler) Login(c *gin.Context) {
	if !h.Svc.Cfg.Auth.EnableAuth {
		Fail(c, http.StatusForbidden, "鉴权已禁用，无需登录")
		return
	}
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "参数错误: "+err.Error())
		return
	}
	for _, u := range h.Svc.Cfg.Auth.Users {
		if u.Username == req.Username && u.Password == req.Password {
			token, err := h.issueToken(u.Username, u.Role)
			if err != nil {
				Fail(c, http.StatusInternalServerError, "签发令牌失败")
				return
			}
			OK(c, gin.H{"token": token, "username": u.Username, "role": u.Role})
			return
		}
	}
	Fail(c, http.StatusUnauthorized, "用户名或密码错误")
}

// GetUser 取当前用户（前端恢复会话用）
func (h *Handler) GetUser(c *gin.Context) {
	username, _ := c.Get("username")
	role, _ := c.Get("role")
	OK(c, gin.H{"username": username, "role": role})
}

func (h *Handler) issueToken(username, role string) (string, error) {
	ttl := time.Duration(h.Svc.Cfg.Auth.TokenTTL) * time.Hour
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	claims := JWTClaims{
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   username,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "code-shield",
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.Svc.Cfg.Auth.JWTSecret))
}

// AuthMiddleware JWT鉴权中间件
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.Svc.Cfg.Auth.EnableAuth {
			c.Set("username", "anonymous")
			c.Set("role", "admin")
			c.Next()
			return
		}
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			Fail(c, http.StatusUnauthorized, "未登录或令牌缺失")
			c.Abort()
			return
		}
		tokenStr := strings.TrimPrefix(auth, "Bearer ")
		claims := &JWTClaims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(h.Svc.Cfg.Auth.JWTSecret), nil
		})
		if err != nil || !token.Valid {
			Fail(c, http.StatusUnauthorized, "令牌无效或已过期")
			c.Abort()
			return
		}
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

// AdminMiddleware 管理员权限拦截
func (h *Handler) AdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		if role != "admin" {
			Fail(c, http.StatusForbidden, "需要管理员权限")
			c.Abort()
			return
		}
		c.Next()
	}
}

// HashPassword 密码哈希（用户表预留）
func HashPassword(p string) string {
	h := sha256.Sum256([]byte(p))
	return hex.EncodeToString(h[:])
}

// _ 编译期断言：models包被引用
var _ = models.StatusPending
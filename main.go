// Code-Shield 服务入口：加载配置、初始化DB、注册路由、启动worker队列、http服务。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"code-shield/cron_jobs"
	"code-shield/handlers"
	"code-shield/models"
	"code-shield/services"
)

// Version 版本号（Makefile LDFLAGS注入）
var Version = "dev"

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("[INFO] Code-Shield v%s 启动中", Version)

	// 1. 加载配置
	cfg, err := models.LoadConfig("")
	if err != nil {
		log.Fatalf("[FATAL] 配置加载失败: %v", err)
	}
	if err := ensureDirs(cfg); err != nil {
		log.Fatalf("[FATAL] 目录初始化失败: %v", err)
	}

	// 2. 初始化领域服务（DB、调度器、队列、流水线、插件）
	svc, err := services.NewServices(cfg)
	if err != nil {
		log.Fatalf("[FATAL] 服务初始化失败: %v", err)
	}

	// 3. 注册路由
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(handlers.TraceMiddleware(), handlers.RecoverMiddleware())
	engine.GET("/api/health", func(c *gin.Context) {
		handlers.OK(c, gin.H{"version": Version, "time": time.Now().Format(time.RFC3339)})
	})
	engine.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"name": "Code-Shield", "version": Version, "api": "/api"})
	})

	h := handlers.New(svc)
	api := engine.Group("/api")
	api.POST("/auth/login", h.Login)
	api.GET("/auth/me", h.AuthMiddleware(), h.GetUser)
	authed := api.Group("", h.AuthMiddleware())
	(&handlers.TaskHandler{Handler: h}).RegisterTaskRoutes(authed)
	(&handlers.TaskTypeHandler{Handler: h}).RegisterTaskTypeRoutes(authed)
	(&handlers.DebugHandler{Handler: h}).RegisterDebugRoutes(authed)
	(&handlers.ScheduleHandler{Handler: h}).RegisterScheduleRoutes(authed)
	(&handlers.RepoHandler{Handler: h}).RegisterRepoRoutes(authed)
	(&handlers.IssueHandler{Handler: h}).RegisterIssueRoutes(authed)
	(&handlers.ReportHandler{Handler: h}).RegisterReportRoutes(authed)
	(&handlers.ConfigHandler{Handler: h}).RegisterConfigRoutes(authed)
	(&handlers.DashboardHandler{Handler: h}).RegisterDashboardRoutes(authed)

	// 4. 启动worker队列 + 定时任务
	svc.Start()
	cronJobs := cron_jobs.New(svc)
	cronJobs.Start()

	// 5. HTTP服务
	srv := &http.Server{
		Addr:              cfg.Server.Port,
		Handler:           engine,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		log.Printf("[INFO] HTTP服务监听 %s (external: %s)", cfg.Server.Port, cfg.Server.ExternalURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] HTTP服务异常: %v", err)
		}
	}()

	// 6. 优雅退出
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Printf("[INFO] 收到退出信号，正在优雅关闭...")
	svc.Stop()
	cronJobs.Stop()
	ctx, cancel := contextWithTimeout(5 * time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	log.Printf("[INFO] 已退出")
}

// ensureDirs 创建运行时目录
func ensureDirs(cfg *models.Config) error {
	dirs := []string{
		cfg.Storage.Root,
		cfg.Storage.RepoDir,
		cfg.Storage.ReportDir,
		cfg.Storage.LogDir,
	}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}
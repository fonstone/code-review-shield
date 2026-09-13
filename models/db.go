package models

import (
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 任务报告状态
const (
	StatusPending   = "pending"
	StatusAnalyzing = "analyzing"
	StatusSuccess   = "success"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusSkipped   = "skipped"
)

// 克隆状态
const (
	ClonePending = "pending"
	CloneSuccess = "success"
	CloneFailed  = "failed"
	CloneSkipped = "skipped"
)

// 队列任务状态
const (
	QueuePending = "pending"
	QueueRunning = "running"
	QueueDone    = "done"
	QueueFailed  = "failed"
)

// 缺陷状态
const (
	FindingOpen       = "open"
	FindingResolved   = "resolved"
	FindingSuppressed = "suppressed"
)

// 缺陷diff状态（由DiffEngine四级漏斗输出）
const (
	DiffNew      = "NEW"
	DiffExisted  = "EXISTED"
	DiffResolved = "RESOLVED"
	DiffReopened = "REOPENED"
)

// 严重度
const (
	SeverityCritical = "CRITICAL"
	SeverityHigh     = "HIGH"
	SeverityMedium   = "MEDIUM"
	SeverityLow      = "LOW"
)

// InitDB 初始化GORM连接
func InitDB(cfg *DatabaseConfig) (*gorm.DB, error) {
	var dialector gorm.Dialector
	switch cfg.Driver {
	case "postgres", "postgresql", "":
		dsn := cfg.DSN
		if dsn == "" {
			dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=Asia/Shanghai",
				cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName)
		}
		dialector = postgres.Open(dsn)
	case "sqlite", "sqlite3":
		name := cfg.DBName
		if name == "" {
			name = "code_shield.db"
		}
		dialector = sqlite.Open(name)
	default:
		return nil, fmt.Errorf("不支持的数据库驱动: %s", cfg.Driver)
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.SetMaxOpenConns(20)
		sqlDB.SetMaxIdleConns(5)
		sqlDB.SetConnMaxLifetime(time.Hour)
	}
	return db, nil
}

// AutoMigrate 自动迁移（与 db/migration/001_init_schema.sql 保持一致）
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&Repository{},
		&User{},
		&TaskType{},
		&TaskReport{},
		&AnalysisFinding{},
		&QueueTask{},
	)
}
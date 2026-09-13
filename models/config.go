package models

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

// Config 系统全局配置，映射 config.yaml
type Config struct {
	Server     ServerConfig     `mapstructure:"server" json:"server"`
	Storage    StorageConfig    `mapstructure:"storage" json:"storage"`
	Database   DatabaseConfig   `mapstructure:"database" json:"database"`
	LLM        LLMConfig        `mapstructure:"llm" json:"llm"`
	Scanner    ScannerConfig    `mapstructure:"scanner" json:"scanner"`
	Debate     DebateConfig     `mapstructure:"debate" json:"debate"`
	Governance GovernanceConfig `mapstructure:"governance" json:"governance"`
	Auth       AuthConfig       `mapstructure:"auth" json:"auth"`
}

type ServerConfig struct {
	Port        string `mapstructure:"port" json:"port"`
	ExternalURL string `mapstructure:"external_url" json:"external_url"`
	WebhookURL  string `mapstructure:"webhook_url" json:"webhook_url"`
}

type StorageConfig struct {
	Root      string `mapstructure:"root" json:"root"`
	RepoDir   string `mapstructure:"repo_dir" json:"repo_dir"`
	ReportDir string `mapstructure:"report_dir" json:"report_dir"`
	LogDir    string `mapstructure:"log_dir" json:"log_dir"`
}

type DatabaseConfig struct {
	Driver   string `mapstructure:"driver" json:"driver"`
	Host     string `mapstructure:"host" json:"host"`
	Port     int    `mapstructure:"port" json:"port"`
	User     string `mapstructure:"user" json:"user"`
	Password string `mapstructure:"password" json:"password"`
	DBName   string `mapstructure:"dbname" json:"dbname"`
	DSN      string `mapstructure:"dsn" json:"dsn"`
}

// LLMResourceConfig 单个LLM算力资源
type LLMResourceConfig struct {
	ID         string `mapstructure:"id" json:"id"`
	Driver     string `mapstructure:"driver" json:"driver"`
	Model      string `mapstructure:"model" json:"model"`
	Concurrent int    `mapstructure:"concurrent" json:"concurrent"`
	BaseURL    string `mapstructure:"base_url" json:"base_url"`
	APIKey     string `mapstructure:"api_key" json:"api_key"`
	CLIPath    string `mapstructure:"cli_path" json:"cli_path"`
	TimeoutSec int    `mapstructure:"timeout_sec" json:"timeout_sec"`
}

type LLMConfig struct {
	DefaultResource string              `mapstructure:"default_resource" json:"default_resource"`
	Resources       []LLMResourceConfig `mapstructure:"resources" json:"resources"`
}

type ScannerConfig struct {
	WorkerCount  int            `mapstructure:"worker_count" json:"worker_count"`
	MaxQueueSize int            `mapstructure:"max_queue_size" json:"max_queue_size"`
	Analysis     AnalysisConfig `mapstructure:"analysis" json:"analysis"`
}

type AnalysisConfig struct {
	MaxRetries     int `mapstructure:"max_retries" json:"max_retries"`
	RetryBackoffMs int `mapstructure:"retry_backoff_ms" json:"retry_backoff_ms"`
}

type DebateTierConfig struct {
	Resource       string `mapstructure:"resource" json:"resource"`
	TimeoutSeconds int    `mapstructure:"timeout_seconds" json:"timeout_seconds"`
}

type DebateConfig struct {
	Enabled         bool                        `mapstructure:"enabled" json:"enabled"`
	FastPassEnabled bool                        `mapstructure:"fast_pass_enabled" json:"fast_pass_enabled"`
	Tiers           map[string]DebateTierConfig `mapstructure:"tiers" json:"tiers"`
}

type GovernanceConfig struct {
	Fingerprint FingerprintConfig `mapstructure:"fingerprint" json:"fingerprint"`
	Lifecycle   LifecycleConfig   `mapstructure:"lifecycle" json:"lifecycle"`
}

type FingerprintConfig struct {
	Enabled             bool    `mapstructure:"enabled" json:"enabled"`
	SimilarityThreshold float64 `mapstructure:"similarity_threshold" json:"similarity_threshold"`
}

type LifecycleConfig struct {
	ScopeGuardEnabled  bool `mapstructure:"scope_guard_enabled" json:"scope_guard_enabled"`
	AutoResolveMissing bool `mapstructure:"auto_resolve_missing" json:"auto_resolve_missing"`
}

type AuthUserConfig struct {
	Username string `mapstructure:"username" json:"username"`
	Password string `mapstructure:"password" json:"password"`
	Role     string `mapstructure:"role" json:"role"`
}

type AuthConfig struct {
	JWTSecret   string           `mapstructure:"jwt_secret" json:"jwt_secret"`
	TokenTTL    int              `mapstructure:"token_ttl_hours" json:"token_ttl_hours"`
	Users       []AuthUserConfig `mapstructure:"users" json:"users"`
	EnableAuth  bool             `mapstructure:"enable_auth" json:"enable_auth"`
}

// LoadConfig 从文件加载配置，支持 CONFIG_PATH 环境变量覆盖
func LoadConfig(path string) (*Config, error) {
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		path = p
	}
	if path == "" {
		path = "config.yaml"
	}
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("CODE_SHIELD")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("server.port", ":8080")
	v.SetDefault("server.external_url", "http://localhost:8080")
	v.SetDefault("storage.root", ".")
	v.SetDefault("storage.repo_dir", "tmp/repos")
	v.SetDefault("storage.report_dir", "tmp/reports")
	v.SetDefault("storage.log_dir", "logs")
	v.SetDefault("database.driver", "postgres")
	v.SetDefault("database.host", "127.0.0.1")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "code_shield")
	v.SetDefault("database.dbname", "code_shield")
	v.SetDefault("llm.default_resource", "native")
	v.SetDefault("scanner.worker_count", 5)
	v.SetDefault("scanner.max_queue_size", 2000)
	v.SetDefault("scanner.analysis.max_retries", 3)
	v.SetDefault("scanner.analysis.retry_backoff_ms", 2000)
	v.SetDefault("debate.enabled", true)
	v.SetDefault("debate.fast_pass_enabled", true)
	v.SetDefault("governance.fingerprint.enabled", true)
	v.SetDefault("governance.fingerprint.similarity_threshold", 0.85)
	v.SetDefault("governance.lifecycle.scope_guard_enabled", true)
	v.SetDefault("governance.lifecycle.auto_resolve_missing", true)
	v.SetDefault("auth.enable_auth", true)
	v.SetDefault("auth.jwt_secret", "code-shield-dev-secret")
	v.SetDefault("auth.token_ttl_hours", 24)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok && !os.IsNotExist(err) {
			return nil, fmt.Errorf("读取配置文件失败: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置失败: %w", err)
	}
	if cfg.Auth.TokenTTL <= 0 {
		cfg.Auth.TokenTTL = 24
	}
	if len(cfg.Auth.Users) == 0 {
		cfg.Auth.Users = []AuthUserConfig{{Username: "admin", Password: "admin123", Role: "admin"}}
	}
	return &cfg, nil
}

// DefaultResource 返回默认LLM资源
func (c *Config) DefaultResource() *LLMResourceConfig {
	for i := range c.LLM.Resources {
		if c.LLM.Resources[i].ID == c.LLM.DefaultResource {
			return &c.LLM.Resources[i]
		}
	}
	if len(c.LLM.Resources) > 0 {
		return &c.LLM.Resources[0]
	}
	return nil
}

// ResourceByID 按ID查找资源
func (c *Config) ResourceByID(id string) *LLMResourceConfig {
	for i := range c.LLM.Resources {
		if c.LLM.Resources[i].ID == id {
			return &c.LLM.Resources[i]
		}
	}
	return nil
}
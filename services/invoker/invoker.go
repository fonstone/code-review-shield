// Package invoker 提供LLM调用抽象层：AIInvoker接口 + 多后端驱动注册表。
// 支持的驱动：native(OpenAI兼容HTTP) / claude / opencode / agy / codex(CLI进程组调用)。
// 本包不依赖engines、runner等上层业务，仅依赖models中的纯配置结构体。
package invoker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// AIRequest LLM调用请求（纯内存结构体）
type AIRequest struct {
	Model       string        // 模型名
	Prompt      string        // 用户提示词
	System      string        // 系统提示词
	MaxTokens   int           // 最大输出token
	Temperature float64       // 温度
	Timeout     time.Duration // 调用超时
	Stream      bool          // 是否流式
}

// Usage token用量统计
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// AIResponse LLM调用响应
type AIResponse struct {
	Content    string        `json:"content"`
	Usage      Usage         `json:"usage"`
	DurationMs int64         `json:"duration_ms"`
	Model      string        `json:"model"`
	RawError   string        `json:"raw_error,omitempty"`
}

// ErrNoContent 空响应
var ErrNoContent = errors.New("LLM返回内容为空")

// AIInvoker LLM执行器接口
type AIInvoker interface {
	// Name 返回驱动名称
	Name() string
	// Model 返回当前模型名
	Model() string
	// Invoke 执行一次LLM调用
	Invoke(ctx context.Context, req AIRequest) (*AIResponse, error)
}

// ResourceConfig LLM资源静态配置（纯配置，无DB依赖）
type ResourceConfig struct {
	ID         string
	Driver     string
	Model      string
	Concurrent int
	BaseURL    string
	APIKey     string
	CLIPath    string
	TimeoutSec int
}

// DriverFactory 驱动工厂：根据资源配置创建驱动实例
type DriverFactory func(res ResourceConfig) (AIInvoker, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]DriverFactory{}
)

// RegisterDriver 注册驱动工厂
func RegisterDriver(name string, f DriverFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = f
}

// SupportedDrivers 列出已注册驱动
func SupportedDrivers() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	return out
}

// CreateInvoker 按配置创建驱动实例（自动选择driver，未知driver报错）
func CreateInvoker(res ResourceConfig) (AIInvoker, error) {
	registryMu.RLock()
	f, ok := registry[res.Driver]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("未知LLM驱动: %s (已注册: %v)", res.Driver, SupportedDrivers())
	}
	return f(res)
}

// WithTimeout 带超时的Invoke包装
func WithTimeout(inv AIInvoker, d time.Duration) AIInvoker {
	return &timeoutInvoker{inner: inv, timeout: d}
}

type timeoutInvoker struct {
	inner   AIInvoker
	timeout time.Duration
}

func (t *timeoutInvoker) Name() string  { return t.inner.Name() }
func (t *timeoutInvoker) Model() string { return t.inner.Model() }

func (t *timeoutInvoker) Invoke(ctx context.Context, req AIRequest) (*AIResponse, error) {
	if t.timeout > 0 && req.Timeout == 0 {
		req.Timeout = t.timeout
	}
	return t.inner.Invoke(ctx, req)
}

func init() {
	RegisterDriver("native", NewNativeInvoker)
	RegisterDriver("claude", NewClaudeInvoker)
	RegisterDriver("opencode", NewOpenCodeInvoker)
	RegisterDriver("agy", NewAgyInvoker)
	RegisterDriver("codex", NewCodexInvoker)
	RegisterDriver("fake", NewFakeInvoker)
}
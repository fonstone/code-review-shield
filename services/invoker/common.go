package invoker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// cliInvoker 通用CLI驱动基类：进程组管理、超时捕获、流式包装
type cliInvoker struct {
	name       string
	model      string
	cliPath    string
	argsPrefix []string
	timeout    time.Duration
}

func (c *cliInvoker) Name() string  { return c.name }
func (c *cliInvoker) Model() string { return c.model }

// execCLI 执行CLI命令，返回stdout与执行耗时。cmd context超时后强制杀死进程组。
func execCLI(ctx context.Context, name string, args ...string) (string, time.Duration, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	dur := time.Since(start)
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if ctx.Err() != nil {
			msg = fmt.Sprintf("命令超时(%s): %s", dur, msg)
		}
		return "", dur, errors.New(msg)
	}
	return strings.TrimSpace(stdout.String()), dur, nil
}

// execCLIPipe 执行CLI命令并捕获stdout/stderr流式输出（用于调试与日志）
func execCLIPipe(ctx context.Context, name string, args ...string) (string, string, time.Duration, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	dur := time.Since(start)
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		if ctx.Err() != nil {
			msg = fmt.Sprintf("命令超时(%s): %s", dur, msg)
		}
		return stdout.String(), stderr.String(), dur, errors.New(msg)
	}
	return stdout.String(), stderr.String(), dur, nil
}

// invokeCLI 统一CLI调用入口：拼接参数、处理超时、包装AIResponse
func (c *cliInvoker) invokeCLI(ctx context.Context, req AIRequest, extraArgs ...string) (*AIResponse, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = c.timeout
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := append([]string{}, c.argsPrefix...)
	args = append(args, extraArgs...)
	args = append(args, req.Prompt)

	out, dur, err := execCLI(callCtx, c.cliPath, args...)
	if err != nil {
		return nil, err
	}
	return &AIResponse{
		Content:    strings.TrimSpace(out),
		DurationMs: dur.Milliseconds(),
		Model:      c.model,
	}, nil
}

// FakeInvoker 测试用假驱动：注册名为"fake"，可编程响应
type FakeInvoker struct {
	model   string
	mu      sync.Mutex
	handler func(req AIRequest) (string, error)
	calls   int
}

// NewFakeInvoker 创建假驱动（测试用）
func NewFakeInvoker(res ResourceConfig) (AIInvoker, error) {
	return &FakeInvoker{model: res.Model}, nil
}

func (f *FakeInvoker) Name() string  { return "fake" }
func (f *FakeInvoker) Model() string { return f.model }

// SetHandler 设置响应处理器（测试注入）
func (f *FakeInvoker) SetHandler(h func(req AIRequest) (string, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = h
}

// CallCount 已调用次数
func (f *FakeInvoker) CallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *FakeInvoker) Invoke(ctx context.Context, req AIRequest) (*AIResponse, error) {
	f.mu.Lock()
	f.calls++
	h := f.handler
	f.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	start := time.Now()
	if h == nil {
		h = func(AIRequest) (string, error) { return "", nil }
	}
	content, err := h(req)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(content) == "" {
		return nil, ErrNoContent
	}
	return &AIResponse{
		Content:    content,
		DurationMs: time.Since(start).Milliseconds(),
		Model:      f.model,
	}, nil
}

// _ 确保 os 被引用（保留给未来流式日志实现）
var _ = os.Stdout
var _ = io.Discard
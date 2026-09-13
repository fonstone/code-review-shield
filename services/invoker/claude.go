package invoker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Claude CLI驱动：调用 claude -p "prompt" 无头模式
type claudeInvoker struct {
	*cliInvoker
}

// NewClaudeInvoker 创建Claude CLI驱动
func NewClaudeInvoker(res ResourceConfig) (AIInvoker, error) {
	path := res.CLIPath
	if path == "" {
		path = "claude"
	}
	if _, err := exec.LookPath(path); err != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, fmt.Errorf("Claude CLI不可用: %w", err)
		}
	}
	timeout := 0
	if res.TimeoutSec > 0 {
		timeout = res.TimeoutSec
	}
	return &claudeInvoker{cliInvoker: &cliInvoker{
		name:    "claude",
		model:   res.Model,
		cliPath: path,
		timeout: time.Duration(timeout) * time.Second,
	}}, nil
}

func (c *claudeInvoker) Invoke(ctx context.Context, req AIRequest) (*AIResponse, error) {
	args := []string{"-p", "--output-format", "text"}
	if req.System != "" {
		prompt := fmt.Sprintf("<system>\n%s\n</system>\n\n%s", req.System, req.Prompt)
		return c.invokeCLI(ctx, req, append(args, prompt)...)
	}
	return c.invokeCLI(ctx, req, args...)
}
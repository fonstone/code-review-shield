package invoker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// OpenCode CLI驱动：调用 opencode run "prompt"
type opencodeInvoker struct {
	*cliInvoker
}

// NewOpenCodeInvoker 创建OpenCode CLI驱动
func NewOpenCodeInvoker(res ResourceConfig) (AIInvoker, error) {
	path := res.CLIPath
	if path == "" {
		path = "opencode"
	}
	if _, err := exec.LookPath(path); err != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, fmt.Errorf("OpenCode CLI不可用: %w", err)
		}
	}
	timeout := 0
	if res.TimeoutSec > 0 {
		timeout = res.TimeoutSec
	}
	return &opencodeInvoker{cliInvoker: &cliInvoker{
		name:    "opencode",
		model:   res.Model,
		cliPath: path,
		timeout: time.Duration(timeout) * time.Second,
	}}, nil
}

func (c *opencodeInvoker) Invoke(ctx context.Context, req AIRequest) (*AIResponse, error) {
	args := []string{"run", "--print-logs=false"}
	if req.System != "" {
		prompt := fmt.Sprintf("System: %s\n\n%s", req.System, req.Prompt)
		return c.invokeCLI(ctx, req, append(args, prompt)...)
	}
	return c.invokeCLI(ctx, req, args...)
}
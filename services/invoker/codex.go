package invoker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Codex CLI驱动：调用 codex exec "prompt"
type codexInvoker struct {
	*cliInvoker
}

// NewCodexInvoker 创建Codex CLI驱动
func NewCodexInvoker(res ResourceConfig) (AIInvoker, error) {
	path := res.CLIPath
	if path == "" {
		path = "codex"
	}
	if _, err := exec.LookPath(path); err != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, fmt.Errorf("Codex CLI不可用: %w", err)
		}
	}
	timeout := 0
	if res.TimeoutSec > 0 {
		timeout = res.TimeoutSec
	}
	return &codexInvoker{cliInvoker: &cliInvoker{
		name:    "codex",
		model:   res.Model,
		cliPath: path,
		timeout: time.Duration(timeout) * time.Second,
	}}, nil
}

func (c *codexInvoker) Invoke(ctx context.Context, req AIRequest) (*AIResponse, error) {
	args := []string{"exec", "--json", "0"}
	if req.System != "" {
		prompt := fmt.Sprintf("System: %s\n\n%s", req.System, req.Prompt)
		return c.invokeCLI(ctx, req, append(args, prompt)...)
	}
	return c.invokeCLI(ctx, req, args...)
}
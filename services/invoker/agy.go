package invoker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Agy CLI驱动：调用 agy -p "prompt"（自定义CLI约定）
type agyInvoker struct {
	*cliInvoker
}

// NewAgyInvoker 创建Agy CLI驱动
func NewAgyInvoker(res ResourceConfig) (AIInvoker, error) {
	path := res.CLIPath
	if path == "" {
		path = "agy"
	}
	if _, err := exec.LookPath(path); err != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			return nil, fmt.Errorf("Agy CLI不可用: %w", err)
		}
	}
	timeout := 0
	if res.TimeoutSec > 0 {
		timeout = res.TimeoutSec
	}
	return &agyInvoker{cliInvoker: &cliInvoker{
		name:    "agy",
		model:   res.Model,
		cliPath: path,
		timeout: time.Duration(timeout) * time.Second,
	}}, nil
}

func (c *agyInvoker) Invoke(ctx context.Context, req AIRequest) (*AIResponse, error) {
	args := []string{"-p", "--output", "text"}
	if req.System != "" {
		prompt := fmt.Sprintf("<system>\n%s\n</system>\n\n%s", req.System, req.Prompt)
		return c.invokeCLI(ctx, req, append(args, prompt)...)
	}
	return c.invokeCLI(ctx, req, args...)
}
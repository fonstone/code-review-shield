package invoker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// nativeInvoker Native HTTP驱动：OpenAI兼容 Chat Completions API
type nativeInvoker struct {
	model   string
	baseURL string
	apiKey  string
	client  *http.Client
	timeout time.Duration
}

// NewNativeInvoker 创建native驱动
func NewNativeInvoker(res ResourceConfig) (AIInvoker, error) {
	if res.BaseURL == "" {
		return nil, fmt.Errorf("native驱动缺少base_url")
	}
	timeout := time.Duration(res.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &nativeInvoker{
		model:   res.Model,
		baseURL: strings.TrimRight(res.BaseURL, "/"),
		apiKey:  res.APIKey,
		client:  &http.Client{Timeout: timeout},
		timeout: timeout,
	}, nil
}

func (n *nativeInvoker) Name() string  { return "native" }
func (n *nativeInvoker) Model() string { return n.model }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message chatMessage `json:"message"`
		Text    string      `json:"text"`
	} `json:"choices"`
	Usage chatUsage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Invoke 执行OpenAI兼容调用
func (n *nativeInvoker) Invoke(ctx context.Context, req AIRequest) (*AIResponse, error) {
	model := req.Model
	if model == "" {
		model = n.model
	}
	payload := chatRequest{
		Model:    model,
		Messages: []chatMessage{{Role: "user", Content: req.Prompt}},
		Stream:   req.Stream,
	}
	if req.System != "" {
		payload.Messages = append([]chatMessage{{Role: "system", Content: req.System}}, payload.Messages...)
	}
	if req.MaxTokens > 0 {
		payload.MaxTokens = req.MaxTokens
	}
	if req.Temperature > 0 {
		payload.Temperature = req.Temperature
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	timeout := req.Timeout
	if timeout <= 0 {
		timeout = n.timeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, n.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if n.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+n.apiKey)
	}
	resp, err := n.client.Do(httpReq)
	if err != nil {
		if callCtx.Err() != nil {
			return nil, fmt.Errorf("LLM调用超时(%s)", time.Since(start).Round(time.Second))
		}
		return nil, fmt.Errorf("LLM调用失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil {
		return nil, fmt.Errorf("LLM响应解析失败(%d): %s", resp.StatusCode, truncate(string(raw), 300))
	}
	if cr.Error != nil {
		return nil, fmt.Errorf("LLM错误(%s): %s", cr.Error.Type, cr.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM HTTP %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}
	content := ""
	if len(cr.Choices) > 0 {
		content = cr.Choices[0].Message.Content
		if content == "" {
			content = cr.Choices[0].Text
		}
	}
	if strings.TrimSpace(content) == "" {
		return nil, ErrNoContent
	}
	return &AIResponse{
		Content:    strings.TrimSpace(content),
		Usage:      Usage{PromptTokens: cr.Usage.PromptTokens, CompletionTokens: cr.Usage.CompletionTokens, TotalTokens: cr.Usage.TotalTokens},
		DurationMs: time.Since(start).Milliseconds(),
		Model:      model,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
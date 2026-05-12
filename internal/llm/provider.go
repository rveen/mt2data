// Package llm provides LLM backend abstractions for narrow fallback use.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Provider sends a text prompt to an LLM and returns the raw text response.
type Provider interface {
	Call(ctx context.Context, systemPrompt, userMsg string) (string, error)
}

// NewAutoProvider returns the first available provider by checking environment variables:
// ANTHROPIC_API_KEY → claude, OPENAI_API_KEY → openai.
// Returns nil (no error) if neither key is set.
func NewAutoProvider(modelOverride string) (Provider, error) {
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		return NewProvider("claude", modelOverride)
	}
	if os.Getenv("OPENAI_API_KEY") != "" {
		return NewProvider("openai", modelOverride)
	}
	return nil, nil
}

// NewProvider constructs a Provider for the named backend ("claude" or "openai").
func NewProvider(name, modelOverride string) (Provider, error) {
	switch strings.ToLower(name) {
	case "", "claude":
		key := os.Getenv("ANTHROPIC_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
		}
		model := modelOverride
		if model == "" {
			model = "claude-sonnet-4-6"
		}
		return &claudeProvider{apiKey: key, model: model}, nil
	case "openai":
		key := os.Getenv("OPENAI_API_KEY")
		if key == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY is not set")
		}
		model := modelOverride
		if model == "" {
			model = "gpt-4o"
		}
		return &openaiProvider{apiKey: key, model: model}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (use claude or openai)", name)
	}
}

// StripCodeFence removes a leading ```json or ``` fence and its closing ``` from
// an LLM response so the remainder can be parsed as plain JSON.
func StripCodeFence(s string) string {
	if after, ok := strings.CutPrefix(s, "```json"); ok {
		s = strings.TrimSpace(after)
	} else if after, ok := strings.CutPrefix(s, "```"); ok {
		s = strings.TrimSpace(after)
	} else {
		return s
	}
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = strings.TrimSpace(s[:idx])
	}
	return s
}

// ---- Claude backend ----

type claudeProvider struct {
	apiKey string
	model  string
}

func (p *claudeProvider) Call(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	body := map[string]any{
		"model":      p.model,
		"max_tokens": 4096,
		"system":     systemPrompt,
		"messages":   []map[string]any{{"role": "user", "content": userMsg}},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.anthropic.com/v1/messages", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude API %d: %s", resp.StatusCode, respBytes)
	}
	var apiResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	return sb.String(), nil
}

// ---- OpenAI backend ----

type openaiProvider struct {
	apiKey string
	model  string
}

func (p *openaiProvider) Call(ctx context.Context, systemPrompt, userMsg string) (string, error) {
	body := map[string]any{
		"model":      p.model,
		"max_tokens": 4096,
		"messages": []map[string]any{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userMsg},
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.openai.com/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openai API %d: %s", resp.StatusCode, respBytes)
	}
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return "", err
	}
	if len(apiResp.Choices) == 0 {
		return "", fmt.Errorf("openai API returned no choices")
	}
	return apiResp.Choices[0].Message.Content, nil
}

package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SimpleCapability struct {
	Supported bool `json:"supported"`
}

type EffortCapabilities struct {
	Supported bool             `json:"supported"`
	Low       SimpleCapability `json:"low"`
	Medium    SimpleCapability `json:"medium"`
	High      SimpleCapability `json:"high"`
	XHigh     SimpleCapability `json:"xhigh"`
	Max       SimpleCapability `json:"max"`
}

type ThinkingCapabilities struct {
	Supported bool                     `json:"supported"`
	Types     ThinkingTypeCapabilities `json:"types"`
}

type ThinkingTypeCapabilities struct {
	Enabled  SimpleCapability `json:"enabled"`
	Adaptive SimpleCapability `json:"adaptive"`
}

type ContextManagementCapabilities struct {
	Supported             bool             `json:"supported"`
	ClearToolUses20250919 SimpleCapability `json:"clear_tool_uses_20250919"`
	ClearThinking20251015 SimpleCapability `json:"clear_thinking_20251015"`
	Compact20260112       SimpleCapability `json:"compact_20260112"`
}

type ModelCapabilities struct {
	Batch             SimpleCapability              `json:"batch"`
	Citations         SimpleCapability              `json:"citations"`
	CodeExecution     SimpleCapability              `json:"code_execution"`
	ContextManagement ContextManagementCapabilities `json:"context_management"`
	Effort            EffortCapabilities            `json:"effort"`
	ImageInput        SimpleCapability              `json:"image_input"`
	PDFInput          SimpleCapability              `json:"pdf_input"`
	StructuredOutputs SimpleCapability              `json:"structured_outputs"`
	Thinking          ThinkingCapabilities          `json:"thinking"`
}

type ModelInfo struct {
	ID             string            `json:"id"`
	DisplayName    string            `json:"display_name"`
	CreatedAt      string            `json:"created_at"`
	MaxInputTokens int               `json:"max_input_tokens"`
	MaxTokens      int               `json:"max_tokens"`
	Capabilities   ModelCapabilities `json:"capabilities"`
}

type modelsResponse struct {
	Data    []ModelInfo `json:"data"`
	HasMore bool        `json:"has_more"`
	FirstID string      `json:"first_id"`
	LastID  string      `json:"last_id"`
}

// IsTextChatModel reports whether the given model ID is an OpenAI text/chat model.
// It returns true for IDs that start with "gpt-<digit>" (e.g. "gpt-5.5", "gpt-5.4",
// "gpt-5.4-mini") or "gpt-o<digit>" (e.g. "gpt-4o", "gpt-o3", "gpt-4o-mini").
// Comparison is case-insensitive. Models without the "gpt-" prefix (e.g. "o3-mini")
// and non-text models (e.g. "dall-e-3") are excluded.
func IsTextChatModel(id string) bool {
	id = strings.ToLower(id)
	rest, ok := strings.CutPrefix(id, "gpt-")
	if !ok || rest == "" {
		return false
	}
	if rest[0] >= '0' && rest[0] <= '9' {
		return true
	}
	return len(rest) >= 2 && rest[0] == 'o' && rest[1] >= '0' && rest[1] <= '9'
}

func FetchModels(ctx context.Context, provider, apiKey, baseURL string) ([]ModelInfo, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL(provider)
	}

	reqURL := strings.TrimSuffix(baseURL, "/") + "/models"

	var models []ModelInfo
	var lastErr error

	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<(attempt-1)) * time.Second):
			}
		}

		models, lastErr = fetchModelsOnce(ctx, reqURL, provider, apiKey)
		if lastErr == nil {
			return models, nil
		}
	}

	return nil, fmt.Errorf("fetching models (3 attempts): %w", lastErr)
}

func fetchModelsOnce(ctx context.Context, url, provider, apiKey string) ([]ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	switch provider {
	case ProviderAnthropic:
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	default:
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("API key rejected (%d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID != "" {
			models = append(models, m)
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned by API")
	}

	return models, nil
}

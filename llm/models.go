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
	Minimal   SimpleCapability `json:"minimal"`
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
	ThinkingLevels    []string                      `json:"-"`
	EffortLevels      []string                      `json:"-"`
}

func (c *ModelCapabilities) UnmarshalJSON(data []byte) error {
	// llmdetails.json uses booleans for simple caps and string arrays for
	// thinking/effort/context_management. Parse that format here.
	*c = ModelCapabilities{}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	for key, val := range raw {
		switch key {
		case "batch":
			json.Unmarshal(val, &c.Batch.Supported)
		case "citations":
			json.Unmarshal(val, &c.Citations.Supported)
		case "code_execution":
			json.Unmarshal(val, &c.CodeExecution.Supported)
		case "image_input":
			json.Unmarshal(val, &c.ImageInput.Supported)
		case "pdf_input":
			json.Unmarshal(val, &c.PDFInput.Supported)
		case "structured_outputs":
			json.Unmarshal(val, &c.StructuredOutputs.Supported)

		case "thinking":
			var arr []string
			if err := json.Unmarshal(val, &arr); err != nil {
				continue
			}
			c.ThinkingLevels = arr
			for _, s := range arr {
				switch s {
				case "enabled":
					c.Thinking.Types.Enabled.Supported = true
					c.Thinking.Supported = true
				case "adaptive":
					c.Thinking.Types.Adaptive.Supported = true
					c.Thinking.Supported = true
				case "low":
					c.Effort.Low.Supported = true
					c.Effort.Supported = true
				case "medium":
					c.Effort.Medium.Supported = true
					c.Effort.Supported = true
				case "high":
					c.Effort.High.Supported = true
					c.Effort.Supported = true
				case "xhigh":
					c.Effort.XHigh.Supported = true
					c.Effort.Supported = true
				case "max":
					c.Effort.Max.Supported = true
					c.Effort.Supported = true
				case "minimal":
					c.Effort.Minimal.Supported = true
					c.Effort.Supported = true
				}
			}

		case "effort":
			var arr []string
			if err := json.Unmarshal(val, &arr); err != nil {
				continue
			}
			c.EffortLevels = arr
			for _, s := range arr {
				switch s {
				case "minimal":
					c.Effort.Minimal.Supported = true
					c.Effort.Supported = true
				case "low":
					c.Effort.Low.Supported = true
					c.Effort.Supported = true
				case "medium":
					c.Effort.Medium.Supported = true
					c.Effort.Supported = true
				case "high":
					c.Effort.High.Supported = true
					c.Effort.Supported = true
				case "xhigh":
					c.Effort.XHigh.Supported = true
					c.Effort.Supported = true
				case "max":
					c.Effort.Max.Supported = true
					c.Effort.Supported = true
				}
			}

		case "context_management":
			var arr []string
			if err := json.Unmarshal(val, &arr); err != nil {
				continue
			}
			for _, s := range arr {
				c.ContextManagement.Supported = true
				switch s {
				case "clear_tool_uses_20250919":
					c.ContextManagement.ClearToolUses20250919.Supported = true
				case "clear_thinking_20251015":
					c.ContextManagement.ClearThinking20251015.Supported = true
				case "compact_20260112":
					c.ContextManagement.Compact20260112.Supported = true
				}
			}
		}
	}

	return nil
}

type ModelInfo struct {
	ID             string            `json:"id"`
	Slug           string            `json:"slug"`
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
// "gpt-5.4-mini"), "gpt-o<digit>" (e.g. "gpt-4o", "gpt-o3", "gpt-4o-mini"), or
// bare "o<digit>" (e.g. "o1", "o3-mini", "o4-mini").
// Comparison is case-insensitive. Non-text models (e.g. "dall-e-3") are excluded.
func IsTextChatModel(id string) bool {
	id = strings.ToLower(id)
	rest, ok := strings.CutPrefix(id, "gpt-")
	if ok && rest != "" {
		if rest[0] >= '0' && rest[0] <= '9' {
			return true
		}
		return len(rest) >= 2 && rest[0] == 'o' && rest[1] >= '0' && rest[1] <= '9'
	}
	if len(id) >= 2 && id[0] == 'o' && id[1] > '0' && id[1] <= '9' {
		return true
	}
	return false
}

const geminiModelsURL = "https://generativelanguage.googleapis.com/v1beta/models"

func FetchModels(ctx context.Context, provider, apiKey, baseURL string) ([]ModelInfo, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL(provider)
	}

	reqURL := strings.TrimSuffix(baseURL, "/") + "/models"
	if provider == ProviderGemini {
		reqURL = geminiModelsURL
	}

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

type geminiModelEntry struct {
	Name               string   `json:"name"`
	DisplayName        string   `json:"displayName"`
	InputTokenLimit    int      `json:"inputTokenLimit"`
	OutputTokenLimit   int      `json:"outputTokenLimit"`
	SupportedMethods   []string `json:"supportedGenerationMethods"`
}

type geminiModelsResponse struct {
	Models []geminiModelEntry `json:"models"`
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
	case ProviderGemini:
		req.Header.Set("x-goog-api-key", apiKey)
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

	if provider == ProviderGemini {
		return parseGeminiModelsResponse(resp.Body)
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

func parseGeminiModelsResponse(r io.Reader) ([]ModelInfo, error) {
	var result geminiModelsResponse
	if err := json.NewDecoder(r).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding Gemini models response: %w", err)
	}

	models := make([]ModelInfo, 0, len(result.Models))
	for _, m := range result.Models {
		if !strings.HasPrefix(m.Name, "models/") {
			continue
		}
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" {
			continue
		}
		if !isGeminiChatModel(id, m.SupportedMethods) {
			continue
		}
		models = append(models, ModelInfo{
			ID:             id,
			DisplayName:    m.DisplayName,
			MaxInputTokens: m.InputTokenLimit,
			MaxTokens:      m.OutputTokenLimit,
		})
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no chat models returned by Gemini API")
	}

	return models, nil
}

func isGeminiChatModel(id string, methods []string) bool {
	if !strings.HasPrefix(id, "gemini-") {
		return false
	}
	for _, m := range methods {
		if m == "generateContent" {
			return true
		}
	}
	return false
}

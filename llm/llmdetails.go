package llm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

//go:embed llmdetails.json
var embeddedLLMDetails []byte

type llmDetailsFile struct {
	OpenAI    providerModels `json:"OpenAI"`
	Anthropic providerModels `json:"Anthropic"`
}

type providerModels struct {
	Data []ModelInfo `json:"data"`
}

// FetchLLMDetails fetches llmdetails.json from GitHub, falling back to the
// embedded copy on failure. Returns a map keyed by model ID for fast lookup.
func FetchLLMDetails(ctx context.Context) (map[string]ModelInfo, error) {
	data, err := fetchRemoteLLMDetails(ctx)
	if err != nil {
		data = embeddedLLMDetails
	}

	return parseLLMDetails(data)
}

func fetchRemoteLLMDetails(ctx context.Context) ([]byte, error) {
	url := "https://raw.githubusercontent.com/sillygru/gurtcli/main/llm/llmdetails.json"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	return body, nil
}

func parseLLMDetails(data []byte) (map[string]ModelInfo, error) {
	var file llmDetailsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parsing llmdetails: %w", err)
	}

	result := make(map[string]ModelInfo)
	for _, m := range file.OpenAI.Data {
		if m.ID != "" {
			result[m.ID] = m
		}
	}
	for _, m := range file.Anthropic.Data {
		if m.ID != "" {
			result[m.ID] = m
		}
	}
	return result, nil
}

// EnrichModels overlays capability data from llmdetails onto API-fetched models.
// Only the Capabilities, MaxInputTokens, and MaxTokens fields are enriched.
func EnrichModels(apiModels []ModelInfo, details map[string]ModelInfo, provider string) []ModelInfo {
	enriched := make([]ModelInfo, len(apiModels))
	for i, m := range apiModels {
		enriched[i] = m
		if d, ok := details[m.ID]; ok {
			enriched[i].Capabilities = d.Capabilities
			if d.MaxInputTokens > 0 {
				enriched[i].MaxInputTokens = d.MaxInputTokens
			}
			if d.MaxTokens > 0 {
				enriched[i].MaxTokens = d.MaxTokens
			}
			if d.DisplayName != "" {
				enriched[i].DisplayName = d.DisplayName
			}
		}
	}
	return enriched
}

func hasNoneThinking(levels []string) bool {
	for _, s := range levels {
		if s == "none" {
			return true
		}
	}
	return false
}

func IsProviderOpenAI(provider string) bool {
	return provider == ProviderOpenAI || provider == ProviderCustom
}

func (e EffortCapabilities) EffortLevels() []string {
	var levels []string
	if e.Low.Supported {
		levels = append(levels, "low")
	}
	if e.Medium.Supported {
		levels = append(levels, "medium")
	}
	if e.High.Supported {
		levels = append(levels, "high")
	}
	if e.XHigh.Supported {
		levels = append(levels, "xhigh")
	}
	if e.Max.Supported {
		levels = append(levels, "max")
	}
	return levels
}

func (m ModelInfo) HasThinking() bool {
	return m.Capabilities.Thinking.Supported
}

func (m ModelInfo) HasEffort() bool {
	return m.Capabilities.Effort.Supported
}

func (m ModelInfo) ThinkingEffortLevels() []string {
	return m.Capabilities.Effort.EffortLevels()
}

func (m ModelInfo) ThinkingHasNone() bool {
	return hasNoneThinking(m.Capabilities.ThinkingLevels)
}

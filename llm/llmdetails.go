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
	Gemini    providerModels `json:"Gemini"`
	Others    providerModels `json:"Others"`
}

type providerModels struct {
	Data []ModelInfo `json:"data"`
}

// FetchLLMDetails fetches llmdetails.json from GitHub, falling back to the
// embedded copy on failure. If forceLocal is true, it skips the remote fetch
// and uses the embedded copy directly.
func FetchLLMDetails(ctx context.Context, forceLocal bool) (map[string]ModelInfo, error) {
	if forceLocal {
		LogDebug("FetchLLMDetails: force-local enabled, using embedded")
		return parseLLMDetails(embeddedLLMDetails)
	}

	data, err := fetchRemoteLLMDetails(ctx)
	if err != nil {
		LogDebug("FetchLLMDetails: remote fetch failed, using embedded: %v", err)
		return parseLLMDetails(embeddedLLMDetails)
	}

	LogDebug("FetchLLMDetails: remote fetch succeeded (%d bytes)", len(data))

	details, err := parseLLMDetails(data)
	if err != nil {
		LogDebug("FetchLLMDetails: remote parse failed (%v), falling back to embedded", err)
		return parseLLMDetails(embeddedLLMDetails)
	}

	return details, nil
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

// LookupModelMaxTokens returns the max input tokens for a model ID from the
// embedded llmdetails.json. Returns 0 if the model is not found.
func LookupModelMaxTokens(modelID string) int {
	details, err := parseLLMDetails(embeddedLLMDetails)
	if err != nil {
		return 0
	}
	if info, ok := details[modelID]; ok {
		return info.MaxInputTokens
	}
	return 0
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
	for _, m := range file.Gemini.Data {
		if m.ID != "" {
			result[m.ID] = m
		}
	}
	for _, m := range file.Others.Data {
		if m.ID != "" {
			result[m.ID] = m
		}
		if m.Slug != "" && m.Slug != m.ID {
			result[m.Slug] = m
		}
	}
	LogDebug("parseLLMDetails: openai=%d anthropic=%d gemini=%d others=%d total=%d",
		len(file.OpenAI.Data), len(file.Anthropic.Data), len(file.Gemini.Data), len(file.Others.Data), len(result))
	return result, nil
}

// EnrichModels overlays capability data from llmdetails onto API-fetched models.
// Only the Capabilities, MaxInputTokens, and MaxTokens fields are enriched.
func EnrichModels(apiModels []ModelInfo, details map[string]ModelInfo, provider string) []ModelInfo {
	enriched := make([]ModelInfo, len(apiModels))
	matched := 0
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
			matched++
		}
	}
	LogDebug("EnrichModels: api_models=%d matched=%d total_details=%d provider=%s",
		len(apiModels), matched, len(details), provider)
	if matched < len(apiModels) {
		var missingIDs []string
		for _, m := range apiModels {
			if _, ok := details[m.ID]; !ok {
				missingIDs = append(missingIDs, m.ID)
				if len(missingIDs) >= 10 {
					break
				}
			}
		}
		LogDebug("EnrichModels: unmatched sample_ids=%v", missingIDs)
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
	return provider == ProviderOpenAI || provider == ProviderCustom || provider == ProviderGemini
}

func (e EffortCapabilities) EffortLevels() []string {
	var levels []string
	if e.Minimal.Supported {
		levels = append(levels, "minimal")
	}
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

func (m ModelInfo) ThinkingTypeOptions() []string {
	var opts []string
	if m.Capabilities.Thinking.Types.Adaptive.Supported {
		opts = append(opts, "adaptive")
	}
	if m.Capabilities.Thinking.Types.Enabled.Supported {
		opts = append(opts, "enabled")
	}
	opts = append(opts, "disabled")
	return opts
}

func (m ModelInfo) HasThinking() bool {
	return m.Capabilities.Thinking.Supported
}

func (m ModelInfo) HasThinkingSupport() bool {
	return m.Capabilities.Thinking.Supported || len(m.Capabilities.ThinkingLevels) > 0
}

func (m ModelInfo) HasEffort() bool {
	return m.Capabilities.Effort.Supported
}

func (m ModelInfo) HasGranularThinkingLevels() bool {
	for _, level := range m.Capabilities.ThinkingLevels {
		switch level {
		case "none", "enabled", "disabled", "adaptive":
			continue
		default:
			return true
		}
	}
	return false
}

func (m ModelInfo) HasAdjustableReasoning() bool {
	return m.HasGranularThinkingLevels() || m.HasExplicitEffort()
}

func (m ModelInfo) HasAdjustableThinking() bool {
	return m.HasGranularThinkingLevels()
}

func (m ModelInfo) HasExplicitEffort() bool {
	return len(m.Capabilities.EffortLevels) > 0
}

func (m ModelInfo) ThinkingEffortLevels() []string {
	return m.Capabilities.Effort.EffortLevels()
}

func (m ModelInfo) ThinkingHasNone() bool {
	return hasNoneThinking(m.Capabilities.ThinkingLevels)
}

func (m ModelInfo) ReasoningLevelOptions() []string {
	var opts []string
	if m.ThinkingHasNone() {
		opts = append(opts, "none")
	}
	if m.Capabilities.Thinking.Types.Adaptive.Supported {
		opts = append(opts, "adaptive")
	}
	seen := make(map[string]bool, len(opts))
	for _, o := range opts {
		seen[o] = true
	}
	for _, level := range m.Capabilities.Effort.EffortLevels() {
		if !seen[level] {
			opts = append(opts, level)
			seen[level] = true
		}
	}
	return opts
}

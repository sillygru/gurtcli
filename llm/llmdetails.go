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
		return parseLLMDetails(embeddedLLMDetails)
	}

	data, err := fetchRemoteLLMDetails(ctx)
	if err != nil {
		return parseLLMDetails(embeddedLLMDetails)
	}

	details, err := parseLLMDetails(data)
	if err != nil {
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
	return result, nil
}

// EnrichModels fills in whatever the models API did not provide from
// llmdetails.json. The API response is prioritized: its context window, max
// tokens, display name, and any capability/level it reports are kept, and the
// static file only fills fields the API left zero or empty.
func EnrichModels(apiModels []ModelInfo, details map[string]ModelInfo, provider string) []ModelInfo {
	enriched := make([]ModelInfo, len(apiModels))
	for i, m := range apiModels {
		enriched[i] = m
		d, ok := details[m.ID]
		if !ok && m.Slug != "" {
			d, ok = details[m.Slug]
		}
		if !ok {
			continue
		}
		enriched[i] = mergeModelInfo(m, d)
	}
	return enriched
}

// mergeModelInfo returns the API model with every field the API left empty
// filled from the llmdetails.json entry.
func mergeModelInfo(api, fallback ModelInfo) ModelInfo {
	if api.DisplayName == "" {
		api.DisplayName = fallback.DisplayName
	}
	if api.Slug == "" {
		api.Slug = fallback.Slug
	}
	if api.MaxInputTokens <= 0 {
		api.MaxInputTokens = fallback.MaxInputTokens
	}
	if api.MaxTokens <= 0 {
		api.MaxTokens = fallback.MaxTokens
	}
	api.Capabilities = mergeCapabilities(api.Capabilities, fallback.Capabilities)
	return api
}

// mergeCapabilities combines two capability sets with the API response taking
// priority. A dimension the API reports (non-empty thinking/effort/context
// levels) is kept as-is; a dimension the API leaves at its zero value is filled
// from llmdetails.json. Simple booleans are ORed because an API response never
// marks a capability as explicitly unsupported — only as present or absent.
//
// The result is round-tripped through the same JSON shape the models
// API/llmdetails.json use so the levels arrays and the derived structured
// fields (Thinking.Types, Effort.Minimal..Max) never disagree.
func mergeCapabilities(apiCaps, detailCaps ModelCapabilities) ModelCapabilities {
	payload := map[string]interface{}{
		"batch":              apiCaps.Batch.Supported || detailCaps.Batch.Supported,
		"citations":          apiCaps.Citations.Supported || detailCaps.Citations.Supported,
		"code_execution":     apiCaps.CodeExecution.Supported || detailCaps.CodeExecution.Supported,
		"image_input":        apiCaps.ImageInput.Supported || detailCaps.ImageInput.Supported,
		"pdf_input":          apiCaps.PDFInput.Supported || detailCaps.PDFInput.Supported,
		"structured_outputs": apiCaps.StructuredOutputs.Supported || detailCaps.StructuredOutputs.Supported,
	}

	thinking := apiCaps.ThinkingLevels
	if len(thinking) == 0 {
		thinking = detailCaps.ThinkingLevels
	}
	effort := apiCaps.EffortLevels
	if len(effort) == 0 {
		effort = detailCaps.EffortLevels
	}
	context := apiCaps.contextManagementTokens()
	if len(context) == 0 {
		context = detailCaps.contextManagementTokens()
	}
	if len(thinking) > 0 {
		payload["thinking"] = thinking
	}
	if len(effort) > 0 {
		payload["effort"] = effort
	}
	if len(context) > 0 {
		payload["context_management"] = context
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return detailCaps
	}
	var merged ModelCapabilities
	if err := json.Unmarshal(data, &merged); err != nil {
		return detailCaps
	}
	return merged
}

// contextManagementTokens returns the context_management capabilities as the
// token list used by llmdetails.json and the models API.
func (c ModelCapabilities) contextManagementTokens() []string {
	var tokens []string
	if c.ContextManagement.ClearToolUses20250919.Supported {
		tokens = append(tokens, "clear_tool_uses_20250919")
	}
	if c.ContextManagement.ClearThinking20251015.Supported {
		tokens = append(tokens, "clear_thinking_20251015")
	}
	if c.ContextManagement.Compact20260112.Supported {
		tokens = append(tokens, "compact_20260112")
	}
	return tokens
}

func hasNoneThinking(levels []string) bool {
	for _, s := range levels {
		if s == "none" {
			return true
		}
	}
	return false
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

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

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
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

func FetchModels(ctx context.Context, provider, apiKey, baseURL string) ([]string, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL(provider)
	}

	reqURL := strings.TrimSuffix(baseURL, "/") + "/models"

	var models []string
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

func fetchModelsOnce(ctx context.Context, url, provider, apiKey string) ([]string, error) {
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

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}

	if len(models) == 0 {
		return nil, fmt.Errorf("no models returned by API")
	}

	return models, nil
}

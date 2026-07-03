package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type StreamEventType int

const (
	StreamDelta StreamEventType = iota
	StreamDone
	StreamError
)

type StreamEvent struct {
	Type    StreamEventType
	Content string
	Err     error
}

type openaiChatBody struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type anthropicChatBody struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type openaiChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

type anthropicChunk struct {
	Type  string `json:"type"`
	Delta struct {
		Text string `json:"text"`
	} `json:"delta"`
}

func StreamChatCompletion(ctx context.Context, provider, apiKey, baseURL string, req ChatRequest) (<-chan StreamEvent, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL(provider)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	url := baseURL + "/chat/completions"
	if provider == ProviderAnthropic {
		url = baseURL + "/messages"
	}

	var bodyBytes []byte
	var err error

	switch provider {
	case ProviderAnthropic:
		bodyBytes, err = json.Marshal(anthropicChatBody{
			Model:    req.Model,
			Messages: req.Messages,
			Stream:   true,
		})
	default:
		bodyBytes, err = json.Marshal(openaiChatBody{
			Model:    req.Model,
			Messages: req.Messages,
			Stream:   true,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	switch provider {
	case ProviderAnthropic:
		httpReq.Header.Set("x-api-key", apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	default:
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{
		Timeout: 0,
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("chat request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		resp.Body.Close()
		return nil, fmt.Errorf("chat API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	events := make(chan StreamEvent, 16)
	go func() {
		defer close(events)
		defer resp.Body.Close()
		readSSE(ctx, resp.Body, provider, events)
	}()

	return events, nil
}

func readSSE(ctx context.Context, r io.Reader, provider string, events chan<- StreamEvent) {
	scanner := bufio.NewScanner(r)
	var eventType string

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}

		line := scanner.Text()

		if strings.HasPrefix(line, "event: ") {
			eventType = strings.TrimPrefix(line, "event: ")
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "" {
				continue
			}

			if data == "[DONE]" {
				events <- StreamEvent{Type: StreamDone}
				return
			}

			emitEvent(events, provider, eventType, data)
			continue
		}

		if line == "" {
			eventType = ""
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		events <- StreamEvent{Type: StreamError, Err: fmt.Errorf("reading stream: %w", err)}
	}
}

func emitEvent(events chan<- StreamEvent, provider, eventType, data string) {
	switch provider {
	case ProviderOpenAI, ProviderCustom:
		var chunk openaiChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return
		}
		if len(chunk.Choices) > 0 {
			content := chunk.Choices[0].Delta.Content
			if content != "" {
				events <- StreamEvent{Type: StreamDelta, Content: content}
			}
		}

	case ProviderAnthropic:
		if eventType == "message_stop" {
			events <- StreamEvent{Type: StreamDone}
			return
		}
		if eventType == "content_block_delta" {
			var chunk anthropicChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				return
			}
			if chunk.Delta.Text != "" {
				events <- StreamEvent{Type: StreamDelta, Content: chunk.Delta.Text}
			}
		}
	}
}

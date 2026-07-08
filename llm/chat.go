package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type Message struct {
	Role             string     `json:"role"`
	Content          string     `json:"content"`
	Reasoning          string        `json:"reasoning,omitempty"`
	ReasoningDuration  time.Duration `json:"reasoning_duration,omitempty"`
	ReasoningVisible   bool          `json:"reasoning_visible,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	Model            string     `json:"model,omitempty"`
	Internal         bool       `json:"-"`
	IsError          bool       `json:"is_error,omitempty"`
}

type ThinkingConfig struct {
	Type        string `json:"type"`
	BudgetTokens int   `json:"budget_tokens,omitempty"`
}

type ChatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	SystemPrompt   string          `json:"-"`
	Tools          []Tool          `json:"-"`
	Thinking       *ThinkingConfig `json:"-"`
	ReasoningEffort string         `json:"-"`
	MaxTokens      int             `json:"-"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type,omitempty"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type StreamEventType int

const (
	StreamDelta StreamEventType = iota
	StreamReasoning
	StreamToolCalls
	StreamDone
	StreamError
	StreamUsage
)

type StreamEvent struct {
	Type         StreamEventType
	Content      string
	Err          error
	ToolCalls    []ToolCall
	InputTokens  int
	OutputTokens int
}

type openaiStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openaiChatBody struct {
	Model           string               `json:"model"`
	Messages        []Message            `json:"messages"`
	Stream          bool                 `json:"stream"`
	StreamOptions   *openaiStreamOptions `json:"stream_options,omitempty"`
	Tools           []Tool               `json:"tools,omitempty"`
	ReasoningEffort string               `json:"reasoning_effort,omitempty"`
}

type anthropicChatBody struct {
	Model     string              `json:"model"`
	Messages  []anthropicMessage  `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
	Stream    bool                `json:"stream"`
	System    string              `json:"system,omitempty"`
	Tools     []Tool              `json:"tools,omitempty"`
	Thinking  *ThinkingConfig     `json:"thinking,omitempty"`
}

type anthropicMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// openaiChunk for SSE parsing
type openaiChunk struct {
	Choices []openaiChoice `json:"choices"`
	Usage   *openaiUsage   `json:"usage,omitempty"`
}

type openaiChoice struct {
	Delta        openaiDelta `json:"delta"`
	FinishReason string      `json:"finish_reason"`
}

type openaiDelta struct {
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content"`
	ToolCalls        []openaiToolCall `json:"tool_calls"`
}

type openaiToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiToolCallFunc `json:"function"`
}

type openaiToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Anthropic SSE types for tool call parsing
type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicMessageStart struct {
	Type    string                `json:"type"`
	Message anthropicStartMessage `json:"message"`
}

type anthropicStartMessage struct {
	Usage anthropicUsage `json:"usage"`
}

type anthropicMessageDeltaEvt struct {
	Type  string              `json:"type"`
	Delta anthropicStopReason `json:"delta"`
	Usage *anthropicUsage     `json:"usage,omitempty"`
}

type anthropicContentBlockStart struct {
	Type         string               `json:"type"`
	Index        int                  `json:"index"`
	ContentBlock anthropicBlockContent `json:"content_block"`
}

type anthropicBlockContent struct {
	Type  string          `json:"type"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicContentBlockDeltaEvt struct {
	Type  string            `json:"type"`
	Index int               `json:"index"`
	Delta anthropicDeltaEvt `json:"delta"`
}

type anthropicDeltaEvt struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

type anthropicStopReason struct {
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
}

func convertToAnthropicMessages(msgs []Message) []anthropicMessage {
	result := make([]anthropicMessage, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case "user":
			if msg.ToolCallID != "" {
				block := anthropicContentBlock{
					Type:      "tool_result",
					ToolUseID: msg.ToolCallID,
					Content:   msg.Content,
				}
				blocks, _ := json.Marshal([]anthropicContentBlock{block})
				result = append(result, anthropicMessage{
					Role:    "user",
					Content: blocks,
				})
			} else {
				content, _ := json.Marshal(msg.Content)
				result = append(result, anthropicMessage{
					Role:    "user",
					Content: content,
				})
			}

		case "assistant":
			if len(msg.ToolCalls) > 0 {
				blocks := make([]anthropicContentBlock, 0)
				if msg.Content != "" {
					blocks = append(blocks, anthropicContentBlock{
						Type: "text",
						Text: msg.Content,
					})
				}
				for _, tc := range msg.ToolCalls {
					input := json.RawMessage(tc.Function.Arguments)
					if !json.Valid([]byte(tc.Function.Arguments)) {
						input = json.RawMessage("{}")
					}
					blocks = append(blocks, anthropicContentBlock{
						Type:  "tool_use",
						ID:    tc.ID,
						Name:  tc.Function.Name,
						Input: input,
					})
				}
				blocksJSON, _ := json.Marshal(blocks)
				result = append(result, anthropicMessage{
					Role:    "assistant",
					Content: blocksJSON,
				})
			} else {
				content, _ := json.Marshal(msg.Content)
				result = append(result, anthropicMessage{
					Role:    "assistant",
					Content: content,
				})
			}

		default:
			content, _ := json.Marshal(msg.Content)
			result = append(result, anthropicMessage{
				Role:    msg.Role,
				Content: content,
			})
		}
	}
	return result
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
		msgs := req.Messages
		if len(msgs) > 0 && msgs[0].Role == "system" {
			msgs = msgs[1:]
		}
		maxTokens := req.MaxTokens
		if maxTokens == 0 {
			maxTokens = 128000
		}
		bodyBytes, err = json.Marshal(anthropicChatBody{
			Model:     req.Model,
			Messages:  convertToAnthropicMessages(msgs),
			MaxTokens: maxTokens,
			Stream:    true,
			System:    req.SystemPrompt,
			Tools:     req.Tools,
			Thinking:  req.Thinking,
		})
	default:
		msgs := make([]Message, 0, len(req.Messages)+1)
		if req.SystemPrompt != "" {
			msgs = append(msgs, Message{Role: "system", Content: req.SystemPrompt})
		}
		msgs = append(msgs, req.Messages...)
		bodyBytes, err = json.Marshal(openaiChatBody{
			Model:           req.Model,
			Messages:        msgs,
			Stream:          true,
			StreamOptions:   &openaiStreamOptions{IncludeUsage: true},
			Tools:           req.Tools,
			ReasoningEffort: req.ReasoningEffort,
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
		Transport: &http.Transport{
			ResponseHeaderTimeout: 30 * time.Second,
		},
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

type openaiToolAccumEntry struct {
	Index    int
	ID       string
	Type     string
	Name     string
	Arguments string
}

func readSSE(ctx context.Context, r io.Reader, provider string, events chan<- StreamEvent) {
	scanner := bufio.NewScanner(r)
	var eventType string

	var openaiAccum []openaiToolAccumEntry
	var anthropicAccum map[int]struct {
		ID   string
		Name string
		Args string
	}
	var pendingToolCalls []ToolCall

	flushOpenAIToolCalls := func() {
		if len(openaiAccum) == 0 {
			return
		}
		sort.Slice(openaiAccum, func(i, j int) bool {
			return openaiAccum[i].Index < openaiAccum[j].Index
		})
		calls := make([]ToolCall, len(openaiAccum))
		for i, entry := range openaiAccum {
			calls[i] = ToolCall{
				ID:   entry.ID,
				Type: entry.Type,
				Function: ToolCallFunction{
					Name:      entry.Name,
					Arguments: entry.Arguments,
				},
			}
		}
		events <- StreamEvent{Type: StreamToolCalls, ToolCalls: calls}
		openaiAccum = nil
	}

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
				if len(openaiAccum) > 0 {
					flushOpenAIToolCalls()
				}
				events <- StreamEvent{Type: StreamDone}
				return
			}

			switch provider {
			case ProviderOpenAI, ProviderGemini, ProviderCustom:
				var chunk openaiChunk
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					continue
				}
				if chunk.Usage != nil {
					events <- StreamEvent{Type: StreamUsage, InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens}
				}
				if len(chunk.Choices) == 0 {
					continue
				}
				choice := chunk.Choices[0]

				if len(choice.Delta.ToolCalls) > 0 {
					for _, tc := range choice.Delta.ToolCalls {
						found := false
						for i := range openaiAccum {
							if openaiAccum[i].Index == tc.Index {
								if tc.ID != "" {
									openaiAccum[i].ID = tc.ID
								}
								if tc.Type != "" {
									openaiAccum[i].Type = tc.Type
								}
								if tc.Function.Name != "" {
									openaiAccum[i].Name = tc.Function.Name
								}
								openaiAccum[i].Arguments += tc.Function.Arguments
								found = true
								break
							}
						}
						if !found {
							openaiAccum = append(openaiAccum, openaiToolAccumEntry{
								Index:     tc.Index,
								ID:        tc.ID,
								Type:      tc.Type,
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							})
						}
					}
					if choice.FinishReason == "tool_calls" {
						flushOpenAIToolCalls()
					}
					continue
				}

				if choice.Delta.ReasoningContent != "" {
					events <- StreamEvent{Type: StreamReasoning, Content: choice.Delta.ReasoningContent}
				}
				if choice.Delta.Content != "" {
					events <- StreamEvent{Type: StreamDelta, Content: choice.Delta.Content}
				}

			case ProviderAnthropic:
				switch eventType {
				case "message_start":
					var start anthropicMessageStart
					if err := json.Unmarshal([]byte(data), &start); err != nil {
						continue
					}
					events <- StreamEvent{Type: StreamUsage, InputTokens: start.Message.Usage.InputTokens}

				case "content_block_start":
					var start anthropicContentBlockStart
					if err := json.Unmarshal([]byte(data), &start); err != nil {
						continue
					}
					if start.ContentBlock.Type == "tool_use" {
						if anthropicAccum == nil {
							anthropicAccum = make(map[int]struct {
								ID   string
								Name string
								Args string
							})
						}
						anthropicAccum[start.Index] = struct {
							ID   string
							Name string
							Args string
						}{
							ID:   start.ContentBlock.ID,
							Name: start.ContentBlock.Name,
						}
					}

				case "content_block_delta":
					var delta anthropicContentBlockDeltaEvt
					if err := json.Unmarshal([]byte(data), &delta); err != nil {
						continue
					}
					switch delta.Delta.Type {
					case "input_json_delta":
						if anthropicAccum != nil {
							if entry, ok := anthropicAccum[delta.Index]; ok {
								entry.Args += delta.Delta.PartialJSON
								anthropicAccum[delta.Index] = entry
							}
						}
					case "text_delta":
						if delta.Delta.Text != "" {
							events <- StreamEvent{Type: StreamDelta, Content: delta.Delta.Text}
						}
					case "thinking_delta":
						if delta.Delta.Thinking != "" {
							events <- StreamEvent{Type: StreamReasoning, Content: delta.Delta.Thinking}
						}
					}

				case "message_delta":
					var msgDelta anthropicMessageDeltaEvt
					if err := json.Unmarshal([]byte(data), &msgDelta); err != nil {
						continue
					}
					if msgDelta.Usage != nil && msgDelta.Usage.OutputTokens > 0 {
						events <- StreamEvent{Type: StreamUsage, OutputTokens: msgDelta.Usage.OutputTokens}
					}
					if msgDelta.Delta.StopReason == "tool_use" && len(anthropicAccum) > 0 {
						indices := make([]int, 0, len(anthropicAccum))
						for idx := range anthropicAccum {
							indices = append(indices, idx)
						}
						sort.Ints(indices)
						calls := make([]ToolCall, len(indices))
						for i, idx := range indices {
							entry := anthropicAccum[idx]
							calls[i] = ToolCall{
								ID:   entry.ID,
								Type: "function",
								Function: ToolCallFunction{
									Name:      entry.Name,
									Arguments: entry.Args,
								},
							}
						}
						pendingToolCalls = calls
						anthropicAccum = nil
					}

				case "message_stop":
					if pendingToolCalls != nil {
						events <- StreamEvent{Type: StreamDone, ToolCalls: pendingToolCalls}
					} else {
						events <- StreamEvent{Type: StreamDone}
					}
					return
				}
			}
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

type openaiSimpleResponse struct {
	Choices []openaiSimpleChoice `json:"choices"`
}

type openaiSimpleChoice struct {
	Message openaiSimpleMessage `json:"message"`
}

type openaiSimpleMessage struct {
	Content string `json:"content"`
}

type anthropicSimpleResponse struct {
	Content []anthropicSimpleContent `json:"content"`
}

type anthropicSimpleContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openaiSimpleChatBody struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	System   string    `json:"-"`
}

type anthropicSimpleBody struct {
	Model     string              `json:"model"`
	Messages  []anthropicMessage  `json:"messages"`
	MaxTokens int                `json:"max_tokens"`
	System    string              `json:"system,omitempty"`
}

// SimpleChatCompletion does a non-streaming chat completion and returns the response text.
func SimpleChatCompletion(ctx context.Context, provider, apiKey, baseURL string, req ChatRequest) (string, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL(provider)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	var bodyBytes []byte
	var err error

	switch provider {
	case ProviderAnthropic:
		msgs := req.Messages
		if len(msgs) > 0 && msgs[0].Role == "system" {
			msgs = msgs[1:]
		}
		maxTokens := req.MaxTokens
		if maxTokens == 0 {
			maxTokens = 128000
		}
		bodyBytes, err = json.Marshal(anthropicSimpleBody{
			Model:     req.Model,
			Messages:  convertToAnthropicMessages(msgs),
			MaxTokens: maxTokens,
			System:    req.SystemPrompt,
		})
	default:
		msgs := make([]Message, 0, len(req.Messages)+1)
		if req.SystemPrompt != "" {
			msgs = append(msgs, Message{Role: "system", Content: req.SystemPrompt})
		}
		msgs = append(msgs, req.Messages...)
		bodyBytes, err = json.Marshal(openaiSimpleChatBody{
			Model:    req.Model,
			Messages: msgs,
		})
	}
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	url := baseURL + "/chat/completions"
	if provider == ProviderAnthropic {
		url = baseURL + "/messages"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	switch provider {
	case ProviderAnthropic:
		httpReq.Header.Set("x-api-key", apiKey)
		httpReq.Header.Set("anthropic-version", "2023-06-01")
	default:
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("chat API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	switch provider {
	case ProviderAnthropic:
		var result anthropicSimpleResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("decoding response: %w", err)
		}
		for _, block := range result.Content {
			if block.Type == "text" && block.Text != "" {
				return strings.TrimSpace(block.Text), nil
			}
		}
		return "", fmt.Errorf("no text content in response")
	default:
		var result openaiSimpleResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("decoding response: %w", err)
		}
		if len(result.Choices) == 0 {
			return "", fmt.Errorf("no choices in response")
		}
		return strings.TrimSpace(result.Choices[0].Message.Content), nil
	}
}

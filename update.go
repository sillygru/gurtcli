package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sillygru/gurtcli/config"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/tools"
)

//go:embed prompts/system.md
var systemPromptTemplate string

var dateSuffixRegex = regexp.MustCompile(`-\d{8}$|-\d{4}-\d{2}-\d{2}$`)

func hasDateSuffix(name string) bool {
	return dateSuffixRegex.MatchString(name)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		h := msg.Height - 10
		if h < 4 {
			h = 4
		}
		m.providerList.SetSize(msg.Width-4, h)
		m.modelList.SetSize(msg.Width-4, h)

		chatViewHeight := msg.Height - 5
		if chatViewHeight < 4 {
			chatViewHeight = 4
		}
		m.chatViewport.Width = msg.Width - 4
		m.chatViewport.Height = chatViewHeight
		m.chatInput.Width = msg.Width - 4
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.state == stateChat && m.isStreaming {
				if m.streamState.cancel != nil {
					m.streamState.cancel()
					m.cancelRequested = true
				}
				return m, nil
			}
			return m, tea.Quit
		}
		switch m.state {
		case stateWelcome:
			return m.updateWelcome(msg)
		case stateProviderPick:
			return m.updateProviderPick(msg)
		case stateCustomURL:
			return m.updateCustomURL(msg)
		case stateAPIKeyInput:
			return m.updateAPIKeyInput(msg)
		case stateModelPick:
			return m.updateModelPick(msg)
		case stateError:
			return m.updateError(msg)
		case stateManualModel:
			return m.updateManualModel(msg)
		case stateChat:
			return m.updateChat(msg)
		}
		return m, nil

	case spinner.TickMsg:
		if m.state == stateModelFetch {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case modelsFetchedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.errChoice = 0
			m.state = stateError
			return m, nil
		}
		models := append([]string(nil), msg.models...)
		if m.provider == llm.ProviderOpenAI {
			filtered := models[:0]
			for _, name := range models {
				if llm.IsTextChatModel(name) && !hasDateSuffix(name) {
					filtered = append(filtered, name)
				}
			}
			models = filtered
		} else if m.provider == llm.ProviderAnthropic {
			filtered := models[:0]
			for _, name := range models {
				if !hasDateSuffix(name) {
					filtered = append(filtered, name)
				}
			}
			models = filtered
		}
		m.models = models
		items := make([]list.Item, len(models))
		for i, name := range models {
			items[i] = item{title: name}
		}
		m.modelList.SetItems(items)
		m.state = stateModelPick
		return m, nil

	case tea.MouseMsg:
		if m.state != stateChat {
			return m, nil
		}
		return m.updateMouse(msg)

	case chatStreamChunk:
		if m.streamingContent == nil {
			m.streamingContent = new(strings.Builder)
		}
		m.streamingContent.WriteString(msg.content)
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case chatStreamReasoning:
		if m.reasoning.content == nil {
			m.reasoning.content = new(strings.Builder)
		}
		m.reasoning.content.WriteString(msg.content)
		if !m.reasoning.active {
			m.reasoning.active = true
			m.reasoning.visible = m.reasoning.defaultVisible
			m.reasoning.startTime = time.Now()
		}
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case chatStreamDone:
		if m.cancelRequested {
			m.cancelRequested = false
			m.streamingContent = nil
			m.isStreaming = false
			m.streamState.cancel = nil
			m.reasoning = reasoningState{}
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: "_Interrupted_",
			})
			m.chatViewport.SetContent(buildChatContent(m))
			m.chatViewport.GotoBottom()
			m.chatInput.Focus()
			if m.queuedMessage != "" {
				qmsg := m.queuedMessage
				m.queuedMessage = ""
				m.messages = append(m.messages, llm.Message{Role: "user", Content: qmsg})
				m.isStreaming = true
				m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
				m.chatViewport.SetContent(buildChatContent(m))
				m.chatViewport.GotoBottom()
				return m, startChatStreamCmd(m)
			}
			return m, nil
		}

		contentStr := ""
		if m.streamingContent != nil {
			contentStr = strings.TrimSpace(m.streamingContent.String())
		}
		reasoningStr := ""
		if m.reasoning.content != nil {
			reasoningStr = m.reasoning.content.String()
		}
		m.streamingContent = nil
		m.isStreaming = false
		m.streamState.cancel = nil
		if m.reasoning.active {
			m.reasoning.duration = time.Since(m.reasoning.startTime).Round(100 * time.Millisecond)
			m.reasoning.active = false
			m.reasoning.content = nil
		}

		if len(msg.toolCalls) > 0 {
			asm := llm.Message{Role: "assistant", Content: contentStr}
			if reasoningStr != "" {
				asm.Reasoning = reasoningStr
			}
			asm.ToolCalls = msg.toolCalls
			m.messages = append(m.messages, asm)
			m.chatViewport.SetContent(buildChatContent(m))
			m.chatViewport.GotoBottom()
			m.toolCallCycle++
			if m.toolCallCycle > maxToolCallCycles {
				m.messages = append(m.messages, llm.Message{
					Role:    "assistant",
					Content: fmt.Sprintf("Reached maximum tool call cycles (%d). Stopping.", maxToolCallCycles),
				})
				m.toolCallCycle = 0
				m.chatViewport.SetContent(buildChatContent(m))
				m.chatViewport.GotoBottom()
				m.chatInput.Focus()
				if m.queuedMessage != "" {
					qmsg := m.queuedMessage
					m.queuedMessage = ""
					m.messages = append(m.messages, llm.Message{Role: "user", Content: qmsg})
					m.isStreaming = true
					m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
					m.chatViewport.SetContent(buildChatContent(m))
					m.chatViewport.GotoBottom()
					return m, startChatStreamCmd(m)
				}
				return m, nil
			}
			return m.processToolCalls(msg.toolCalls)
		}

		if contentStr != "" || reasoningStr != "" {
			msg := llm.Message{Role: "assistant", Content: contentStr}
			if reasoningStr != "" {
				msg.Reasoning = reasoningStr
			}
			m.messages = append(m.messages, msg)
		}
		m.toolCallCycle = 0
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		m.chatInput.Focus()
		if m.queuedMessage != "" {
			qmsg := m.queuedMessage
			m.queuedMessage = ""
			m.messages = append(m.messages, llm.Message{Role: "user", Content: qmsg})
			m.isStreaming = true
			m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
			m.chatViewport.SetContent(buildChatContent(m))
			m.chatViewport.GotoBottom()
			return m, startChatStreamCmd(m)
		}
		return m, nil

	case chatStreamError:
		m.streamingContent = nil
		m.isStreaming = false
		m.streamState.cancel = nil
		if m.cancelRequested {
			m.cancelRequested = false
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: "_Interrupted_",
			})
		} else {
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("_Error: %v_", msg.err),
			})
		}
		m.queuedMessage = ""
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		m.chatInput.Focus()
		return m, nil
	}

	return m, nil
}

func (m model) updateWelcome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() != "enter" {
		return m, nil
	}
	if m.reconfigure {
		if m.provider == "" {
			m.state = stateProviderPick
			return m, nil
		}
		key, _ := config.GetAPIKey(m.provider, m.customURL)
		if key != "" {
			m.apiKey = key
		}
		if m.apiKey == "" {
			m.state = stateAPIKeyInput
			m.keyInput.Focus()
			return m, nil
		}
		if m.modelName == "" {
			m.state = stateModelFetch
			return m, tea.Batch(
				m.spinner.Tick,
				fetchModelsCmd(m.provider, m.apiKey, m.customURL),
			)
		}
		return m.enterChatState(), nil
	}
	if m.provider == "" {
		m.state = stateProviderPick
		return m, nil
	}
	key, _ := config.GetAPIKey(m.provider, m.customURL)
	if key != "" {
		m.apiKey = key
	}
	if m.apiKey == "" {
		m.state = stateAPIKeyInput
		m.keyInput.Focus()
		return m, nil
	}
	if m.modelName == "" {
		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			fetchModelsCmd(m.provider, m.apiKey, m.customURL),
		)
	}
	return m.enterChatState(), nil
}

func (m model) updateProviderPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.providerList, cmd = m.providerList.Update(msg)

	if msg.String() == "esc" {
		return m.enterChatState(), nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	m.provider = providerFromIndex(m.providerList.Index())
	if m.provider == llm.ProviderCustom {
		m.state = stateCustomURL
		m.urlInput.Focus()
		return m, nil
	}
	m.customURL = ""

	key, _ := config.GetAPIKey(m.provider, m.customURL)
	if key != "" {
		m.apiKey = key
		if m.modelName != "" {
			return m.enterChatState(), nil
		}
		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			fetchModelsCmd(m.provider, m.apiKey, m.customURL),
		)
	}

	m.state = stateAPIKeyInput
	m.keyInput.Focus()
	return m, nil
}

func (m model) updateCustomURL(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.urlInput, cmd = m.urlInput.Update(msg)

	if msg.String() == "esc" {
		m.state = stateProviderPick
		return m, nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	m.customURL = strings.TrimSpace(m.urlInput.Value())
	if m.customURL == "" {
		return m, nil
	}

	key, _ := config.GetAPIKey(m.provider, m.customURL)
	if key != "" {
		m.apiKey = key
		if m.modelName != "" {
			return m.enterChatState(), nil
		}
		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			fetchModelsCmd(m.provider, m.apiKey, m.customURL),
		)
	}

	m.state = stateAPIKeyInput
	m.keyInput.Focus()
	return m, nil
}

func (m model) updateAPIKeyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)

	if msg.String() == "esc" {
		if m.provider == llm.ProviderCustom {
			m.state = stateCustomURL
			m.urlInput.Focus()
		} else {
			m.state = stateProviderPick
		}
		return m, nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	key := strings.TrimSpace(m.keyInput.Value())
	if key == "" {
		return m, nil
	}
	m.apiKey = key

	if err := config.SetAPIKey(m.provider, m.customURL, key); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}

	if m.modelName != "" {
		return m.enterChatState(), nil
	}

	m.state = stateModelFetch
	return m, tea.Batch(
		m.spinner.Tick,
		fetchModelsCmd(m.provider, m.apiKey, m.customURL),
	)
}

func (m model) updateModelPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.modelList, cmd = m.modelList.Update(msg)

	if msg.String() == "esc" {
		return m.enterChatState(), nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	selected, ok := m.modelList.SelectedItem().(item)
	if !ok {
		return m, nil
	}
	m.modelName = selected.title

	if err := config.Save(&config.Config{
		Provider:         m.provider,
		Model:            m.modelName,
		CustomBaseURL:    m.customURL,
		ReasoningVisible: m.reasoning.defaultVisible,
	}); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}

	return m.enterChatState(), nil
}

func (m model) updateError(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	acts := m.errorActions()
	switch msg.String() {
	case "up":
		m.errChoice--
		if m.errChoice < 0 {
			m.errChoice = len(acts) - 1
		}
	case "down":
		m.errChoice++
		if m.errChoice >= len(acts) {
			m.errChoice = 0
		}
	case "enter":
		switch errorAction(m.errChoice) {
		case errorRetry:
			m.state = stateModelFetch
			return m, tea.Batch(
				m.spinner.Tick,
				fetchModelsCmd(m.provider, m.apiKey, m.customURL),
			)
		case errorChangeURL:
			m.state = stateCustomURL
			m.urlInput.Reset()
			m.urlInput.Focus()
			return m, nil
		case errorChangeKey:
			m.state = stateAPIKeyInput
			m.keyInput.Focus()
			return m, nil
		case errorManual:
			m.state = stateManualModel
			m.manualInput.Focus()
			return m, nil
		case errorQuit:
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) updateManualModel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.manualInput, cmd = m.manualInput.Update(msg)

	if msg.String() == "esc" && m.isMidSession() {
		return m.enterChatState(), nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	name := strings.TrimSpace(m.manualInput.Value())
	if name == "" {
		return m, nil
	}
	m.modelName = name

	if err := config.Save(&config.Config{
		Provider:         m.provider,
		Model:            m.modelName,
		CustomBaseURL:    m.customURL,
		ReasoningVisible: m.reasoning.defaultVisible,
	}); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}

	return m.enterChatState(), nil
}

func (m model) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.pendingPerm != nil {
		if msg.String() == "enter" {
			input := strings.TrimSpace(m.chatInput.Value())
			m.chatInput.Reset()
			tc := m.pendingPerm.toolCall
			remaining := m.pendingPerm.remaining
			m.pendingPerm = nil

			switch input {
			case "y", "Y":
				m = m.executeTool(tc)
				return m.processToolCalls(remaining)
			case "n", "N":
				m.messages = append(m.messages, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    "User denied this operation.",
				})
				m.toolCallCycle = 0
				m.chatViewport.SetContent(buildChatContent(m))
				m.chatViewport.GotoBottom()
				m.chatInput.Focus()
				return m, nil
			case "a", "A":
				m.alwaysAllowPerms = true
				m = m.executeTool(tc)
				return m.processToolCalls(remaining)
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.chatInput, cmd = m.chatInput.Update(msg)
		return m, cmd
	}

	if msg.String() == "esc" && m.isStreaming && m.streamState.cancel != nil {
		m.streamState.cancel()
		m.cancelRequested = true
		return m, nil
	}

	if m.suggestions.active && len(m.suggestions.items) > 0 && !m.isStreaming && m.pendingPerm == nil {
		switch msg.String() {
		case "up":
			m.suggestions.selected--
			if m.suggestions.selected < 0 {
				m.suggestions.selected = len(m.suggestions.items) - 1
			}
			return m, nil
		case "down":
			m.suggestions.selected++
			if m.suggestions.selected >= len(m.suggestions.items) {
				m.suggestions.selected = 0
			}
			return m, nil
		case "tab", "enter":
			sel := m.suggestions.selected
			if sel >= 0 && sel < len(m.suggestions.items) {
				m.chatInput.SetValue("/" + m.suggestions.items[sel].name + " ")
				m.chatInput.SetCursor(len("/" + m.suggestions.items[sel].name + " "))
			}
			m.suggestions = suggestionState{}
			return m, nil
		case "esc":
			m.suggestions = suggestionState{}
			return m, nil
		}
	}

	if msg.String() == "enter" {
		input := strings.TrimSpace(m.chatInput.Value())
		if input == "" {
			return m, nil
		}
		if m.isStreaming {
			m.queuedMessage = input
			m.chatInput.Reset()
			return m, nil
		}
		m.chatInput.Reset()

		if strings.HasPrefix(input, "/") {
			return m.handleSlashCommand(input)
		}

		m.messages = append(m.messages, llm.Message{Role: "user", Content: input})
		m.isStreaming = true
		m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()

		return m, startChatStreamCmd(m)
	}

	var cmd tea.Cmd
	m.chatViewport, _ = m.chatViewport.Update(msg)
	m.chatInput, cmd = m.chatInput.Update(msg)
	m = m.updateSuggestions()
	return m, cmd
}

func (m model) executeTool(tc llm.ToolCall) model {
	args := json.RawMessage(tc.Function.Arguments)
	result, err := tools.Execute(context.Background(), tc.Function.Name, args, tools.Options{
		WorkspaceRoot: m.workspaceRoot,
	})
	content := result
	if err != nil {
		content = fmt.Sprintf("Error: %v", err)
	}
	m.messages = append(m.messages, llm.Message{
		Role:       "tool",
		ToolCallID: tc.ID,
		Content:    content,
	})
	return m
}

func (m model) processToolCalls(tcs []llm.ToolCall) (tea.Model, tea.Cmd) {
	for i, tc := range tcs {
		if tools.IsDestructive(tc.Function.Name) && !m.yolo && !m.alwaysAllowPerms {
			m.pendingPerm = &pendingPerm{
				toolCall:  tc,
				remaining: tcs[i+1:],
			}
			m.chatViewport.SetContent(buildChatContent(m))
			m.chatViewport.GotoBottom()
			m.chatInput.Focus()
			return m, nil
		}
		m = m.executeTool(tc)
	}
	return m, startChatStreamCmd(m)
}

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if !m.reasoning.active && (m.reasoning.content == nil || m.reasoning.content.Len() == 0) {
		return m, nil
	}
	viewportStartRow := 2
	contentLine := m.chatViewport.YOffset + msg.Y - viewportStartRow
	if contentLine < 0 {
		return m, nil
	}
	content := buildChatContent(m)
	lines := strings.Split(content, "\n")
	if contentLine >= len(lines) {
		return m, nil
	}
	line := lines[contentLine]
	if strings.Contains(line, "[▶") || strings.Contains(line, "[▼") {
		m.reasoning.visible = !m.reasoning.visible
		m.chatViewport.SetContent(buildChatContent(m))
	}
	return m, nil
}

func (m model) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	m.suggestions = suggestionState{}
	parts := strings.Fields(input)
	cmd := strings.TrimPrefix(parts[0], "/")

	switch cmd {
	case "model":
		if m.isStreaming {
			return m, nil
		}
		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			fetchModelsCmd(m.provider, m.apiKey, m.customURL),
		)

	case "provider":
		if m.isStreaming {
			return m, nil
		}
		m.modelName = ""
		m.customURL = ""
		m.state = stateProviderPick
		return m, nil

	case "auth":
		if m.isStreaming {
			return m, nil
		}
		m.state = stateAPIKeyInput
		m.keyInput.Reset()
		m.keyInput.Focus()
		return m, nil

	case "exit":
		return m, tea.Quit

	case "reasoning":
		oldVisible := m.reasoning.visible
		newVisible := !oldVisible
		if len(parts) > 1 {
			switch strings.ToLower(parts[1]) {
			case "true", "yes":
				newVisible = true
				m.reasoning.defaultVisible = true
			case "false", "no":
				newVisible = false
				m.reasoning.defaultVisible = false
			}
		}
		m.reasoning.visible = newVisible
		config.Save(&config.Config{
			Provider:         m.provider,
			Model:            m.modelName,
			CustomBaseURL:    m.customURL,
			ReasoningVisible: m.reasoning.defaultVisible,
		})
		m.messages = append(m.messages, llm.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("Reasoning changed to %s (was %s)",
				map[bool]string{true: "visible", false: "hidden"}[newVisible],
				map[bool]string{true: "visible", false: "hidden"}[oldVisible]),
		})
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case "help":
		var b strings.Builder
		b.WriteString("Available commands:\n")
		for _, sc := range slashCommands {
			fmt.Fprintf(&b, "  /%s - %s\n", sc.name, sc.description)
		}
		m.messages = append(m.messages, llm.Message{
			Role:    "assistant",
			Content: b.String(),
		})
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		return m, nil

	default:
		m.messages = append(m.messages, llm.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("_Unknown command: /%s. Type /help for available commands._", cmd),
		})
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		return m, nil
	}
}

func (m model) updateSuggestions() model {
	val := m.chatInput.Value()
	if !strings.HasPrefix(val, "/") || m.isStreaming || m.pendingPerm != nil {
		m.suggestions = suggestionState{}
		return m
	}

	input := strings.TrimPrefix(val, "/")

	var matches []slashCommand
	for _, sc := range slashCommands {
		if strings.HasPrefix(sc.name, input) {
			matches = append(matches, sc)
		}
	}

	if len(matches) == 0 {
		m.suggestions = suggestionState{}
		return m
	}

	selected := m.suggestions.selected
	if selected < 0 || selected >= len(matches) {
		selected = 0
	}

	m.suggestions = suggestionState{
		items:    matches,
		selected: selected,
		active:   true,
	}
	return m
}

func startChatStreamCmd(m model) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.streamState.cancel = cancel

		systemPrompt, err := renderSystemPrompt(m)
		if err != nil {
			cancel()
			return chatStreamError{err: fmt.Errorf("rendering system prompt: %w", err)}
		}

		baseURL := m.customURL
		req := llm.ChatRequest{
			Model:        m.modelName,
			Messages:     m.messages,
			SystemPrompt: systemPrompt,
			Tools:        tools.Definitions(),
		}

		events, err := llm.StreamChatCompletion(ctx, m.provider, m.apiKey, baseURL, req)
		if err != nil {
			cancel()
			m.streamState.cancel = nil
			return chatStreamError{err: err}
		}

		go func() {
			for event := range events {
				switch event.Type {
				case llm.StreamDelta:
					globalProgram.Send(chatStreamChunk{content: event.Content})
				case llm.StreamReasoning:
					globalProgram.Send(chatStreamReasoning{content: event.Content})
				case llm.StreamToolCalls:
					globalProgram.Send(chatStreamDone{toolCalls: event.ToolCalls})
					return
				case llm.StreamDone:
					globalProgram.Send(chatStreamDone{toolCalls: event.ToolCalls})
					return
				case llm.StreamError:
					globalProgram.Send(chatStreamError{err: event.Err})
					return
				}
			}
			globalProgram.Send(chatStreamDone{})
		}()

		return nil
	}
}

func renderSystemPrompt(m model) (string, error) {
	tmpl, err := template.New("system").Parse(systemPromptTemplate)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]string{
		"OS":        runtime.GOOS,
		"Arch":      runtime.GOARCH,
		"Workspace": m.workspaceRoot,
		"CWD":       m.workspaceRoot,
		"Model":     m.modelName,
	})
	if err != nil {
		return "", err
	}

	agentsPath := filepath.Join(m.workspaceRoot, "AGENTS.md")
	if data, err := os.ReadFile(agentsPath); err == nil && len(bytes.TrimSpace(data)) > 0 {
		buf.WriteString("\n\n## AGENTS.md\n\n")
		buf.Write(bytes.TrimSpace(data))
	}

	return buf.String(), nil
}

func buildChatContent(m model) string {
	var b strings.Builder
	streamingLen := 0
	if m.streamingContent != nil {
		streamingLen = m.streamingContent.Len()
	}
	reasoningLen := 0
	if m.reasoning.content != nil {
		reasoningLen = m.reasoning.content.Len()
	}

	if len(m.messages) == 0 && streamingLen == 0 {
		b.WriteString(m.styles.dim.Render("  No messages yet. Send a message to start."))
		b.WriteString("\n")
		return b.String()
	}

	lastIsCurrent := false
	if len(m.messages) > 0 {
		last := m.messages[len(m.messages)-1]
		lastIsCurrent = last.Role == "assistant" && (m.reasoning.active || streamingLen > 0)
	}

	// Render all finalized messages except the last one if it's the current response
	skipLast := lastIsCurrent
	for i, msg := range m.messages {
		isLast := i == len(m.messages)-1
		if isLast && skipLast {
			continue
		}
		switch msg.Role {
		case "user":
			b.WriteString(m.styles.dim.Render("you"))
			b.WriteString("\n")
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "assistant":
			b.WriteString(m.styles.header.Render("gurtcli"))
			b.WriteString("\n")
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					b.WriteString(m.styles.dim.Render(fmt.Sprintf("  [tool: %s]", tc.Function.Name)))
					b.WriteString("\n")
				}
			}
			if msg.Reasoning != "" {
				b.WriteString(m.styles.reasoningToggle.Render("[▶ Show reasoning]"))
				b.WriteString("\n")
			}
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "tool":
			b.WriteString("  ")
			b.WriteString(m.styles.dim.Render(msg.Content))
			b.WriteString("\n\n")
		}
	}

	// Render current response (last assistant message reasoning or streaming)
	if lastIsCurrent || m.reasoning.active || streamingLen > 0 {
		b.WriteString(m.styles.header.Render("gurtcli"))
		b.WriteString("\n")

		if reasoningLen > 0 {
			toggleChar := "▶"
			statusText := "Thought for"
			if m.reasoning.active {
				toggleChar = "▼"
				statusText = "Thinking"
			} else if m.reasoning.visible {
				toggleChar = "▼"
			}

			elapsed := m.reasoning.duration
			if m.reasoning.active {
				elapsed = time.Since(m.reasoning.startTime).Round(100 * time.Millisecond)
			}

			b.WriteString(m.styles.reasoningToggle.Render(fmt.Sprintf("[%s %s %v]", toggleChar, statusText, elapsed)))
			b.WriteString("\n")
			if m.reasoning.visible {
				b.WriteString(m.styles.reasoningContent.Render(m.reasoning.content.String()))
				b.WriteString("\n")
			}
		}

		if lastIsCurrent {
			b.WriteString(m.messages[len(m.messages)-1].Content)
		} else if streamingLen > 0 && m.streamingContent != nil {
			b.WriteString(m.streamingContent.String())
		}
		b.WriteString("\n")
	}

	return b.String()
}

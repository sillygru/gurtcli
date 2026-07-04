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
	"github.com/sillygru/gurtcli/sessions"
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
		m.sessionList.SetSize(msg.Width-4, h)

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
		case stateCustomModePick:
			return m.updateCustomModePick(msg)
		case stateCustomURL:
			return m.updateCustomURL(msg)
		case stateAPIKeyInput:
			return m.updateAPIKeyInput(msg)
		case stateCustomName:
			return m.updateCustomName(msg)
		case stateModelPick:
			return m.updateModelPick(msg)
		case stateReasoningConfig:
			return m.updateReasoningConfig(msg)
		case stateError:
			return m.updateError(msg)
		case stateManualModel:
			return m.updateManualModel(msg)
		case stateChat:
			return m.updateChat(msg)
		case stateSessionPick:
			return m.updateSessionPick(msg)
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
		models := append([]llm.ModelInfo(nil), msg.models...)
		if m.provider == llm.ProviderOpenAI {
			filtered := models[:0]
			for _, model := range models {
				id := strings.ToLower(model.ID)
				if strings.Contains(id, "transcribe") || strings.Contains(id, "embed") || strings.Contains(id, "tts") || strings.Contains(id, "search") || strings.Contains(id, "chat-latest") {
					continue
				}
				if llm.IsTextChatModel(model.ID) && !hasDateSuffix(model.ID) {
					filtered = append(filtered, model)
				}
			}
			models = filtered
		} else if m.provider == llm.ProviderAnthropic {
			filtered := models[:0]
			for _, model := range models {
				if !hasDateSuffix(model.ID) {
					filtered = append(filtered, model)
				}
			}
			models = filtered
		} else if m.provider == llm.ProviderGemini {
			filtered := models[:0]
			for _, model := range models {
				id := strings.ToLower(model.ID)
				if !strings.HasPrefix(id, "gemini-") || hasDateSuffix(model.ID) {
					continue
				}
				if strings.Contains(id, "banana") || strings.Contains(id, "image") || strings.Contains(id, "computer") || strings.Contains(id, "robotics") || strings.Contains(id, "tts") || strings.Contains(id, "custom") || strings.Contains(id, "latest") || strings.Contains(id, "omni") || strings.Contains(id, "00") {
					continue
				}
				filtered = append(filtered, model)
			}
			models = filtered
		}
		if m.provider == llm.ProviderGemini {
			for i, j := 0, len(models)-1; i < j; i, j = i+1, j-1 {
				models[i], models[j] = models[j], models[i]
			}
		}
		if m.provider == llm.ProviderOpenAI {
			for i, j := 0, len(models)-1; i < j; i, j = i+1, j-1 {
				models[i], models[j] = models[j], models[i]
			}
		}
		m.models = models
		items := make([]list.Item, len(models))
		for i, model := range models {
			items[i] = modelItem{info: model}
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
				return m, tea.Batch(m.persistSessionCmd(), startChatStreamCmd(m))
			}
			return m, m.persistSessionCmd()
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
					return m, tea.Batch(m.persistSessionCmd(), startChatStreamCmd(m))
				}
				return m, m.persistSessionCmd()
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
			return m, tea.Batch(m.persistSessionCmd(), startChatStreamCmd(m))
		}
		return m, m.persistSessionCmd()

	case chatStreamUsage:
		if msg.inputTokens > 0 {
			m.inputTokens = msg.inputTokens
		}
		if msg.outputTokens > 0 {
			m.outputTokens += msg.outputTokens
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
		return m, m.persistSessionCmd()

	case updateCheckResult:
		if msg.err != nil || msg.latestVersion == "" {
			return m, nil
		}

		if !msg.needsUpdate {
			if cfg, _ := config.Load(); cfg != nil && cfg.UpdateVersion != "" {
				cfg.UpdateVersion = ""
				config.Save(cfg)
			}
			return m, nil
		}

		cfg, _ := config.Load()
		if cfg != nil && cfg.UpdateVersion == msg.latestVersion {
			return m, performUpdateCmd(msg.latestVersion)
		}

		if cfg == nil {
			cfg = &config.Config{}
		}
		cfg.UpdateVersion = msg.latestVersion
		if err := config.Save(cfg); err != nil {
			return m, nil
		}

		m.updateAvailable = true
		m.latestVersion = msg.latestVersion
		return m, nil

	case updatePerformResult:
		if msg.upToDate {
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: "You're already on the latest version.",
			})
		} else if msg.err != nil {
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("_Update failed: %v_", msg.err),
			})
		}
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case sessionSaveErrorMsg:
		m.messages = append(m.messages, llm.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("_Session save failed: %v_", msg.err),
		})
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
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
		key, _ := config.GetAPIKey(m.provider, m.customURL, m.savedEndpointName)
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
	key, _ := config.GetAPIKey(m.provider, m.customURL, m.savedEndpointName)
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
	// Delete confirmation mode
	if m.confirmDeleteEndpoint != "" {
		switch msg.String() {
		case "y", "Y":
			cfg, _ := config.Load()
			if cfg != nil {
				cfg.RemoveSavedEndpoint(m.confirmDeleteEndpoint)
				config.DeleteAPIKey("custom", "", m.confirmDeleteEndpoint)
				if err := config.Save(cfg); err != nil {
					m.err = err
					m.errChoice = 0
					m.state = stateError
					return m, nil
				}
				// Rebuild provider list
				m.providerList.SetItems(buildProviderItems(cfg.SavedEndpoints))
			}
			if m.savedEndpointName == m.confirmDeleteEndpoint {
				m.savedEndpointName = ""
				m.customURL = ""
				m.provider = ""
				m.modelName = ""
			}
			m.confirmDeleteEndpoint = ""
			return m, nil
		case "n", "N", "esc":
			m.confirmDeleteEndpoint = ""
			return m, nil
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.providerList, cmd = m.providerList.Update(msg)

	if msg.String() == "esc" {
		return m.enterChatState(), nil
	}

	// Delete saved endpoint
	if msg.String() == "d" {
		idx := m.providerList.Index()
		cfg, _ := config.Load()
		savedEndpoints := []config.SavedEndpoint{}
		if cfg != nil {
			savedEndpoints = cfg.SavedEndpoints
		}
		res := resolveProviderPick(savedEndpoints, idx)
		if res.kind == pickSavedEndpoint && res.savedEndpointIdx >= 0 && res.savedEndpointIdx < len(savedEndpoints) {
			m.confirmDeleteEndpoint = savedEndpoints[res.savedEndpointIdx].Name
			return m, nil
		}
		return m, nil
	}

	if msg.String() != "enter" {
		return m, cmd
	}

	cfg, _ := config.Load()
	savedEndpoints := []config.SavedEndpoint{}
	if cfg != nil {
		savedEndpoints = cfg.SavedEndpoints
	}
	res := resolveProviderPick(savedEndpoints, m.providerList.Index())

	switch res.kind {
	case pickOpenAI:
		m.provider = llm.ProviderOpenAI
		m.customURL = ""
		m.savedEndpointName = ""
		return m.continueProviderPick()

	case pickAnthropic:
		m.provider = llm.ProviderAnthropic
		m.customURL = ""
		m.savedEndpointName = ""
		return m.continueProviderPick()

	case pickGemini:
		m.provider = llm.ProviderGemini
		m.customURL = ""
		m.savedEndpointName = ""
		return m.continueProviderPick()

	case pickSavedEndpoint:
		if res.savedEndpointIdx >= 0 && res.savedEndpointIdx < len(savedEndpoints) {
			ep := savedEndpoints[res.savedEndpointIdx]
			m.provider = llm.ProviderCustom
			m.customURL = ep.BaseURL
			m.savedEndpointName = ep.Name
			return m.continueProviderPick()
		}
		return m, nil

	case pickCustom:
		m.provider = llm.ProviderCustom
		m.customURL = ""
		m.savedEndpointName = ""
		m.customMode = 0
		m.customModeCursor = 0
		m.state = stateCustomModePick
		return m, nil
	}

	return m, nil
}

func (m model) updateCustomModePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.customModeCursor--
		if m.customModeCursor < 0 {
			m.customModeCursor = 1
		}
	case "down":
		m.customModeCursor++
		if m.customModeCursor > 1 {
			m.customModeCursor = 0
		}
	case "esc":
		m.state = stateProviderPick
		return m, nil
	case "enter":
		switch m.customModeCursor {
		case 0:
			m.customMode = customModeOneTime
			m.state = stateCustomURL
			m.urlInput.Focus()
		case 1:
			m.customMode = customModeSave
			m.state = stateCustomName
			m.nameInput.Reset()
			m.nameInput.Focus()
		}
		return m, nil
	}
	return m, nil
}

func (m model) continueProviderPick() (tea.Model, tea.Cmd) {
	key, _ := config.GetAPIKey(m.provider, m.customURL, m.savedEndpointName)
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
		if m.customMode == customModeSave {
			m.state = stateCustomName
			m.nameInput.Focus()
		} else {
			m.state = stateCustomModePick
		}
		return m, nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	m.customURL = strings.TrimSpace(m.urlInput.Value())
	if m.customURL == "" {
		return m, nil
	}

	if m.forceKeyAfterURL {
		m.forceKeyAfterURL = false
		m.state = stateAPIKeyInput
		m.keyInput.Focus()
		return m, nil
	}

	key, _ := config.GetAPIKey(m.provider, m.customURL, m.savedEndpointName)
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
		if m.customMode == customModeSave {
			m.state = stateCustomURL
			m.urlInput.Focus()
		} else if m.savedEndpointName != "" {
			m.state = stateProviderPick
		} else {
			m.state = stateCustomURL
			m.urlInput.Focus()
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

	if m.customMode == customModeSave {
		cfg, _ := config.Load()
		if cfg == nil {
			cfg = &config.Config{}
		}

		if err := cfg.AddSavedEndpoint(m.savedEndpointName, m.customURL); err != nil {
			m.err = err
			m.errChoice = 0
			m.state = stateError
			return m, nil
		}

		if err := config.SetAPIKey(m.provider, m.customURL, m.savedEndpointName, key); err != nil {
			m.err = err
			m.errChoice = 0
			m.state = stateError
			return m, nil
		}

		cfg.Provider = m.provider
		cfg.Model = m.modelName
		cfg.CustomBaseURL = m.customURL
		cfg.SavedEndpointName = m.savedEndpointName
		cfg.ReasoningVisible = m.reasoning.defaultVisible
		cfg.ThinkingType = m.thinkingType
		cfg.EffortLevel = m.effortLevel

		if err := config.Save(cfg); err != nil {
			m.err = err
			m.errChoice = 0
			m.state = stateError
			return m, nil
		}

		// Rebuild provider list to include new saved endpoint
		m.providerList.SetItems(buildProviderItems(cfg.SavedEndpoints))
		m.customMode = 0

		if m.modelName != "" {
			return m.enterChatState(), nil
		}

		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			fetchModelsCmd(m.provider, m.apiKey, m.customURL),
		)
	}

	// One-time: save key and proceed
	if err := config.SetAPIKey(m.provider, m.customURL, m.savedEndpointName, key); err != nil {
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

func (m model) updateCustomName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)

	if msg.String() == "esc" {
		m.state = stateCustomModePick
		return m, nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	name := strings.TrimSpace(m.nameInput.Value())
	if name == "" {
		return m, nil
	}
	m.savedEndpointName = name

	// Name entered, now ask for URL
	m.state = stateCustomURL
	m.urlInput.Reset()
	m.urlInput.Focus()
	return m, nil
}

func saveConfig(m model) error {
	cfg, _ := config.Load()
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfg.Provider = m.provider
	cfg.Model = m.modelName
	cfg.CustomBaseURL = m.customURL
	cfg.SavedEndpointName = m.savedEndpointName
	cfg.ReasoningVisible = m.reasoning.defaultVisible
	cfg.ThinkingType = m.thinkingType
	cfg.EffortLevel = m.effortLevel
	return config.Save(cfg)
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

	selected, ok := m.modelList.SelectedItem().(modelItem)
	if !ok {
		return m, nil
	}
	m.modelName = selected.info.ID
	m.maxInputTokens = selected.info.MaxInputTokens

	switch m.provider {
	case llm.ProviderAnthropic:
		m.thinkingOptions = nil
		if selected.info.Capabilities.Thinking.Types.Adaptive.Supported {
			m.thinkingOptions = append(m.thinkingOptions, "adaptive")
		}
		if selected.info.Capabilities.Thinking.Types.Enabled.Supported {
			m.thinkingOptions = append(m.thinkingOptions, "enabled")
		}
		m.effortOptions = nil
		eff := selected.info.Capabilities.Effort
		if eff.Low.Supported {
			m.effortOptions = append(m.effortOptions, "low")
		}
		if eff.Medium.Supported {
			m.effortOptions = append(m.effortOptions, "medium")
		}
		if eff.High.Supported {
			m.effortOptions = append(m.effortOptions, "high")
		}
		if eff.XHigh.Supported {
			m.effortOptions = append(m.effortOptions, "xhigh")
		}
		if eff.Max.Supported {
			m.effortOptions = append(m.effortOptions, "max")
		}
		if len(m.thinkingOptions) == 0 {
			m.thinkingOptions = []string{"adaptive", "enabled", "disabled"}
		}
		if len(m.effortOptions) == 0 {
			m.effortOptions = []string{"low", "medium", "high"}
		}

		m.reasoningField = 0
		if m.thinkingType == "" {
			m.thinkingType = m.thinkingOptions[0]
		}
		if m.effortLevel == "" {
			m.effortLevel = m.effortOptions[0]
		}

		m.state = stateReasoningConfig
		return m, nil

	case llm.ProviderOpenAI, llm.ProviderGemini:
		m.thinkingOptions = nil
		m.effortOptions = nil
		eff := selected.info.Capabilities.Effort
		if eff.Minimal.Supported {
			m.effortOptions = append(m.effortOptions, "minimal")
		}
		if eff.Low.Supported {
			m.effortOptions = append(m.effortOptions, "low")
		}
		if eff.Medium.Supported {
			m.effortOptions = append(m.effortOptions, "medium")
		}
		if eff.High.Supported {
			m.effortOptions = append(m.effortOptions, "high")
		}
		if eff.XHigh.Supported {
			m.effortOptions = append(m.effortOptions, "xhigh")
		}
		if eff.Max.Supported {
			m.effortOptions = append(m.effortOptions, "max")
		}
		if selected.info.ThinkingHasNone() {
			m.effortOptions = append([]string{"none"}, m.effortOptions...)
		}

		if len(m.effortOptions) == 0 {
			break
		}

		m.reasoningField = 0
		if m.effortLevel == "" {
			m.effortLevel = m.effortOptions[0]
		}

		m.state = stateReasoningConfig
		return m, nil
	}

	if err := saveConfig(m); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}

	return m.enterChatState(), nil
}

func (m model) updateReasoningConfig(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "down":
		if len(m.thinkingOptions) > 0 {
			if msg.String() == "up" {
				m.reasoningField--
				if m.reasoningField < 0 {
					m.reasoningField = 1
				}
			} else {
				m.reasoningField++
				if m.reasoningField > 1 {
					m.reasoningField = 0
				}
			}
		}
	case "left":
		if m.reasoningField == 0 && len(m.thinkingOptions) > 0 {
			for i := len(m.thinkingOptions) - 1; i > 0; i-- {
				if m.thinkingOptions[i] == m.thinkingType {
					m.thinkingType = m.thinkingOptions[i-1]
					break
				}
			}
		} else {
			for i := len(m.effortOptions) - 1; i > 0; i-- {
				if m.effortOptions[i] == m.effortLevel {
					m.effortLevel = m.effortOptions[i-1]
					break
				}
			}
		}
	case "right":
		if m.reasoningField == 0 && len(m.thinkingOptions) > 0 {
			for i := 0; i < len(m.thinkingOptions)-1; i++ {
				if m.thinkingOptions[i] == m.thinkingType {
					m.thinkingType = m.thinkingOptions[i+1]
					break
				}
			}
		} else {
			for i := 0; i < len(m.effortOptions)-1; i++ {
				if m.effortOptions[i] == m.effortLevel {
					m.effortLevel = m.effortOptions[i+1]
					break
				}
			}
		}
	case "esc":
		m.state = stateModelPick
		return m, nil
	case "enter":
		if err := saveConfig(m); err != nil {
			m.err = err
			m.errChoice = 0
			m.state = stateError
			return m, nil
		}
		return m.enterChatState(), nil
	}
	return m, nil
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
		if m.provider == llm.ProviderCustom {
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
				m.forceKeyAfterURL = true
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
		} else {
			switch m.errChoice {
			case 0:
				m.state = stateModelFetch
				return m, tea.Batch(
					m.spinner.Tick,
					fetchModelsCmd(m.provider, m.apiKey, m.customURL),
				)
			case 1:
				m.state = stateAPIKeyInput
				m.keyInput.Focus()
				return m, nil
			case 2:
				m.state = stateManualModel
				m.manualInput.Focus()
				return m, nil
			case 3:
				return m, tea.Quit
			}
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

	if err := saveConfig(m); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}

	return m.enterChatState(), nil
}

func (m model) updateSessionPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.sessionList, cmd = m.sessionList.Update(msg)

	if msg.String() == "esc" {
		return m.enterChatState(), nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	selected, ok := m.sessionList.SelectedItem().(sessionItem)
	if !ok {
		return m, cmd
	}
	if selected.meta.ID == m.sessionID {
		return m.enterChatState(), nil
	}

	var saveCmd tea.Cmd
	if len(m.messages) > 0 {
		saveCmd = m.persistSessionCmd()
	}

	loaded, err := sessions.Load(m.workspaceRoot, selected.meta.ID)
	if err != nil {
		m.messages = append(m.messages, llm.Message{
			Role:    "assistant",
			Content: fmt.Sprintf("_Failed to load session: %v_", err),
		})
		m.state = stateChat
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		return m, saveCmd
	}

	m = m.applySession(loaded)
	m = m.enterChatState()
	return m, saveCmd
}

func (m model) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.updateCheckStarted {
		m.updateCheckStarted = true
		m2, cmd := m.handleChatMessage(msg)
		return m2, tea.Batch(checkForUpdateCmd(), cmd)
	}
	return m.handleChatMessage(msg)
}

func (m model) handleChatMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
				return m, m.persistSessionCmd()
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

		return m, tea.Batch(m.persistSessionCmd(), startChatStreamCmd(m))
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
			return m, m.persistSessionCmd()
		}
		m = m.executeTool(tc)
	}
	return m, tea.Batch(m.persistSessionCmd(), startChatStreamCmd(m))
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
		m.savedEndpointName = ""
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
		saveConfig(m)
		m.messages = append(m.messages, llm.Message{
			Role: "assistant",
			Content: fmt.Sprintf("Reasoning changed to %s (was %s)",
				map[bool]string{true: "visible", false: "hidden"}[newVisible],
				map[bool]string{true: "visible", false: "hidden"}[oldVisible]),
		})
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case "thinking":
		if len(parts) < 2 {
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Current thinking type: %s\nUsage: /thinking <type>  (adaptive, enabled, disabled)", m.thinkingType),
			})
			m.chatViewport.SetContent(buildChatContent(m))
			m.chatViewport.GotoBottom()
			return m, nil
		}
		newType := strings.ToLower(parts[1])
		switch newType {
		case "adaptive", "enabled", "disabled":
			oldType := m.thinkingType
			m.thinkingType = newType
			saveConfig(m)
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Thinking changed to %s (was %s)", newType, oldType),
			})
		default:
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Unknown thinking type: %s. Available: adaptive, enabled, disabled", newType),
			})
		}
		m.chatViewport.SetContent(buildChatContent(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case "effort":
		if len(parts) < 2 {
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Current effort level: %s\nUsage: /effort <level>  (minimal, low, medium, high, xhigh, max)", m.effortLevel),
			})
			m.chatViewport.SetContent(buildChatContent(m))
			m.chatViewport.GotoBottom()
			return m, nil
		}
		newEffort := strings.ToLower(parts[1])
		switch newEffort {
		case "minimal", "low", "medium", "high", "xhigh", "max":
			oldEffort := m.effortLevel
			m.effortLevel = newEffort
			saveConfig(m)
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Effort changed to %s (was %s)", newEffort, oldEffort),
			})
		default:
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("Unknown effort level: %s. Available: minimal, low, medium, high, xhigh, max", newEffort),
			})
		}
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

	case "session":
		if m.isStreaming {
			return m, nil
		}
		metas, err := sessions.List(m.workspaceRoot)
		if err != nil {
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: fmt.Sprintf("_Failed to list sessions: %v_", err),
			})
			m.chatViewport.SetContent(buildChatContent(m))
			m.chatViewport.GotoBottom()
			return m, nil
		}
		items := make([]list.Item, len(metas))
		for i, meta := range metas {
			items[i] = sessionItem{meta: meta, active: meta.ID == m.sessionID}
		}
		m.sessionList.SetItems(items)
		m.state = stateSessionPick
		return m, nil

	case "new":
		if m.isStreaming {
			return m, nil
		}
		var saveCmd tea.Cmd
		if len(m.messages) > 0 {
			saveCmd = m.persistSessionCmd()
		}
		m = m.resetToNewSession()
		m = m.enterChatState()
		return m, saveCmd

	case "update":
		if m.isStreaming {
			return m, nil
		}
		if m.latestVersion == "" {
			return m, checkAndUpdateCmd()
		}
		return m, performUpdateCmd(m.latestVersion)

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

		var thinkingCfg *llm.ThinkingConfig
		if m.thinkingType == "adaptive" && m.provider == llm.ProviderAnthropic {
			thinkingCfg = &llm.ThinkingConfig{Type: "adaptive"}
		} else if m.thinkingType == "enabled" && m.provider == llm.ProviderAnthropic {
			thinkingCfg = &llm.ThinkingConfig{Type: "enabled", BudgetTokens: 32000}
		}

		reasoningEffort := ""
		if (m.provider == llm.ProviderOpenAI || m.provider == llm.ProviderGemini) && m.effortLevel != "" && m.effortLevel != "none" {
			reasoningEffort = m.effortLevel
		}

		req := llm.ChatRequest{
			Model:           m.modelName,
			Messages:        m.messages,
			SystemPrompt:    systemPrompt,
			Tools:           tools.Definitions(),
			Thinking:        thinkingCfg,
			MaxTokens:       128000,
			ReasoningEffort: reasoningEffort,
		}

		events, err := llm.StreamChatCompletion(ctx, m.provider, m.apiKey, baseURL, req)
		if err != nil {
			cancel()
			m.streamState.cancel = nil
			return chatStreamError{err: err}
		}

		go func() {
			var pendingToolCalls []llm.ToolCall
			doneSent := false
			for event := range events {
				switch event.Type {
				case llm.StreamDelta:
					globalProgram.Send(chatStreamChunk{content: event.Content})
				case llm.StreamReasoning:
					globalProgram.Send(chatStreamReasoning{content: event.Content})
				case llm.StreamUsage:
					globalProgram.Send(chatStreamUsage{inputTokens: event.InputTokens, outputTokens: event.OutputTokens})
				case llm.StreamToolCalls:
					pendingToolCalls = event.ToolCalls
				case llm.StreamDone:
					if !doneSent {
						calls := event.ToolCalls
						if len(calls) == 0 && len(pendingToolCalls) > 0 {
							calls = pendingToolCalls
						}
						globalProgram.Send(chatStreamDone{toolCalls: calls})
						doneSent = true
					}
					return
				case llm.StreamError:
					globalProgram.Send(chatStreamError{err: event.Err})
					return
				}
			}
			if !doneSent {
				globalProgram.Send(chatStreamDone{})
			}
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
			b.WriteString(m.styles.userLabel.Render("you"))
			b.WriteString("\n")
			b.WriteString(msg.Content)
			b.WriteString("\n\n")
		case "assistant":
			b.WriteString(m.styles.header.Render("gurtcli"))
			b.WriteString("\n")
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					b.WriteString(m.styles.toolLabel.Render(fmt.Sprintf("  %s", tc.Function.Name)))
					b.WriteString("\n")
					renderToolCallArgs(&b, m, tc)
				}
			}
			if msg.Reasoning != "" {
				b.WriteString(m.styles.reasoningToggle.Render("[▶ Show reasoning]"))
				b.WriteString("\n")
			}
			if msg.Content != "" {
				lines := strings.Split(msg.Content, "\n")
				for i, line := range lines {
					lines[i] = m.styles.divider.Render("│") + " " + line
				}
				b.WriteString(strings.Join(lines, "\n"))
			}
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
			content := m.messages[len(m.messages)-1].Content
			if content != "" {
				lines := strings.Split(content, "\n")
				for i, line := range lines {
					lines[i] = m.styles.divider.Render("│") + " " + line
				}
				b.WriteString(strings.Join(lines, "\n"))
			}
		} else if streamingLen > 0 && m.streamingContent != nil {
			content := m.streamingContent.String()
			if content != "" {
				lines := strings.Split(content, "\n")
				for i, line := range lines {
					lines[i] = m.styles.divider.Render("│") + " " + line
				}
				b.WriteString(strings.Join(lines, "\n"))
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

func renderToolCallArgs(b *strings.Builder, m model, tc llm.ToolCall) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return
	}

	switch tc.Function.Name {
	case "run_bash":
		if title, ok := args["title"].(string); ok && title != "" {
			b.WriteString(m.styles.dim.Render(fmt.Sprintf("  %s", title)))
			b.WriteString("\n")
		}
		if cmd, ok := args["command"].(string); ok && cmd != "" {
			b.WriteString(fmt.Sprintf("  $ %s", cmd))
			b.WriteString("\n")
		}

	case "edit_file":
		if path, ok := args["filePath"].(string); ok && path != "" {
			b.WriteString(m.styles.dim.Render(fmt.Sprintf("  %s", path)))
			b.WriteString("\n")
		}
		oldStr, _ := args["oldString"].(string)
		newStr, _ := args["newString"].(string)
		if oldStr != "" || newStr != "" {
			oldLines := strings.Split(oldStr, "\n")
			newLines := strings.Split(newStr, "\n")
			for _, l := range oldLines {
				b.WriteString(m.styles.diffDel.Render(fmt.Sprintf("  - %s", l)))
				b.WriteString("\n")
			}
			for _, l := range newLines {
				b.WriteString(m.styles.diffAdd.Render(fmt.Sprintf("  + %s", l)))
				b.WriteString("\n")
			}
		}

	case "write_file":
		if path, ok := args["filePath"].(string); ok && path != "" {
			b.WriteString(m.styles.dim.Render(fmt.Sprintf("  %s", path)))
			b.WriteString("\n")
		}

	default:
		if path, ok := args["filePath"].(string); ok && path != "" {
			b.WriteString(m.styles.dim.Render(fmt.Sprintf("  %s", path)))
			b.WriteString("\n")
		}
	}
}

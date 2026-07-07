package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/rand"
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
	"github.com/charmbracelet/lipgloss"
	"github.com/sillygru/gurtcli/config"
	"github.com/sillygru/gurtcli/debug"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/sessions"
	"github.com/sillygru/gurtcli/tools"
	"github.com/sillygru/gurtcli/ui"
)

//go:embed prompts/system.md
var systemPromptTemplate string

//go:embed prompts/session-title.md
var sessionTitlePrompt string

type sessionTitleGeneratedMsg struct {
	title string
}

var dateSuffixRegex = regexp.MustCompile(`-\d{8}$|-\d{4}-\d{2}-\d{2}$`)

// partialMouseEventRe matches the tail of a split SGR mouse sequence
// (e.g. "<64;117;26M") that the input reader parsed as key runes.
var partialMouseEventRe = regexp.MustCompile(`^<\d+;\d+;\d+[Mm]?$`)

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

		chatViewHeight := msg.Height - 6
		if chatViewHeight < 4 {
			chatViewHeight = 4
		}
		m.chatViewport.Width = msg.Width - 4
		m.chatViewport.Height = chatViewHeight
		m.chatInput.Width = msg.Width - 4
		return m.adjustViewportHeight(), nil

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
		case stateAllowManage:
			return m.updateAllowManage(msg)
		case stateDotenvPrompt:
			return m.updateDotenvPrompt(msg)
		case stateDotenvPick:
			return m.updateDotenvPick(msg)
		case stateDotenvKeyName:
			return m.updateDotenvKeyName(msg)
		case stateDotenvKeyExists:
			return m.updateDotenvKeyExists(msg)
		}
		return m, nil

	case spinner.TickMsg:
		if m.state == stateModelFetch {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case llmDetailsLoadedMsg:
		m.llmDetails = msg.details
		m.llmDetailsReady = true
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
			excluded := make([]string, 0)
			filtered := models[:0]
			for _, model := range models {
				id := strings.ToLower(model.ID)
				if strings.Contains(id, "transcribe") || strings.Contains(id, "embed") || strings.Contains(id, "tts") || strings.Contains(id, "search") || strings.Contains(id, "chat-latest") {
					excluded = append(excluded, model.ID)
					continue
				}
				if llm.IsTextChatModel(model.ID) && !hasDateSuffix(model.ID) {
					filtered = append(filtered, model)
				} else {
					excluded = append(excluded, model.ID)
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
			excluded := make([]string, 0)
			filtered := models[:0]
			for _, model := range models {
				id := strings.ToLower(model.ID)
				if !strings.HasPrefix(id, "gemini-") || hasDateSuffix(model.ID) {
					excluded = append(excluded, model.ID)
					continue
				}
				if strings.Contains(id, "banana") || strings.Contains(id, "image") || strings.Contains(id, "computer") || strings.Contains(id, "robotics") || strings.Contains(id, "tts") || strings.Contains(id, "custom") || strings.Contains(id, "latest") || strings.Contains(id, "omni") || strings.Contains(id, "00") {
					excluded = append(excluded, model.ID)
					continue
				}
				filtered = append(filtered, model)
			}
			models = filtered
		}
		if m.provider == llm.ProviderGemini || m.provider == llm.ProviderOpenAI {
			for i, j := 0, len(models)-1; i < j; i, j = i+1, j-1 {
				models[i], models[j] = models[j], models[i]
			}
		}
		m.models = models
		items := make([]list.Item, len(models))
		for i, model := range models {
			items[i] = modelItem{info: model, provider: m.provider}
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
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
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
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case chatStreamDone:
		if m.cancelRequested {
			m.cancelRequested = false
			m.streamingContent = nil
			m.isStreaming = false
			m.workingMsg = ""
			m.workingSpinnerIdx = 0
			m.streamState.cancel = nil
			m.reasoning = reasoningState{}
			m.messages = append(m.messages, llm.Message{
				Role:    "assistant",
				Content: "_Interrupted_",
			})
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			m.chatViewport.GotoBottom()
			m.chatInput.Focus()
			if m.queuedMessage != "" {
				return m.replayQueuedMessage()
			}
			return m, m.persistSessionCmd()
		}

		contentStr := ""
		if m.streamingContent != nil {
			contentStr = strings.TrimSpace(m.streamingContent.String())
		}
		reasoningStr := ""
		if m.reasoning.content != nil {
			reasoningStr = strings.TrimSpace(m.reasoning.content.String())
		}
		m.streamingContent = nil
		m.isStreaming = false
		m.workingMsg = ""
		m.workingSpinnerIdx = 0
		m.streamState.cancel = nil
		if m.reasoning.active {
			m.reasoning.duration = time.Since(m.reasoning.startTime).Round(100 * time.Millisecond)
			m.reasoning.active = false
			m.reasoning.content = nil
		}

		if len(msg.toolCalls) > 0 {
			asm := llm.Message{Role: "assistant", Content: contentStr, Model: m.modelName}
			if reasoningStr != "" {
				asm.Reasoning = reasoningStr
				asm.ReasoningDuration = m.reasoning.duration
			}
			asm.ToolCalls = msg.toolCalls
			m.messages = append(m.messages, asm)
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			m.chatViewport.GotoBottom()
			m.toolCallCycle++
			if m.toolCallCycle > maxToolCallCycles {
				m.messages = append(m.messages, llm.Message{
					Role:    "assistant",
					Content: "_Interrupted_",
					Model:   m.modelName,
				})
				m.toolCallCycle = 0
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
				m.chatViewport.GotoBottom()
				m.chatInput.Focus()
				if m.queuedMessage != "" {
					return m.replayQueuedMessage()
				}
				return m, m.persistSessionCmd()
			}
			return m.processToolCalls(msg.toolCalls)
		}

		if contentStr != "" || reasoningStr != "" {
			msg := llm.Message{Role: "assistant", Content: contentStr, Model: m.modelName}
			if reasoningStr != "" {
				msg.Reasoning = reasoningStr
				msg.ReasoningDuration = m.reasoning.duration
			}
			m.messages = append(m.messages, msg)
		}
		m.toolCallCycle = 0
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		m.chatInput.Focus()
		if m.queuedMessage != "" {
			return m.replayQueuedMessage()
		}
		return m, m.persistSessionCmd()

	case chatStreamUsage:
		if msg.inputTokens > 0 {
			if msg.inputTokens >= m.contextInputTokens {
				m.contextInputTokens = msg.inputTokens
			} else {
				m.contextInputTokens += msg.inputTokens
			}
			m.inputTokens += msg.inputTokens
		}
		if msg.outputTokens > 0 {
			m.outputTokens += msg.outputTokens
		}
		return m, nil

	case workingTickMsg:
		if !m.isStreaming {
			return m, nil
		}
		m.workingSpinnerIdx++
		if m.workingSpinnerIdx%40 == 0 {
			m.workingMsg = workingMessages[rand.Intn(len(workingMessages))]
		}
		return m, workingTickCmd()

	case toastTimeoutMsg:
		if m.toast != nil && m.toast.id == msg.id {
			if m.yolo {
				m.toastSeq++
				m.toast = &toastMsg{text: "YOLO mode", id: m.toastSeq}
			} else {
				m.toast = nil
			}
		}
		return m, nil

	case chatStreamError:
		m.streamingContent = nil
		m.isStreaming = false
		m.workingMsg = ""
		m.workingSpinnerIdx = 0
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
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		m.chatInput.Focus()
		return m, m.persistSessionCmd()

	case sessionTitleGeneratedMsg:
		if msg.title != "" && m.sessionName == "" {
			m.sessionName = msg.title
			return m, tea.Batch(m.persistSessionCmd(), tea.SetWindowTitle("gurt | "+m.sessionName))
		}
		return m, nil

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
				Role:     "assistant",
				Internal: true,
				Content:  "You're already on the latest version.",
			})
		} else if msg.err != nil {
			m.messages = append(m.messages, llm.Message{
				Role:     "assistant",
				Internal: true,
				Content:  fmt.Sprintf("_Update failed: %v_", msg.err),
			})
		}
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case sessionSaveErrorMsg:
		m.messages = append(m.messages, llm.Message{
			Role:     "assistant",
			Internal: true,
			Content:  fmt.Sprintf("_Session save failed: %v_", msg.err),
		})
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case resourceStatsMsg:
		m.debugStats = resourceStats{cpuPercent: msg.cpuPercent, memMB: msg.memMB}
		if m.debug {
			return m, resourceMonitorTickCmd()
		}
		return m, nil

	case versionCheckResult:
		var b strings.Builder
		b.WriteString(VersionString())
		b.WriteString("\n")
		if msg.err != nil {
			b.WriteString(fmt.Sprintf("? Could not check for updates: %v", msg.err))
		} else if msg.needsUpdate {
			b.WriteString(fmt.Sprintf("✗ A new version is available: %s\n  Run /update to upgrade.", msg.latestVersion))
		} else {
			b.WriteString("✓ You're on the latest version.")
		}
		m.messages = append(m.messages, llm.Message{
			Role:     "assistant",
			Internal: true,
			Content:  b.String(),
		})
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
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
				m.fetchModelsCmd(),
			)
		}
		m2, cmd := m.enterChatState()
	return m2, cmd
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
			m.fetchModelsCmd(),
		)
	}
	m2, cmd := m.enterChatState()
	return m2, cmd
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
		m2, cmd := m.enterChatState()
	return m2, cmd
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

func (m model) currentModelInfo() llm.ModelInfo {
	for i := range m.models {
		if m.models[i].ID == m.modelName {
			return m.models[i]
		}
	}
	if m.llmDetailsReady {
		if info, ok := m.llmDetails[m.modelName]; ok {
			return info
		}
	}
	return llm.ModelInfo{ID: m.modelName}
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
			m2, cmd := m.enterChatState()
	return m2, cmd
		}
		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			m.fetchModelsCmd(),
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
			m2, cmd := m.enterChatState()
	return m2, cmd
		}
		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			m.fetchModelsCmd(),
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

	if err := config.SetAPIKey(m.provider, m.customURL, m.savedEndpointName, key); err != nil {
		m.err = err
		m.state = stateDotenvPrompt
		m.dotenvCursor = 0
		return m, nil
	}

	return m.continueAfterAPIKey()
}

func (m model) updateDotenvPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "down":
		m.dotenvCursor = (m.dotenvCursor + 1) % 2
	case "enter":
		switch m.dotenvCursor {
		case 0:
			return m.continueAfterAPIKey()
		case 1:
			m.dotenvInput.SetValue("GURT_API_KEY")
			m.dotenvInput.Focus()
			m.state = stateDotenvKeyName
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateDotenvPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	total := len(m.dotenvKeys) + 1
	switch msg.String() {
	case "up":
		m.dotenvPickCursor--
		if m.dotenvPickCursor < 0 {
			m.dotenvPickCursor = total - 1
		}
	case "down":
		m.dotenvPickCursor++
		if m.dotenvPickCursor >= total {
			m.dotenvPickCursor = 0
		}
	case "enter":
		if m.dotenvPickCursor < len(m.dotenvKeys) {
			dk, err := config.GetDotenvKeys()
			if err != nil {
				m.err = err
				m.errChoice = 0
				m.state = stateError
				return m, nil
			}
			name := m.dotenvKeys[m.dotenvPickCursor]
			m.apiKey = dk[name]
			m.dotenvKeyName = name
			return m.continueAfterAPIKey()
		}
		m.state = stateAPIKeyInput
		m.keyInput.Reset()
		m.keyInput.Focus()
		return m, nil
	}
	return m, nil
}

func (m model) updateDotenvKeyName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.dotenvInput, cmd = m.dotenvInput.Update(msg)

	if msg.String() == "esc" {
		m.state = stateDotenvPrompt
		return m, nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	name := strings.TrimSpace(m.dotenvInput.Value())
	if name == "" {
		return m, nil
	}

	dk, err := config.GetDotenvKeys()
	if err == nil {
		if existing, ok := dk[name]; ok {
			m.dotenvKeyName = name
			m.dotenvExistingKeyValue = existing
			m.dotenvKeyExistsCursor = 0
			m.state = stateDotenvKeyExists
			return m, nil
		}
	}

	if err := config.SaveDotenv(name, m.apiKey); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}
	m.dotenvKeyName = name
	return m.continueAfterAPIKey()
}

func (m model) updateDotenvKeyExists(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.dotenvKeyExistsCursor = (m.dotenvKeyExistsCursor + 2) % 3
	case "down":
		m.dotenvKeyExistsCursor = (m.dotenvKeyExistsCursor + 1) % 3
	case "enter":
		switch m.dotenvKeyExistsCursor {
		case 0:
			if err := config.SaveDotenv(m.dotenvKeyName, m.apiKey); err != nil {
				m.err = err
				m.errChoice = 0
				m.state = stateError
				return m, nil
			}
			return m.continueAfterAPIKey()
		case 1:
			m.apiKey = m.dotenvExistingKeyValue
			return m.continueAfterAPIKey()
		case 2:
			m.dotenvInput.Reset()
			m.dotenvInput.Focus()
			m.state = stateDotenvKeyName
			return m, nil
		}
	}
	return m, nil
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
	cfg.AlwaysAllowTools = m.alwaysAllowTools
	cfg.AlwaysAllowCommandPrefixes = m.alwaysAllowCommandPrefixes
	cfg.TelemetryEnabled = &m.telemetryEnabled
	cfg.Theme = m.themeName
	return config.Save(cfg)
}

func (m model) updateModelPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.modelList, cmd = m.modelList.Update(msg)

	if msg.String() == "esc" {
		m2, cmd := m.enterChatState()
	return m2, cmd
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
		for _, level := range selected.info.Capabilities.ThinkingLevels {
			if level == "enabled" || level == "adaptive" {
				m.thinkingOptions = append(m.thinkingOptions, level)
			}
		}
		if len(m.thinkingOptions) == 0 {
			m.thinkingOptions = []string{"adaptive", "enabled", "disabled"}
		}

		m.effortOptions = append([]string(nil), selected.info.Capabilities.EffortLevels...)
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
		return m.showReasoningConfig()

	case llm.ProviderOpenAI, llm.ProviderGemini, llm.ProviderCustom:
		m.thinkingOptions = nil
		m.thinkingType = ""
		m.effortOptions = selected.info.ReasoningLevelOptions()

		if len(m.effortOptions) == 0 {
			break
		}
		return m.showReasoningConfig()
	}

	if err := saveConfig(m); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}

	m2, cmd := m.enterChatState()
	return m2, cmd
}

func (m model) showReasoningConfig() (tea.Model, tea.Cmd) {
	if len(m.thinkingOptions) > 0 && m.thinkingType == "" {
		m.thinkingType = m.thinkingOptions[0]
	}
	if len(m.effortOptions) > 0 && m.effortLevel == "" {
		m.effortLevel = m.effortOptions[0]
	}
	if len(m.effortOptions) > 0 {
		valid := false
		for _, opt := range m.effortOptions {
			if opt == m.effortLevel {
				valid = true
				break
			}
		}
		if !valid {
			m.effortLevel = m.effortOptions[0]
		}
	}
	if len(m.thinkingOptions) > 0 {
		valid := false
		for _, opt := range m.thinkingOptions {
			if opt == m.thinkingType {
				valid = true
				break
			}
		}
		if !valid {
			m.thinkingType = m.thinkingOptions[0]
		}
	}

	m.reasoningField = 0
	m.state = stateReasoningConfig
	return m, nil
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
			for i := range m.thinkingOptions {
				if m.thinkingOptions[i] == m.thinkingType {
					m.thinkingType = m.thinkingOptions[(i-1+len(m.thinkingOptions))%len(m.thinkingOptions)]
					break
				}
			}
		} else if len(m.effortOptions) > 0 {
			for i := range m.effortOptions {
				if m.effortOptions[i] == m.effortLevel {
					m.effortLevel = m.effortOptions[(i-1+len(m.effortOptions))%len(m.effortOptions)]
					break
				}
			}
		}
	case "right":
		if m.reasoningField == 0 && len(m.thinkingOptions) > 0 {
			for i := range m.thinkingOptions {
				if m.thinkingOptions[i] == m.thinkingType {
					m.thinkingType = m.thinkingOptions[(i+1)%len(m.thinkingOptions)]
					break
				}
			}
		} else if len(m.effortOptions) > 0 {
			for i := range m.effortOptions {
				if m.effortOptions[i] == m.effortLevel {
					m.effortLevel = m.effortOptions[(i+1)%len(m.effortOptions)]
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
		m2, cmd := m.enterChatState()
	return m2, cmd
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
					m.fetchModelsCmd(),
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
					m.fetchModelsCmd(),
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
		m2, cmd := m.enterChatState()
	return m2, cmd
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

	m2, cmd := m.enterChatState()
	return m2, cmd
}

func (m model) updateSessionPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.sessionList, cmd = m.sessionList.Update(msg)

	if msg.String() == "esc" {
		m2, cmd := m.enterChatState()
	return m2, cmd
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	selected, ok := m.sessionList.SelectedItem().(sessionItem)
	if !ok {
		return m, cmd
	}
	if selected.meta.ID == m.sessionID {
		m2, cmd := m.enterChatState()
	return m2, cmd
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
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, saveCmd
	}

	m = m.applySession(loaded)
	var checkCmd tea.Cmd
	m, checkCmd = m.enterChatState()
	title := "gurt"
	if m.sessionName != "" {
		title = "gurt | " + m.sessionName
	}
	cmds := []tea.Cmd{tea.SetWindowTitle(title)}
	if checkCmd != nil {
		cmds = append(cmds, checkCmd)
	}
	if saveCmd != nil {
		cmds = append(cmds, saveCmd)
	}
	return m, tea.Batch(cmds...)
}

func toggleInList(list []string, item string) []string {
	for i, s := range list {
		if s == item {
			return append(list[:i], list[i+1:]...)
		}
	}
	return append(list, item)
}

func (m model) updateAllowManage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Tool check/uncheck mode
	if m.allowManageAdding && m.allowManageAddType == "tool" {
		switch msg.String() {
		case "up":
			if m.allowToolCheckCursor > 0 {
				m.allowToolCheckCursor--
			}
		case "down":
			if m.allowToolCheckCursor < len(m.allowToolCheckItems)-1 {
				m.allowToolCheckCursor++
			}
		case "enter", " ":
			name := m.allowToolCheckItems[m.allowToolCheckCursor]
			m.alwaysAllowTools = toggleInList(m.alwaysAllowTools, name)
		case "esc":
			saveConfig(m)
			m.allowManageAdding = false
			m.allowManageAddType = ""
			m.allowToolCheckItems = nil
			m.allowToolCheckCursor = 0
			m.chatInput.Focus()
			return m, nil
		}
		return m, nil
	}

	// Command prefix add mode (text input)
	if m.allowManageAdding && m.allowManageAddType == "command" {
		var cmd tea.Cmd
		m.allowManageInput, cmd = m.allowManageInput.Update(msg)

		switch msg.String() {
		case "enter":
			val := strings.TrimSpace(m.allowManageInput.Value())
			if val != "" {
				m.alwaysAllowCommandPrefixes = append(m.alwaysAllowCommandPrefixes, val)
				m.allowManageCursor = len(m.alwaysAllowCommandPrefixes) - 1
				saveConfig(m)
			}
			m.allowManageAdding = false
			m.allowManageAddType = ""
			m.allowManageInput.Reset()
			m.chatInput.Focus()
			return m, nil
		case "esc":
			m.allowManageAdding = false
			m.allowManageAddType = ""
			m.allowManageInput.Reset()
			m.chatInput.Focus()
			return m, nil
		}
		return m, cmd
	}

	// Main grid navigation (row-major, fills rows then wraps)
	cmds := m.alwaysAllowCommandPrefixes
	if len(cmds) == 0 {
		if msg.String() == "esc" {
			m2, cmd := m.enterChatState()
	return m2, cmd
		}
		return m, nil
	}

	numRows, numCols, _ := m.cmdGridDimensions()
	if numCols < 1 {
		numCols = 1
	}

	switch msg.String() {
	case "up":
		if m.allowManageCursor >= numCols {
			m.allowManageCursor -= numCols
		}
	case "down":
		if m.allowManageCursor+numCols < len(cmds) {
			m.allowManageCursor += numCols
		}
	case "left":
		if m.allowManageCursor%numCols != 0 {
			m.allowManageCursor--
		}
	case "right":
		if m.allowManageCursor%numCols != numCols-1 && m.allowManageCursor+1 < len(cmds) {
			m.allowManageCursor++
		}
	case "t":
		m.allowManageAdding = true
		m.allowManageAddType = "tool"
		m.allowToolCheckItems = []string{"read_file", "write_file", "edit_file", "delete_file"}
		m.allowToolCheckCursor = 0
		return m, nil
	case "c":
		m.allowManageAdding = true
		m.allowManageAddType = "command"
		m.allowManageInput.Reset()
		m.allowManageInput.Placeholder = "command prefix (e.g. npm, git push)"
		m.allowManageInput.Focus()
		return m, nil
	case "d", "x":
		if m.allowManageCursor >= 0 && m.allowManageCursor < len(cmds) {
			m.alwaysAllowCommandPrefixes = append(cmds[:m.allowManageCursor], cmds[m.allowManageCursor+1:]...)
			if m.allowManageCursor >= len(m.alwaysAllowCommandPrefixes) {
				m.allowManageCursor = len(m.alwaysAllowCommandPrefixes) - 1
			}
			if m.allowManageCursor < 0 {
				m.allowManageCursor = 0
			}
			saveConfig(m)
		}
		return m, nil
	case "esc":
		m2, cmd := m.enterChatState()
	return m2, cmd
	}

	// Adjust scroll to keep cursor row in view
	cursorRow := m.allowManageCursor / numCols
	firstRow := m.allowManageScroll / numCols
	if cursorRow < firstRow {
		m.allowManageScroll = cursorRow * numCols
	}
	if cursorRow >= firstRow+numRows {
		m.allowManageScroll = (cursorRow - numRows + 1) * numCols
	}

	// Clamp scroll
	totalRows := (len(m.alwaysAllowCommandPrefixes) + numCols - 1) / numCols
	maxScrollRow := totalRows - numRows
	if maxScrollRow < 0 {
		maxScrollRow = 0
	}
	maxScroll := maxScrollRow * numCols
	if m.allowManageScroll > maxScroll {
		m.allowManageScroll = maxScroll
	}
	if m.allowManageScroll < 0 {
		m.allowManageScroll = 0
	}

	return m, nil
}

func (m model) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.handleChatMessage(msg)
}

func (m model) handleChatMessage(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.selection.exists || m.selection.active {
		m.selection = textSelection{}
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
	}

	if m.pendingPerm != nil {
		tc := m.pendingPerm.toolCall
		optionCount := len(ui.PermissionOptions(tc.Function.Name, ""))

		switch msg.String() {
		case "up":
			m.permCursor--
			if m.permCursor < 0 {
				m.permCursor = optionCount - 1
			}
			return m, nil
		case "down":
			m.permCursor++
			if m.permCursor >= optionCount {
				m.permCursor = 0
			}
			return m, nil
		case "enter":
			remaining := m.pendingPerm.remaining
			cursor := m.permCursor
			m.pendingPerm = nil
			m.permCursor = 0
			m = m.adjustViewportHeight()

			name := tc.Function.Name
			deny := func() (tea.Model, tea.Cmd) {
				m.messages = append(m.messages, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    "User denied this operation.",
				})
				m.toolCallCycle = 0
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
				m.chatViewport.GotoBottom()
				m.chatInput.Focus()
				return m, m.persistSessionCmd()
			}

			switch cursor {
			case 0: // Yes
				m = m.executeTool(tc)
				return m.processToolCalls(remaining)
			case 1:
				switch name {
				case "run_bash":
					if cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments)); err == nil {
						prefix := tools.BashCommandPrefix(cmd)
						if prefix != "" {
							m.allowedBashPrefixesSession[prefix] = true
						}
					}
					m = m.executeTool(tc)
					return m.processToolCalls(remaining)
				case "edit_file", "write_file":
					m.allowEdits = true
					m = m.executeTool(tc)
					return m.processToolCalls(remaining)
				case "delete_file":
					m.allowDeletions = true
					m = m.executeTool(tc)
					return m.processToolCalls(remaining)
				default:
					m.alwaysAllowPerms = true
					m = m.executeTool(tc)
					return m.processToolCalls(remaining)
				}
			case 2:
				switch name {
				case "run_bash":
					m = m.allowBashPrefix(tc)
					return m.processToolCalls(remaining)
				case "edit_file", "write_file", "delete_file":
					m.alwaysAllowPerms = true
					m = m.executeTool(tc)
					return m.processToolCalls(remaining)
				}
				return deny()
			case 3:
				switch name {
				case "run_bash":
					m.alwaysAllowPerms = true
					m = m.executeTool(tc)
					return m.processToolCalls(remaining)
				case "edit_file", "write_file":
					m.alwaysAllowTools = toggleInList(m.alwaysAllowTools, "edit_file")
					m.alwaysAllowTools = toggleInList(m.alwaysAllowTools, "write_file")
					saveConfig(m)
					m = m.executeTool(tc)
					return m.processToolCalls(remaining)
				}
				return deny()
			case 4: // No
				return deny()
			}
			return m, nil
		}
		return m, nil
	}

	if msg.String() == "esc" && m.isStreaming && m.streamState.cancel != nil {
		m.streamState.cancel()
		m.cancelRequested = true
		return m, nil
	}

	if m.showThemePicker {
		switch msg.String() {
		case "up":
			m.themePickerCursor--
			if m.themePickerCursor < 0 {
				m.themePickerCursor = len(ui.ThemeRegistry) - 1
			}
			entry := ui.ThemeRegistry[m.themePickerCursor]
			m.theme = entry.NewFunc()
			m.themeName = entry.Name
			m.applyThemeToLists()
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			return m, nil
		case "down":
			m.themePickerCursor++
			if m.themePickerCursor >= len(ui.ThemeRegistry) {
				m.themePickerCursor = 0
			}
			entry := ui.ThemeRegistry[m.themePickerCursor]
			m.theme = entry.NewFunc()
			m.themeName = entry.Name
			m.applyThemeToLists()
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			return m, nil
		case "enter":
			entry := ui.ThemeRegistry[m.themePickerCursor]
			m.theme = entry.NewFunc()
			m.themeName = entry.Name
			m.showThemePicker = false
			m.applyThemeToLists()
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			if cfg, _ := config.Load(); cfg != nil {
				cfg.Theme = entry.Name
				_ = config.Save(cfg)
			}
			m.chatInput.Focus()
			m = m.adjustViewportHeight()
			return m, nil
		case "esc":
			m.theme = m.themePickerOrigTheme
			m.themeName = m.themePickerOrigName
			m.applyThemeToLists()
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			m.showThemePicker = false
			m.chatInput.Focus()
			m = m.adjustViewportHeight()
			return m, nil
		}
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
			return m.adjustViewportHeight(), nil
		case "esc":
			m.suggestions = suggestionState{}
			return m.adjustViewportHeight(), nil
		}
	}

	if msg.String() == "ctrl+y" {
		m.yolo = !m.yolo
		m.toastSeq++
		if m.yolo {
			m.toast = &toastMsg{text: "YOLO mode", id: m.toastSeq}
		} else {
			m.toast = nil
		}
		return m, nil
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
		m.workingMsg = workingMessages[rand.Intn(len(workingMessages))]
		m.workingSpinnerIdx = 0
		m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()

		cmds := []tea.Cmd{m.persistSessionCmd(), startChatStreamCmd(m), workingTickCmd()}
		if m.needsTitle {
			m.needsTitle = false
			cmds = append(cmds, generateTitleCmd(m))
		}
		return m, tea.Batch(cmds...)
	}

	// Filter partial SGR mouse events that the input reader
	// couldn't decode — they arrive as Alt+[ or <digits>;… runes.
	if msg.Alt && len(msg.Runes) == 1 && msg.Runes[0] == '[' {
		return m, nil
	}
	if msg.Type == tea.KeyRunes && partialMouseEventRe.MatchString(string(msg.Runes)) {
		return m, nil
	}

	var cmd tea.Cmd
	switch msg.String() {
	case "up", "down", "pgup", "pgdown", "home", "end":
		m.chatViewport, _ = m.chatViewport.Update(msg)
	}
	m.chatInput, cmd = m.chatInput.Update(msg)
	m = m.updateSuggestions()
	m = m.adjustViewportHeight()
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

// allowBashPrefix adds the command prefix from a run_bash tool call to the
// always-allowed list, persists it to config, and executes the tool.
func (m model) allowBashPrefix(tc llm.ToolCall) model {
	cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments))
	if err == nil {
		prefix := tools.BashCommandPrefix(cmd)
		if prefix != "" {
			m.allowedBashPrefixes[prefix] = true
			// Persist to config
			if cfg, err := config.Load(); err == nil && cfg != nil {
				already := false
				for _, p := range cfg.AllowedBashPrefixes {
					if p == prefix {
						already = true
						break
					}
				}
				if !already {
					cfg.AllowedBashPrefixes = append(cfg.AllowedBashPrefixes, prefix)
					config.Save(cfg)
				}
			} else if err == nil {
				cfg = &config.Config{}
				cfg.AllowedBashPrefixes = append(cfg.AllowedBashPrefixes, prefix)
				config.Save(cfg)
			}
		}
	}
	return m.executeTool(tc)
}

func (m model) processToolCalls(tcs []llm.ToolCall) (tea.Model, tea.Cmd) {
	for i, tc := range tcs {
		// Check if tool name matches the always-allow tools list (exact match).
		// Tools in this list are auto-allowed unconditionally.
		if len(m.alwaysAllowTools) > 0 {
			matched := false
			for _, name := range m.alwaysAllowTools {
				if tc.Function.Name == name {
					matched = true
					break
				}
			}
			if matched {
				m = m.executeTool(tc)
				continue
			}
		}

		// Check if bash command matches session-allowed or config-allowed prefixes.
		if !m.yolo && !m.alwaysAllowPerms && tc.Function.Name == "run_bash" {
			cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments))
			if err == nil {
				matched := false
				// Check session-level prefixes first
				for prefix := range m.allowedBashPrefixesSession {
					if strings.HasPrefix(cmd, prefix) {
						matched = true
						break
					}
				}
				// Then check config-level prefixes
				if !matched {
					for _, prefix := range m.alwaysAllowCommandPrefixes {
						if strings.HasPrefix(cmd, prefix) {
							matched = true
							break
						}
					}
				}
				if matched {
					m = m.executeTool(tc)
					continue
				}
			}
		}

		// Check session-level edit/write allow
		if m.allowEdits && (tc.Function.Name == "edit_file" || tc.Function.Name == "write_file") {
			m = m.executeTool(tc)
			continue
		}

		// Check session-level delete allow
		if m.allowDeletions && tc.Function.Name == "delete_file" {
			m = m.executeTool(tc)
			continue
		}

		if tools.IsDestructive(tc.Function.Name) && !m.yolo && !m.alwaysAllowPerms {
			m.pendingPerm = &pendingPerm{
				toolCall:  tc,
				remaining: tcs[i+1:],
			}
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			m.chatViewport.GotoBottom()
			m.chatInput.Blur()
			m = m.adjustViewportHeight()
			return m, m.persistSessionCmd()
		}
		m = m.executeTool(tc)
	}
	m.isStreaming = true
	m.workingMsg = workingMessages[rand.Intn(len(workingMessages))]
	m.workingSpinnerIdx = 0
	return m, tea.Batch(m.persistSessionCmd(), startChatStreamCmd(m), workingTickCmd())
}

func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// 1. Scroll wheel → viewport
	if msg.Action == tea.MouseActionPress && (msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
		if m.selection.exists {
			m.selection = textSelection{}
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
		}

		// Don't forward wheel events past boundaries to avoid the
		// bubbletea input reader splitting SGR mouse events across
		// a read boundary, which leaks raw escape bytes as keypresses.
		if (msg.Button == tea.MouseButtonWheelUp && m.chatViewport.AtTop()) ||
			(msg.Button == tea.MouseButtonWheelDown && m.chatViewport.AtBottom()) {
			return m, nil
		}

		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(msg)
		return m, cmd
	}

	// 2. Motion during drag → update selection focus
	if msg.Action == tea.MouseActionMotion && m.selection.active {
		line, col, ok := computeContentPosition(m, msg)
		if ok {
			m.selection.focusY = line
			m.selection.focusX = col
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
		}
		return m, nil
	}

	// 3. Button press
	if msg.Action == tea.MouseActionPress {
		// Non-left click: clear selection
		if msg.Button != tea.MouseButtonLeft {
			if m.selection.exists {
				m.selection = textSelection{}
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
			}
			return m, nil
		}

		// Left click: check reasoning markers first, then start selection
		line, col, ok := computeContentPosition(m, msg)
		if !ok {
			return m, nil
		}
		if line >= 0 {
			content := buildChatContent(m)
			lines := strings.Split(content, "\n")
			if line < len(lines) {
				const hitboxRadius = 2

				// ▸ — collapsed live reasoning
				if findMarker(lines, line, hitboxRadius, "▸") >= 0 {
					m.reasoning.visible = !m.reasoning.visible
					m.selection = textSelection{}
					m.chatViewport.SetContent(buildChatContentHighlighted(m))
					return m, nil
				}

				// ◌ — active live reasoning
				if findMarker(lines, line, hitboxRadius, "◌") >= 0 {
					m.reasoning.visible = !m.reasoning.visible
					m.selection = textSelection{}
					m.chatViewport.SetContent(buildChatContentHighlighted(m))
					return m, nil
				}

				// ◷ or ▾ — stored reasoning
				if idx := findMarker(lines, line, hitboxRadius, "◷", "▾"); idx >= 0 {
					count := 0
					for i := 0; i <= idx; i++ {
						if strings.Contains(lines[i], "◷") {
							count++
						}
					}
					msgIdx := 0
					found := 0
					for i := range m.messages {
						if m.messages[i].Reasoning != "" {
							found++
							if found == count {
								msgIdx = i
								break
							}
						}
					}
					if found == count {
						m.messages[msgIdx].ReasoningVisible = !m.messages[msgIdx].ReasoningVisible
						yOff := m.chatViewport.YOffset
						m.selection = textSelection{}
						m.chatViewport.SetContent(buildChatContentHighlighted(m))
						if yOff > m.chatViewport.YOffset {
							m.chatViewport.GotoBottom()
						} else {
							m.chatViewport.YOffset = yOff
						}
						return m, m.persistSessionCmd()
					}

					// No matching stored message — toggle live reasoning.
					m.reasoning.visible = !m.reasoning.visible
					m.selection = textSelection{}
					m.chatViewport.SetContent(buildChatContentHighlighted(m))
					return m, nil
				}
			}
		}

		// Start new text selection
		m.selection = textSelection{
			anchorY: line, anchorX: col,
			focusY: line, focusX: col,
			active: true,
		}
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		return m, nil
	}

	// 4. Button release → auto-copy and clear
	if msg.Action == tea.MouseActionRelease {
		if m.selection.active {
			m.selection.active = false
			m.selection.exists = true
			text := extractSelectedText(buildChatContent(m), m.selection)
			copyToClipboard(text)
			m.selection = textSelection{}
			m.chatViewport.SetContent(buildChatContent(m))
			if text != "" {
				m.toastSeq++
				m.toast = &toastMsg{text: "Copied to clipboard", id: m.toastSeq}
				return m, toastTimeoutCmd(m.toastSeq)
			}
		}
		return m, nil
	}

	return m, nil
}

// findMarker scans lines in [contentLine-radius, contentLine+radius] for any
// of the given markers, returning the first matching line index, or -1.
func findMarker(lines []string, contentLine, radius int, markers ...string) int {
	start := contentLine - radius
	if start < 0 {
		start = 0
	}
	end := contentLine + radius
	if end >= len(lines) {
		end = len(lines) - 1
	}
	for i := start; i <= end; i++ {
		for _, m := range markers {
			if strings.Contains(lines[i], m) {
				return i
			}
		}
	}
	return -1
}

func (m model) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	m.suggestions = suggestionState{}
	m = m.adjustViewportHeight()
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
			m.fetchModelsCmd(),
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

	case "show-reasoning":
		oldVisible := m.reasoning.visible
		newVisible := !oldVisible
		if len(parts) > 1 {
			switch strings.ToLower(parts[1]) {
			case "true", "yes":
				newVisible = true
			case "false", "no":
				newVisible = false
			}
		}
		m.reasoning.visible = newVisible
		m.reasoning.defaultVisible = newVisible
		saveConfig(m)
		for i := range m.messages {
			if m.messages[i].Reasoning != "" {
				m.messages[i].ReasoningVisible = newVisible
			}
		}
		m.messages = append(m.messages, llm.Message{
			Role:     "assistant",
			Internal: true,
			Content: fmt.Sprintf("Reasoning changed to %s (was %s)",
				map[bool]string{true: "visible", false: "hidden"}[newVisible],
				map[bool]string{true: "visible", false: "hidden"}[oldVisible]),
		})
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case "reasoning":
		if m.provider == llm.ProviderCustom {
			model := m.currentModelInfo()
			if !model.HasEffort() && !model.HasThinkingSupport() {
				m.messages = append(m.messages, llm.Message{
					Role:     "assistant",
					Internal: true,
					Content:  "Thinking is not supported for this custom API provider",
				})
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
				m.chatViewport.GotoBottom()
				return m, nil
			}
		}

		model := m.currentModelInfo()

		if m.provider == llm.ProviderAnthropic {
			opts := model.ThinkingTypeOptions()
			partsStr := strings.Join(opts, ", ")
			if len(parts) < 2 {
				m.messages = append(m.messages, llm.Message{
					Role:     "assistant",
					Internal: true,
					Content:  fmt.Sprintf("Current thinking type: %s\nUsage: /reasoning <type>  (%s)", m.thinkingType, partsStr),
				})
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
				m.chatViewport.GotoBottom()
				return m, nil
			}
			newType := strings.ToLower(parts[1])
			valid := false
			for _, opt := range opts {
				if newType == opt {
					valid = true
					break
				}
			}
			if valid {
				oldType := m.thinkingType
				m.thinkingType = newType
				saveConfig(m)
				m.messages = append(m.messages, llm.Message{
					Role:     "assistant",
					Internal: true,
					Content:  fmt.Sprintf("Reasoning set to %s (was %s)", newType, oldType),
				})
			} else {
				m.messages = append(m.messages, llm.Message{
					Role:     "assistant",
					Internal: true,
					Content:  fmt.Sprintf("Unknown thinking type: %s. Available: %s", newType, partsStr),
				})
			}
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			m.chatViewport.GotoBottom()
			return m, nil
		}

		// Custom provider with thinking types
		if m.provider == llm.ProviderCustom && (model.Capabilities.Thinking.Types.Enabled.Supported || model.Capabilities.Thinking.Types.Adaptive.Supported) {
			opts := model.ThinkingTypeOptions()
			partsStr := strings.Join(opts, ", ")
			if len(parts) < 2 {
				m.messages = append(m.messages, llm.Message{
					Role:     "assistant",
					Internal: true,
					Content:  fmt.Sprintf("Current thinking type: %s\nUsage: /reasoning <type>  (%s)", m.thinkingType, partsStr),
				})
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
				m.chatViewport.GotoBottom()
				return m, nil
			}
			newType := strings.ToLower(parts[1])
			valid := false
			for _, opt := range opts {
				if newType == opt {
					valid = true
					break
				}
			}
			if valid {
				oldType := m.thinkingType
				m.thinkingType = newType
				saveConfig(m)
				m.messages = append(m.messages, llm.Message{
					Role:     "assistant",
					Internal: true,
					Content:  fmt.Sprintf("Reasoning set to %s (was %s)", newType, oldType),
				})
			} else {
				m.messages = append(m.messages, llm.Message{
					Role:     "assistant",
					Internal: true,
					Content:  fmt.Sprintf("Unknown thinking type: %s. Available: %s", newType, partsStr),
				})
			}
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			m.chatViewport.GotoBottom()
			return m, nil
		}

		// OpenAI/Gemini: /thinking sets the reasoning effort level
		opts := model.ReasoningLevelOptions()
		if len(opts) == 0 {
			opts = m.effortOptions
		}
		if len(opts) == 0 {
			opts = []string{"none", "low", "medium", "high", "xhigh"}
		}
		partsStr := strings.Join(opts, ", ")
		if len(parts) < 2 {
			m.messages = append(m.messages, llm.Message{
				Role:     "assistant",
				Internal: true,
				Content:  fmt.Sprintf("Current reasoning level: %s\nUsage: /reasoning <level>  (%s)", m.effortLevel, partsStr),
			})
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			m.chatViewport.GotoBottom()
			return m, nil
		}
		newLevel := strings.ToLower(parts[1])
		valid := false
		for _, opt := range opts {
			if newLevel == opt {
				valid = true
				break
			}
		}
		if valid {
			oldLevel := m.effortLevel
			m.effortLevel = newLevel
			saveConfig(m)
			m.messages = append(m.messages, llm.Message{
				Role:     "assistant",
				Internal: true,
				Content:  fmt.Sprintf("Reasoning set to %s (was %s)", newLevel, oldLevel),
			})
		} else {
			m.messages = append(m.messages, llm.Message{
				Role:     "assistant",
				Internal: true,
				Content:  fmt.Sprintf("Unknown reasoning level: %s. Available: %s", newLevel, partsStr),
			})
		}
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case "effort":
		model := m.currentModelInfo()

		var opts []string
		if m.provider == llm.ProviderAnthropic {
			opts = model.Capabilities.Effort.EffortLevels()
		} else {
			opts = model.ReasoningLevelOptions()
		}
		if len(opts) == 0 {
			opts = m.effortOptions
		}
		if len(opts) == 0 {
			if m.provider == llm.ProviderAnthropic {
				opts = []string{"low", "medium", "high"}
			} else {
				opts = []string{"none", "low", "medium", "high", "xhigh"}
			}
		}
		partsStr := strings.Join(opts, ", ")
		if len(parts) < 2 {
			m.messages = append(m.messages, llm.Message{
				Role:     "assistant",
				Internal: true,
				Content:  fmt.Sprintf("Current effort level: %s\nUsage: /effort <level>  (%s)", m.effortLevel, partsStr),
			})
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			m.chatViewport.GotoBottom()
			return m, nil
		}
		newEffort := strings.ToLower(parts[1])
		valid := false
		for _, opt := range opts {
			if newEffort == opt {
				valid = true
				break
			}
		}
		if valid {
			oldEffort := m.effortLevel
			m.effortLevel = newEffort
			saveConfig(m)
			m.messages = append(m.messages, llm.Message{
				Role:     "assistant",
				Internal: true,
				Content:  fmt.Sprintf("Effort changed to %s (was %s)", newEffort, oldEffort),
			})
		} else {
			m.messages = append(m.messages, llm.Message{
				Role:     "assistant",
				Internal: true,
				Content:  fmt.Sprintf("Unknown effort level: %s. Available: %s", newEffort, partsStr),
			})
		}
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case "help":
		var b strings.Builder
		b.WriteString("Available commands:\n")
		for _, sc := range slashCommands {
			fmt.Fprintf(&b, "  /%s - %s\n", sc.name, sc.description)
		}
		m.messages = append(m.messages, llm.Message{
			Role:     "assistant",
			Internal: true,
			Content:  b.String(),
		})
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, nil

	case "theme":
		m.showThemePicker = true
		m.themePickerOrigTheme = m.theme
		m.themePickerOrigName = m.themeName
		m.themePickerCursor = 0
		for i, entry := range ui.ThemeRegistry {
			if entry.Name == m.themeName {
				m.themePickerCursor = i
				break
			}
		}
		m.chatInput.Blur()
		m = m.adjustViewportHeight()
		return m, nil

	case "telemetry":
		oldVal := m.telemetryEnabled
		m.telemetryEnabled = !oldVal
		saveConfig(m)
		status := "enabled"
		if !m.telemetryEnabled {
			status = "disabled"
		}
		m.messages = append(m.messages, llm.Message{
			Role:     "assistant",
			Internal: true,
			Content:  fmt.Sprintf("Telemetry %s (was %s). No personal data is collected. See README for details.", status, map[bool]string{true: "enabled", false: "disabled"}[oldVal]),
		})
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, sendTelemetryCmd("telemetry_toggle")

	case "session":
		if m.isStreaming {
			return m, nil
		}
		metas, err := sessions.List(m.workspaceRoot)
		if err != nil {
			m.messages = append(m.messages, llm.Message{
				Role:     "assistant",
				Internal: true,
				Content:  fmt.Sprintf("_Failed to list sessions: %v_", err),
			})
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			m.chatViewport.GotoBottom()
			return m, nil
		}
		items := make([]list.Item, len(metas))
		for i, meta := range metas {
			items[i] = sessionItem{meta: meta}
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
		var checkCmd tea.Cmd
		m, checkCmd = m.enterChatState()
		cmds := []tea.Cmd{tea.SetWindowTitle("gurt")}
		if checkCmd != nil {
			cmds = append(cmds, checkCmd)
		}
		if saveCmd != nil {
			cmds = append(cmds, saveCmd)
		}
		return m, tea.Batch(cmds...)

	case "allow":
		if m.isStreaming {
			return m, nil
		}
		m.state = stateAllowManage
		m.allowManageCursor = 0
		m.allowManageScroll = 0
		m.allowManageAdding = false
		m.allowManageInput.Reset()
		return m, nil

	case "update":
		if m.isStreaming {
			return m, nil
		}
		if m.latestVersion == "" {
			return m, checkAndUpdateCmd()
		}
		return m, performUpdateCmd(m.latestVersion)

	case "version":
		return m, checkVersionCmd()

	default:
		m.messages = append(m.messages, llm.Message{
			Role:     "assistant",
			Internal: true,
			Content:  fmt.Sprintf("_Unknown command: /%s. Type /help for available commands._", cmd),
		})
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
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

func (m model) adjustViewportHeight() model {
	fixed := 0
	fixed++ // title
	if m.updateAvailable {
		fixed++ // update banner
	}
	fixed++ // top divider
	// spacer line — 0 when idle, 1 when streaming or showing context bar
	if (m.isStreaming && m.workingMsg != "") || m.maxInputTokens > 0 || m.contextInputTokens > 0 || m.inputTokens > 0 {
		fixed++
	}
	fixed++ // toast line (always 1 — blank line when no toast)
	fixed++ // bottom divider
	// bottom section
	switch {
	case m.pendingPerm != nil:
		fixed += m.permOverlayHeight()
	case m.showThemePicker:
		fixed += m.themePickerOverlayHeight()
	default:
		fixed += 2 // input line + help line
		if m.suggestions.active && len(m.suggestions.items) > 0 {
			fixed += len(m.suggestions.items)
		}
	}
	m.chatViewport.Height = m.height - fixed
	if m.chatViewport.Height < 1 {
		m.chatViewport.Height = 1
	}
	return m
}

func (m model) permOverlayHeight() int {
	tc := m.pendingPerm.toolCall
	bashPrefix := ""
	if tc.Function.Name == "run_bash" {
		if cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments)); err == nil {
			bashPrefix = tools.BashCommandPrefix(cmd)
		}
	}
	content := ui.RenderPermissionPrompt(m.theme, tc, m.width, m.permCursor, bashPrefix) + "\n" +
		m.theme.Dim.Render("  ↑/↓ navigate • enter select")
	boxW := m.width - 2
	if boxW < 30 {
		boxW = 30
	}
	box := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Base)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.Mauve)).
		Width(boxW).
		Padding(1, 1)
	return strings.Count(box.Render(content), "\n") + 1
}

func (m model) themePickerOverlayHeight() int {
	boxW := m.width - 4
	if boxW < 30 {
		boxW = 30
	}
	if boxW > 50 {
		boxW = 50
	}
	var pc strings.Builder
	pc.WriteString(m.theme.Header.Render("Select Theme"))
	pc.WriteString("\n\n")
	for _, entry := range ui.ThemeRegistry {
		style := m.theme.Dim
		pc.WriteString(style.Render("  " + entry.Name))
		pc.WriteString("\n")
	}
	pc.WriteString("\n")
	pc.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter select • esc dismiss"))
	popup := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Base)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.Mauve)).
		Width(boxW).
		Padding(1, 2)
	return strings.Count(popup.Render(pc.String()), "\n") + 1
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
		if m.provider == llm.ProviderCustom {
			if m.thinkingType == "disabled" {
				// Thinking disabled, don't send reasoning_effort
			} else if m.thinkingType == "enabled" && m.effortLevel != "" && m.effortLevel != "none" {
				reasoningEffort = m.effortLevel
			} else if m.thinkingType == "enabled" {
				reasoningEffort = "high"
			} else if m.effortLevel != "" && m.effortLevel != "none" {
				reasoningEffort = m.effortLevel
			}
		} else if (m.provider == llm.ProviderOpenAI || m.provider == llm.ProviderGemini) && m.effortLevel != "" && m.effortLevel != "none" {
			reasoningEffort = m.effortLevel
		}

		if reasoningEffort == "enabled" {
			reasoningEffort = "high"
		}

		maxTokens := 128000
		if info := m.currentModelInfo(); info.MaxTokens > 0 {
			maxTokens = info.MaxTokens
		}

		req := llm.ChatRequest{
			Model:           m.modelName,
			Messages:        filterInternal(m.messages),
			SystemPrompt:    systemPrompt,
			Tools:           tools.Definitions(),
			Thinking:        thinkingCfg,
			MaxTokens:       maxTokens,
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
			var debugEvents []debug.StreamEvent
			for event := range events {
				if m.debug {
					e := debug.StreamEvent{Type: eventTypeName(event.Type)}
					switch event.Type {
					case llm.StreamDelta:
						e.Content = event.Content
					case llm.StreamReasoning:
						e.Content = event.Content
					case llm.StreamUsage:
						e.InputTokens = event.InputTokens
						e.OutputTokens = event.OutputTokens
					case llm.StreamToolCalls:
						e.ToolCalls = len(event.ToolCalls)
					case llm.StreamError:
						e.Error = event.Err.Error()
					}
					debugEvents = append(debugEvents, e)
				}
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
					if m.debug {
						debug.SaveRecord(m.sessionID, m.modelName, req, debugEvents)
					}
					return
				case llm.StreamError:
					globalProgram.Send(chatStreamError{err: event.Err})
					if m.debug {
						debug.SaveRecord(m.sessionID, m.modelName, req, debugEvents)
					}
					return
				}
			}
			if !doneSent {
				globalProgram.Send(chatStreamDone{})
				if m.debug {
					debug.SaveRecord(m.sessionID, m.modelName, req, debugEvents)
				}
			}
		}()

		return nil
	}
}

func (m model) replayQueuedMessage() (tea.Model, tea.Cmd) {
	qmsg := m.queuedMessage
	m.queuedMessage = ""
	m.messages = append(m.messages, llm.Message{Role: "user", Content: qmsg})
	m.isStreaming = true
	m.workingMsg = workingMessages[rand.Intn(len(workingMessages))]
	m.workingSpinnerIdx = 0
	m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
	m.chatViewport.SetContent(buildChatContentHighlighted(m))
	m.chatViewport.GotoBottom()
	return m, tea.Batch(m.persistSessionCmd(), startChatStreamCmd(m), workingTickCmd())
}

func resourceMonitorTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return resourceMonitorCmd()
	})
}

func workingTickCmd() tea.Cmd {
	delay := time.Duration(150+rand.Intn(150)) * time.Millisecond
	return tea.Tick(delay, func(t time.Time) tea.Msg {
		return workingTickMsg{}
	})
}

func generateTitleCmd(m model) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		titleModel := m.smallModelForProvider()
		baseURL := m.customURL

		titleMsg := ""
		for _, msg := range m.messages {
			if msg.Role == "user" && msg.Content != "" {
				titleMsg = msg.Content
				break
			}
		}
		if titleMsg == "" {
			return nil
		}

		req := llm.ChatRequest{
			Model:        titleModel,
			Messages:     []llm.Message{{Role: "user", Content: titleMsg}},
			SystemPrompt: sessionTitlePrompt,
		}

		title, err := llm.SimpleChatCompletion(ctx, m.provider, m.apiKey, baseURL, req)
		if err != nil || title == "" {
			return nil
		}

		return sessionTitleGeneratedMsg{title: title}
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
		"Date":      time.Now().Format("January 2, 2006"),
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
		b.WriteString(m.theme.EmptyState.Render("No messages yet. Send a message to start."))
		b.WriteString("\n")
		return b.String()
	}

	toolNames := buildToolNameLookup(m.messages)

	lastIsCurrent := false
	if len(m.messages) > 0 {
		last := m.messages[len(m.messages)-1]
		lastIsCurrent = last.Role == "assistant" && (m.reasoning.active || streamingLen > 0)
	}

	skipLast := lastIsCurrent
	for i, msg := range m.messages {
		isLast := i == len(m.messages)-1
		if isLast && skipLast {
			continue
		}
		switch msg.Role {
		case "user":
			b.WriteString(ui.RenderUserMessage(m.theme, msg.Content))
			b.WriteString("\n\n")
		case "assistant":
			if msg.Internal {
				// no label for slash command output
			} else {
				b.WriteString(ui.RenderAssistantLabel(m.theme, m.displayNameForModel(msg.Model)))
				b.WriteString("\n")
			}
			if msg.Reasoning != "" {
				if msg.ReasoningVisible {
					b.WriteString(ui.RenderReasoning(m.theme, false, true, msg.ReasoningDuration, msg.Reasoning, m.chatViewport.Width))
					b.WriteString("\n")
				} else {
					b.WriteString(ui.RenderReasoningStored(m.theme, msg.ReasoningDuration))
					b.WriteString("\n")
				}
			}
			if msg.Content != "" {
				b.WriteString(ui.RenderAssistantContent(m.theme, msg.Content, m.chatViewport.Width))
				b.WriteString("\n")
			}
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					b.WriteString(ui.RenderToolCall(m.theme, tc, m.chatViewport.Width))
					b.WriteString("\n")
				}
			}
			b.WriteString("\n")
		case "tool":
			toolName := toolNames[msg.ToolCallID]
			b.WriteString(ui.RenderToolResult(m.theme, toolName, msg.Content, m.chatViewport.Width))
			b.WriteString("\n\n")
		}
	}

	if lastIsCurrent || m.reasoning.active || streamingLen > 0 {
		b.WriteString(ui.RenderAssistantLabel(m.theme, m.modelDisplayName()))
		b.WriteString("\n")

		if reasoningLen > 0 {
			elapsed := m.reasoning.duration
			if m.reasoning.active {
				elapsed = time.Since(m.reasoning.startTime).Round(100 * time.Millisecond)
			}
			content := ""
			if m.reasoning.content != nil {
				content = m.reasoning.content.String()
			}
			b.WriteString(ui.RenderReasoning(m.theme, m.reasoning.active, m.reasoning.visible, elapsed, content, m.chatViewport.Width))
			b.WriteString("\n")
		}

		if lastIsCurrent {
			content := m.messages[len(m.messages)-1].Content
			if content != "" {
				b.WriteString(ui.RenderAssistantContent(m.theme, content, m.chatViewport.Width))
				b.WriteString("\n")
			}
		} else if streamingLen > 0 && m.streamingContent != nil {
			content := m.streamingContent.String()
			if content != "" {
				b.WriteString(ui.RenderAssistantContent(m.theme, content, m.chatViewport.Width))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

func buildToolNameLookup(messages []llm.Message) map[string]string {
	names := make(map[string]string)
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		for _, tc := range msg.ToolCalls {
			if tc.ID != "" {
				names[tc.ID] = tc.Function.Name
			}
		}
	}
	return names
}

func filterInternal(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, 0, len(msgs))
	for _, m := range msgs {
		if !m.Internal {
			out = append(out, m)
		}
	}
	return out
}

func eventTypeName(t llm.StreamEventType) string {
	switch t {
	case llm.StreamDelta:
		return "delta"
	case llm.StreamReasoning:
		return "reasoning"
	case llm.StreamToolCalls:
		return "tool_calls"
	case llm.StreamDone:
		return "done"
	case llm.StreamError:
		return "error"
	case llm.StreamUsage:
		return "usage"
	default:
		return "unknown"
	}
}

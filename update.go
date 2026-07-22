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
	"sync"
	"text/template"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/sillygru/gurtcli/config"
	"github.com/sillygru/gurtcli/debug"
	"github.com/sillygru/gurtcli/files"
	"github.com/sillygru/gurtcli/history"
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

type toolResultMsg struct {
	toolCallID string
	content    string
	isError    bool
}

var dateSuffixRegex = regexp.MustCompile(`-\d{8}$|-\d{4}-\d{2}-\d{2}$`)

var builderPool = sync.Pool{New: func() any { return new(strings.Builder) }}

// partialMouseEventRe matches the tail of a split SGR mouse sequence
// (e.g. "<64;117;26M") that the input reader parsed as key runes.
var partialMouseEventRe = regexp.MustCompile(`^<\d+;\d+;\d+[Mm]?$`)
var atFileRefRe = regexp.MustCompile(`@(\S+)`)

func hasDateSuffix(name string) bool {
	return dateSuffixRegex.MatchString(name)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		layout := ui.NewLayout(msg.Width, msg.Height)
		contentWidth := layout.ContentWidth()
		h := layout.ListHeight()
		m.providerList.SetSize(contentWidth, h)
		m.modelList.SetSize(contentWidth, h)
		m.sessionList.SetSize(contentWidth, h)

		chatViewHeight := msg.Height - 6
		if chatViewHeight < 1 {
			chatViewHeight = 1
		}
		m.chatViewport.SetWidth(contentWidth)
		m.chatViewport.SetHeight(chatViewHeight)
		m = m.resizeInputs(layout)
		// A resize re-wraps the transcript, so the cells a selection points at
		// are no longer the ones the user picked.
		m.selection = textSelection{}
		m.lastClick = clickTracker{}
		m = m.adjustViewportHeight()
		if m.state == stateChat {
			m.stableContent = buildChatContent(m)
			m.stableMsgCount = len(m.messages)
			m = m.extendTranscriptCache()
			m.chatViewport.SetContent(m.stableContent)
		}
		return m, nil

	case tea.PasteMsg:
		if m.state == stateChat {
			if m.pendingPerm != nil {
				if m.pendingPerm.confirmSudo {
					var cmd tea.Cmd
					m.sudoPasswordInput, cmd = m.sudoPasswordInput.Update(msg)
					return m, cmd
				}
				return m, nil
			}
			if m.showThemePicker {
				return m, nil
			}
			var cmd tea.Cmd
			m.chatInput, cmd = m.chatInput.Update(msg)
			m = m.updateSuggestions()
			m = m.adjustViewportHeight()
			return m, cmd
		}
		switch m.state {
		case stateCustomURL:
			m.urlInput, _ = m.urlInput.Update(msg)
		case stateAPIKeyInput:
			m.keyInput, _ = m.keyInput.Update(msg)
		case stateCustomName:
			m.nameInput, _ = m.nameInput.Update(msg)
		case stateManualModel:
			m.manualInput, _ = m.manualInput.Update(msg)
		case stateDotenvKeyName:
			m.dotenvInput, _ = m.dotenvInput.Update(msg)
		case stateAllowManage:
			if m.allowManageAdding && m.allowManageAddType == "command" {
				m.allowManageInput, _ = m.allowManageInput.Update(msg)
			}
		}
		return m, nil

	case tea.KeyPressMsg:
		// Cheap and idempotent: guarantees the state's input is live no matter
		// which path got us here, so no transition can strand the user.
		m = m.focusForState()
		if msg.String() == "ctrl+c" {
			if m.state == stateChat && (m.isStreaming || (m.toolExec != nil && m.toolExec.active)) {
				m.cancelRequested = true
				if m.streamState.cancel != nil {
					m.streamState.cancel()
				}
				if m.toolExec != nil && m.toolExec.cancel != nil {
					m.toolExec.cancel()
				}
				m = m.resetStreamingState()
				m.messages = append(m.messages, llm.Message{
					Role:    "assistant",
					Content: "_Interrupted_",
				})
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
				if m.stickToBottom {
					m.chatViewport.GotoBottom()
				}
				return m, m.persistSessionCmd()
			}
			if m.pendingPerm != nil {
				m.pendingPerm = nil
				m.permCursor = 0
				m.chatInput.Focus()
				m = m.adjustViewportHeight()
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

	case tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg, tea.MouseMotionMsg:
		if m.state != stateChat {
			return m, nil
		}
		return m.updateMouse(msg)

	case chatStreamChunk:
		if m.streamingContent == nil {
			m.streamingContent = new(strings.Builder)
		}
		m.streamingContent.WriteString(msg.content)
		if time.Since(m.lastStreamRender) > 50*time.Millisecond {
			m.lastStreamRender = time.Now()
			if len(m.messages) != m.stableMsgCount || m.stableContent == "" {
				m.stableContent = buildChatContent(m)
				m.stableMsgCount = len(m.messages)
			}
			content := m.stableContent + renderStreamingPart(m)
			if m.selection.active || m.selection.exists {
				content = applySelectionHighlight(content, m.selection)
			}
			m.chatViewport.SetContent(content)
			if m.stickToBottom {
				m.chatViewport.GotoBottom()
			}
		}
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
		if time.Since(m.lastStreamRender) > 50*time.Millisecond {
			m.lastStreamRender = time.Now()
			if len(m.messages) != m.stableMsgCount || m.stableContent == "" {
				m.stableContent = buildChatContent(m)
				m.stableMsgCount = len(m.messages)
			}
			content := m.stableContent + renderStreamingPart(m)
			if m.selection.active || m.selection.exists {
				content = applySelectionHighlight(content, m.selection)
			}
			m.chatViewport.SetContent(content)
			if m.stickToBottom {
				m.chatViewport.GotoBottom()
			}
		}
		return m, nil

	case chatStreamDone:
		if m.cancelRequested {
			m.cancelRequested = false
			m.retry = retryState{token: m.retry.token + 1}
			m.chatInput.Focus()
			return m, nil
		}
		// A stream that ran to completion clears the ladder, so the next
		// unrelated failure starts over at the base delay. Deliberately not
		// done on the first chunk: an endpoint that always dies mid-stream
		// would then retry forever.
		m.retry = retryState{token: m.retry.token + 1}

		contentStr := ""
		if m.streamingContent != nil {
			contentStr = strings.TrimSpace(m.streamingContent.String())
		}
		reasoningStr := ""
		if m.reasoning.content != nil {
			reasoningStr = strings.TrimSpace(m.reasoning.content.String())
		}
		reasoningDuration := m.reasoning.duration
		if m.reasoning.active {
			reasoningDuration = time.Since(m.reasoning.startTime).Round(100 * time.Millisecond)
		}
		m = m.resetStreamingState()

		if len(msg.toolCalls) > 0 {
			asm := llm.Message{Role: "assistant", Content: contentStr, Model: m.modelName}
			if reasoningStr != "" {
				asm.Reasoning = reasoningStr
				asm.ReasoningDuration = reasoningDuration
			}
			asm.ToolCalls = msg.toolCalls
			m.messages = append(m.messages, asm)
			m = m.extendTranscriptCache().trimMessages()
			m.stableContent = buildChatContent(m)
			m.stableMsgCount = len(m.messages)
			m.chatViewport.SetContent(m.stableContent)
			if m.stickToBottom {
				m.chatViewport.GotoBottom()
			}
			m.toolCallCycle++
			if m.toolCallCycle > maxToolCallCycles {
				m.messages = append(m.messages, llm.Message{
					Role:    "assistant",
					Content: "_Interrupted_",
					Model:   m.modelName,
				})
				m = m.extendTranscriptCache().trimMessages()
				m.toolCallCycle = 0
				m.stableContent = buildChatContent(m)
				m.stableMsgCount = len(m.messages)
				m.chatViewport.SetContent(m.stableContent)
				if m.stickToBottom {
					m.chatViewport.GotoBottom()
				}
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
				msg.ReasoningDuration = reasoningDuration
			}
			m.messages = append(m.messages, msg)
			m = m.extendTranscriptCache().trimMessages()
		}
		m.stableContent = buildChatContent(m)
		m.stableMsgCount = len(m.messages)
		m.chatViewport.SetContent(m.stableContent)
		if m.stickToBottom {
			m.chatViewport.GotoBottom()
		}
		m.chatInput.Focus()
		if m.queuedMessage != "" {
			return m.replayQueuedMessage()
		}
		return m, m.persistSessionCmd()

	case chatStreamUsage:
		// The input/output/cache counters are session-lifetime sums used for
		// cost accounting. The context counters are a snapshot of the most
		// recent request, which is what the context bar shows.
		if msg.promptTotalTokens > 0 {
			m.contextInputTokens = msg.promptTotalTokens
			m.contextCacheTokens = msg.cacheHitTokens
			// A new prompt supersedes the previous turn's response, which is
			// already folded into this request's history.
			m.contextOutputTokens = 0
		}
		if msg.inputTokens > 0 {
			m.inputTokens += msg.inputTokens
		}
		if msg.outputTokens > 0 {
			m.outputTokens += msg.outputTokens
			m.contextOutputTokens += msg.outputTokens
		}
		if msg.reasoningTokens > 0 {
			m.reasoningOutputTokens += msg.reasoningTokens
		}
		if msg.cacheHitTokens > 0 {
			m.cacheHitTokens += msg.cacheHitTokens
		}
		return m, nil

	case toolResultMsg:
		if m.cancelRequested {
			m.cancelRequested = false
			return m, nil
		}
		m.toolExec.active = false
		m.toolExec.title = ""
		m.toolExec.label = ""
		m.messages = append(m.messages, llm.Message{
			Role:       "tool",
			ToolCallID: msg.toolCallID,
			Content:    msg.content,
			IsError:    msg.isError,
		})
		m = m.extendTranscriptCache().trimMessages()
		m.stableContent = buildChatContent(m)
		m.stableMsgCount = len(m.messages)
		m.chatViewport.SetContent(m.stableContent)
		if m.stickToBottom {
			m.chatViewport.GotoBottom()
		}
		return m.executeNextTool()

	case retryFireMsg:
		if !m.retry.active || m.retry.needsOK || msg.token != m.retry.token {
			return m, nil
		}
		m.retry.active = false
		m.workingMsg = workingMessages[rand.Intn(len(workingMessages))]
		m.workingSpinnerIdx = 0
		return m, tea.Batch(startChatStreamCmd(m), workingTickCmd())

	case workingTickMsg:
		m.workingSpinnerIdx++
		if m.retry.active {
			// The retry countdown redraws off this tick, so keep it running
			// even though nothing is streaming yet.
			return m, workingTickCmd()
		}
		if m.isStreaming || m.toolExec.active {
			if m.workingSpinnerIdx%40 == 0 {
				m.workingMsg = workingMessages[rand.Intn(len(workingMessages))]
			}
			return m, workingTickCmd()
		}
		return m, nil

	case toastTimeoutMsg:
		if m.toast != nil && m.toast.id == msg.id {
			if m.yolo || m.alwaysAllowPerms {
				m.toastSeq++
				m.toast = &toastMsg{text: "YOLO mode", id: m.toastSeq}
			} else {
				m.toast = nil
			}
		}
		return m, nil

	case chatStreamError:
		if m.cancelRequested {
			m.cancelRequested = false
			m.retry = retryState{token: m.retry.token + 1}
			m.chatInput.Focus()
			return m, nil
		}
		if llm.Retryable(msg.err) && m.retry.attempt < maxRetryAttempts {
			return m.scheduleRetry(msg.err)
		}

		attempts := m.retry.attempt
		m = m.resetStreamingState()
		errText := fmt.Sprintf("_Error: %v_", msg.err)
		if attempts > 0 {
			errText = fmt.Sprintf("_Error after %d retries: %v_", attempts, msg.err)
		}
		m.messages = append(m.messages, llm.Message{
			Role:    "assistant",
			Content: errText,
		})
		m.queuedMessage = ""
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		if m.stickToBottom {
			m.chatViewport.GotoBottom()
		}
		m.chatInput.Focus()
		return m, m.persistSessionCmd()

	case sessionTitleGeneratedMsg:
		if msg.title != "" && m.sessionName == "" {
			m.sessionName = msg.title
			m.windowTitle = "gurt | " + m.sessionName
			return m, tea.Batch(m.persistSessionCmd())
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
		m.isStreaming = false
		m.workingMsg = ""
		m.workingSpinnerIdx = 0
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
		if m.stickToBottom {
			m.chatViewport.GotoBottom()
		}
		return m, nil

	case sessionSaveErrorMsg:
		m.messages = append(m.messages, llm.Message{
			Role:     "assistant",
			Internal: true,
			Content:  fmt.Sprintf("_Session save failed: %v_", msg.err),
		})
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		if m.stickToBottom {
			m.chatViewport.GotoBottom()
		}
		return m, nil

	case resourceStatsMsg:
		m.debugStats = resourceStats{cpuPercent: msg.cpuPercent, memMB: msg.memMB}
		if m.debug {
			return m, resourceMonitorTickCmd()
		}
		return m, nil

	case versionCheckResult:
		m.isStreaming = false
		m.workingMsg = ""
		m.workingSpinnerIdx = 0
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
		if m.stickToBottom {
			m.chatViewport.GotoBottom()
		}
		return m, nil
	}

	return m, nil
}

func (m model) updateWelcome(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateProviderPick(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateCustomModePick(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateCustomURL(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateAPIKeyInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

const dotenvPromptOptions = 3

func (m model) updateDotenvPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.dotenvCursor = (m.dotenvCursor - 1 + dotenvPromptOptions) % dotenvPromptOptions
	case "down":
		m.dotenvCursor = (m.dotenvCursor + 1) % dotenvPromptOptions
	case "enter":
		switch m.dotenvCursor {
		case 0:
			if err := config.SetCredFileAPIKey(m.provider, m.customURL, m.savedEndpointName, m.apiKey); err != nil {
				m.err = err
				m.errChoice = 0
				m.state = stateError
				return m, nil
			}
			return m.continueAfterAPIKey()
		case 1:
			return m.continueAfterAPIKey()
		case 2:
			m.dotenvInput.SetValue("GURT_API_KEY")
			m.dotenvInput.Focus()
			m.state = stateDotenvKeyName
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateDotenvPick(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateDotenvKeyName(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateDotenvKeyExists(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateCustomName(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	savedTools := make([]string, 0, len(m.alwaysAllowTools))
	for _, t := range m.alwaysAllowTools {
		if t != "read_file" {
			savedTools = append(savedTools, t)
		}
	}
	cfg.AlwaysAllowTools = savedTools
	cfg.AlwaysAllowCommandPrefixes = m.alwaysAllowCommandPrefixes
	cfg.AlwaysAllowExternal = m.alwaysAllowExternal
	cfg.TelemetryEnabled = &m.telemetryEnabled
	cfg.Theme = m.themeName
	return config.Save(cfg)
}

func (m model) updateModelPick(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateReasoningConfig(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateError(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateManualModel(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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

func (m model) updateSessionPick(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	m.windowTitle = "gurt"
	if m.sessionName != "" {
		m.windowTitle = "gurt | " + m.sessionName
	}
	cmds := []tea.Cmd{}
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

func (m model) updateAllowManage(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
			m.state = stateChat
			m.chatInput.Focus()
			return m, nil
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
		m.allowToolCheckItems = []string{"write_file", "edit_file", "delete_file"}
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
		m.state = stateChat
		m.chatInput.Focus()
		return m, nil
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

func (m model) updateChat(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return m.handleChatMessage(msg)
}

func (m model) handleChatMessage(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.selection.exists || m.selection.active {
		m.selection = textSelection{}
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
	}

	if m.pendingPerm != nil {
		// Sudo password input phase — capture keystrokes into the password field.
		if m.pendingPerm.confirmSudo {
			if msg.String() == "enter" {
				pw := m.sudoPasswordInput.Value()
				if pw == "" {
					return m, nil
				}
				tc := m.pendingPerm.toolCall
				remaining := m.pendingPerm.remaining

				// Modify the command to pipe the password to sudo -S.
				cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments))
				if err == nil {
					// Strip leading "sudo" from the command.
					sudoCmd := strings.TrimSpace(cmd)
					sudoCmd = strings.TrimPrefix(sudoCmd, "sudo")
					sudoCmd = strings.TrimSpace(sudoCmd)
					modifiedCmd := "echo " + tools.EscapeShellArg(pw) + " | sudo -S " + sudoCmd

					// Rebuild tool call arguments.
					var args tools.RunBashArgs
					args.Command = modifiedCmd
					args.Title, _ = tools.ExtractBashTitle(json.RawMessage(tc.Function.Arguments))
					args.Timeout = 30000
					if rawArgs, err := json.Marshal(args); err == nil {
						tc.Function.Arguments = string(rawArgs)
					}
				}

				m.pendingPerm = nil
				m.permCursor = 0
				m.sudoPasswordInput.SetValue("")
				m.chatInput.Focus()
				m = m.adjustViewportHeight()

				m.toolQueue = append(m.toolQueue, tc)
				return m.processToolCalls(remaining)
			}

			if msg.String() == "esc" {
				m.pendingPerm = nil
				m.permCursor = 0
				m.sudoPasswordInput.SetValue("")
				m.chatInput.Focus()
				m = m.adjustViewportHeight()
				return m, nil
			}

			var cmd tea.Cmd
			m.sudoPasswordInput, cmd = m.sudoPasswordInput.Update(msg)
			return m, cmd
		}

		tc := m.pendingPerm.toolCall
		optionCount := len(ui.PermissionOptions(tc.Function.Name, "", m.pendingPerm.externalPath, m.pendingPerm.sudo))

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
		case "pgup":
			return m.scrollPerm(-5), nil
		case "pgdown":
			return m.scrollPerm(5), nil
		case "esc":
			m.pendingPerm = nil
			m.permCursor = 0
			m.chatInput.Focus()
			m = m.adjustViewportHeight()
			return m, nil
		case "enter":
			remaining := m.pendingPerm.remaining
			externalPath := m.pendingPerm.externalPath
			isSudo := m.pendingPerm.sudo
			cursor := m.permCursor
			m.pendingPerm = nil
			m.permCursor = 0
			m.chatInput.Focus()
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
				return m, m.persistSessionCmd()
			}

			// External path permission prompt.
			if externalPath != "" {
				fileDir := filepath.Dir(externalPath)
				if !filepath.IsAbs(fileDir) {
					fileDir = filepath.Join(m.workspaceRoot, fileDir)
				}
				switch cursor {
				case 0: // Allow this operation
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				case 1: // Allow this directory for session
					m.allowedExternalPathsSession[filepath.Clean(fileDir)] = true
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				case 2: // Allow every directory for this session
					m.allowAllExternal = true
					m.toastSeq++
					m.toast = &toastMsg{text: "Allowing all external dirs for session", id: m.toastSeq}
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				case 3: // Always allow external (forever)
					m.alwaysAllowExternal = true
					m.toastSeq++
					m.toast = &toastMsg{text: "Always allowing external dirs", id: m.toastSeq}
					saveConfig(m)
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				case 4: // No
					return deny()
				}
				return m, nil
			}

			switch cursor {
			case 0: // Yes
				// If this is a sudo prompt, enter password input phase.
				if isSudo {
					m.pendingPerm = &pendingPerm{
						toolCall:    tc,
						remaining:   remaining,
						sudo:        true,
						confirmSudo: true,
					}
					m.permScroll = 0
					m.chatInput.Blur()
					m.sudoPasswordInput.Focus()
					m.sudoPasswordInput.SetValue("")
					m = m.adjustViewportHeight()
					return m, nil
				}
				m.toolQueue = append(m.toolQueue, tc)
				return m.processToolCalls(remaining)
			case 1:
				// For sudo, option 1 is "No".
				if isSudo {
					return deny()
				}
				switch name {
				case "run_bash":
					if cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments)); err == nil {
						prefix := tools.BashCommandPrefix(cmd)
						if prefix != "" {
							m.allowedBashPrefixesSession[prefix] = true
						}
					}
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				case "edit_file", "write_file":
					m.allowEdits = true
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				case "delete_file":
					m.allowDeletions = true
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				default:
					m.alwaysAllowPerms = true
					m.toastSeq++
					m.toast = &toastMsg{text: "YOLO mode", id: m.toastSeq}
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				}
			case 2:
				switch name {
				case "run_bash":
					return m.allowBashPrefix(tc, remaining)
				case "edit_file", "write_file", "delete_file":
					m.alwaysAllowPerms = true
					m.toastSeq++
					m.toast = &toastMsg{text: "YOLO mode", id: m.toastSeq}
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				}
				return deny()
			case 3:
				switch name {
				case "run_bash":
					m.alwaysAllowPerms = true
					m.toastSeq++
					m.toast = &toastMsg{text: "YOLO mode", id: m.toastSeq}
					m.toolQueue = append(m.toolQueue, tc)
					return m.processToolCalls(remaining)
				case "edit_file", "write_file":
					m.alwaysAllowTools = toggleInList(m.alwaysAllowTools, "edit_file")
					m.alwaysAllowTools = toggleInList(m.alwaysAllowTools, "write_file")
					saveConfig(m)
					m.toolQueue = append(m.toolQueue, tc)
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

	// A retry whose wait exceeds longRetryWaitThreshold parks until the user
	// says to sit it out.
	if m.retry.active && m.retry.needsOK {
		switch msg.String() {
		case "enter", "r":
			m.retry.needsOK = false
			m.retry.token++
			m.retry.until = time.Now().Add(m.retry.delay)
			return m, tea.Batch(retryFireCmd(m.retry.delay, m.retry.token), workingTickCmd())
		}
	}

	if msg.String() == "esc" && (m.isStreaming || (m.toolExec != nil && m.toolExec.active)) {
		m.cancelRequested = true
		if m.streamState.cancel != nil {
			m.streamState.cancel()
		}
		if m.toolExec != nil && m.toolExec.cancel != nil {
			m.toolExec.cancel()
		}
		m = m.resetStreamingState()
		m.messages = append(m.messages, llm.Message{
			Role:    "assistant",
			Content: "_Interrupted_",
		})
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		if m.stickToBottom {
			m.chatViewport.GotoBottom()
		}
		m.chatInput.Focus()
		return m, m.persistSessionCmd()
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
				if m.suggestions.isFiles {
					val := m.chatInput.Value()
					atIdx := strings.LastIndex(val, "@")
					m.chatInput.SetValue(val[:atIdx] + "@" + m.suggestions.items[sel].name + " ")
				} else {
					m.chatInput.SetValue("/" + m.suggestions.items[sel].name + " ")
				}
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

	// History navigation (up/down) — when input is empty or unchanged from
	// the last history entry. Once the user modifies the text (making it
	// "dirty"), arrows only move the cursor within the textarea.
	val := m.chatInput.Value()
	clean := val == "" || val == m.historyLoadedValue
	if clean && !m.suggestions.active && !m.showThemePicker && m.pendingPerm == nil {
		switch msg.String() {
		case "up":
			if len(m.history) > 0 && m.historyIndex < len(m.history)-1 {
				if m.historyIndex == -1 {
					m.historyDraft = val
				}
				m.historyIndex++
				m.chatInput.SetValue(m.history[len(m.history)-1-m.historyIndex])
				m.historyLoadedValue = m.chatInput.Value()
			}
			return m, nil
		case "down":
			if m.historyIndex >= 0 {
				m.historyIndex--
				if m.historyIndex >= 0 {
					m.chatInput.SetValue(m.history[len(m.history)-1-m.historyIndex])
					m.historyLoadedValue = m.chatInput.Value()
				} else {
					m.chatInput.SetValue(m.historyDraft)
					m.historyLoadedValue = m.historyDraft
				}
			}
			return m, nil
		}
	}

	if msg.String() == "enter" {
		input := strings.TrimSpace(m.chatInput.Value())
		if input == "" {
			return m, nil
		}
		isCommand := strings.HasPrefix(input, "/")

		if m.isStreaming || (m.toolExec != nil && m.toolExec.cancel != nil) {
			if isCommand {
				cmd := strings.TrimPrefix(strings.Fields(input)[0], "/")
				switch cmd {
				case "show-reasoning", "theme", "version", "help", "telemetry", "reasoning", "effort", "allow", "auth":
					return m.handleSlashCommand(input)
				}
			} else {
				m.history = history.Add(m.history, input)
				m.historyIndex = -1
				m.historyDraft = ""
				history.Save(m.history)
			}
			m.queuedMessage = input
			m.chatInput.Reset()
			m.historyLoadedValue = ""
			m = m.adjustViewportHeight()
			return m, nil
		}
		m.chatInput.Reset()
		m.historyLoadedValue = ""
		m = m.adjustViewportHeight()

		if isCommand {
			return m.handleSlashCommand(input)
		}

		m.history = history.Add(m.history, input)
		m.historyIndex = -1
		m.historyDraft = ""
		history.Save(m.history)

		today := time.Now().Format("January 2, 2006")
		if today != m.lastDateMessage {
			m.lastDateMessage = today
			input = "System: Current date is " + today + ".\n\n" + input
		}
		m.messages = append(m.messages, llm.Message{Role: "user", Content: input})
		m = m.extendTranscriptCache().trimMessages()
		m.isStreaming = true
		m.workingMsg = workingMessages[rand.Intn(len(workingMessages))]
		m.workingSpinnerIdx = 0
		m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		m.stickToBottom = true

		if m.cachedSystemPrompt == "" {
			if sp, err := renderSystemPrompt(m); err == nil {
				m.cachedSystemPrompt = sp
			}
		}
		cmds := []tea.Cmd{m.persistSessionCmd(), startChatStreamCmd(m), workingTickCmd()}
		if m.needsTitle {
			m.needsTitle = false
			cmds = append(cmds, generateTitleCmd(m))
		}
		return m, tea.Batch(cmds...)
	}

	// Ctrl+V: paste from clipboard directly (bubbles' internal Paste command
	// returns an unexported pasteMsg that the program-level type switch can't
	// route, so we handle it here instead).
	if msg.String() == "ctrl+v" {
		text, err := clipboard.ReadAll()
		if err == nil && text != "" {
			m.chatInput.InsertString(text)
			m = m.updateSuggestions()
			m = m.adjustViewportHeight()
		}
		return m, nil
	}

	// Ctrl+A / Cmd+A: select all text in the input field and copy to clipboard.
	// On macOS in most terminals Cmd+A sends the same control code as Ctrl+A
	// (ASCII 0x01), so msg.String() == "ctrl+a" works. On terminals with
	// enhanced keyboard reporting, it's reported as "super+a" with the
	// ModSuper modifier.
	if msg.String() == "ctrl+a" || msg.String() == "super+a" || (msg.Code == 'a' && msg.Mod.Contains(tea.ModSuper)) {
		text := m.chatInput.Value()
		if text != "" {
			copyToClipboard(text)
			m.toastSeq++
			m.toast = &toastMsg{text: "Copied input field to clipboard", id: m.toastSeq}
			return m, toastTimeoutCmd(m.toastSeq)
		}
		return m, nil
	}

	// Filter partial SGR mouse events that the input reader
	// couldn't decode — they arrive as Alt+[ or <digits>;… runes.
	if msg.Mod.Contains(tea.ModAlt) && len(msg.Text) == 1 && msg.Text[0] == '[' {
		return m, nil
	}
	if len(msg.Text) > 0 && partialMouseEventRe.MatchString(msg.Text) {
		return m, nil
	}

	var cmd tea.Cmd
	switch msg.String() {
	case "pgup", "pgdown", "home", "end":
		m.chatViewport, _ = m.chatViewport.Update(msg)
		m.stickToBottom = m.chatViewport.AtBottom()
	}
	m.chatInput, cmd = m.chatInput.Update(msg)
	m = m.updateSuggestions()
	m = m.adjustViewportHeight()
	return m, cmd
}

func (m model) executeNextTool() (tea.Model, tea.Cmd) {
	if len(m.toolQueue) == 0 {
		m.toolCallCycle = 0
		if m.queuedMessage != "" {
			return m.replayQueuedMessage()
		}
		m.isStreaming = true
		m.workingMsg = workingMessages[rand.Intn(len(workingMessages))]
		m.workingSpinnerIdx = 0
		m = m.adjustViewportHeight()
		return m, tea.Batch(m.persistSessionCmd(), startChatStreamCmd(m), workingTickCmd())
	}

	tc := m.toolQueue[0]
	m.toolQueue = m.toolQueue[1:]

	m.toolExec.active = true
	m.toolExec.toolName = tc.Function.Name
	m.toolExec.title = ""
	m.toolExec.label = tools.ToolFriendlyLabel(tc.Function.Name, json.RawMessage(tc.Function.Arguments))
	if tc.Function.Name == "run_bash" {
		if title, err := tools.ExtractBashTitle(json.RawMessage(tc.Function.Arguments)); err == nil {
			m.toolExec.title = title
		}
	}
	m.workingSpinnerIdx = 0
	m = m.adjustViewportHeight()

	ctx, cancel := context.WithCancel(context.Background())
	m.toolExec.cancel = cancel

	args := json.RawMessage(tc.Function.Arguments)

	// Build list of allowed external directories.
	var allowedDirs []string
	for dir := range m.allowedExternalPathsSession {
		allowedDirs = append(allowedDirs, dir)
	}
	if m.alwaysAllowExternal || m.allowAllExternal {
		allowedDirs = append(allowedDirs, "/")
	}
	if m.sessionOutputsDir != "" {
		allowedDirs = append(allowedDirs, m.sessionOutputsDir)
	}
	opts := tools.Options{
		WorkspaceRoot:       m.workspaceRoot,
		AllowedExternalDirs: allowedDirs,
		SessionID:           m.sessionID,
		SessionOutputsDir:   m.sessionOutputsDir,
	}

	return m, tea.Batch(func() tea.Msg {
		defer cancel()
		result, err := tools.Execute(ctx, tc.Function.Name, args, opts)
		if ctx.Err() != nil {
			return nil
		}
		content := result
		if content == "" && err == nil {
			content = "(no output)"
		}
		if err != nil {
			content = fmt.Sprintf("Error: %v", err)
		}
		return toolResultMsg{toolCallID: tc.ID, content: content, isError: err != nil}
	})
}

// allowBashPrefix adds the command prefix from a run_bash tool call to the
// always-allowed list, persists it to config, queues the tool, and processes
// remaining tool calls.
func (m model) allowBashPrefix(tc llm.ToolCall, remaining []llm.ToolCall) (tea.Model, tea.Cmd) {
	cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments))
	if err == nil {
		prefix := tools.BashCommandPrefix(cmd)
		if prefix != "" {
			m.allowedBashPrefixes[prefix] = true
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
	m.toolQueue = append(m.toolQueue, tc)
	return m.processToolCalls(remaining)
}

func (m model) processToolCalls(tcs []llm.ToolCall) (tea.Model, tea.Cmd) {
	for i, tc := range tcs {
		if len(m.alwaysAllowTools) > 0 {
			matched := false
			for _, name := range m.alwaysAllowTools {
				if tc.Function.Name == name {
					matched = true
					break
				}
			}
			if matched {
				m.toolQueue = append(m.toolQueue, tc)
				continue
			}
		}

		// In YOLO mode, reject sudo commands instead of running them.
		if (m.yolo || m.alwaysAllowPerms) && tc.Function.Name == "run_bash" {
			if cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments)); err == nil && tools.IsSudoCommand(cmd) {
				m.messages = append(m.messages, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content:    "sudo commands are not allowed in YOLO mode",
				})
				m.toolCallCycle = 0
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
				if m.stickToBottom {
					m.chatViewport.GotoBottom()
				}
				return m, m.persistSessionCmd()
			}
		}

		if !m.yolo && !m.alwaysAllowPerms && tc.Function.Name == "run_bash" {
			cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments))
			if err == nil {
				// Check if the command uses sudo.
				if tools.IsSudoCommand(cmd) {
					m.pendingPerm = &pendingPerm{
						toolCall:  tc,
						remaining: tcs[i+1:],
						sudo:      true,
					}
					m.permScroll = 0
					m.chatViewport.SetContent(buildChatContentHighlighted(m))
					if m.stickToBottom {
						m.chatViewport.GotoBottom()
					}
					m.chatInput.Blur()
					m = m.adjustViewportHeight()
					return m, m.persistSessionCmd()
				}

				matched := false
				for prefix := range m.allowedBashPrefixesSession {
					if strings.HasPrefix(cmd, prefix) {
						matched = true
						break
					}
				}
				if !matched {
					for _, prefix := range m.alwaysAllowCommandPrefixes {
						if strings.HasPrefix(cmd, prefix) {
							matched = true
							break
						}
					}
				}
				if matched {
					m.toolQueue = append(m.toolQueue, tc)
					continue
				}
			}
		}

		if m.allowEdits && (tc.Function.Name == "edit_file" || tc.Function.Name == "write_file") {
			m.toolQueue = append(m.toolQueue, tc)
			continue
		}

		if m.allowDeletions && tc.Function.Name == "delete_file" {
			m.toolQueue = append(m.toolQueue, tc)
			continue
		}

		// Check for file tools targeting paths outside the workspace root.
		if !m.yolo && !m.alwaysAllowPerms && !m.alwaysAllowExternal && !m.allowAllExternal {
			if tc.Function.Name == "read_file" || tc.Function.Name == "write_file" ||
				tc.Function.Name == "edit_file" || tc.Function.Name == "delete_file" {
				if filePath, err := tools.ExtractFileToolPath(tc.Function.Name, json.RawMessage(tc.Function.Arguments)); err == nil && tools.IsPathOutsideWorkspace(m.workspaceRoot, filePath) {
					// Skip prompt if the path is within the session outputs directory.
					cleanPath := filepath.Clean(filePath)
					if m.sessionOutputsDir != "" {
						cleanOutputsDir := filepath.Clean(m.sessionOutputsDir)
						if strings.HasPrefix(cleanPath, cleanOutputsDir) {
							m.toolQueue = append(m.toolQueue, tc)
							continue
						}
					}
					fileDir := filepath.Dir(filePath)
					if !filepath.IsAbs(fileDir) {
						fileDir = filepath.Join(m.workspaceRoot, fileDir)
					}
					if !m.allowedExternalPathsSession[filepath.Clean(fileDir)] {
						m.pendingPerm = &pendingPerm{
							toolCall:     tc,
							remaining:    tcs[i+1:],
							externalPath: filePath,
						}
						m.permScroll = 0
					m.chatViewport.SetContent(buildChatContentHighlighted(m))
					if m.stickToBottom {
						m.chatViewport.GotoBottom()
					}
					m.chatInput.Blur()
					m = m.adjustViewportHeight()
					return m, m.persistSessionCmd()
					}
				}
			}
		}

		if tools.IsDestructive(tc.Function.Name) && !m.yolo && !m.alwaysAllowPerms {
			m.pendingPerm = &pendingPerm{
				toolCall:  tc,
				remaining: tcs[i+1:],
			}
			m.permScroll = 0
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			if m.stickToBottom {
				m.chatViewport.GotoBottom()
			}
			m.chatInput.Blur()
			m = m.adjustViewportHeight()
			return m, m.persistSessionCmd()
		}
		m.toolQueue = append(m.toolQueue, tc)
	}
	return m.executeNextTool()
}

func (m model) updateMouse(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch mouse := msg.(type) {
	case tea.MouseWheelMsg:
		// The permission prompt owns the wheel while it is up.
		if m.pendingPerm != nil {
			switch mouse.Button {
			case tea.MouseWheelUp:
				return m.scrollPerm(-3), nil
			case tea.MouseWheelDown:
				return m.scrollPerm(3), nil
			}
			return m, nil
		}

		if m.selection.exists {
			m.selection = textSelection{}
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
		}

		// Don't forward wheel events past boundaries to avoid the
		// bubbletea input reader splitting SGR mouse events across
		// a read boundary, which leaks raw escape bytes as keypresses.
		if (mouse.Button == tea.MouseWheelUp && m.chatViewport.AtTop()) ||
			(mouse.Button == tea.MouseWheelDown && m.chatViewport.AtBottom()) {
			return m, nil
		}

		var cmd tea.Cmd
		m.chatViewport, cmd = m.chatViewport.Update(mouse)
		m.stickToBottom = m.chatViewport.AtBottom()
		return m, cmd

	case tea.MouseMotionMsg:
		if m.selection.active {
			line, col, ok := computeContentPosition(m, mouse.Mouse())
			if ok {
				m.selection.focusY = line
				m.selection.focusX = col
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
			}
		}
		return m, nil

	case tea.MouseClickMsg:
		// Non-left click: clear selection
		if mouse.Button != tea.MouseLeft {
			if m.selection.exists {
				m.selection = textSelection{}
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
			}
			return m, nil
		}

		// Outside the transcript the click lands on chrome — the context meter,
		// the status bar, the permission prompt — where there is nothing to drag
		// but plenty worth copying. A pending prompt owns every click, the same
		// way it owns the wheel, because it can grow to cover the transcript.
		if m.pendingPerm != nil || !insideViewport(m, mouse.Y) {
			m.lastClick = clickTracker{}
			if m.selection.exists {
				m.selection = textSelection{}
				m.chatViewport.SetContent(buildChatContentHighlighted(m))
			}
			if zone, ok := hitTestCopyZone(m, mouse.X, mouse.Y); ok {
				return m.copyWithToast(zone.text, "Copied "+zone.label)
			}
			return m, nil
		}

		// Left click: check reasoning markers first, then start selection
		line, col, ok := computeContentPosition(m, mouse.Mouse())
		if !ok {
			return m, nil
		}
		var clicks int
		m, clicks = m.trackClick(mouse.X, mouse.Y)

		// The reasoning disclosure markers own the first click on their row.
		if clicks == 1 {
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
						yOff := m.chatViewport.YOffset()
						m.selection = textSelection{}
						m = m.invalidateTranscriptCache()
						m.chatViewport.SetContent(buildChatContentHighlighted(m))
						if yOff > m.chatViewport.YOffset() {
							m.chatViewport.GotoBottom()
						} else {
							m.chatViewport.SetYOffset(yOff)
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

		// Repeat clicks select structure instead of starting a drag: two clicks
		// grab the word under the cursor, three grab the whole line — or the
		// whole code block, which is the snippet people are usually after.
		if clicks >= 2 {
			return m.selectByGesture(line, col, clicks)
		}

		// Start new text selection
		m.selection = textSelection{
			anchorY: line, anchorX: col,
			focusY: line, focusX: col,
			active: true,
		}
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		return m, nil

	case tea.MouseReleaseMsg:
		if !m.selection.active {
			return m, nil
		}
		m.selection.active = false

		// A press and release on the same cell is a click, not a drag: there is
		// nothing to copy, and copying one character on every stray click would
		// trample whatever the user already had on the clipboard.
		if m.selection.anchorY == m.selection.focusY && m.selection.anchorX == m.selection.focusX {
			m.selection = textSelection{}
			m.chatViewport.SetContent(buildChatContentHighlighted(m))
			return m, nil
		}

		// The highlight stays up after the copy so the user can see what landed
		// on the clipboard; the next click, scroll or keystroke clears it.
		m.selection.exists = true
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		text := extractSelectedText(buildChatContent(m), m.selection)
		return m.copyWithToast(text, copiedLabel(text))
	}

	return m, nil
}

// selectByGesture handles double and triple clicks: word, then line or the
// enclosing styled block. The selection is copied straight away and left on
// screen so the user can see what they got.
func (m model) selectByGesture(line, col, clicks int) (tea.Model, tea.Cmd) {
	content := buildChatContent(m)

	var (
		sel textSelection
		ok  bool
	)
	if clicks == 2 {
		sel, ok = selectWordAt(content, line, col)
	} else {
		if sel, ok = selectBlockAt(content, line, codeBlockStylePrefix(m.theme)); !ok {
			sel, ok = selectLineAt(content, line)
		}
	}
	if !ok {
		return m, nil
	}

	m.selection = sel
	m.chatViewport.SetContent(buildChatContentHighlighted(m))
	text := extractSelectedText(content, sel)
	return m.copyWithToast(text, copiedLabel(text))
}

// trackClick folds a click into the double/triple click run in progress and
// reports how many clicks that run is up to.
func (m model) trackClick(x, y int) (model, int) {
	const clickInterval = 450 * time.Millisecond

	now := time.Now()
	sameSpot := y == m.lastClick.y && (x-m.lastClick.x < 2 && m.lastClick.x-x < 2)
	if m.lastClick.count > 0 && sameSpot && now.Sub(m.lastClick.at) < clickInterval {
		m.lastClick.count++
	} else {
		m.lastClick.count = 1
	}
	if m.lastClick.count > 3 {
		m.lastClick.count = 3
	}
	m.lastClick.x, m.lastClick.y, m.lastClick.at = x, y, now
	return m, m.lastClick.count
}

// copyWithToast copies text and raises a toast naming what was copied.
func (m model) copyWithToast(text, label string) (tea.Model, tea.Cmd) {
	if strings.TrimSpace(text) == "" {
		return m, nil
	}
	cmd := copyCmd(text)
	m.toastSeq++
	m.toast = &toastMsg{text: label, id: m.toastSeq}
	return m, tea.Batch(cmd, toastTimeoutCmd(m.toastSeq))
}

// copiedLabel describes a copied chunk of transcript by its shape: short
// single-line copies quote themselves, longer ones report their size.
func copiedLabel(text string) string {
	if text == "" {
		return ""
	}
	if lines := strings.Count(text, "\n") + 1; lines > 1 {
		return fmt.Sprintf("Copied %d lines", lines)
	}
	if len([]rune(text)) <= 30 {
		return "Copied \"" + text + "\""
	}
	return "Copied to clipboard"
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

	streamingSafe := map[string]bool{
		"show-reasoning": true,
		"theme":          true,
		"version":        true,
		"help":           true,
		"telemetry":      true,
		"reasoning":      true,
		"effort":         true,
		"allow":          true,
		"auth":           true,
	}

	if m.isStreaming && !streamingSafe[cmd] {
		m.messages = append(m.messages, llm.Message{
			Role:     "assistant",
			Internal: true,
			Content:  fmt.Sprintf("_Cannot run /%s while LLM is streaming. Cancel the current response first._", cmd),
		})
		m.chatViewport.SetContent(buildChatContentHighlighted(m))
		m.chatViewport.GotoBottom()
		return m, nil
	}

	switch cmd {
	case "model":
		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			m.fetchModelsCmd(),
		)

	case "provider":
		m.modelName = ""
		m.customURL = ""
		m.savedEndpointName = ""
		m.state = stateProviderPick
		return m, nil

	case "auth":
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
		m = m.invalidateTranscriptCache()
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
		var saveCmd tea.Cmd
		if len(m.messages) > 0 {
			saveCmd = m.persistSessionCmd()
		}
		m = m.resetToNewSession()
		var checkCmd tea.Cmd
		m, checkCmd = m.enterChatState()
		m.windowTitle = "gurt"
		cmds := []tea.Cmd{}
		if checkCmd != nil {
			cmds = append(cmds, checkCmd)
		}
		if saveCmd != nil {
			cmds = append(cmds, saveCmd)
		}
		return m, tea.Batch(cmds...)

	case "allow":
		m.state = stateAllowManage
		m.allowManageCursor = 0
		m.allowManageScroll = 0
		m.allowManageAdding = false
		m.allowManageInput.Reset()
		return m, nil

	case "update":
		m.isStreaming = true
		m.workingMsg = "Downloading update..."
		m.workingSpinnerIdx = 0
		if m.latestVersion == "" {
			return m, tea.Batch(checkAndUpdateCmd(), workingTickCmd())
		}
		return m, tea.Batch(performUpdateCmd(m.latestVersion), workingTickCmd())

	case "version":
		m.isStreaming = true
		m.workingMsg = "Checking for updates..."
		m.workingSpinnerIdx = 0
		return m, tea.Batch(checkVersionCmd(), workingTickCmd())

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
	if m.isStreaming || m.pendingPerm != nil {
		m.suggestions = suggestionState{}
		return m
	}

	// File suggestions (@ references) — only after at least one char
	if strings.Contains(val, "@") && !files.IsHomeDir(m.workspaceRoot) {
		atIdx := strings.LastIndex(val, "@")
		afterAt := val[atIdx+1:]

		if afterAt != "" && !strings.ContainsAny(afterAt, " \t") {
			if !m.filesCached {
				fileList, err := files.WalkWorkspace(m.workspaceRoot, files.MaxWalkFiles)
				if err == nil && len(fileList) > 0 {
					m.fileList = fileList
					m.filesCached = true
				}
			}

			if m.filesCached && len(m.fileList) > 0 {
				var matches []suggestionItem
				afterAtLower := strings.ToLower(afterAt)
				for _, f := range m.fileList {
					if strings.Contains(strings.ToLower(f), afterAtLower) {
						matches = append(matches, suggestionItem{name: f})
					}
				}
				if len(matches) > 0 {
					selected := m.suggestions.selected
					if selected < 0 || selected >= len(matches) {
						selected = 0
					}
					m.suggestions = suggestionState{
						items:    matches,
						selected: selected,
						active:   true,
						isFiles:  true,
					}
					return m
				}
			}
		}
	}

	// Slash command suggestions (/ references)
	if strings.HasPrefix(val, "/") {
		input := strings.TrimPrefix(val, "/")

		var matches []suggestionItem
		for _, sc := range slashCommands {
			if strings.HasPrefix(sc.name, input) {
				matches = append(matches, suggestionItem{name: sc.name, description: sc.description})
			}
		}

		if len(matches) > 0 {
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
	}

	m.suggestions = suggestionState{}
	return m
}

func (m model) adjustViewportHeight() model {
	// title + top divider + spacer + toast + bottom divider; every one of these
	// occupies a row even when empty, so chatView and this must agree exactly or
	// the view overflows the screen and scrolls the terminal.
	fixed := m.chatChromeLines()
	// bottom section
	switch {
	case m.pendingPerm != nil:
		fixed += m.permOverlayHeight()
	case m.showThemePicker:
		fixed += m.themePickerOverlayHeight()
	default:
		fixed += m.chatInput.Height() + len(m.fitBottomBar().rows)
		fixed += len(m.suggestionRows())
		fixed += len(m.queuedRows())
	}
	m.chatViewport.SetHeight(m.height - fixed)
	if m.chatViewport.Height() < 1 {
		m.chatViewport.SetHeight(1)
	}
	return m
}

// scrollPerm scrolls the permission prompt body by delta lines, clamped to the
// body that is actually there.
func (m model) scrollPerm(delta int) model {
	// How many body lines fit depends on how they soft-wrap, so it varies with
	// the offset. Measuring the window at the top gives a fixed bound to clamp
	// against: scrolling stays stable, and the end of the body is always
	// reachable (a shorter tail simply leaves the window part-empty).
	top := m
	top.permScroll = 0
	_, _, total, visible := top.renderPermOverlay()
	maxScroll := total - visible
	if maxScroll < 0 {
		maxScroll = 0
	}

	m.permScroll += delta
	if m.permScroll > maxScroll {
		m.permScroll = maxScroll
	}
	if m.permScroll < 0 {
		m.permScroll = 0
	}
	m.permScrollTotal = total
	return m
}

func (m model) permOverlayHeight() int {
	_, height, _, _ := m.renderPermOverlay()
	return height
}

func (m model) themePickerOverlayHeight() int {
	boxW := ui.NewLayout(m.width, m.height).PopupWidth()
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

const (
	// maxRetryAttempts bounds how many times a failed chat request is
	// repeated before the error surfaces in the transcript.
	maxRetryAttempts = 8
	// longRetryWaitThreshold is the point past which a wait is no longer
	// automatic. Usage-limit resets can be hours out, and silently parking the
	// TUI that long is worse than asking.
	longRetryWaitThreshold = 5 * time.Minute
)

// scheduleRetry arms a retry after a failed request, discarding whatever the
// dead stream had already produced. isStreaming stays true so the cancel keys
// and the spinner tick keep working while the countdown runs.
func (m model) scheduleRetry(err error) (tea.Model, tea.Cmd) {
	m.streamingContent = nil
	m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
	m.isStreaming = true
	m.workingMsg = ""
	m.workingSpinnerIdx = 0
	if m.streamState != nil {
		m.streamState.cancel = nil
	}

	delay, fromProvider := llm.RetryHint(err)
	if !fromProvider {
		delay = llm.BackoffDelay(m.retry.attempt + 1)
	}

	m.retry = retryState{
		active:    true,
		attempt:   m.retry.attempt + 1,
		delay:     delay,
		until:     time.Now().Add(delay),
		err:       err,
		token:     m.retry.token + 1,
		needsOK:   delay > longRetryWaitThreshold,
		rateLimit: llm.IsRateLimit(err),
	}

	// Drop the partial assistant text from the transcript view.
	m.chatViewport.SetContent(buildChatContentHighlighted(m))
	if m.stickToBottom {
		m.chatViewport.GotoBottom()
	}

	if m.retry.needsOK {
		return m, workingTickCmd()
	}
	return m, tea.Batch(retryFireCmd(delay, m.retry.token), workingTickCmd())
}

func retryFireCmd(d time.Duration, token int) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return retryFireMsg{token: token}
	})
}

func startChatStreamCmd(m model) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		m.streamState.cancel = cancel

		systemPrompt := m.cachedSystemPrompt
		if systemPrompt == "" {
			var err error
			systemPrompt, err = renderSystemPrompt(m)
			if err != nil {
				cancel()
				return chatStreamError{err: fmt.Errorf("rendering system prompt: %w", err)}
			}
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

		msgs := stripReasoning(filterInternal(m.messages))
		if attachments := collectFileAttachments(m, msgs); attachments != "" {
			// Append file attachments as a separate user message rather than
			// modifying the system prompt. This keeps the system prompt stable
			// for prompt caching (both Anthropic's cache_control and OpenAI's
			// prefix-based caching).
			msgs = append(msgs, llm.Message{Role: "user", Content: attachments})
		}

		req := llm.ChatRequest{
			Model:           m.modelName,
			Messages:        msgs,
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
			for {
				select {
				case <-ctx.Done():
					if !doneSent {
						globalProgram.Send(chatStreamDone{})
					}
					return
				case event, ok := <-events:
					if !ok {
						if !doneSent {
							globalProgram.Send(chatStreamDone{})
							if m.debug {
								debug.SaveRecord(m.sessionID, m.modelName, req, debugEvents)
							}
						}
						return
					}
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
						globalProgram.Send(chatStreamUsage{inputTokens: event.InputTokens, outputTokens: event.OutputTokens, reasoningTokens: event.ReasoningTokens, cacheHitTokens: event.CacheHitTokens, cacheWriteTokens: event.CacheWriteTokens, promptTotalTokens: event.PromptTotalTokens})
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
			}
		}()

		return nil
	}
}

func (m model) resetStreamingState() model {
	m.streamingContent = nil
	m.isStreaming = false
	m.workingMsg = ""
	m.workingSpinnerIdx = 0
	m.toolExec.active = false
	m.toolExec.title = ""
	m.toolExec.label = ""
	m.toolExec.cancel = nil
	m.toolQueue = nil
	m.toolCallCycle = 0
	m.streamState.cancel = nil
	// Bump the token so any retry already scheduled by tea.Tick is ignored
	// when it fires.
	m.retry = retryState{token: m.retry.token + 1}
	m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
	m.stickToBottom = true
	return m
}

func (m model) replayQueuedMessage() (tea.Model, tea.Cmd) {
	qmsg := m.queuedMessage
	m.queuedMessage = ""

	if strings.HasPrefix(qmsg, "/") {
		return m.handleSlashCommand(qmsg)
	}

	today := time.Now().Format("January 2, 2006")
	if today != m.lastDateMessage {
		m.lastDateMessage = today
		qmsg = "System: Current date is " + today + ".\n\n" + qmsg
	}
	m.messages = append(m.messages, llm.Message{Role: "user", Content: qmsg})
	m = m.extendTranscriptCache().trimMessages()
	m.isStreaming = true
	m.workingMsg = workingMessages[rand.Intn(len(workingMessages))]
	m.workingSpinnerIdx = 0
	m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
	m.chatViewport.SetContent(buildChatContentHighlighted(m))
	m.chatViewport.GotoBottom()
	m.stickToBottom = true
	if m.cachedSystemPrompt == "" {
		if sp, err := renderSystemPrompt(m); err == nil {
			m.cachedSystemPrompt = sp
		}
	}
	return m, tea.Batch(m.persistSessionCmd(), startChatStreamCmd(m), workingTickCmd())
}

func resourceMonitorTickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return resourceMonitorCmd()
	})
}

func workingTickCmd() tea.Cmd {
	return tea.Tick(450*time.Millisecond, func(t time.Time) tea.Msg {
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
				// Strip the "System: Current date is ..." prefix that is
				// prepended to the first user message of each day.
				titleMsg = stripDatePrefixForTitle(titleMsg)
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

// stripDatePrefixForTitle removes the "System: Current date is ...\n\n" prefix
// from the first user message so the title generator sees the actual question.
func stripDatePrefixForTitle(s string) string {
	const prefix = "System: Current date is "
	if strings.HasPrefix(s, prefix) {
		if idx := strings.Index(s, "\n\n"); idx != -1 {
			return s[idx+2:]
		}
	}
	return s
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

// renderStreamingPart builds only the currently-streaming assistant message
// (label + reasoning + content). This avoids rebuilding all finalized messages
// on every streaming chunk.
func renderStreamingPart(m model) string {
	streamingLen := 0
	if m.streamingContent != nil {
		streamingLen = m.streamingContent.Len()
	}
	reasoningLen := 0
	if m.reasoning.content != nil {
		reasoningLen = m.reasoning.content.Len()
	}
	if streamingLen == 0 && reasoningLen == 0 {
		return ""
	}
	b := builderPool.Get().(*strings.Builder)
	defer builderPool.Put(b)
	b.Reset()
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
		b.WriteString(ui.RenderReasoning(m.theme, m.reasoning.active, m.reasoning.visible, elapsed, content, m.chatViewport.Width()))
		b.WriteString("\n")
	}
	if streamingLen > 0 && m.streamingContent != nil {
		content := m.streamingContent.String()
		if content != "" {
			b.WriteString(ui.RenderAssistantContent(m.theme, content, m.chatViewport.Width(), commandNames()))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	return ui.FitWidth(b.String(), m.chatViewport.Width())
}

// transcriptCacheKeyStr returns the cache key for transcript rendering.
// When theme or width changes, the key changes and the cache is invalidated.
func (m model) transcriptCacheKeyStr() string {
	return fmt.Sprintf("%s-%d", m.themeName, m.width)
}

// transcriptBoundary returns the index of the first message that belongs to an
// unresolved block. Everything before this index is finalized (no tool calls
// awaiting results) and can be cached. During streaming all messages are
// finalized because the in-progress assistant message lives in
// streamingContent, not in m.messages.
func (m model) transcriptBoundary() int {
	if len(m.messages) == 0 {
		return 0
	}
	resolved := make(map[string]bool)
	for i := len(m.messages) - 1; i >= 0; i-- {
		msg := m.messages[i]
		if msg.Role == "tool" {
			resolved[msg.ToolCallID] = true
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			allDone := true
			for _, tc := range msg.ToolCalls {
				if !resolved[tc.ID] {
					allDone = false
					break
				}
			}
			if allDone {
				for _, tc := range msg.ToolCalls {
					delete(resolved, tc.ID)
				}
			} else {
				return i
			}
		}
	}
	return len(m.messages)
}

// extendTranscriptCache extends the render cache to include all finalized
// messages up to the current transcriptBoundary. Call this after appending
// or modifying messages so the next buildChatContent call is fast.
func (m model) extendTranscriptCache() model {
	key := m.transcriptCacheKeyStr()
	if m.transcriptCachedKey != key {
		m.transcriptCacheContent = ""
		m.transcriptCacheUpTo = 0
		m.transcriptCachedKey = key
	}
	boundary := m.transcriptBoundary()
	if boundary > m.transcriptCacheUpTo {
		m.transcriptCacheContent += renderMessageRange(m, m.transcriptCacheUpTo, boundary)
		m.transcriptCacheUpTo = boundary
	}
	return m
}

// invalidateTranscriptCache clears the render cache so the next call to
// buildChatContent does a full rebuild. Use when messages are modified
// in-place (e.g. toggling ReasoningVisible) or when the structure changes
// in ways that cannot be incrementally extended.
func (m model) invalidateTranscriptCache() model {
	m.transcriptCacheContent = ""
	m.transcriptCacheUpTo = 0
	m.transcriptCachedKey = ""
	return m
}

// renderMessageRange renders messages [from, to) using the same logic as
// buildChatContent's main message loop. The range must be within
// m.messages. Tool results are rendered inline with their parent assistant
// message when found within the full messages array.
func renderMessageRange(m model, from, to int) string {
	if from >= to || from >= len(m.messages) {
		return ""
	}
	if to > len(m.messages) {
		to = len(m.messages)
	}

	b := builderPool.Get().(*strings.Builder)
	defer builderPool.Put(b)
	b.Reset()

	toolNames := buildToolNameLookup(m.messages)
	skipResultIDs := make(map[string]bool)

	for i := from; i < to; i++ {
		msg := m.messages[i]
		switch msg.Role {
		case "user":
			display := sessions.StripDatePrefix(msg.Content)
			b.WriteString(ui.RenderUserMessage(m.theme, display, m.chatViewport.Width(), commandNames()))
			b.WriteString("\n")
		case "assistant":
			if msg.Internal {
			} else if i == 0 || m.messages[i-1].Role == "user" {
				b.WriteString(ui.RenderAssistantLabel(m.theme, m.displayNameForModel(msg.Model)))
				b.WriteString("\n")
			}
			if msg.Reasoning != "" {
				if msg.ReasoningVisible {
					b.WriteString(ui.RenderReasoning(m.theme, false, true, msg.ReasoningDuration, msg.Reasoning, m.chatViewport.Width()))
					b.WriteString("\n")
				} else {
					b.WriteString(ui.RenderReasoningStored(m.theme, msg.ReasoningDuration))
					b.WriteString("\n")
				}
			}
			if msg.Content != "" {
				b.WriteString(ui.RenderAssistantContent(m.theme, msg.Content, m.chatViewport.Width(), commandNames()))
				b.WriteString("\n")
			}
			if len(msg.ToolCalls) > 0 {
				for _, tc := range msg.ToolCalls {
					found := false
					for j := i + 1; j < len(m.messages); j++ {
						if m.messages[j].Role == "tool" && m.messages[j].ToolCallID == tc.ID {
							b.WriteString(ui.RenderUnifiedToolCard(m.theme, tc, m.messages[j].Content, m.chatViewport.Width(), m.messages[j].IsError))
							b.WriteString("\n")
							skipResultIDs[tc.ID] = true
							found = true
							break
						}
					}
					if !found {
						b.WriteString(ui.RenderToolCall(m.theme, tc, m.chatViewport.Width()))
						b.WriteString("\n")
					}
				}
			}
		case "tool":
			if skipResultIDs[msg.ToolCallID] {
				continue
			}
			toolName := toolNames[msg.ToolCallID]
			b.WriteString(ui.RenderToolResult(m.theme, toolName, msg.Content, m.chatViewport.Width(), msg.IsError))
			b.WriteString("\n")
		}
	}

	// The renderers below aim at the viewport width; this is the backstop that
	// makes it true no matter what they emitted, so nothing is ever cut off at
	// the right edge. Applied here rather than in buildChatContent so the
	// transcript cache stores already-fitted text.
	return ui.FitWidth(b.String(), m.chatViewport.Width())
}

// trimMessages removes old messages when the session exceeds the cap. The
// cache is invalidated since message indices shift.
func (m model) trimMessages() model {
	if len(m.messages) <= maxSessionMessages {
		return m
	}
	excess := len(m.messages) - maxSessionMessages
	if excess+1 >= len(m.messages) {
		excess = len(m.messages) - 2
	}
	if excess <= 0 {
		return m
	}
	n := copy(m.messages, m.messages[excess:])
	m.messages = m.messages[:n]
	return m.invalidateTranscriptCache()
}

func buildChatContent(m model) string {
	streamingLen := 0
	if m.streamingContent != nil {
		streamingLen = m.streamingContent.Len()
	}
	reasoningLen := 0
	if m.reasoning.content != nil {
		reasoningLen = m.reasoning.content.Len()
	}

	if len(m.messages) == 0 && streamingLen == 0 {
		b := builderPool.Get().(*strings.Builder)
		defer builderPool.Put(b)
		b.Reset()
		b.WriteString(m.theme.EmptyState.Render("No messages yet. Send a message to start."))
		b.WriteString("\n")
		return b.String()
	}

	var b strings.Builder
	b.Grow(len(m.transcriptCacheContent) + 512)

	cacheValid := m.transcriptCachedKey == m.transcriptCacheKeyStr()
	if cacheValid && m.transcriptCacheUpTo >= len(m.messages) {
		b.WriteString(m.transcriptCacheContent)
	} else if cacheValid && m.transcriptCacheUpTo > 0 {
		b.WriteString(m.transcriptCacheContent)
		b.WriteString(renderMessageRange(m, m.transcriptCacheUpTo, len(m.messages)))
	} else {
		b.WriteString(renderMessageRange(m, 0, len(m.messages)))
	}

	if streamingLen > 0 || reasoningLen > 0 {
		b.WriteString(renderStreamingPart(m))
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

func stripReasoning(msgs []llm.Message) []llm.Message {
	out := make([]llm.Message, len(msgs))
	for i, m := range msgs {
		m.Reasoning = ""
		m.ReasoningDuration = 0
		m.ReasoningVisible = false
		out[i] = m
	}
	return out
}

func collectFileAttachments(m model, msgs []llm.Message) string {
	if len(msgs) == 0 || m.workspaceRoot == "" || files.IsHomeDir(m.workspaceRoot) {
		return ""
	}

	lastIdx := len(msgs) - 1
	lastMsg := msgs[lastIdx]
	if lastMsg.Role != "user" {
		return ""
	}

	matches := atFileRefRe.FindAllStringSubmatch(lastMsg.Content, -1)
	if len(matches) == 0 {
		return ""
	}

	var attachments strings.Builder
	for _, match := range matches {
		relPath := match[1]
		content, err := files.ReadFileContents(m.workspaceRoot, relPath)
		if err != nil || content == "" {
			continue
		}
		ext := filepath.Ext(relPath)
		lang := strings.TrimPrefix(ext, ".")
		attachments.WriteString(fmt.Sprintf("Contents of %s:\n```%s\n%s\n```\n\n", relPath, lang, content))
	}

	return attachments.String()
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

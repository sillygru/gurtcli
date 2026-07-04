package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sillygru/gurtcli/config"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/sessions"
	"github.com/sillygru/gurtcli/ui"
)

const maxToolCallCycles = 25

var globalProgram *tea.Program

type state int

const (
	stateWelcome state = iota
	stateProviderPick
	stateCustomModePick
	stateCustomURL
	stateAPIKeyInput
	stateCustomName
	stateModelFetch
	stateModelPick
	stateReasoningConfig
	stateError
	stateManualModel
	stateChat
	stateSessionPick
)

const (
	customModeOneTime = iota + 1
	customModeSave
)

type errorAction int

const (
	errorRetry errorAction = iota
	errorChangeURL
	errorChangeKey
	errorManual
	errorQuit
)

func (m model) errorActions() []string {
	if m.provider == llm.ProviderCustom {
		return []string{"Retry", "Change URL & API key", "Change API key only", "Enter model manually", "Quit"}
	}
	return []string{"Retry", "Change API key", "Enter model manually", "Quit"}
}

type item struct {
	title, desc string
}

func (i item) FilterValue() string { return i.title }
func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }

type modelItem struct {
	info     llm.ModelInfo
	provider string
}

func (m modelItem) FilterValue() string { return m.info.ID + " " + m.info.DisplayName }
func (m modelItem) Title() string {
	if m.info.DisplayName != "" {
		return m.info.DisplayName
	}
	return m.info.ID
}
func (m modelItem) Description() string {
	var tags []string
	if m.info.HasThinkingSupport() {
		tags = append(tags, "reasoning")
		if m.info.HasAdjustableReasoning() {
			tags = append(tags, "adjustable")
		}
		if m.info.Capabilities.Thinking.Types.Adaptive.Supported {
			tags = append(tags, "adaptive")
		}
	}
	desc := m.info.ID
	if len(tags) > 0 {
		desc += " [" + strings.Join(tags, ", ") + "]"
	}
	return desc
}

type sessionItem struct {
	meta   sessions.Metadata
	active bool
}

func (s sessionItem) FilterValue() string { return s.meta.Name + " " + s.meta.ID }
func (s sessionItem) Title() string {
	prefix := "  "
	if s.active {
		prefix = "• "
	}
	return prefix + s.meta.Name
}
func (s sessionItem) Description() string {
	return fmt.Sprintf("%s • %d messages", s.meta.UpdatedAt.Format("Jan 2 15:04"), s.meta.MessageCount)
}

type sessionSaveErrorMsg struct {
	err error
}

type modelsFetchedMsg struct {
	models []llm.ModelInfo
	err    error
}

type llmDetailsLoadedMsg struct {
	details map[string]llm.ModelInfo
}

type chatStreamChunk struct {
	content string
}

type chatStreamReasoning struct {
	content string
}

type chatStreamDone struct {
	toolCalls []llm.ToolCall
}

type chatStreamError struct {
	err error
}

type chatStreamUsage struct {
	inputTokens  int
	outputTokens int
}

type reasoningState struct {
	content        *strings.Builder
	startTime      time.Time
	visible        bool
	active         bool
	duration       time.Duration
	defaultVisible bool
}

type pendingPerm struct {
	toolCall  llm.ToolCall
	remaining []llm.ToolCall
}

type streamState struct {
	cancel context.CancelFunc
}

type suggestionState struct {
	items    []slashCommand
	selected int
	active   bool
}

type model struct {
	state            state
	yolo             bool
	reconfigure      bool
	forceLocal       bool
	theme            ui.Theme
	width            int
	height           int
	workspaceRoot    string
	alwaysAllowPerms bool
	toolCallCycle    int
	pendingPerm      *pendingPerm

	provider  string
	modelName string
	customURL string
	apiKey    string
	models    []llm.ModelInfo
	err       error
	errChoice int

	thinkingType    string
	effortLevel     string
	reasoningField  int
	thinkingOptions []string
	effortOptions   []string

	forceKeyAfterURL    bool
	customMode          int
	customModeCursor    int
	savedEndpointName   string
	confirmDeleteEndpoint string

	providerList list.Model
	modelList    list.Model
	sessionList  list.Model
	urlInput     textinput.Model
	keyInput     textinput.Model
	nameInput    textinput.Model
	manualInput  textinput.Model
	spinner      spinner.Model

	messages         []llm.Message
	chatInput        textinput.Model
	chatViewport     viewport.Model
	isStreaming      bool
	streamingContent *strings.Builder
	reasoning        reasoningState
	streamState      *streamState
	cancelRequested  bool
	queuedMessage    string
	suggestions      suggestionState

	sessionID        string
	sessionName      string
	sessionCreatedAt time.Time
	needsTitle       bool

	maxInputTokens int
	inputTokens    int
	outputTokens   int

	updateAvailable    bool
	latestVersion      string
	updateCheckStarted bool

	llmDetails      map[string]llm.ModelInfo
	llmDetailsReady bool
}

func (m model) enterChatState() model {
	if m.sessionID == "" {
		m = m.initNewSession()
	}
	m.chatInput.Focus()
	m.reasoning = reasoningState{}
	m.streamingContent = nil
	if m.maxInputTokens == 0 {
		for _, mdl := range m.models {
			if mdl.ID == m.modelName || mdl.DisplayName == m.modelName {
				m.maxInputTokens = mdl.MaxInputTokens
				break
			}
		}
	}
	m.inputTokens = 0
	m.outputTokens = 0
	m.chatViewport.SetContent(buildChatContent(m))
	m.chatViewport.GotoBottom()
	m.state = stateChat
	return m
}

type providerPickKind int

const (
	pickOpenAI providerPickKind = iota
	pickAnthropic
	pickGemini
	pickSavedEndpoint
	pickCustom
)

type providerPickResult struct {
	kind             providerPickKind
	savedEndpointIdx int
}

func buildProviderItems(endpoints []config.SavedEndpoint) []list.Item {
	items := []list.Item{
		item{title: "OpenAI", desc: "GPT-5.5, GPT-5.4, GPT-5.4-mini, ..."},
		item{title: "Anthropic", desc: "fable 5, opus 4.8, sonnet 5, ..."},
		item{title: "Google Gemini", desc: "Gemini 3.5 Flash, Gemini 2.5 Pro, ..."},
	}
	for _, ep := range endpoints {
		items = append(items, item{title: ep.Name, desc: ep.BaseURL})
	}
	items = append(items, item{title: "Custom", desc: "Any OpenAI-compatible API endpoint"})
	return items
}

func resolveProviderPick(endpoints []config.SavedEndpoint, idx int) providerPickResult {
	if idx == 0 {
		return providerPickResult{kind: pickOpenAI}
	}
	if idx == 1 {
		return providerPickResult{kind: pickAnthropic}
	}
	if idx == 2 {
		return providerPickResult{kind: pickGemini}
	}
	savedCount := len(endpoints)
	if idx >= 3 && idx < 3+savedCount {
		return providerPickResult{kind: pickSavedEndpoint, savedEndpointIdx: idx - 3}
	}
	return providerPickResult{kind: pickCustom}
}

type slashCommand struct {
	name        string
	description string
}

var slashCommands = []slashCommand{
	{name: "auth", description: "Change API key for current provider"},
	{name: "exit", description: "Quit the application"},
	{name: "help", description: "Show available commands"},
	{name: "model", description: "Change model for current provider"},
	{name: "new", description: "Start a new session"},
	{name: "provider", description: "Change provider and model"},
	{name: "reasoning", description: "Toggle reasoning display"},
	{name: "session", description: "Switch to a saved session"},
	{name: "thinking", description: "Set thinking type (adaptive/enabled/disabled)"},
	{name: "effort", description: "Set effort level (low/medium/high/xhigh/max)"},
	{name: "update", description: "Update to the latest version"},
}

func (m model) isMidSession() bool {
	return len(m.messages) > 0
}

func (m model) smallModelForProvider() string {
	switch m.provider {
	case llm.ProviderOpenAI:
		return "gpt-5.4-nano"
	case llm.ProviderAnthropic:
		return "claude-haiku-4-5"
	case llm.ProviderGemini:
		return "gemini-2.5-flash-lite"
	default:
		return m.modelName
	}
}

func initialModel(yolo bool, providerArg, modelArg string, reconfigure bool, forceLocal bool) model {
	cleanOldBinary()
	s := ui.DefaultTheme()

	cfg, _ := config.Load()

	savedEndpoints := []config.SavedEndpoint{}
	if cfg != nil {
		savedEndpoints = cfg.SavedEndpoints
	}

	providerItems := buildProviderItems(savedEndpoints)

	pd := list.NewDefaultDelegate()
	pd.ShowDescription = true
	pd.Styles.SelectedTitle = pd.Styles.SelectedTitle.
		Foreground(lipgloss.Color(ui.ColorMauve)).
		Background(lipgloss.Color(ui.ColorSurface0)).
		Bold(true)
	pd.Styles.SelectedDesc = pd.Styles.SelectedDesc.
		Foreground(lipgloss.Color(ui.ColorOverlay2))
	pd.Styles.NormalTitle = pd.Styles.NormalTitle.
		Foreground(lipgloss.Color(ui.ColorText))
	pd.Styles.NormalDesc = pd.Styles.NormalDesc.
		Foreground(lipgloss.Color(ui.ColorOverlay1))
	pl := list.New(providerItems, pd, 0, 0)
	pl.Title = "Pick a provider"
	pl.SetShowHelp(false)
	pl.SetShowStatusBar(false)
	pl.SetFilteringEnabled(false)
	pl.DisableQuitKeybindings()

	md := list.NewDefaultDelegate()
	md.ShowDescription = true
	md.Styles.SelectedTitle = md.Styles.SelectedTitle.
		Foreground(lipgloss.Color(ui.ColorMauve)).
		Background(lipgloss.Color(ui.ColorSurface0)).
		Bold(true)
	md.Styles.NormalTitle = md.Styles.NormalTitle.
		Foreground(lipgloss.Color(ui.ColorText))
	ml := list.New(nil, md, 0, 0)
	ml.Title = "Pick a model"
	ml.SetShowHelp(false)
	ml.SetShowStatusBar(false)
	ml.DisableQuitKeybindings()

	sd := list.NewDefaultDelegate()
	sd.ShowDescription = true
	sd.Styles.SelectedTitle = sd.Styles.SelectedTitle.
		Foreground(lipgloss.Color(ui.ColorMauve)).
		Background(lipgloss.Color(ui.ColorSurface0)).
		Bold(true)
	sd.Styles.SelectedDesc = sd.Styles.SelectedDesc.
		Foreground(lipgloss.Color(ui.ColorOverlay2))
	sd.Styles.NormalTitle = sd.Styles.NormalTitle.
		Foreground(lipgloss.Color(ui.ColorText))
	sd.Styles.NormalDesc = sd.Styles.NormalDesc.
		Foreground(lipgloss.Color(ui.ColorOverlay1))
	sl := list.New(nil, sd, 0, 0)
	sl.Title = "Sessions"
	sl.SetShowHelp(false)
	sl.SetShowStatusBar(false)
	sl.DisableQuitKeybindings()

	urlIn := textinput.New()
	urlIn.Placeholder = "https://api.example.com/v1"
	urlIn.Width = 60
	urlIn.CharLimit = 200

	ki := textinput.New()
	ki.Placeholder = "sk-..."
	ki.Width = 60
	ki.CharLimit = 200
	ki.EchoMode = textinput.EchoPassword
	ki.EchoCharacter = '•'

	ni := textinput.New()
	ni.Placeholder = "e.g. My Groq API"
	ni.Width = 60
	ni.CharLimit = 100

	mi := textinput.New()
	mi.Placeholder = "model-name"
	mi.Width = 60
	mi.CharLimit = 100

	ci := textinput.New()
	ci.Placeholder = "Ask something..."
	ci.Width = 60
	ci.CharLimit = 4096

	cv := viewport.New(0, 0)

	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ui.ColorMauve))
	sp.Spinner = spinner.Dot

	provider := providerArg
	modelName := modelArg
	customURL := ""
	apiKey := ""
	savedEndpointName := ""

	if cfg != nil && !reconfigure {
		if provider == "" {
			provider = cfg.Provider
			customURL = cfg.CustomBaseURL
			savedEndpointName = cfg.SavedEndpointName
		}
		if modelName == "" {
			modelName = cfg.Model
		}
	}

	// If provider arg matches a saved endpoint name, load its URL
	if savedEndpointName == "" && provider != "" && (provider != llm.ProviderOpenAI && provider != llm.ProviderAnthropic && provider != llm.ProviderGemini && provider != llm.ProviderCustom) {
		if cfg != nil {
			if ep, ok := cfg.GetSavedEndpoint(provider); ok {
				provider = llm.ProviderCustom
				customURL = ep.BaseURL
				savedEndpointName = ep.Name
			}
		}
	}

	var startState state
	if reconfigure {
		if provider != "" {
			key, _ := config.GetAPIKey(provider, customURL, savedEndpointName)
			apiKey = key
		}
		startState = stateWelcome
	} else if provider == "" {
		startState = stateWelcome
	} else {
		key, _ := config.GetAPIKey(provider, customURL, savedEndpointName)
		apiKey = key
		if apiKey == "" {
			startState = stateAPIKeyInput
		} else if modelName == "" {
			startState = stateModelFetch
		} else {
			startState = stateChat
		}
	}

	wd, _ := os.Getwd()

	m := model{
		state:              startState,
		yolo:               yolo,
		forceLocal:         forceLocal,
		reconfigure:        reconfigure,
		theme:              s,
		provider:           provider,
		modelName:          modelName,
		customURL:          customURL,
		savedEndpointName:  savedEndpointName,
		apiKey:             apiKey,
		workspaceRoot:      wd,
		providerList:       pl,
		modelList:          ml,
		sessionList:        sl,
		urlInput:           urlIn,
		keyInput:           ki,
		nameInput:          ni,
		manualInput:        mi,
		spinner:            sp,
		messages:           []llm.Message{},
		chatInput:          ci,
		chatViewport:       cv,
		streamState:        &streamState{},
	}

	if cfg != nil && !reconfigure {
		m.reasoning.defaultVisible = cfg.ReasoningVisible
		m.reasoning.visible = cfg.ReasoningVisible
		m.thinkingType = cfg.ThinkingType
		m.effortLevel = cfg.EffortLevel
	}

	if startState == stateChat {
		if m.maxInputTokens == 0 && m.modelName != "" {
			m.maxInputTokens = llm.LookupModelMaxTokens(m.modelName)
		}
		m = m.enterChatState()
	}

	return m
}

func (m model) initNewSession() model {
	m.sessionID = sessions.GenerateID()
	m.sessionCreatedAt = time.Now()
	m.sessionName = ""
	m.needsTitle = true
	return m
}

func (m model) toSession() *sessions.Session {
	msgs := make([]llm.Message, len(m.messages))
	copy(msgs, m.messages)
	return &sessions.Session{
		ID:                m.sessionID,
		Name:              m.sessionName,
		CreatedAt:         m.sessionCreatedAt,
		Provider:          m.provider,
		Model:             m.modelName,
		CustomURL:         m.customURL,
		SavedEndpointName: m.savedEndpointName,
		ThinkingType:      m.thinkingType,
		EffortLevel:       m.effortLevel,
		ReasoningVisible:  m.reasoning.defaultVisible,
		WorkspaceRoot:     m.workspaceRoot,
		Messages:          msgs,
	}
}

func (m model) applySession(s *sessions.Session) model {
	m.sessionID = s.ID
	m.sessionName = s.Name
	m.sessionCreatedAt = s.CreatedAt
	m.messages = append([]llm.Message(nil), s.Messages...)
	m.provider = s.Provider
	m.modelName = s.Model
	if m.maxInputTokens == 0 && m.modelName != "" {
		m.maxInputTokens = llm.LookupModelMaxTokens(m.modelName)
	}
	m.customURL = s.CustomURL
	m.savedEndpointName = s.SavedEndpointName
	m.thinkingType = s.ThinkingType
	m.effortLevel = s.EffortLevel
	m.reasoning.defaultVisible = s.ReasoningVisible
	m.reasoning.visible = s.ReasoningVisible
	m.toolCallCycle = 0
	m.pendingPerm = nil
	m.queuedMessage = ""
	m.isStreaming = false
	m.streamingContent = nil
	m.reasoning = reasoningState{defaultVisible: s.ReasoningVisible, visible: s.ReasoningVisible}
	return m
}

func (m model) resetToNewSession() model {
	m.messages = []llm.Message{}
	m.toolCallCycle = 0
	m.pendingPerm = nil
	m.queuedMessage = ""
	m.isStreaming = false
	m.streamingContent = nil
	m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible, visible: m.reasoning.defaultVisible}
	return m.initNewSession()
}

func (m model) persistSessionCmd() tea.Cmd {
	sess := m.toSession()
	if sess.Name == "" {
		sess.Name = sessions.NameForMessages(sess.Messages)
	}
	return func() tea.Msg {
		if err := sessions.Save(sess); err != nil {
			return sessionSaveErrorMsg{err: err}
		}
		if err := sessions.SetActiveSession(sess.WorkspaceRoot, sess.ID); err != nil {
			return sessionSaveErrorMsg{err: fmt.Errorf("setting active session: %w", err)}
		}
		return nil
	}
}

func (m model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, prefetchLLMDetailsCmd(m.forceLocal))
	if m.state == stateModelFetch && m.provider != "" {
		cmds = append(cmds, m.spinner.Tick, m.fetchModelsCmd())
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func prefetchLLMDetailsCmd(forceLocal bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		details, err := llm.FetchLLMDetails(ctx, forceLocal)
		if err != nil || len(details) == 0 {
			llm.LogDebug("prefetchLLMDetailsCmd: failed, will fetch later if needed: %v", err)
			return nil
		}

		llm.LogDebug("prefetchLLMDetailsCmd: loaded %d model details", len(details))
		return llmDetailsLoadedMsg{details: details}
	}
}

func (m model) fetchModelsCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		models, err := llm.FetchModels(ctx, m.provider, m.apiKey, m.customURL)
		if err != nil {
			return modelsFetchedMsg{err: err}
		}

		if m.llmDetailsReady && len(m.llmDetails) > 0 {
			models = llm.EnrichModels(models, m.llmDetails, m.provider)
			llm.LogDebug("fetchModelsCmd: enriched from prefetched, first=%q display=%q",
				models[0].ID, models[0].DisplayName)
			return modelsFetchedMsg{models: models}
		}

		llm.LogDebug("fetchModelsCmd: prefetched not ready, fetching inline (%d models)", len(models))
		details, err := llm.FetchLLMDetails(ctx, m.forceLocal)
		if err == nil && len(details) > 0 {
			models = llm.EnrichModels(models, details, m.provider)
		} else {
			llm.LogDebug("fetchModelsCmd: no llmdetails to enrich (%d models)", len(models))
		}

		return modelsFetchedMsg{models: models}
	}
}

package main

import (
	"context"
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
)

// Catppuccin Mocha color palette (purple-emphasized)
const (
	cpMauve    = "#cba6f7"
	cpLavender = "#b4befe"
	cpPink     = "#f5c2e7"
	cpSubtext1 = "#bac2de"
	cpSubtext0 = "#a6adc8"
	cpOverlay2 = "#9399b2"
	cpOverlay1 = "#7f849c"
	cpOverlay0 = "#6c7086"
	cpSurface2 = "#585b70"
	cpSurface1 = "#45475a"
	cpSurface0 = "#313244"
	cpText     = "#cdd6f4"
	cpRed      = "#f38ba8"
	cpGreen    = "#a6e3a1"
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
)

const (
	customModeOneTime = iota + 1
	customModeSave
)

type styles struct {
	header           lipgloss.Style
	dim              lipgloss.Style
	err              lipgloss.Style
	reasoningToggle  lipgloss.Style
	reasoningContent lipgloss.Style
	divider          lipgloss.Style
	userLabel        lipgloss.Style
	inputPrompt      lipgloss.Style
	toolLabel        lipgloss.Style
	diffAdd          lipgloss.Style
	diffDel          lipgloss.Style
	permPrompt       lipgloss.Style
	permKey          lipgloss.Style
	statusBar        lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		header:           lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cpMauve)).Padding(0, 1),
		dim:              lipgloss.NewStyle().Foreground(lipgloss.Color(cpOverlay1)),
		err:              lipgloss.NewStyle().Foreground(lipgloss.Color(cpRed)).Bold(true),
		reasoningToggle:  lipgloss.NewStyle().Foreground(lipgloss.Color(cpSubtext1)),
		reasoningContent: lipgloss.NewStyle().Foreground(lipgloss.Color(cpOverlay0)).Padding(0, 2),
		divider:          lipgloss.NewStyle().Foreground(lipgloss.Color(cpSurface2)),
		userLabel:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cpLavender)),
		inputPrompt:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cpMauve)),
		toolLabel:        lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color(cpPink)),
		diffAdd:          lipgloss.NewStyle().Foreground(lipgloss.Color(cpGreen)),
		diffDel:          lipgloss.NewStyle().Foreground(lipgloss.Color(cpRed)),
		permPrompt:       lipgloss.NewStyle().Foreground(lipgloss.Color(cpText)),
		permKey:          lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cpMauve)),
		statusBar:        lipgloss.NewStyle().Foreground(lipgloss.Color(cpSubtext0)),
	}
}

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
	info llm.ModelInfo
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
	if m.info.Capabilities.Thinking.Supported {
		if m.info.Capabilities.Thinking.Types.Adaptive.Supported {
			tags = append(tags, "adaptive")
		}
		if m.info.Capabilities.Thinking.Types.Enabled.Supported {
			tags = append(tags, "thinking")
		}
	}
	if m.info.Capabilities.Effort.Supported {
		tags = append(tags, "effort")
	}
	desc := m.info.ID
	if len(tags) > 0 {
		desc += " [" + strings.Join(tags, ", ") + "]"
	}
	return desc
}

type modelsFetchedMsg struct {
	models []llm.ModelInfo
	err    error
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
	styles           styles
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
}

func (m model) enterChatState() model {
	m.chatInput.Focus()
	m.reasoning = reasoningState{}
	m.streamingContent = nil
	m.chatViewport.SetContent(buildChatContent(m))
	m.chatViewport.GotoBottom()
	m.state = stateChat
	return m
}

type providerPickKind int

const (
	pickOpenAI providerPickKind = iota
	pickAnthropic
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
	savedCount := len(endpoints)
	if idx >= 2 && idx < 2+savedCount {
		return providerPickResult{kind: pickSavedEndpoint, savedEndpointIdx: idx - 2}
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
	{name: "provider", description: "Change provider and model"},
	{name: "reasoning", description: "Toggle reasoning display"},
	{name: "thinking", description: "Set thinking type (adaptive/enabled/disabled)"},
	{name: "effort", description: "Set effort level (low/medium/high/xhigh/max)"},
}

func (m model) isMidSession() bool {
	return len(m.messages) > 0
}

func initialModel(yolo bool, providerArg, modelArg string, reconfigure bool) model {
	s := defaultStyles()

	cfg, _ := config.Load()

	savedEndpoints := []config.SavedEndpoint{}
	if cfg != nil {
		savedEndpoints = cfg.SavedEndpoints
	}

	providerItems := buildProviderItems(savedEndpoints)

	pd := list.NewDefaultDelegate()
	pd.ShowDescription = true
	pd.Styles.SelectedTitle = pd.Styles.SelectedTitle.
		Foreground(lipgloss.Color(cpMauve)).
		Background(lipgloss.Color(cpSurface0)).
		Bold(true)
	pd.Styles.SelectedDesc = pd.Styles.SelectedDesc.
		Foreground(lipgloss.Color(cpOverlay2))
	pd.Styles.NormalTitle = pd.Styles.NormalTitle.
		Foreground(lipgloss.Color(cpText))
	pd.Styles.NormalDesc = pd.Styles.NormalDesc.
		Foreground(lipgloss.Color(cpOverlay1))
	pl := list.New(providerItems, pd, 0, 0)
	pl.Title = "Pick a provider"
	pl.SetShowHelp(false)
	pl.SetShowStatusBar(false)
	pl.SetFilteringEnabled(false)
	pl.DisableQuitKeybindings()

	md := list.NewDefaultDelegate()
	md.ShowDescription = true
	md.Styles.SelectedTitle = md.Styles.SelectedTitle.
		Foreground(lipgloss.Color(cpMauve)).
		Background(lipgloss.Color(cpSurface0)).
		Bold(true)
	md.Styles.NormalTitle = md.Styles.NormalTitle.
		Foreground(lipgloss.Color(cpText))
	ml := list.New(nil, md, 0, 0)
	ml.Title = "Pick a model"
	ml.SetShowHelp(false)
	ml.SetShowStatusBar(false)
	ml.DisableQuitKeybindings()

	ui := textinput.New()
	ui.Placeholder = "https://api.example.com/v1"
	ui.Width = 60
	ui.CharLimit = 200

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
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(cpMauve))
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
	if savedEndpointName == "" && provider != "" && (provider != llm.ProviderOpenAI && provider != llm.ProviderAnthropic && provider != llm.ProviderCustom) {
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
		reconfigure:        reconfigure,
		styles:             s,
		provider:           provider,
		modelName:          modelName,
		customURL:          customURL,
		savedEndpointName:  savedEndpointName,
		apiKey:             apiKey,
		workspaceRoot:      wd,
		providerList:       pl,
		modelList:          ml,
		urlInput:           ui,
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
		m = m.enterChatState()
	}

	return m
}

func (m model) Init() tea.Cmd {
	if m.state == stateModelFetch && m.provider != "" {
		return tea.Batch(
			m.spinner.Tick,
			fetchModelsCmd(m.provider, m.apiKey, m.customURL),
		)
	}
	return nil
}

func fetchModelsCmd(provider, apiKey, baseURL string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := llm.FetchModels(ctx, provider, apiKey, baseURL)
		if err != nil {
			return modelsFetchedMsg{err: err}
		}
		return modelsFetchedMsg{models: models}
	}
}

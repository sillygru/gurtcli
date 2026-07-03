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
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sillygru/gurtcli/config"
	"github.com/sillygru/gurtcli/llm"
)

const maxToolCallCycles = 25

var globalProgram *tea.Program

type state int

const (
	stateWelcome state = iota
	stateProviderPick
	stateCustomURL
	stateAPIKeyInput
	stateModelFetch
	stateModelPick
	stateError
	stateManualModel
	stateChat
)

type styles struct {
	header           lipgloss.Style
	dim              lipgloss.Style
	err              lipgloss.Style
	reasoningToggle  lipgloss.Style
	reasoningContent lipgloss.Style
}

func defaultStyles() styles {
	return styles{
		header:           lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Padding(0, 1),
		dim:              lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		err:              lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
		reasoningToggle:  lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
		reasoningContent: lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Padding(0, 2),
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
		return []string{"Retry", "Change custom URL", "Change API key", "Enter model manually", "Quit"}
	}
	return []string{"Retry", "Change API key", "Enter model manually", "Quit"}
}

type item struct {
	title, desc string
}

func (i item) FilterValue() string { return i.title }
func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }

type modelsFetchedMsg struct {
	models []string
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
	state           state
	yolo            bool
	reconfigure     bool
	styles          styles
	width           int
	height          int
	workspaceRoot   string
	alwaysAllowPerms bool
	toolCallCycle   int
	pendingPerm     *pendingPerm

	provider  string
	modelName string
	customURL string
	apiKey    string
	models    []string
	err       error
	errChoice int

	providerList list.Model
	modelList    list.Model
	urlInput     textinput.Model
	keyInput     textinput.Model
	manualInput  textinput.Model
	spinner      spinner.Model

	messages        []llm.Message
	chatInput       textinput.Model
	chatViewport    viewport.Model
	isStreaming     bool
	streamingContent *strings.Builder
	reasoning       reasoningState
	streamState     *streamState
	suggestions     suggestionState
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

var providerItems = []list.Item{
	item{title: "OpenAI", desc: "GPT-5.5, GPT-5.4, GPT-5.4-mini, ..."},
	item{title: "Anthropic", desc: "fable 5, opus 4.8, sonnet 5, ..."},
	item{title: "Custom (OpenAI-compatible)", desc: "Any OpenAI-compatible API endpoint"},
}

func providerFromIndex(idx int) string {
	switch idx {
	case 0:
		return llm.ProviderOpenAI
	case 1:
		return llm.ProviderAnthropic
	case 2:
		return llm.ProviderCustom
	default:
		return ""
	}
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
}

func (m model) isMidSession() bool {
	return len(m.messages) > 0
}

func initialModel(yolo bool, providerArg, modelArg string, reconfigure bool) model {
	s := defaultStyles()

	pd := list.NewDefaultDelegate()
	pd.ShowDescription = true
	pl := list.New(providerItems, pd, 0, 0)
	pl.Title = "Pick a provider"
	pl.SetShowHelp(false)
	pl.SetShowStatusBar(false)
	pl.SetFilteringEnabled(false)
	pl.DisableQuitKeybindings()

	md := list.NewDefaultDelegate()
	md.ShowDescription = false
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
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	sp.Spinner = spinner.Dot

	cfg, _ := config.Load()

	provider := providerArg
	modelName := modelArg
	customURL := ""
	apiKey := ""

	if cfg != nil && !reconfigure {
		if provider == "" {
			provider = cfg.Provider
			customURL = cfg.CustomBaseURL
		}
		if modelName == "" {
			modelName = cfg.Model
		}
	}

	var startState state
	if reconfigure {
		if provider != "" {
			key, _ := config.GetAPIKey(provider, customURL)
			apiKey = key
		}
		startState = stateWelcome
	} else if provider == "" {
		startState = stateWelcome
	} else {
		key, _ := config.GetAPIKey(provider, customURL)
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
		state:        startState,
		yolo:         yolo,
		reconfigure:  reconfigure,
		styles:       s,
		provider:     provider,
		modelName:    modelName,
		customURL:    customURL,
		apiKey:       apiKey,
		workspaceRoot: wd,
		providerList: pl,
		modelList:    ml,
		urlInput:     ui,
		keyInput:     ki,
		manualInput:  mi,
		spinner:      sp,
		messages:     []llm.Message{},
		chatInput:    ci,
		chatViewport: cv,
		streamState:  &streamState{},
	}

	if cfg != nil && !reconfigure {
		m.reasoning.defaultVisible = cfg.ReasoningVisible
		m.reasoning.visible = cfg.ReasoningVisible
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

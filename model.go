package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sillygru/gurtcli/config"
	"github.com/sillygru/gurtcli/history"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/sessions"
	"github.com/sillygru/gurtcli/telemetry"
	"github.com/sillygru/gurtcli/tools"
	"github.com/sillygru/gurtcli/ui"
)

const maxToolCallCycles = 25
const maxSessionMessages = 2000

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
	stateAllowManage
	stateDotenvPrompt
	stateDotenvPick
	stateDotenvKeyName
	stateDotenvKeyExists
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
	meta sessions.Metadata
}

func (s sessionItem) FilterValue() string { return s.meta.Name + " " + s.meta.ID }
func (s sessionItem) Title() string {
	return "  " + s.meta.Name
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

// retryFireMsg is delivered when a retry's wait elapses. The token must match
// model.retry.token or the wait was cancelled or re-armed in the meantime.
type retryFireMsg struct {
	token int
}

type chatStreamUsage struct {
	inputTokens       int
	outputTokens      int
	reasoningTokens   int
	cacheHitTokens    int
	cacheWriteTokens  int
	promptTotalTokens int
}

type resourceStatsMsg struct {
	cpuPercent float64
	memMB      float64
}

type workingTickMsg struct{}

type walkFilesDoneMsg struct {
	files []string
}

type versionCheckResult struct {
	latestVersion string
	needsUpdate   bool
	err           error
}

type toastMsg struct {
	text string
	id   int
}

type toastTimeoutMsg struct {
	id int
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
	toolCall     llm.ToolCall
	remaining    []llm.ToolCall
	externalPath string // set when the prompt is about an external path
	sudo         bool   // true when the command starts with sudo and needs password
	sudoPassword string // populated after user enters password; cleared after use
	confirmSudo  bool   // true after user has confirmed the sudo prompt (entering password phase)
}

type streamState struct {
	cancel context.CancelFunc
}

// retryState tracks an in-progress wait between a failed chat request and the
// attempt that repeats it. It is zeroed on success, on cancellation, and once
// the attempts are exhausted.
type retryState struct {
	active  bool
	attempt int       // 1-based, counts up to maxRetryAttempts
	until   time.Time // when the retry fires; drives the countdown
	delay   time.Duration
	err     error
	// token invalidates a scheduled retryFireMsg after the user cancels or
	// re-arms the wait, so a stale tick can't resurrect a dead request.
	token int
	// needsOK is set when the provider asked for a wait longer than
	// longRetryWaitThreshold; the retry only fires after the user confirms.
	needsOK bool
	// rateLimit distinguishes a usage-limit rejection from a generic failure.
	rateLimit bool
}

type toolExecState struct {
	cancel   context.CancelFunc
	active   bool
	toolName string
	title    string
	label    string
}

// textSelection is a stream selection over the transcript. All X coordinates
// are terminal *cells*, not runes or bytes: a wide glyph covers two columns on
// screen, so anything counted in runes drifts away from the mouse as soon as a
// line holds CJK text or an emoji. anchor is the cell the drag started on and
// is always part of the selection; focus is the cell under the cursor now and
// is included too, which is what every terminal does and what users expect.
type textSelection struct {
	anchorY int  // Content line where the drag started
	anchorX int  // Cell in that line
	focusY  int  // Content line under the cursor
	focusX  int  // Cell under the cursor
	active  bool // User is currently dragging
	exists  bool // Selection finalized (mouse released) and still shown
}

// clickTracker turns a stream of clicks into single/double/triple gestures.
type clickTracker struct {
	x, y  int
	count int
	at    time.Time
}

type suggestionItem struct {
	name        string
	description string
}

type suggestionState struct {
	items    []suggestionItem
	selected int
	active   bool
	isFiles  bool
}

type resourceStats struct {
	cpuPercent float64
	memMB      float64
}

type model struct {
	state                       state
	yolo                        bool
	reconfigure                 bool
	forceLocal                  bool
	debug                       bool
	debugStats                  resourceStats
	theme                       ui.Theme
	themeName                   string
	width                       int
	height                      int
	workspaceRoot               string
	cwdDisplay                  string
	alwaysAllowPerms            bool
	allowEdits                  bool
	allowDeletions              bool
	allowedBashPrefixes         map[string]bool
	allowedBashPrefixesSession  map[string]bool
	alwaysAllowTools            []string
	alwaysAllowCommandPrefixes  []string
	allowedExternalPathsSession map[string]bool
	allowAllExternal            bool
	alwaysAllowExternal         bool
	allowManageCursor           int
	allowManageScroll           int
	allowManageInput            textinput.Model
	allowManageAdding           bool
	allowManageAddType          string
	allowToolCheckItems         []string
	allowToolCheckCursor        int
	toolCallCycle               int
	pendingPerm                 *pendingPerm
	permCursor                  int
	permScroll                  int
	permScrollTotal             int
	sudoPasswordInput           textinput.Model

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

	forceKeyAfterURL       bool
	customMode             int
	customModeCursor       int
	dotenvCursor           int
	dotenvPickCursor       int
	dotenvKeyName          string
	dotenvExistingKeyValue string
	dotenvKeyExistsCursor  int
	dotenvKeys             []string
	savedEndpointName      string
	confirmDeleteEndpoint  string

	providerList list.Model
	providerDel  *list.DefaultDelegate
	modelList    list.Model
	modelDel     *list.DefaultDelegate
	sessionList  list.Model
	sessionDel   *list.DefaultDelegate
	urlInput     textinput.Model
	keyInput     textinput.Model
	nameInput    textinput.Model
	manualInput  textinput.Model
	dotenvInput  textinput.Model
	spinner      spinner.Model

	messages             []llm.Message
	chatInput            textarea.Model
	chatViewport         viewport.Model
	stableContent        string
	stableMsgCount       int
	msgRenders           []*string
	isStreaming          bool
	stickToBottom        bool
	streamingContent     *strings.Builder
	lastStreamRender     time.Time
	reasoning            reasoningState
	streamState          *streamState
	retry                retryState
	toolExec             *toolExecState
	toolQueue            []llm.ToolCall
	cancelRequested      bool
	queuedMessage        string
	selection            textSelection
	lastClick            clickTracker
	toast                *toastMsg
	toastSeq             int
	suggestions          suggestionState
	fileList             []string
	filesCached          bool
	lastDateMessage      string
	cachedSystemPrompt     string
	transcriptCacheContent string
	transcriptCacheUpTo    int
	transcriptCachedKey    string
	history                []string
	historyIndex         int
	historyDraft         string
	historyLoadedValue   string
	windowTitle          string
	showThemePicker      bool
	themePickerCursor    int
	themePickerOrigTheme ui.Theme
	themePickerOrigName  string

	sessionID        string
	sessionName      string
	sessionCreatedAt time.Time
	needsTitle       bool

	maxInputTokens        int
	inputTokens           int
	outputTokens          int
	reasoningOutputTokens int
	cacheHitTokens        int

	// Context-window state describes the *last* request only, not session
	// totals: how much history was actually sent to the model.
	contextInputTokens  int
	contextCacheTokens  int
	contextOutputTokens int
	workingMsg          string
	workingMsgIndex     int
	workingSpinnerIdx   int

	telemetryEnabled   bool
	updateAvailable    bool
	latestVersion      string
	updateCheckStarted bool

	sessionOutputsDir string

	llmDetails      map[string]llm.ModelInfo
	llmDetailsReady bool

	bufferedInitCmd tea.Cmd
}

func (m model) cmdGridDimensions() (numRows, numCols, colWidth int) {
	cmds := m.alwaysAllowCommandPrefixes
	if len(cmds) == 0 {
		return 0, 0, 0
	}

	layout := ui.NewLayout(m.width, m.height)
	availableWidth := layout.ContentWidth() - 2
	if availableWidth < 8 {
		availableWidth = 8
	}

	// Reserve space: header (gurt + blank line) + footer (divider + help)
	availableHeight := m.height - 4
	if availableHeight < 1 {
		availableHeight = 1
	}

	maxItemLen := 0
	for _, c := range cmds {
		if len(c) > maxItemLen {
			maxItemLen = len(c)
		}
	}

	// Each cell: indicator(2) + command text + spacing(2)
	colWidth = maxItemLen + 4
	if colWidth < 14 {
		colWidth = 14
	}

	// How many columns fit horizontally
	numCols = availableWidth / colWidth
	if numCols < 1 {
		numCols = 1
	}

	// How many rows needed for all commands
	numRows = (len(cmds) + numCols - 1) / numCols

	// Cap rows to available terminal height; recalculate columns if needed
	if numRows > availableHeight {
		numRows = availableHeight
		numCols = (len(cmds) + numRows - 1) / numRows
		if numCols < 1 {
			numCols = 1
		}
	}

	return numRows, numCols, colWidth
}

func (m model) enterChatState() (model, tea.Cmd) {
	if m.sessionID == "" {
		m = m.initNewSession()
	}
	m.chatInput.Focus()
	m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible}
	m.streamingContent = nil
	if m.maxInputTokens == 0 {
		for _, mdl := range m.models {
			if mdl.ID == m.modelName || mdl.DisplayName == m.modelName {
				m.maxInputTokens = mdl.MaxInputTokens
				break
			}
		}
	}
	m = m.extendTranscriptCache()
	m.stableContent = buildChatContent(m)
	m.stableMsgCount = len(m.messages)
	m.chatViewport.SetContent(m.stableContent)
	m.chatViewport.GotoBottom()
	m.state = stateChat
	var cmds []tea.Cmd
	if !m.updateCheckStarted {
		m.updateCheckStarted = true
		cmds = append(cmds, checkForUpdateCmd())
	}
	if m.debug {
		cmds = append(cmds, resourceMonitorTickCmd())
	}
	return m, tea.Batch(cmds...)
}

func (m model) continueAfterAPIKey() (tea.Model, tea.Cmd) {
	if m.dotenvKeyName != "" {
		cfg, _ := config.Load()
		if cfg == nil {
			cfg = &config.Config{}
		}
		cfg.DotenvKeyName = m.dotenvKeyName
		config.Save(cfg)
	}

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

		m.providerList.SetItems(buildProviderItems(cfg.SavedEndpoints))
		m.customMode = 0

		if m.modelName != "" {
			return m.enterChatState()
		}

		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			m.fetchModelsCmd(),
		)
	}

	if m.modelName != "" {
		return m.enterChatState()
	}

	m.state = stateModelFetch
	return m, tea.Batch(
		m.spinner.Tick,
		m.fetchModelsCmd(),
	)
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
	{name: "show-reasoning", description: "Toggle reasoning visibility"},
	{name: "session", description: "Switch to a saved session"},
	{name: "reasoning", description: "Set thinking type or reasoning effort"},
	{name: "effort", description: "Set effort level (low/medium/high/xhigh/max)"},
	{name: "update", description: "Update to the latest version"},
	{name: "allow", description: "Manage always-allowed tools and commands"},
	{name: "theme", description: "Change the color theme"},
	{name: "telemetry", description: "Toggle anonymous usage telemetry"},
	{name: "version", description: "Show current version and check for updates"},
}

// commandNames returns the names of all registered slash commands for highlighting.
func commandNames() []string {
	names := make([]string, len(slashCommands))
	for i, sc := range slashCommands {
		names[i] = sc.name
	}
	return names
}

func (m model) modelDisplayName() string {
	info := m.currentModelInfo()
	if info.DisplayName != "" {
		return info.DisplayName
	}
	return m.modelName
}

func (m model) displayNameForModel(modelID string) string {
	if modelID == "" {
		return m.modelDisplayName()
	}
	if m.llmDetails != nil {
		if info, ok := m.llmDetails[modelID]; ok && info.DisplayName != "" {
			return info.DisplayName
		}
	}
	return modelID
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

func initialModel(yolo bool, providerArg, modelArg string, reconfigure bool, forceLocal bool, debug bool) model {
	cleanOldBinary()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: config corrupt, using defaults: %v\n", err)
		cfg = nil
	}

	themeName := "Catppuccin Mocha"
	if cfg != nil && cfg.Theme != "" {
		themeName = cfg.Theme
	}
	s := ui.ThemeByName(themeName)

	savedEndpoints := []config.SavedEndpoint{}
	if cfg != nil {
		savedEndpoints = cfg.SavedEndpoints
	}

	providerItems := buildProviderItems(savedEndpoints)

	pd := list.NewDefaultDelegate()
	pd.ShowDescription = true
	pd.Styles.SelectedTitle = pd.Styles.SelectedTitle.
		Foreground(lipgloss.Color(s.Mauve)).
		Background(lipgloss.Color(s.Surface0)).
		Bold(true)
	pd.Styles.SelectedDesc = pd.Styles.SelectedDesc.
		Foreground(lipgloss.Color(s.Overlay2)).
		Background(lipgloss.Color(s.Surface0))
	pd.Styles.NormalTitle = pd.Styles.NormalTitle.
		Foreground(lipgloss.Color(s.Text)).
		Background(lipgloss.Color(s.Base))
	pd.Styles.NormalDesc = pd.Styles.NormalDesc.
		Foreground(lipgloss.Color(s.Overlay1)).
		Background(lipgloss.Color(s.Base))
	pl := list.New(providerItems, pd, 0, 0)
	pl.Title = "Pick a provider"
	pl.SetShowHelp(false)
	pl.SetShowStatusBar(false)
	pl.SetFilteringEnabled(false)
	pl.DisableQuitKeybindings()

	md := list.NewDefaultDelegate()
	md.ShowDescription = true
	md.Styles.SelectedTitle = md.Styles.SelectedTitle.
		Foreground(lipgloss.Color(s.Mauve)).
		Background(lipgloss.Color(s.Surface0)).
		Bold(true)
	md.Styles.SelectedDesc = md.Styles.SelectedDesc.
		Foreground(lipgloss.Color(s.Overlay2)).
		Background(lipgloss.Color(s.Surface0))
	md.Styles.NormalTitle = md.Styles.NormalTitle.
		Foreground(lipgloss.Color(s.Text)).
		Background(lipgloss.Color(s.Base))
	md.Styles.NormalDesc = md.Styles.NormalDesc.
		Foreground(lipgloss.Color(s.Overlay1)).
		Background(lipgloss.Color(s.Base))
	ml := list.New(nil, md, 0, 0)
	ml.Title = "Pick a model"
	ml.SetShowHelp(false)
	ml.SetShowStatusBar(false)
	ml.DisableQuitKeybindings()

	sd := list.NewDefaultDelegate()
	sd.ShowDescription = true
	sd.Styles.SelectedTitle = sd.Styles.SelectedTitle.
		Foreground(lipgloss.Color(s.Mauve)).
		Background(lipgloss.Color(s.Surface0)).
		Bold(true)
	sd.Styles.SelectedDesc = sd.Styles.SelectedDesc.
		Foreground(lipgloss.Color(s.Overlay2)).
		Background(lipgloss.Color(s.Surface0))
	sd.Styles.NormalTitle = sd.Styles.NormalTitle.
		Foreground(lipgloss.Color(s.Text)).
		Background(lipgloss.Color(s.Base))
	sd.Styles.NormalDesc = sd.Styles.NormalDesc.
		Foreground(lipgloss.Color(s.Overlay1)).
		Background(lipgloss.Color(s.Base))
	sl := list.New(nil, sd, 0, 0)
	sl.Title = "Sessions"
	sl.SetShowHelp(false)
	sl.SetShowStatusBar(false)
	sl.DisableQuitKeybindings()

	urlIn := textinput.New()
	urlIn.Placeholder = "https://api.example.com/v1"
	urlIn.SetWidth(60)
	urlIn.CharLimit = 200

	ki := textinput.New()
	ki.Placeholder = "sk-..."
	ki.SetWidth(60)
	ki.CharLimit = 200
	ki.EchoMode = textinput.EchoPassword
	ki.EchoCharacter = '•'

	ni := textinput.New()
	ni.Placeholder = "e.g. My Groq API"
	ni.SetWidth(60)
	ni.CharLimit = 100

	mi := textinput.New()
	mi.Placeholder = "model-name"
	mi.SetWidth(60)
	mi.CharLimit = 100

	di := textinput.New()
	di.Placeholder = "GURT_API_KEY"
	di.SetWidth(60)
	di.CharLimit = 200

	ci := textarea.New()
	ci.Placeholder = "Ask something..."
	ci.SetWidth(60)
	ci.CharLimit = 4096
	ci.ShowLineNumbers = false
	ci.Prompt = ""
	ci.DynamicHeight = true
	ci.MinHeight = 1
	ci.MaxHeight = 6
	ci.MaxContentHeight = 1000
	ci.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("shift+enter", "ctrl+j"))
	taStyles := textarea.DefaultStyles(true)
	taStyles.Focused.CursorLine = lipgloss.NewStyle()
	taStyles.Blurred.CursorLine = lipgloss.NewStyle()
	ci.SetStyles(taStyles)

	cv := viewport.New()
	cv.FillHeight = true

	sp := spinner.New()
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(s.Mauve)).Background(lipgloss.Color(s.Base))
	sp.Spinner = spinner.Dot

	provider := providerArg
	modelName := modelArg
	customURL := ""
	apiKey := ""
	savedEndpointName := ""

	telemetryEnabled := true
	if cfg != nil && cfg.TelemetryEnabled != nil {
		telemetryEnabled = *cfg.TelemetryEnabled
	}

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
	var envKeys []string
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
			dk, _ := config.GetDotenvKeys()
			if len(dk) > 0 {
				startState = stateDotenvPick
				envKeys = make([]string, 0, len(dk))
				for k := range dk {
					envKeys = append(envKeys, k)
				}
				sort.Strings(envKeys)
			} else {
				startState = stateAPIKeyInput
			}
		} else if modelName == "" {
			startState = stateModelFetch
		} else {
			startState = stateChat
		}
	}

	wd, _ := os.Getwd()
	cwdDisplay := wd
	if homedir, err := os.UserHomeDir(); err == nil {
		cwdDisplay = strings.Replace(wd, homedir, "~", 1)
	}

	allowedBashPrefixes := make(map[string]bool)
	if cfg != nil {
		for _, p := range cfg.AllowedBashPrefixes {
			allowedBashPrefixes[p] = true
		}
	}

	alwaysAllowTools := []string{}
	alwaysAllowCommandPrefixes := []string{}
	if cfg == nil {
		alwaysAllowCommandPrefixes = tools.DefaultSafeBashPrefixes()
	} else {
		for _, t := range cfg.AlwaysAllowTools {
			if t != "read_file" {
				alwaysAllowTools = append(alwaysAllowTools, t)
			}
		}
		alwaysAllowCommandPrefixes = append(alwaysAllowCommandPrefixes, cfg.AlwaysAllowCommandPrefixes...)
	}

	alwaysAllowExternal := false
	if cfg != nil {
		alwaysAllowExternal = cfg.AlwaysAllowExternal
	}

	allowIn := textinput.New()
	allowIn.Placeholder = "command prefix (e.g. npm, git push)"
	allowIn.SetWidth(60)
	allowIn.CharLimit = 200

	sudoIn := textinput.New()
	sudoIn.Placeholder = "enter sudo password"
	sudoIn.EchoMode = textinput.EchoPassword
	sudoIn.EchoCharacter = '•'
	sudoIn.SetWidth(60)
	sudoIn.CharLimit = 200

	outputsDir := filepath.Join(wd, ".config", "gurtcli", "session-outputs")
	if hd, err := os.UserHomeDir(); err == nil {
		outputsDir = filepath.Join(hd, ".config", "gurtcli", "session-outputs")
	}

	m := model{
		state:                       startState,
		telemetryEnabled:            telemetryEnabled,
		yolo:                        yolo,
		forceLocal:                  forceLocal,
		reconfigure:                 reconfigure,
		debug:                       debug,
		forceKeyAfterURL:            false,
		theme:                       s,
		themeName:                   themeName,
		provider:                    provider,
		modelName:                   modelName,
		customURL:                   customURL,
		savedEndpointName:           savedEndpointName,
		apiKey:                      apiKey,
		dotenvKeys:                  envKeys,
		workspaceRoot:               wd,
		cwdDisplay:                  cwdDisplay,
		allowedBashPrefixes:         allowedBashPrefixes,
		allowedBashPrefixesSession:  make(map[string]bool),
		alwaysAllowTools:            alwaysAllowTools,
		alwaysAllowCommandPrefixes:  alwaysAllowCommandPrefixes,
		allowedExternalPathsSession: make(map[string]bool),
		alwaysAllowExternal:         alwaysAllowExternal,
		sessionOutputsDir:           outputsDir,
		allowManageCursor:           0,
		allowManageScroll:           0,
		allowManageInput:            allowIn,
		permScroll:                  0,
		permScrollTotal:             0,
		sudoPasswordInput:           sudoIn,
		providerList:                pl,
		providerDel:                 &pd,
		modelList:                   ml,
		modelDel:                    &md,
		sessionList:                 sl,
		sessionDel:                  &sd,
		urlInput:                    urlIn,
		keyInput:                    ki,
		nameInput:                   ni,
		manualInput:                 mi,
		dotenvInput:                 di,
		spinner:                     sp,
		messages:                    []llm.Message{},
		chatInput:                   ci,
		chatViewport:                cv,
		streamState:                 &streamState{},
		toolExec:                    &toolExecState{},
		windowTitle:                 "gurt",
	}

	h, _ := history.Load()
	m.history = h
	m.historyIndex = -1

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
		var chatCmd tea.Cmd
		m, chatCmd = m.enterChatState()
		m.bufferedInitCmd = chatCmd
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
	msgs := make([]llm.Message, 0, len(m.messages))
	for _, msg := range m.messages {
		if !msg.Internal {
			msgs = append(msgs, msg)
		}
	}
	return &sessions.Session{
		ID:                 m.sessionID,
		Name:               m.sessionName,
		CreatedAt:          m.sessionCreatedAt,
		Provider:           m.provider,
		Model:              m.modelName,
		CustomURL:          m.customURL,
		SavedEndpointName:  m.savedEndpointName,
		ThinkingType:       m.thinkingType,
		EffortLevel:        m.effortLevel,
		ReasoningVisible:   m.reasoning.defaultVisible,
		WorkspaceRoot:      m.workspaceRoot,
		Messages:           msgs,
		InputTokens:        m.inputTokens,
		OutputTokens:       m.outputTokens,
		ReasoningTokens:    m.reasoningOutputTokens,
		CacheHitTokens:     m.cacheHitTokens,
		ContextTokens:      m.contextInputTokens + m.contextOutputTokens,
		ContextCacheTokens: m.contextCacheTokens,
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
	m.permCursor = 0
	m.permScroll = 0
	m.queuedMessage = ""
	m.isStreaming = false
	m.streamingContent = nil
	m.reasoning = reasoningState{defaultVisible: s.ReasoningVisible, visible: s.ReasoningVisible}
	m.inputTokens = s.InputTokens
	m.outputTokens = s.OutputTokens
	m.reasoningOutputTokens = s.ReasoningTokens
	m.cacheHitTokens = s.CacheHitTokens
	// Sessions saved before context tracking existed report 0 here; the bar
	// stays hidden until the next request reports a real prompt size.
	m.contextInputTokens = s.ContextTokens
	m.contextCacheTokens = s.ContextCacheTokens
	m.contextOutputTokens = 0
	m = m.invalidateTranscriptCache()
	return m
}

func (m model) resetToNewSession() model {
	m.messages = []llm.Message{}
	m.toolCallCycle = 0
	m.pendingPerm = nil
	m.permCursor = 0
	m.permScroll = 0
	m.queuedMessage = ""
	m.isStreaming = false
	m.streamingContent = nil
	m.reasoning = reasoningState{defaultVisible: m.reasoning.defaultVisible, visible: m.reasoning.defaultVisible}
	m.inputTokens = 0
	m.contextInputTokens = 0
	m.contextCacheTokens = 0
	m.contextOutputTokens = 0
	m.outputTokens = 0
	m.reasoningOutputTokens = 0
	m.fileList = nil
	m.filesCached = false
	m.lastDateMessage = ""
	m.cacheHitTokens = 0
	m.cachedSystemPrompt = ""
	m = m.invalidateTranscriptCache()
	return m.initNewSession()
}

// applyThemeToLists restyles the list delegates, spinner, and viewport
// to match the current m.theme. Call this after changing the theme.
func (m *model) applyThemeToLists() {
	s := m.theme
	styleDelegate := func(d *list.DefaultDelegate) {
		d.ShowDescription = true
		d.Styles.SelectedTitle = d.Styles.SelectedTitle.
			Foreground(lipgloss.Color(s.Mauve)).
			Background(lipgloss.Color(s.Surface0)).
			Bold(true)
		d.Styles.SelectedDesc = d.Styles.SelectedDesc.
			Foreground(lipgloss.Color(s.Overlay2)).
			Background(lipgloss.Color(s.Surface0))
		d.Styles.NormalTitle = d.Styles.NormalTitle.
			Foreground(lipgloss.Color(s.Text)).
			Background(lipgloss.Color(s.Base))
		d.Styles.NormalDesc = d.Styles.NormalDesc.
			Foreground(lipgloss.Color(s.Overlay1)).
			Background(lipgloss.Color(s.Base))
	}

	styleDelegate(m.providerDel)
	styleDelegate(m.modelDel)
	styleDelegate(m.sessionDel)

	m.chatViewport.FillHeight = true
	m.spinner.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(s.Mauve)).Background(lipgloss.Color(s.Base))
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
		return nil
	}
}

func (m model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, prefetchLLMDetailsCmd(m.forceLocal), workingTickCmd())
	if m.telemetryEnabled {
		cmds = append(cmds, sendTelemetryCmd("startup"))
	}
	if m.state == stateModelFetch && m.provider != "" {
		cmds = append(cmds, m.spinner.Tick, m.fetchModelsCmd())
	}
	if m.bufferedInitCmd != nil {
		cmds = append(cmds, m.bufferedInitCmd)
	}
	return tea.Batch(cmds...)
}

func prefetchLLMDetailsCmd(forceLocal bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		details, err := llm.FetchLLMDetails(ctx, forceLocal)
		if err != nil || len(details) == 0 {
			return nil
		}

		return llmDetailsLoadedMsg{details: details}
	}
}

func sendTelemetryCmd(eventType string) tea.Cmd {
	return func() tea.Msg {
		cfgDir, err := config.Dir()
		if err != nil {
			return nil
		}
		id := telemetry.LoadOrCreateUUID(cfgDir)
		telemetry.SendEvent(id, Version, eventType, TelemetrySecret)
		return nil
	}
}

func toastTimeoutCmd(id int) tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return toastTimeoutMsg{id: id}
	})
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
			return modelsFetchedMsg{models: models}
		}

		details, err := llm.FetchLLMDetails(ctx, m.forceLocal)
		if err == nil && len(details) > 0 {
			models = llm.EnrichModels(models, details, m.provider)
		}

		return modelsFetchedMsg{models: models}
	}
}

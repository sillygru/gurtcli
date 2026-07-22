package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/sessions"
	"github.com/sillygru/gurtcli/tools"
	"github.com/sillygru/gurtcli/ui"
)

func (m model) View() tea.View {
	var content string
	switch m.state {
	case stateWelcome:
		content = m.welcomeView()
	case stateProviderPick:
		content = m.providerPickView()
	case stateCustomModePick:
		content = m.customModePickView()
	case stateCustomURL:
		content = m.customURLView()
	case stateAPIKeyInput:
		content = m.apiKeyView()
	case stateModelFetch:
		content = m.modelFetchView()
	case stateModelPick:
		content = m.modelPickView()
	case stateReasoningConfig:
		content = m.reasoningConfigView()
	case stateError:
		content = m.errorView()
	case stateManualModel:
		content = m.manualModelView()
	case stateCustomName:
		content = m.customNameView()
	case stateSessionPick:
		content = m.sessionPickView()
	case stateChat:
		content = m.chatView()
	case stateAllowManage:
		content = m.allowManageView()
	case stateDotenvPrompt:
		content = m.dotenvPromptView()
	case stateDotenvPick:
		content = m.dotenvPickView()
	case stateDotenvKeyName:
		content = m.dotenvKeyNameView()
	case stateDotenvKeyExists:
		content = m.dotenvKeyExistsView()
	}
	if content == "" {
		return tea.NewView("")
	}
	v := tea.NewView(ui.WrapScreen(content, m.width, m.height, m.theme.Base))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = m.windowTitle
	v.KeyboardEnhancements.ReportAlternateKeys = true
	return v
}

// The setup screens are plain prose and lists, and they are the first thing a
// new SSH user sees — often on a phone in portrait. None of them can assume the
// 60-ish columns their strings were written for, so all of them lay their text
// out through the three helpers below.

// wrapProse wraps a line of explanatory text to the terminal, breaking inside a
// word when a word is itself wider than the screen (long URLs, key names).
// These screens are short, so spending extra rows is free; truncating would
// hide the URL or the error that the row exists to show.
func (m model) wrapProse(s string) string {
	return ansi.Wrap(s, ui.NewLayout(m.width, m.height).ContentWidth(), "")
}

// wrapOption wraps one entry of a cursor-driven list, indenting continuation
// rows past the "> " marker so the option still reads as one item.
func (m model) wrapOption(prefix, text string) string {
	width := ui.NewLayout(m.width, m.height).ContentWidth() - len(prefix)
	if width < 1 {
		width = 1
	}
	lines := strings.Split(ansi.Wrap(text, width, ""), "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
		} else {
			lines[i] = strings.Repeat(" ", len(prefix)) + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

// wrapHelp lays out a "a • b • c" footer, breaking between segments rather than
// inside them so a hint is never split across two rows.
func (m model) wrapHelp(s string) string {
	width := ui.NewLayout(m.width, m.height).ContentWidth()
	if lipgloss.Width(s) <= width {
		return s
	}

	var rows []string
	row := ""
	for _, seg := range strings.Split(s, segmentSeparator) {
		switch {
		case row == "":
			row = seg
		case lipgloss.Width(row)+lipgloss.Width(segmentSeparator)+lipgloss.Width(seg) <= width:
			row += segmentSeparator + seg
		default:
			rows = append(rows, row)
			row = seg
		}
	}
	if row != "" {
		rows = append(rows, row)
	}
	// A single segment can still be wider than the screen on its own.
	return ansi.Wrap(strings.Join(rows, "\n"), width, "")
}

// gap is the blank line the setup screens put between their sections. On a
// short terminal — a phone in landscape, a split pane — those blank rows are
// what pushes the options and the prompt off the bottom, so they collapse.
func (m model) gap() string {
	if ui.NewLayout(m.width, m.height).IsShort() {
		return "\n"
	}
	return "\n\n"
}

func (m model) welcomeView() string {
	// Not RenderScreenHeader: it indents the subtitle itself, which would stack
	// with the indent wrapOption adds and overflow a narrow screen.
	return m.theme.Brand.Render("  gurt") + "\n" +
		m.theme.Dim.Render(m.wrapOption("  ", "A coding agent in your terminal.")) + m.gap() +
		m.theme.Dim.Render(m.wrapOption("  ", "Press enter to start.")) + "\n" +
		ui.RenderFooterHelp(m.theme, m.wrapOption("  ", "ctrl+c quit"))
}

func (m model) providerPickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())

	if m.confirmDeleteEndpoint != "" {
		b.WriteString(m.theme.Error.Render(m.wrapProse(fmt.Sprintf("Delete saved endpoint %q? (y/n)", m.confirmDeleteEndpoint))))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(m.providerList.View())
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓ navigate • enter select • d delete saved • ctrl+c quit")))
	return b.String()
}

func (m model) customModePickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse("Custom endpoint mode:")))
	b.WriteString(m.gap())
	items := []string{"Use one-time", "Save for later"}
	for i, item := range items {
		prefix := "  "
		style := m.theme.Dim
		if i == m.customModeCursor {
			prefix = "> "
			style = m.theme.Header
		}
		b.WriteString(style.Render(m.wrapOption(prefix, item)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓ navigate • enter select • esc back • ctrl+c quit")))
	return b.String()
}

func (m model) customURLView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse("Enter the base URL for your custom provider:")))
	b.WriteString(m.gap())
	b.WriteString(m.urlInput.View())
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("enter confirm • ctrl+c quit")))
	return b.String()
}

func (m model) apiKeyView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse(fmt.Sprintf("Enter your API key for %s:", llm.DisplayName(m.provider)))))
	b.WriteString(m.gap())
	if m.customURL != "" {
		b.WriteString(m.theme.Dim.Render(m.wrapProse("Endpoint: " + m.customURL)))
		b.WriteString(m.gap())
	}
	b.WriteString(m.keyInput.View())
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("enter confirm • ctrl+c quit")))
	return b.String()
}

func (m model) modelFetchView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse(fmt.Sprintf("Fetching models from %s...", llm.DisplayName(m.provider)))))
	b.WriteString(m.gap())
	b.WriteString(m.spinner.View())
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("ctrl+c quit")))
	return b.String()
}

func (m model) modelPickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.modelList.View())
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("Type to filter • ↑/↓ navigate • enter select • ctrl+c quit")))
	return b.String()
}

func (m model) reasoningConfigView() string {
	var b strings.Builder
	divider := m.theme.Divider.Render(strings.Repeat("─", ui.NewLayout(m.width, m.height).RuleWidth()))
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Header.Render(m.modelName))
	b.WriteString("\n")
	b.WriteString(divider)
	b.WriteString(m.gap())

	if len(m.thinkingOptions) > 0 && len(m.effortOptions) > 0 {
		// Two-field mode: Thinking + Effort (Anthropic)
		think := m.thinkingType
		thinkLine := m.theme.Dim.Render(fmt.Sprintf("  Thinking:  %s", think))
		if m.reasoningField == 0 {
			thinkLine = fmt.Sprintf("  %s Thinking:  %s %s ", m.theme.Header.Render("▶"), m.theme.Header.Render(think), m.theme.Dim.Render("← →"))
		}
		b.WriteString(thinkLine)
		b.WriteString("\n")

		effort := m.effortLevel
		effortLine := m.theme.Dim.Render(fmt.Sprintf("  Effort:    %s", effort))
		if m.reasoningField == 1 {
			effortLine = fmt.Sprintf("  %s Effort:    %s %s ", m.theme.Header.Render("▶"), m.theme.Header.Render(effort), m.theme.Dim.Render("← →"))
		}
		b.WriteString(effortLine)
		b.WriteString(m.gap())

		b.WriteString(divider)
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓ navigate • ←/→ change • enter confirm • esc go back")))
	} else if len(m.thinkingOptions) > 0 {
		// Thinking-only mode (no effort levels)
		think := m.thinkingType
		thinkLine := m.theme.Dim.Render(fmt.Sprintf("  Thinking:  %s", think))
		if m.reasoningField == 0 {
			thinkLine = fmt.Sprintf("  %s Thinking:  %s %s ", m.theme.Header.Render("▶"), m.theme.Header.Render(think), m.theme.Dim.Render("← →"))
		}
		b.WriteString(thinkLine)
		b.WriteString(m.gap())

		b.WriteString(divider)
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render(m.wrapHelp("←/→ change • enter confirm • esc go back")))
	} else {
		// Single-field mode: Reasoning effort (OpenAI)
		effort := m.effortLevel
		effortLine := m.theme.Dim.Render(fmt.Sprintf("  Reasoning: %s", effort))
		if m.reasoningField == 0 {
			effortLine = fmt.Sprintf("  %s Reasoning: %s %s ", m.theme.Header.Render("▶"), m.theme.Header.Render(effort), m.theme.Dim.Render("← →"))
		}
		b.WriteString(effortLine)
		b.WriteString(m.gap())

		b.WriteString(divider)
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render(m.wrapHelp("←/→ change • enter confirm • esc go back")))
	}
	return b.String()
}

func (m model) errorView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Error.Render("Error"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse(m.err.Error())))
	b.WriteString(m.gap())
	for i, action := range m.errorActions() {
		prefix := "  "
		if i == m.errChoice {
			prefix = "> "
		}
		style := m.theme.Dim
		if i == m.errChoice {
			style = m.theme.Header
		}
		b.WriteString(style.Render(m.wrapOption(prefix, action)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓ navigate • enter select • ctrl+c quit")))
	return b.String()
}

func (m model) dotenvPromptView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Error.Render(m.wrapProse("Could not save API key to OS keychain")))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse(m.err.Error())))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse("The key is active for this session. How should it be saved for future sessions?")))
	b.WriteString(m.gap())
	items := []string{
		"Save to ~/.config/gurtcli/credentials.json (this machine, your user can read it)",
		"Use key this session only (will need to re-enter next time)",
		"Save API key to .env file",
	}
	for i, item := range items {
		prefix := "  "
		style := m.theme.Dim
		if i == m.dotenvCursor {
			prefix = "> "
			style = m.theme.Header
		}
		b.WriteString(style.Render(m.wrapOption(prefix, item)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓ navigate • enter select • ctrl+c quit")))
	return b.String()
}

func (m model) dotenvPickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse("Found API key(s) in environment file:")))
	b.WriteString(m.gap())
	items := append(m.dotenvKeys, "Enter a new API key")
	for i, item := range items {
		prefix := "  "
		style := m.theme.Dim
		if i == m.dotenvPickCursor {
			prefix = "> "
			style = m.theme.Header
		}
		b.WriteString(style.Render(m.wrapOption(prefix, item)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓ navigate • enter select • ctrl+c quit")))
	return b.String()
}

func (m model) dotenvKeyNameView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse("Save API key to .env file.")))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse("Enter the environment variable name:")))
	b.WriteString(m.gap())
	b.WriteString(m.dotenvInput.View())
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("enter confirm • esc back • ctrl+c quit")))
	return b.String()
}

func (m model) dotenvKeyExistsView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse(fmt.Sprintf("Key %q already exists in .env.", m.dotenvKeyName))))
	b.WriteString(m.gap())
	items := []string{
		"Overwrite existing value",
		"Load existing value and continue",
		"Change key name",
	}
	for i, item := range items {
		prefix := "  "
		style := m.theme.Dim
		if i == m.dotenvKeyExistsCursor {
			prefix = "> "
			style = m.theme.Header
		}
		b.WriteString(style.Render(m.wrapOption(prefix, item)))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓ navigate • enter select • ctrl+c quit")))
	return b.String()
}

func (m model) customNameView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse("Name this endpoint to save for later:")))
	b.WriteString(m.gap())
	b.WriteString(m.nameInput.View())
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("enter confirm • ctrl+c quit")))
	return b.String()
}

func (m model) manualModelView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapProse("Enter the model name:")))
	b.WriteString(m.gap())
	b.WriteString(m.manualInput.View())
	b.WriteString(m.gap())
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("enter confirm • ctrl+c quit")))
	return b.String()
}

func (m model) sessionDisplayName() string {
	if m.sessionName != "" {
		return m.sessionName
	}
	name := sessions.NameForMessages(m.messages)
	if name != "Empty session" {
		return name
	}
	return "New session"
}

// segment is one " • "-joined part of the help or status line. The two views
// of it — what gets drawn and what a click on it copies — are kept together so
// the copy zones cannot drift away from the rendered columns.
type segment struct {
	display string // as it appears in the row
	text    string // what a click copies
	label   string // how the toast names it
}

// joinSegments renders segments the way the help and status lines join them.
func joinSegments(segs []segment) string {
	parts := make([]string, 0, len(segs))
	for _, s := range segs {
		parts = append(parts, s.display)
	}
	return strings.Join(parts, segmentSeparator)
}

const segmentSeparator = " • "

// providerLabel returns the provider name as the status line shows it.
func (m model) providerLabel() string {
	if m.savedEndpointName != "" {
		return m.savedEndpointName
	}
	if m.provider == llm.ProviderCustom {
		return "Custom"
	}
	return llm.DisplayName(m.provider)
}

// statusSegments returns the right-hand side of the bottom bar.
func (m model) statusSegments() []segment {
	session := m.sessionDisplayName()
	provider := m.providerLabel()
	name := m.modelDisplayName()
	return []segment{
		{display: session, text: session, label: "session name"},
		{display: provider, text: provider, label: "provider"},
		{display: name, text: name, label: "model name"},
	}
}

// helpSegments returns the left-hand side of the bottom bar in its default
// state: the build and where it is running. The working directory copies as an
// absolute path even though it is displayed with a ~.
func (m model) helpSegments() []segment {
	segs := []segment{
		{display: VersionString(), text: VersionString(), label: "version"},
		{display: m.cwdDisplay, text: m.workspaceRoot, label: "working directory"},
	}
	if m.updateAvailable {
		note := "Update " + m.latestVersion + " — /update"
		segs = append(segs, segment{display: note, text: "/update", label: "update command"})
	}
	return segs
}

// chatHelpText returns the left-hand hint line and whether it is the default
// one — the transient hints shown while streaming or picking a suggestion have
// nothing worth copying.
func (m model) chatHelpText() (string, bool) {
	switch {
	case m.isStreaming:
		return "esc cancel • ctrl+c quit", false
	case m.suggestions.active && len(m.suggestions.items) > 0:
		return "↑↓ navigate • tab select • esc dismiss", false
	default:
		return joinSegments(m.helpSegments()), true
	}
}

// fitBottomBar decides what survives on the bottom row at the current width.
//
// The row must stay exactly one row tall — adjustViewportHeight budgets for one
// — so on a narrow terminal segments are dropped rather than allowed to wrap.
// Least useful goes first: version, then the working directory, then the update
// notice, then the session name, then the provider. The model name is what
// people actually need to see, so it survives to the end and is truncated only
// if even it does not fit.
//
// Returns the help text, the help segments it was built from (nil when the help
// side is a transient hint with nothing worth copying), and the status
// segments. Both the renderer and the click-to-copy zones read this, so the two
// cannot disagree about which columns hold what.
func (m model) fitBottomBar() (help string, helpSegs, statusSegs []segment) {
	helpText, isDefault := m.chatHelpText()
	statusSegs = m.statusSegments()
	if isDefault {
		helpSegs = m.helpSegments()
	}

	// One column of separation between the two halves, minimum.
	fits := func() bool {
		return lipgloss.Width(helpText)+lipgloss.Width(joinSegments(statusSegs))+1 <= m.width
	}

	for !fits() && len(helpSegs) > 0 {
		helpSegs = helpSegs[1:]
		helpText = joinSegments(helpSegs)
	}
	for !fits() && len(statusSegs) > 1 {
		statusSegs = statusSegs[1:]
	}
	if !fits() {
		// Only the model name is left. Give the whole row to it.
		helpSegs = nil
		helpText = ""
		if len(statusSegs) == 1 && lipgloss.Width(statusSegs[0].display) > m.width {
			statusSegs[0].display = ansi.Truncate(statusSegs[0].display, m.width, "…")
		}
	}
	return helpText, helpSegs, statusSegs
}

func (m model) helpWithStatus() string {
	help, _, statusSegs := m.fitBottomBar()
	helpRendered := m.theme.Dim.Render(help)
	if help == "" {
		helpRendered = ""
	}
	statusRendered := m.theme.StatusBar.Render(joinSegments(statusSegs))
	pad := m.width - lipgloss.Width(helpRendered) - lipgloss.Width(statusRendered)
	if pad < 1 {
		pad = 1
	}
	return helpRendered + strings.Repeat(" ", pad) + statusRendered
}

func (m model) sessionPickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())
	b.WriteString(m.sessionList.View())
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓ navigate • enter switch • esc back • ctrl+c quit")))
	return b.String()
}

func (m model) allowManageView() string {
	var b strings.Builder
	layout := ui.NewLayout(m.width, m.height)
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString(m.gap())

	// Tool checker mode
	if m.allowManageAdding && m.allowManageAddType == "tool" {
		b.WriteString(m.theme.Header.Render("Toggle Always-Allowed Tools"))
		b.WriteString(m.gap())
		for i, name := range m.allowToolCheckItems {
			checked := false
			for _, t := range m.alwaysAllowTools {
				if t == name {
					checked = true
					break
				}
			}
			box := m.theme.CheckboxOff.Render("☐")
			if checked {
				box = m.theme.CheckboxOn.Render("☑")
			}
			prefix := "  "
			style := m.theme.Dim
			if i == m.allowToolCheckCursor {
				prefix = "> "
				style = m.theme.Header
			}
			b.WriteString(style.Render(m.wrapOption(prefix, box+" "+name)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓ navigate • enter/space toggle • esc done")))
		return b.String()
	}

	// Command prefix add mode (text input)
	if m.allowManageAdding && m.allowManageAddType == "command" {
		b.WriteString(m.theme.Header.Render("Add Command Prefix"))
		b.WriteString(m.gap())
		b.WriteString(m.allowManageInput.View())
		b.WriteString(m.gap())
		b.WriteString(m.theme.Dim.Render(m.wrapHelp("enter confirm • esc cancel")))
		return b.String()
	}

	// Multi-column grid view (row-major, fills rows then wraps)
	cmds := m.alwaysAllowCommandPrefixes
	if len(cmds) == 0 {
		b.WriteString(m.theme.Dim.Render("  No command prefixes configured."))
		b.WriteString(m.gap())
		b.WriteString(m.theme.Divider.Render(strings.Repeat("─", layout.RuleWidth())))
	} else {
		numRows, numCols, colWidth := m.cmdGridDimensions()
		if numCols < 1 {
			numCols = 1
		}

		start := m.allowManageScroll
		for row := 0; row < numRows; row++ {
			hasItems := false
			for col := 0; col < numCols; col++ {
				idx := start + row*numCols + col
				if idx >= len(cmds) {
					continue
				}
				hasItems = true
				indicator := "  "
				if idx == m.allowManageCursor {
					indicator = "> "
				}
				cell := indicator + cmds[idx]
				cell = fmt.Sprintf("%-*s", colWidth, cell)
				if idx == m.allowManageCursor {
					b.WriteString(m.theme.Header.Render(cell))
				} else {
					b.WriteString(m.theme.Dim.Render(cell))
				}
			}
			if hasItems {
				b.WriteString("\n")
			}
		}
		// The divider spans the grid, but never the edge of the screen.
		divWidth := numCols * colWidth
		if divWidth > layout.ContentWidth() {
			divWidth = layout.ContentWidth()
		}
		b.WriteString(m.theme.Divider.Render(strings.Repeat("─", divWidth)))
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render(m.wrapHelp("↑/↓←/→ navigate • t tool • c command • d/x delete • esc back")))
	return b.String()
}

// renderChatInput renders the chat input with inline @file and /command highlighting.
// For multi-line input it falls back to the textarea's default rendering.
func (m model) renderChatInput() string {
	val := m.chatInput.Value()

	// Multi-line: fall back to textarea's default rendering
	if strings.Contains(val, "\n") {
		return m.chatInput.View()
	}

	inputWidth := ui.NewLayout(m.width, m.height).ChatInputWidth()

	// Empty: show placeholder
	if val == "" {
		ph := m.chatInput.Placeholder
		if ph != "" {
			return m.theme.Dim.Render(ansi.Truncate("  "+ph, inputWidth, ""))
		}
		return ""
	}

	cmdNames := commandNames()
	baseStyle := m.theme.UserContent
	cursorCol := m.chatInput.Column()

	// Wrap at input width; ansi.Hardwrap preserves character alignment
	wrapped := ansi.Hardwrap(val, inputWidth, true)
	wrappedLines := strings.Split(wrapped, "\n")

	visualLine := cursorCol / inputWidth
	if visualLine >= len(wrappedLines) {
		visualLine = len(wrappedLines) - 1
	}
	visualCol := cursorCol - (visualLine * inputWidth)

	var b strings.Builder
	for i, line := range wrappedLines {
		if i > 0 {
			b.WriteRune('\n')
		}

		if i == visualLine {
			before, atChar, after := line, " ", ""
			if visualCol < len(line) {
				before = line[:visualCol]
				atChar = string(line[visualCol])
				after = line[visualCol+1:]
			}

			b.WriteString(ui.HighlightInline(before, baseStyle, m.theme.FileRef, m.theme.CmdRef, cmdNames))

			cursorStyle := lipgloss.NewStyle().
				Foreground(lipgloss.Color(m.theme.Base)).
				Background(lipgloss.Color(m.theme.Text))
			b.WriteString(cursorStyle.Render(atChar))

			b.WriteString(ui.HighlightInline(after, baseStyle, m.theme.FileRef, m.theme.CmdRef, cmdNames))
		} else {
			b.WriteString(ui.HighlightInline(line, baseStyle, m.theme.FileRef, m.theme.CmdRef, cmdNames))
		}
	}
	return b.String()
}

// showsChatTitle reports whether the chat screen can afford its title row. On a
// short terminal the model name is already in the status bar, so the title and
// the rule under it are two rows better spent on the transcript.
func (m model) showsChatTitle() bool {
	return !ui.NewLayout(m.width, m.height).IsShort()
}

// chatChromeLines is the number of rows chatView always renders around the
// transcript viewport: title, top rule, spacer, toast, bottom rule — minus the
// title and its rule when the terminal is too short for them.
//
// chatView, adjustViewportHeight and computeViewportStartRow must all agree
// with this exactly, or the view overflows the screen and the mouse lands on
// the wrong row.
func (m model) chatChromeLines() int {
	if m.showsChatTitle() {
		return 5
	}
	return 3
}

// permOverlayMaxHeight is the tallest the permission overlay may grow while
// still leaving a row of transcript above it.
func (m model) permOverlayMaxHeight() int {
	h := m.height - m.chatChromeLines() - 1
	if h < 5 {
		h = 5
	}
	return h
}

// renderPermOverlay renders the permission box so that it fits on screen. The
// tool-call body is shrunk first; if the prompt still cannot fit alongside the
// transcript, the box is rendered in a compact form, and failing that it is
// allowed to take the whole screen (chatView then drops the chrome around it).
//
// Returns the rendered box, its height in rows, the total number of body lines
// available to scroll through, and the size of the visible body window.
func (m model) renderPermOverlay() (string, int, int, int) {
	maxHeight := m.permOverlayMaxHeight()

	box, height, total, visible := m.permBox(maxHeight, false)
	if height <= maxHeight {
		return box, height, total, visible
	}
	if cBox, cHeight, cTotal, cVisible := m.permBox(maxHeight, true); cHeight <= maxHeight {
		return cBox, cHeight, cTotal, cVisible
	}
	// Nothing fits with the chat chrome in place — fill the screen instead.
	return m.permBox(m.height, true)
}

// permBox renders the permission box, shrinking the tool-call body until the
// box fits within maxHeight rows. Measuring the rendered box (rather than
// predicting its height) keeps the overlay honest about soft-wrapped lines and
// per-tool option counts. compact drops the box padding and the key hints to
// buy back rows on short terminals.
func (m model) permBox(maxHeight int, compact bool) (string, int, int, int) {
	verticalPad := 1
	if compact {
		verticalPad = 0
	}
	boxStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(m.theme.Base)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(m.theme.Mauve)).
		Width(ui.NewLayout(m.width, m.height).PopupWidth()).
		Padding(verticalPad, 1)

	// Sudo password input phase: fixed size, nothing to scroll.
	if m.pendingPerm.confirmSudo {
		var pw strings.Builder
		pw.WriteString(m.theme.PermPrompt.Render("  Enter sudo password"))
		pw.WriteString("\n\n")
		pw.WriteString("  " + m.sudoPasswordInput.View())
		pw.WriteString("\n\n")
		pw.WriteString(m.theme.Dim.Render("  enter confirm • esc cancel"))
		box := boxStyle.Render(pw.String())
		return box, lipgloss.Height(box), 0, 0
	}

	tc := m.pendingPerm.toolCall
	bashPrefix := ""
	if tc.Function.Name == "run_bash" {
		if cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments)); err == nil {
			bashPrefix = tools.BashCommandPrefix(cmd)
		}
	}

	// Body content is laid out for the inside of the box, not the terminal, so
	// it does not get soft-wrapped into extra rows after the fact.
	innerWidth := ui.NewLayout(m.width, m.height).PopupWidth() - 4

	maxBody := maxHeight
	box := ""
	height := 0
	total := 0
	for i := 0; i < 8; i++ {
		content, totalLines := ui.RenderPermissionPrompt(m.theme, ui.PermPrompt{
			Call:         tc,
			Width:        innerWidth,
			Cursor:       m.permCursor,
			BashPrefix:   bashPrefix,
			ExternalPath: m.pendingPerm.externalPath,
			Sudo:         m.pendingPerm.sudo,
			ScrollOffset: m.permScroll,
			MaxBodyLines: maxBody,
			Compact:      compact,
			HideBody:     maxBody == 0,
		})
		if !compact {
			content += "\n" + m.theme.Dim.Render("  ↑/↓ navigate • enter select • pgup/pgdn scroll")
		}
		box = boxStyle.Render(content)
		height = lipgloss.Height(box)
		total = totalLines
		if height <= maxHeight || maxBody <= 0 {
			break
		}
		// Body lines can soft-wrap, so a row over budget may cost more than one
		// body line; shrink by the overflow and re-measure. A last pass with no
		// body at all keeps the question and its options on screen.
		maxBody -= height - maxHeight
		if maxBody < 0 {
			maxBody = 0
		}
	}
	return box, height, total, maxBody
}

func (m model) chatView() string {
	var b strings.Builder
	layout := ui.NewLayout(m.width, m.height)

	// Measure the permission overlay first: it is anchored to the bottom of the
	// screen and must never be pushed off, so the transcript viewport gets
	// whatever height is left over rather than the other way around.
	permBox := ""
	if m.pendingPerm != nil {
		box, boxHeight, _, _ := m.renderPermOverlay()
		permBox = box
		if boxHeight > m.permOverlayMaxHeight() {
			// Terminal too short for both — the prompt wins the screen, still
			// anchored to the bottom. If it is taller than the screen even at
			// its smallest, the top is what gets clipped, so that the options
			// and the box's own bottom edge stay visible.
			rows := strings.Split(box, "\n")
			if pad := m.height - len(rows); pad > 0 {
				rows = append(make([]string, pad), rows...)
			} else if pad < 0 {
				rows = rows[-pad:]
			}
			return "\x1b[2 q\x1b[?25l" + strings.Join(rows, "\n")
		}
		m.chatViewport.SetHeight(m.height - m.chatChromeLines() - boxHeight)
	}

	b.WriteString("\x1b[2 q")  // DECSCUSR: non-blinking block cursor
	b.WriteString("\x1b[?25l") // hide hardware cursor

	if m.showsChatTitle() {
		b.WriteString(m.theme.Brand.Render("  " + m.modelDisplayName()))
		b.WriteString("\n")
		b.WriteString(ui.RenderRule(m.theme, layout))
		b.WriteString("\n")
	}

	b.WriteString(m.chatViewport.View())
	b.WriteString("\n")

	b.WriteString(m.renderSpacerLine())
	b.WriteString("\n")

	{
		toastText := ""
		if m.toast != nil {
			style := m.theme.Toast
			if (m.yolo || m.alwaysAllowPerms) && m.toast.text == "YOLO mode" {
				style = lipgloss.NewStyle().
					Background(lipgloss.Color(m.theme.Peach)).
					Foreground(lipgloss.Color(m.theme.Crust)).
					Bold(true).
					Padding(0, 1)
			}
			// The toast owns one row, so a long message shrinks rather than
			// wraps. The style adds its own padding, so the budget is measured
			// from a rendered empty toast rather than assumed.
			overhead := lipgloss.Width(style.Render("  "))
			text := m.toast.text
			if lipgloss.Width(text)+overhead > m.width {
				text = ansi.Truncate(text, m.width-overhead, "…")
			}
			toastText = style.Render(" " + text + " ")
		}
		pad := (m.width - lipgloss.Width(toastText)) / 2
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(toastText)
		b.WriteString("\n")
	}

	b.WriteString(ui.RenderRule(m.theme, layout))
	b.WriteString("\n")

	if m.pendingPerm != nil {
		b.WriteString(permBox)
	} else if m.showThemePicker {
		boxW := layout.PopupWidth()
		popup := lipgloss.NewStyle().
			Background(lipgloss.Color(m.theme.Base)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(m.theme.Mauve)).
			Width(boxW).
			Padding(1, 2)

		var pc strings.Builder
		pc.WriteString(m.theme.Header.Render("Select Theme"))
		pc.WriteString("\n\n")
		for i, entry := range ui.ThemeRegistry {
			prefix := "  "
			style := m.theme.Dim
			if i == m.themePickerCursor {
				prefix = "> "
				style = m.theme.Header
			}
			pc.WriteString(style.Render(prefix + entry.Name))
			pc.WriteString("\n")
		}
		pc.WriteString("\n")
		pc.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter select • esc dismiss"))

		b.WriteString(popup.Render(pc.String()))
	} else {
		if m.suggestions.active && len(m.suggestions.items) > 0 {
			// Each suggestion is one row and the list is height-budgeted as one
			// row per item, so a long name or description has to be cut rather
			// than allowed to wrap.
			avail := layout.ContentWidth()
			sigil, sigilColor := "/", m.theme.Mauve
			if m.suggestions.isFiles {
				sigil, sigilColor = "@", m.theme.Blue
			}

			for i, item := range m.suggestions.items {
				prefix := "  "
				style := m.theme.Dim
				if i == m.suggestions.selected {
					prefix = "> "
					style = m.theme.Header
				}

				name := ansi.Truncate(sigil+item.name, avail-len(prefix), "…")
				if i == m.suggestions.selected {
					b.WriteString(style.Render(prefix + name))
				} else {
					b.WriteString(style.Render(prefix))
					b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(sigilColor)).Bold(true).Background(lipgloss.Color(m.theme.Base)).Render(name))
				}

				// File suggestions are just paths; only commands carry a
				// description, and it takes whatever the name left behind.
				if !m.suggestions.isFiles {
					rest := avail - len(prefix) - lipgloss.Width(name)
					if rest > 4 {
						b.WriteString(m.theme.Dim.Render(ansi.Truncate("  "+item.description, rest, "…")))
					}
				}
				b.WriteString("\n")
			}
		}

		if m.queuedMessage != "" {
			// The notice is one row; the preview shrinks so it stays that way.
			notice := "  ⏎ \"" + m.queuedMessage + "\" queued — will send after next tool call"
			b.WriteString(m.theme.QueuedMessage.Render(ansi.Truncate(notice, layout.ContentWidth(), "…")))
			b.WriteString("\n")
		}

		promptStyle := m.theme.InputPrompt
		if strings.Contains(m.chatInput.Value(), "@") {
			promptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(m.theme.Blue)).
				Bold(true).
				Background(lipgloss.Color(m.theme.Base))
		}
		b.WriteString(promptStyle.Render("  ❯ "))
		b.WriteString(m.renderChatInput())
		b.WriteString("\n")
		b.WriteString(m.helpWithStatus())
	}

	return b.String()
}

var workingSpinnerFrames = []string{"◐", "◓", "◑", "◒"}

var workingMessages = []string{
	"Fidgeting with files",
	"Reticulating splines",
	"Wrangling tokens",
	"Booping the API",
	"Spelunking through code",
	"Cogitating deeply",
	"Herding models",
	"Marinating on that",
	"Flummoxing files",
	"Brewing thoughts",
	"Jitterbugging with context",
	"Combobulating code",
	"Prestidigitating patches",
	"Gallivanting through git",
	"Tomfoolering with types",
	"Shenaniganing with syntax",
	"Boondoggling builds",
	"Vibing",
}

// spacerParts returns the two halves of the row below the transcript, after
// deciding what fits. renderSpacerLine draws them and appendSpacerZones places
// the click targets, so both must agree on what was actually dropped.
func (m model) spacerParts() (left, right string) {
	ctxBar := m.renderContextBar()
	debugBar := m.renderDebugBar()

	if m.retry.active {
		left = m.renderRetryStatus()
	} else if m.toolExec != nil && m.toolExec.active {
		idx := m.workingSpinnerIdx % len(workingSpinnerFrames)
		spinner := workingSpinnerFrames[idx]
		label := m.toolExec.label
		if label == "" {
			label = m.toolExec.toolName
		}
		left = m.theme.WorkingStatus.Render(spinner + " " + label)
	} else if m.isStreaming && m.workingMsg != "" {
		idx := m.workingSpinnerIdx % len(workingSpinnerFrames)
		spinner := workingSpinnerFrames[idx]
		left = m.theme.WorkingStatus.Render(spinner + " " + m.workingMsg)
	}

	right = ctxBar
	if debugBar != "" {
		if right != "" {
			right = debugBar + "  " + right
		} else {
			right = debugBar
		}
	}

	// One row, always. What the agent is doing right now beats the meters, so a
	// terminal too narrow for both keeps the left side and drops the right.
	if lipgloss.Width(left)+lipgloss.Width(right)+1 > m.width {
		if left != "" {
			right = ""
		} else if lipgloss.Width(right) > m.width {
			right = ansi.Truncate(right, m.width, "")
		}
	}
	if lipgloss.Width(left) > m.width {
		left = ansi.Truncate(left, m.width, "")
	}
	return left, right
}

func (m model) renderSpacerLine() string {
	left, right := m.spacerParts()
	if left == "" && right == "" {
		return ""
	}

	pad := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if pad < 1 {
		pad = 1
	}

	var b strings.Builder
	if left != "" {
		b.WriteString(left)
	}
	if right != "" {
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(right)
	}
	return b.String()
}

// renderRetryStatus draws the countdown between a failed request and the
// attempt that repeats it. It occupies the same single row as the working
// spinner it replaces, so no layout math changes.
func (m model) renderRetryStatus() string {
	label := "Failed"
	if m.retry.rateLimit {
		label = "Rate limited"
	}

	if m.retry.needsOK {
		lead := m.theme.Error.Render(fmt.Sprintf("✗ %s — resets in %s", label, formatRetryWait(m.retry.delay)))
		return lead + m.theme.Dim.Render("  enter/r to wait it out • esc to cancel")
	}

	remaining := time.Until(m.retry.until)
	if remaining < 0 {
		remaining = 0
	}
	return m.theme.Error.Render(fmt.Sprintf(
		"✗ %s — retrying in %s (attempt %d/%d)",
		label, formatRetryWait(remaining), m.retry.attempt, maxRetryAttempts,
	))
}

// formatRetryWait renders a countdown at second granularity, coarsening to
// minutes and hours for the long usage-limit waits.
func formatRetryWait(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int(d.Round(time.Second).Seconds())
	switch {
	case secs >= 3600:
		h := secs / 3600
		mins := (secs % 3600) / 60
		if mins == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, mins)
	case secs >= 60:
		mins := secs / 60
		rem := secs % 60
		if rem == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm%ds", mins, rem)
	default:
		return fmt.Sprintf("%ds", secs)
	}
}

func (m model) renderDebugBar() string {
	if !m.debug {
		return ""
	}
	label := fmt.Sprintf("CPU:%5.1f%% RAM:%5.1fMB", m.debugStats.cpuPercent, m.debugStats.memMB)
	return m.theme.Dim.Render(label)
}

func (m model) renderContextBar() string {
	// The window holds the history that gets resent every request: the last
	// prompt total plus the response that arrived after it. Session-lifetime
	// sums belong in /stats, not here.
	used := m.contextInputTokens + m.contextOutputTokens
	if used <= 0 {
		return ""
	}

	// Below ~30 columns the spacer row belongs to the working spinner; a meter
	// there would only push it off the screen.
	if m.width > 0 && m.width < 30 {
		return ""
	}

	barWidth := 20
	switch {
	case m.width < 45:
		barWidth = 0 // no room for the graphic; the numbers still fit
	case m.width < 60:
		barWidth = 12
	}

	// Clamped because a misreporting endpoint should show a wrong-but-sane
	// number rather than garbage like "823% cached".
	cachePct := 0
	if m.contextInputTokens > 0 {
		cachePct = int(float64(m.contextCacheTokens) / float64(m.contextInputTokens) * 100)
	}
	if cachePct > 100 {
		cachePct = 100
	}

	cacheStr := ""
	// The cache percentage is the first thing to go: it is the least actionable
	// part of the readout and costs a dozen columns.
	if cachePct > 0 && barWidth > 0 {
		cacheStr = m.theme.Dim.Render(fmt.Sprintf("· %d%% cached", cachePct))
	}

	if m.maxInputTokens <= 0 {
		return m.theme.ContextBar.Render(strings.TrimSpace(formatTokens(used) + " " + cacheStr))
	}

	if barWidth == 0 {
		return m.theme.ContextBar.Render(fmt.Sprintf("%s/%s", formatTokens(used), formatTokens(m.maxInputTokens)))
	}

	pct := float64(used) / float64(m.maxInputTokens)
	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	filledPart := lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Teal)).Render(strings.Repeat("━", filled))
	emptyPart := m.theme.Dim.Render(strings.Repeat("─", barWidth-filled))
	bar := filledPart + emptyPart

	return m.theme.ContextBar.Render(fmt.Sprintf(" %s  %s / %s %s", bar, formatTokens(used), formatTokens(m.maxInputTokens), cacheStr))
}

func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dK", (n+500)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

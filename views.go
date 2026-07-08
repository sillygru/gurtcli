package main

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

func (m model) welcomeView() string {
	return m.theme.Brand.Render("  gurt") + "\n\n" +
		m.theme.Dim.Render("  A coding agent in your terminal.") + "\n\n" +
		m.theme.Dim.Render("  Press enter to start.") + "\n" +
		m.theme.Dim.Render("  ctrl+c quit")
}

func (m model) providerPickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")

	if m.confirmDeleteEndpoint != "" {
		b.WriteString(m.theme.Error.Render(fmt.Sprintf("Delete saved endpoint %q? (y/n)", m.confirmDeleteEndpoint)))
		b.WriteString("\n")
		return b.String()
	}

	b.WriteString(m.providerList.View())
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter select • d delete saved • ctrl+c quit"))
	return b.String()
}

func (m model) customModePickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("Custom endpoint mode:"))
	b.WriteString("\n\n")
	items := []string{"Use one-time", "Save for later"}
	for i, item := range items {
		prefix := "  "
		style := m.theme.Dim
		if i == m.customModeCursor {
			prefix = "> "
			style = m.theme.Header
		}
		b.WriteString(style.Render(prefix + item))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter select • esc back • ctrl+c quit"))
	return b.String()
}

func (m model) customURLView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("Enter the base URL for your custom provider:"))
	b.WriteString("\n\n")
	b.WriteString(m.urlInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("enter confirm • ctrl+c quit"))
	return b.String()
}

func (m model) apiKeyView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render(fmt.Sprintf("Enter your API key for %s:", llm.DisplayName(m.provider))))
	b.WriteString("\n\n")
	if m.customURL != "" {
		b.WriteString(m.theme.Dim.Render("Endpoint: " + m.customURL))
		b.WriteString("\n\n")
	}
	b.WriteString(m.keyInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("enter confirm • ctrl+c quit"))
	return b.String()
}

func (m model) modelFetchView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render(fmt.Sprintf("Fetching models from %s...", llm.DisplayName(m.provider))))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("ctrl+c quit"))
	return b.String()
}

func (m model) modelPickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.modelList.View())
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render("Type to filter • ↑/↓ navigate • enter select • ctrl+c quit"))
	return b.String()
}

func (m model) reasoningConfigView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Header.Render(m.modelName))
	b.WriteString("\n")
	b.WriteString(m.theme.Divider.Render(strings.Repeat("─", 40)))
	b.WriteString("\n\n")

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
		b.WriteString("\n\n")

		b.WriteString(m.theme.Divider.Render(strings.Repeat("─", 40)))
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render("↑/↓ navigate • ←/→ change • enter confirm • esc go back"))
	} else if len(m.thinkingOptions) > 0 {
		// Thinking-only mode (no effort levels)
		think := m.thinkingType
		thinkLine := m.theme.Dim.Render(fmt.Sprintf("  Thinking:  %s", think))
		if m.reasoningField == 0 {
			thinkLine = fmt.Sprintf("  %s Thinking:  %s %s ", m.theme.Header.Render("▶"), m.theme.Header.Render(think), m.theme.Dim.Render("← →"))
		}
		b.WriteString(thinkLine)
		b.WriteString("\n\n")

		b.WriteString(m.theme.Divider.Render(strings.Repeat("─", 40)))
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render("←/→ change • enter confirm • esc go back"))
	} else {
		// Single-field mode: Reasoning effort (OpenAI)
		effort := m.effortLevel
		effortLine := m.theme.Dim.Render(fmt.Sprintf("  Reasoning: %s", effort))
		if m.reasoningField == 0 {
			effortLine = fmt.Sprintf("  %s Reasoning: %s %s ", m.theme.Header.Render("▶"), m.theme.Header.Render(effort), m.theme.Dim.Render("← →"))
		}
		b.WriteString(effortLine)
		b.WriteString("\n\n")

		b.WriteString(m.theme.Divider.Render(strings.Repeat("─", 40)))
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render("←/→ change • enter confirm • esc go back"))
	}
	return b.String()
}

func (m model) errorView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Error.Render("Error"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render(m.err.Error()))
	b.WriteString("\n\n")
	for i, action := range m.errorActions() {
		prefix := "  "
		if i == m.errChoice {
			prefix = "> "
		}
		style := m.theme.Dim
		if i == m.errChoice {
			style = m.theme.Header
		}
		b.WriteString(style.Render(prefix + action))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter select • ctrl+c quit"))
	return b.String()
}

func (m model) dotenvPromptView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Error.Render("Could not save API key to OS keychain"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render(m.err.Error()))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("The key is active for this session. How should it be saved for future sessions?"))
	b.WriteString("\n\n")
	items := []string{
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
		b.WriteString(style.Render(prefix + item))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter select • ctrl+c quit"))
	return b.String()
}

func (m model) dotenvPickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("Found API key(s) in environment file:"))
	b.WriteString("\n\n")
	items := append(m.dotenvKeys, "Enter a new API key")
	for i, item := range items {
		prefix := "  "
		style := m.theme.Dim
		if i == m.dotenvPickCursor {
			prefix = "> "
			style = m.theme.Header
		}
		b.WriteString(style.Render(prefix + item))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter select • ctrl+c quit"))
	return b.String()
}

func (m model) dotenvKeyNameView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("Save API key to .env file."))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("Enter the environment variable name:"))
	b.WriteString("\n\n")
	b.WriteString(m.dotenvInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("enter confirm • esc back • ctrl+c quit"))
	return b.String()
}

func (m model) dotenvKeyExistsView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render(fmt.Sprintf("Key %q already exists in .env.", m.dotenvKeyName)))
	b.WriteString("\n\n")
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
		b.WriteString(style.Render(prefix + item))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter select • ctrl+c quit"))
	return b.String()
}

func (m model) customNameView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("Name this endpoint to save for later:"))
	b.WriteString("\n\n")
	b.WriteString(m.nameInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("enter confirm • ctrl+c quit"))
	return b.String()
}

func (m model) manualModelView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("Enter the model name:"))
	b.WriteString("\n\n")
	b.WriteString(m.manualInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("enter confirm • ctrl+c quit"))
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

func (m model) helpWithStatus(help string) string {
	providerLabel := llm.DisplayName(m.provider)
	if m.savedEndpointName != "" {
		providerLabel = m.savedEndpointName
	} else if m.provider == llm.ProviderCustom {
		providerLabel = "Custom"
	}
	helpRendered := m.theme.Dim.Render(help)
	statusRendered := m.theme.StatusBar.Render(fmt.Sprintf("%s • %s • %s", m.sessionDisplayName(), providerLabel, m.modelDisplayName()))
	pad := m.width - lipgloss.Width(helpRendered) - lipgloss.Width(statusRendered)
	if pad < 1 {
		pad = 1
	}
	return helpRendered + strings.Repeat(" ", pad) + statusRendered
}

func (m model) sessionPickView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString(m.sessionList.View())
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter switch • esc back • ctrl+c quit"))
	return b.String()
}

func (m model) allowManageView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")

	// Tool checker mode
	if m.allowManageAdding && m.allowManageAddType == "tool" {
		b.WriteString(m.theme.Header.Render("Toggle Always-Allowed Tools"))
		b.WriteString("\n\n")
		for i, name := range m.allowToolCheckItems {
			checked := false
			for _, t := range m.alwaysAllowTools {
				if t == name {
					checked = true
					break
				}
			}
			box := "[ ]"
			if checked {
				box = "[x]"
			}
			prefix := "  "
			style := m.theme.Dim
			if i == m.allowToolCheckCursor {
				prefix = "> "
				style = m.theme.Header
			}
			b.WriteString(style.Render(prefix + box + " " + name))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(m.theme.Dim.Render("↑/↓ navigate • enter/space toggle • esc done"))
		return b.String()
	}

	// Command prefix add mode (text input)
	if m.allowManageAdding && m.allowManageAddType == "command" {
		b.WriteString(m.theme.Header.Render("Add Command Prefix"))
		b.WriteString("\n\n")
		b.WriteString(m.allowManageInput.View())
		b.WriteString("\n\n")
		b.WriteString(m.theme.Dim.Render("enter confirm • esc cancel"))
		return b.String()
	}

	// Multi-column grid view (row-major, fills rows then wraps)
	cmds := m.alwaysAllowCommandPrefixes
	if len(cmds) == 0 {
		b.WriteString(m.theme.Dim.Render("  No command prefixes configured."))
		b.WriteString("\n\n")
		b.WriteString(m.theme.Divider.Render(strings.Repeat("─", 40)))
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
		divWidth := numCols * colWidth
		if divWidth < 40 {
			divWidth = 40
		}
		b.WriteString(m.theme.Divider.Render(strings.Repeat("─", divWidth)))
	}
	b.WriteString("\n")
	b.WriteString(m.theme.Dim.Render("↑/↓←/→ navigate • t tool • c command • d/x delete • esc back"))
	return b.String()
}

func (m model) chatView() string {
	var b strings.Builder
	b.WriteString("\x1b[2 q") // DECSCUSR: non-blinking block cursor

	b.WriteString(m.theme.Brand.Render("  " + m.modelDisplayName()))
	b.WriteString("\n")

	dividerLen := m.width
	if dividerLen < 4 {
		dividerLen = 40
	}
	b.WriteString(m.theme.Divider.Render(strings.Repeat("─", dividerLen)))
	b.WriteString("\n")

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
			toastText = style.Render(" " + m.toast.text + " ")
		}
		pad := (m.width - lipgloss.Width(toastText)) / 2
		if pad > 0 {
			b.WriteString(strings.Repeat(" ", pad))
		}
		b.WriteString(toastText)
		b.WriteString("\n")
	}

	b.WriteString(m.theme.Divider.Render(strings.Repeat("─", dividerLen)))
	b.WriteString("\n")

	if m.pendingPerm != nil {
		tc := m.pendingPerm.toolCall

		bashPrefix := ""
		if tc.Function.Name == "run_bash" {
			if cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments)); err == nil {
				bashPrefix = tools.BashCommandPrefix(cmd)
			}
		}

		boxW := m.width - 2
		if boxW < 30 {
			boxW = 30
		}
		permBox := lipgloss.NewStyle().
			Background(lipgloss.Color(m.theme.Base)).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(m.theme.Mauve)).
			Width(boxW).
			Padding(1, 1)

		content := ui.RenderPermissionPrompt(m.theme, tc, m.width, m.permCursor, bashPrefix, m.pendingPerm.externalPath) + "\n" +
			m.theme.Dim.Render("  ↑/↓ navigate • enter select")

		b.WriteString(permBox.Render(content))
	} else if m.showThemePicker {
		boxW := m.width - 4
		if boxW < 30 {
			boxW = 30
		}
		if boxW > 50 {
			boxW = 50
		}
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
		help := VersionString() + " • " + m.cwdDisplay
		if m.updateAvailable {
			help = VersionString() + " • " + m.cwdDisplay + " — Update " + m.latestVersion + " — /update"
		}
		if m.isStreaming {
			help = "esc cancel • ctrl+c quit"
		} else if m.suggestions.active && len(m.suggestions.items) > 0 {
			help = "↑↓ navigate • tab select • esc dismiss"
		for i, item := range m.suggestions.items {
			prefix := "  "
			style := m.theme.Dim
			if i == m.suggestions.selected {
				prefix = "> "
				style = m.theme.Header
			}
			if m.suggestions.isFiles {
				if i == m.suggestions.selected {
					b.WriteString(style.Render(prefix + "@" + item.name))
				} else {
					b.WriteString(style.Render(prefix))
					b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Blue)).Bold(true).Background(lipgloss.Color(m.theme.Base)).Render("@" + item.name))
				}
			} else {
				if i == m.suggestions.selected {
					b.WriteString(style.Render(prefix + "/" + item.name))
				} else {
					b.WriteString(style.Render(prefix))
					b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(m.theme.Mauve)).Bold(true).Background(lipgloss.Color(m.theme.Base)).Render("/" + item.name))
				}
				b.WriteString(m.theme.Dim.Render("  " + item.description))
			}
			b.WriteString("\n")
		}
		}

		if m.queuedMessage != "" {
			preview := m.queuedMessage
			if len(preview) > 60 {
				preview = preview[:60] + "..."
			}
			b.WriteString(m.theme.QueuedMessage.Render("  ⏎ \"" + preview + "\" queued — will send after next tool call"))
			b.WriteString("\n")
		}

		b.WriteString(m.theme.InputPrompt.Render("  ❯ "))
		b.WriteString(m.chatInput.View())
		b.WriteString("\n")
		b.WriteString(m.helpWithStatus(help))
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

func (m model) renderSpacerLine() string {
	ctxBar := m.renderContextBar()
	debugBar := m.renderDebugBar()

	var left string
	if m.toolExec.active {
		idx := m.workingSpinnerIdx % len(workingSpinnerFrames)
		spinner := workingSpinnerFrames[idx]
		label := m.toolExec.title
		if label == "" {
			label = m.toolExec.toolName
		}
		left = m.theme.WorkingStatus.Render(spinner + " " + label)
	} else if m.isStreaming && m.workingMsg != "" {
		idx := m.workingSpinnerIdx % len(workingSpinnerFrames)
		spinner := workingSpinnerFrames[idx]
		left = m.theme.WorkingStatus.Render(spinner + " " + m.workingMsg)
	}

	right := ctxBar
	if debugBar != "" {
		if right != "" {
			right = debugBar + "  " + right
		} else {
			right = debugBar
		}
	}

	if left == "" && right == "" {
		return ""
	}

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	pad := m.width - leftWidth - rightWidth
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

func (m model) renderDebugBar() string {
	if !m.debug {
		return ""
	}
	label := fmt.Sprintf("CPU:%5.1f%% RAM:%5.1fMB", m.debugStats.cpuPercent, m.debugStats.memMB)
	return m.theme.Dim.Render(label)
}

func (m model) renderContextBar() string {
	tokens := m.contextInputTokens
	if tokens <= 0 && m.maxInputTokens <= 0 {
		return ""
	}

	if m.maxInputTokens <= 0 {
		return m.theme.ContextBar.Render(formatTokens(tokens))
	}

	if tokens <= 0 {
		return m.theme.ContextBar.Render(fmt.Sprintf(" %s   0%%  0 / %s", strings.Repeat("░", 20), formatTokens(m.maxInputTokens)))
	}
	pct := float64(tokens) / float64(m.maxInputTokens)
	barWidth := 20
	filled := int(pct * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}
	bar := strings.Repeat("▓", filled) + strings.Repeat("░", barWidth-filled)
	pctStr := fmt.Sprintf("%3.0f%%", pct*100)
	return m.theme.ContextBar.Render(fmt.Sprintf(" %s  %s  %s / %s", bar, pctStr, formatTokens(tokens), formatTokens(m.maxInputTokens)))
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

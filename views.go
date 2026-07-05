package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/sessions"
	"github.com/sillygru/gurtcli/ui"
)

func (m model) View() string {
	switch m.state {
	case stateWelcome:
		return m.welcomeView()
	case stateProviderPick:
		return m.providerPickView()
	case stateCustomModePick:
		return m.customModePickView()
	case stateCustomURL:
		return m.customURLView()
	case stateAPIKeyInput:
		return m.apiKeyView()
	case stateCustomName:
		return m.customNameView()
	case stateModelFetch:
		return m.modelFetchView()
	case stateModelPick:
		return m.modelPickView()
	case stateReasoningConfig:
		return m.reasoningConfigView()
	case stateError:
		return m.errorView()
	case stateManualModel:
		return m.manualModelView()
	case stateSessionPick:
		return m.sessionPickView()
	case stateChat:
		return m.chatView()
	case stateAllowManage:
		return m.allowManageView()
	}
	return ""
}

func (m model) welcomeView() string {
	return m.theme.Brand.Render("  gurt") + "\n\n" +
		m.theme.Dim.Render("  A coding agent in your terminal.") + "\n\n" +
		"  Press enter to start." + "\n" +
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
	b.WriteString("Custom endpoint mode:\n\n")
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
	b.WriteString("Enter the base URL for your custom provider:\n\n")
	b.WriteString(m.urlInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("enter confirm • ctrl+c quit"))
	return b.String()
}

func (m model) apiKeyView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Enter your API key for %s:\n\n", llm.DisplayName(m.provider))
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
	fmt.Fprintf(&b, "Fetching models from %s...\n\n", llm.DisplayName(m.provider))
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
		thinkLine := fmt.Sprintf("  Thinking:  %s", think)
		if m.reasoningField == 0 {
			thinkLine = fmt.Sprintf("  %s Thinking:  %s %s ", m.theme.Header.Render("▶"), m.theme.Header.Render(think), m.theme.Dim.Render("← →"))
		}
		b.WriteString(thinkLine)
		b.WriteString("\n")

		effort := m.effortLevel
		effortLine := fmt.Sprintf("  Effort:    %s", effort)
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
		thinkLine := fmt.Sprintf("  Thinking:  %s", think)
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
		effortLine := fmt.Sprintf("  Reasoning: %s", effort)
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
	b.WriteString(m.err.Error())
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

func (m model) customNameView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString("Name this endpoint to save for later:\n\n")
	b.WriteString(m.nameInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.theme.Dim.Render("enter confirm • ctrl+c quit"))
	return b.String()
}

func (m model) manualModelView() string {
	var b strings.Builder
	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n\n")
	b.WriteString("Enter the model name:\n\n")
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
	statusRendered := m.theme.StatusBar.Render(fmt.Sprintf("%s • %s • %s", m.sessionDisplayName(), providerLabel, m.modelName))
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

	b.WriteString(m.theme.Brand.Render("  gurt"))
	b.WriteString("\n")

	if m.updateAvailable {
		b.WriteString(m.theme.UpdateBanner.Render(fmt.Sprintf("  Update %s available — run /update", m.latestVersion)))
		b.WriteString("\n")
	}

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

	b.WriteString(m.theme.Divider.Render(strings.Repeat("─", dividerLen)))
	b.WriteString("\n")

	if m.pendingPerm != nil {
		tc := m.pendingPerm.toolCall

		boxW := m.width - 2
		if boxW < 30 {
			boxW = 30
		}
		permBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ui.ColorMauve)).
			Width(boxW).
			Padding(1, 1)

		content := ui.RenderPermissionPrompt(m.theme, tc, m.width) + "\n\n" +
			m.theme.InputPrompt.Render("  ❯ ") + m.chatInput.View()

		b.WriteString(permBox.Render(content))
	} else {
		b.WriteString(m.theme.InputPrompt.Render("  ❯ "))
		b.WriteString(m.chatInput.View())

		help := "enter send • ↑↓ scroll • ctrl+c quit"
		if m.isStreaming {
			help = "esc cancel • ctrl+c quit"
		} else if m.suggestions.active && len(m.suggestions.items) > 0 {
			b.WriteString("\n")
			for i, item := range m.suggestions.items {
				prefix := "  "
				style := m.theme.Dim
				if i == m.suggestions.selected {
					prefix = "> "
					style = m.theme.Header
				}
				b.WriteString(style.Render(prefix + "/" + item.name))
				b.WriteString(m.theme.Dim.Render("  " + item.description))
				b.WriteString("\n")
			}
			help = "↑↓ navigate • tab select • esc dismiss"
		}
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

	var left string
	if m.isStreaming && m.workingMsg != "" {
		idx := m.workingSpinnerIdx % len(workingSpinnerFrames)
		spinner := workingSpinnerFrames[idx]
		left = m.theme.WorkingStatus.Render(spinner + " " + m.workingMsg)
	}

	if left == "" && ctxBar == "" {
		return ""
	}

	leftWidth := lipgloss.Width(left)
	ctxWidth := lipgloss.Width(ctxBar)
	pad := m.width - leftWidth - ctxWidth
	if pad < 1 {
		pad = 1
	}

	var b strings.Builder
	if left != "" {
		b.WriteString(left)
	}
	if ctxBar != "" {
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(ctxBar)
	}
	return b.String()
}

func (m model) renderContextBar() string {
	if m.maxInputTokens <= 0 {
		return ""
	}
	tokens := m.inputTokens
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
	return m.theme.ContextBar.Render(fmt.Sprintf(" %s  %s  %s", bar, pctStr, formatTokens(tokens)))
}

func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%dK", n/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

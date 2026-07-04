package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sillygru/gurtcli/llm"
)

func (m model) View() string {
	switch m.state {
	case stateWelcome:
		return m.welcomeView()
	case stateProviderPick:
		return m.providerPickView()
	case stateCustomURL:
		return m.customURLView()
	case stateAPIKeyInput:
		return m.apiKeyView()
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
	case stateChat:
		return m.chatView()
	}
	return ""
}

func (m model) welcomeView() string {
	return m.styles.header.Render("gurtcli") + "\n\n" +
		m.styles.dim.Render("A coding agent in your terminal.") + "\n\n" +
		"Press enter to start." + "\n" +
		m.styles.dim.Render("ctrl+c quit")
}

func (m model) providerPickView() string {
	var b strings.Builder
	b.WriteString(m.styles.header.Render("gurtcli"))
	b.WriteString("\n\n")
	b.WriteString(m.providerList.View())
	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("↑/↓ navigate • enter select • ctrl+c quit"))
	return b.String()
}

func (m model) customURLView() string {
	var b strings.Builder
	b.WriteString(m.styles.header.Render("gurtcli"))
	b.WriteString("\n\n")
	b.WriteString("Enter the base URL for your custom provider:\n\n")
	b.WriteString(m.urlInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.styles.dim.Render("enter confirm • ctrl+c quit"))
	return b.String()
}

func (m model) apiKeyView() string {
	var b strings.Builder
	b.WriteString(m.styles.header.Render("gurtcli"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Enter your API key for %s:\n\n", llm.DisplayName(m.provider))
	if m.customURL != "" {
		b.WriteString(m.styles.dim.Render("Endpoint: " + m.customURL))
		b.WriteString("\n\n")
	}
	b.WriteString(m.keyInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.styles.dim.Render("enter confirm • ctrl+c quit"))
	return b.String()
}

func (m model) modelFetchView() string {
	var b strings.Builder
	b.WriteString(m.styles.header.Render("gurtcli"))
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "Fetching models from %s...\n\n", llm.DisplayName(m.provider))
	b.WriteString(m.spinner.View())
	b.WriteString("\n\n")
	b.WriteString(m.styles.dim.Render("ctrl+c quit"))
	return b.String()
}

func (m model) modelPickView() string {
	var b strings.Builder
	b.WriteString(m.styles.header.Render("gurtcli"))
	b.WriteString("\n\n")
	b.WriteString(m.modelList.View())
	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("Type to filter • ↑/↓ navigate • enter select • ctrl+c quit"))
	return b.String()
}

func (m model) reasoningConfigView() string {
	var b strings.Builder
	b.WriteString(m.styles.header.Render("gurtcli"))
	b.WriteString("\n\n")
	b.WriteString(m.styles.header.Render(m.modelName))
	b.WriteString("\n")
	b.WriteString(m.styles.divider.Render(strings.Repeat("─", 40)))
	b.WriteString("\n\n")

	think := m.thinkingType
	thinkLine := fmt.Sprintf("  Thinking:  %s", think)
	if m.reasoningField == 0 {
		thinkLine = fmt.Sprintf("  %s Thinking:  %s %s ", m.styles.header.Render("▶"), m.styles.header.Render(think), m.styles.dim.Render("← →"))
	}
	b.WriteString(thinkLine)
	b.WriteString("\n")

	effort := m.effortLevel
	effortLine := fmt.Sprintf("  Effort:    %s", effort)
	if m.reasoningField == 1 {
		effortLine = fmt.Sprintf("  %s Effort:    %s %s ", m.styles.header.Render("▶"), m.styles.header.Render(effort), m.styles.dim.Render("← →"))
	}
	b.WriteString(effortLine)
	b.WriteString("\n\n")

	b.WriteString(m.styles.divider.Render(strings.Repeat("─", 40)))
	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("↑/↓ navigate • ←/→ change • enter confirm • esc skip"))
	return b.String()
}

func (m model) errorView() string {
	var b strings.Builder
	b.WriteString(m.styles.header.Render("gurtcli"))
	b.WriteString("\n\n")
	b.WriteString(m.styles.err.Render("Error"))
	b.WriteString("\n\n")
	b.WriteString(m.err.Error())
	b.WriteString("\n\n")
	for i, action := range m.errorActions() {
		prefix := "  "
		if i == m.errChoice {
			prefix = "> "
		}
		style := m.styles.dim
		if i == m.errChoice {
			style = m.styles.header
		}
		b.WriteString(style.Render(prefix + action))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.styles.dim.Render("↑/↓ navigate • enter select • ctrl+c quit"))
	return b.String()
}

func (m model) manualModelView() string {
	var b strings.Builder
	b.WriteString(m.styles.header.Render("gurtcli"))
	b.WriteString("\n\n")
	b.WriteString("Enter the model name:\n\n")
	b.WriteString(m.manualInput.View())
	b.WriteString("\n\n")
	b.WriteString(m.styles.dim.Render("enter confirm • ctrl+c quit"))
	return b.String()
}

func (m model) helpWithStatus(help string) string {
	providerLabel := llm.DisplayName(m.provider)
	if m.provider == llm.ProviderCustom {
		providerLabel = "Custom"
	}
	helpRendered := m.styles.dim.Render(help)
	statusRendered := m.styles.statusBar.Render(fmt.Sprintf("%s • %s", providerLabel, m.modelName))
	pad := m.width - lipgloss.Width(helpRendered) - lipgloss.Width(statusRendered)
	if pad < 1 {
		pad = 1
	}
	return helpRendered + strings.Repeat(" ", pad) + statusRendered
}

func (m model) chatView() string {
	var b strings.Builder

	b.WriteString(m.styles.header.Render("gurtcli"))
	b.WriteString("\n")

	dividerLen := m.width
	if dividerLen < 4 {
		dividerLen = 40
	}
	b.WriteString(m.styles.divider.Render(strings.Repeat("─", dividerLen)))
	b.WriteString("\n")

	b.WriteString(m.chatViewport.View())
	b.WriteString("\n")

	b.WriteString(m.styles.divider.Render(strings.Repeat("─", dividerLen)))
	b.WriteString("\n")

	if m.pendingPerm != nil {
		tc := m.pendingPerm.toolCall

		boxW := m.width - 2
		if boxW < 30 {
			boxW = 30
		}
		permBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(cpMauve)).
			Width(boxW).
			Padding(1, 1)

		var detailBuf strings.Builder
		detailBuf.WriteString(m.styles.toolLabel.Render(fmt.Sprintf("  %s", tc.Function.Name)))
		detailBuf.WriteString("\n")
		renderToolCallArgs(&detailBuf, m, tc)

		content := "\n" +
			detailBuf.String() +
			m.styles.inputPrompt.Render("❯ ") + m.chatInput.View() + "\n" +
			m.styles.dim.Render("(y)es / (n)o / allow for (a)ll")

		b.WriteString(permBox.Render(content))
	} else {
		b.WriteString(m.styles.inputPrompt.Render("❯ "))
		b.WriteString(m.chatInput.View())

		help := "enter send • ↑↓ scroll • ctrl+c quit"
		if m.isStreaming {
			help = "esc cancel • ctrl+c quit"
		} else if m.suggestions.active && len(m.suggestions.items) > 0 {
			b.WriteString("\n")
			for i, item := range m.suggestions.items {
				prefix := "  "
				style := m.styles.dim
				if i == m.suggestions.selected {
					prefix = "> "
					style = m.styles.header
				}
				b.WriteString(style.Render(prefix + "/" + item.name))
				b.WriteString(m.styles.dim.Render("  " + item.description))
				b.WriteString("\n")
			}
			help = "↑↓ navigate • tab select • esc dismiss"
		}
		b.WriteString("\n")
		b.WriteString(m.helpWithStatus(help))
	}

	return b.String()
}

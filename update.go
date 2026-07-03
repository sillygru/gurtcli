package main

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/sillygru/gurtcli/config"
	"github.com/sillygru/gurtcli/llm"
)

var dateSuffixRegex = regexp.MustCompile(`-\d{8}$|-\d{4}-\d{2}-\d{2}$`)

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
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		switch m.state {
		case stateWelcome:
			return m.updateWelcome(msg)
		case stateProviderPick:
			return m.updateProviderPick(msg)
		case stateCustomURL:
			return m.updateCustomURL(msg)
		case stateAPIKeyInput:
			return m.updateAPIKeyInput(msg)
		case stateModelPick:
			return m.updateModelPick(msg)
		case stateError:
			return m.updateError(msg)
		case stateManualModel:
			return m.updateManualModel(msg)
		}
		return m, nil

	case spinner.TickMsg:
		if m.state == stateModelFetch {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case modelsFetchedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.errChoice = 0
			m.state = stateError
			return m, nil
		}
		// Copy so we never mutate the tea.Msg value.
		models := append([]string(nil), msg.models...)
		// For OpenAI, only show text/chat models (gpt-<digit> or gpt-o<digit>).
		// Also filter out models with date suffixes (e.g., gpt-5.5-pro-2026-04-23).
		// For Anthropic, filter out models with date suffixes.
		if m.provider == llm.ProviderOpenAI {
			filtered := models[:0]
			for _, name := range models {
				if llm.IsTextChatModel(name) && !hasDateSuffix(name) {
					filtered = append(filtered, name)
				}
			}
			models = filtered
		} else if m.provider == llm.ProviderAnthropic {
			filtered := models[:0]
			for _, name := range models {
				if !hasDateSuffix(name) {
					filtered = append(filtered, name)
				}
			}
			models = filtered
		}
		m.models = models
		items := make([]list.Item, len(models))
		for i, name := range models {
			items[i] = item{title: name}
		}
		m.modelList.SetItems(items)
		m.state = stateModelPick
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
		key, _ := config.GetAPIKey(m.provider, m.customURL)
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
				fetchModelsCmd(m.provider, m.apiKey, m.customURL),
			)
		}
		m.state = stateChat
		return m, nil
	}
	if m.provider == "" {
		m.state = stateProviderPick
		return m, nil
	}
	key, _ := config.GetAPIKey(m.provider, m.customURL)
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
			fetchModelsCmd(m.provider, m.apiKey, m.customURL),
		)
	}
	m.state = stateChat
	return m, nil
}

func (m model) updateProviderPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.providerList, cmd = m.providerList.Update(msg)

	if msg.String() != "enter" {
		return m, cmd
	}

	m.provider = providerFromIndex(m.providerList.Index())
	if m.provider == llm.ProviderCustom {
		m.state = stateCustomURL
		m.urlInput.Focus()
		return m, nil
	}

	key, _ := config.GetAPIKey(m.provider, m.customURL)
	if key != "" {
		m.apiKey = key
		if m.modelName != "" {
			m.state = stateChat
			return m, nil
		}
		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			fetchModelsCmd(m.provider, m.apiKey, m.customURL),
		)
	}

	m.state = stateAPIKeyInput
	m.keyInput.Focus()
	return m, nil
}

func (m model) updateCustomURL(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.urlInput, cmd = m.urlInput.Update(msg)

	if msg.String() != "enter" {
		return m, cmd
	}

	m.customURL = strings.TrimSpace(m.urlInput.Value())
	if m.customURL == "" {
		return m, nil
	}

	key, _ := config.GetAPIKey(m.provider, m.customURL)
	if key != "" {
		m.apiKey = key
		if m.modelName != "" {
			m.state = stateChat
			return m, nil
		}
		m.state = stateModelFetch
		return m, tea.Batch(
			m.spinner.Tick,
			fetchModelsCmd(m.provider, m.apiKey, m.customURL),
		)
	}

	m.state = stateAPIKeyInput
	m.keyInput.Focus()
	return m, nil
}

func (m model) updateAPIKeyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.keyInput, cmd = m.keyInput.Update(msg)

	if msg.String() != "enter" {
		return m, cmd
	}

	key := strings.TrimSpace(m.keyInput.Value())
	if key == "" {
		return m, nil
	}
	m.apiKey = key

	if err := config.SetAPIKey(m.provider, m.customURL, key); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}

	if m.modelName != "" {
		m.state = stateChat
		return m, nil
	}

	m.state = stateModelFetch
	return m, tea.Batch(
		m.spinner.Tick,
		fetchModelsCmd(m.provider, m.apiKey, m.customURL),
	)
}

func (m model) updateModelPick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.modelList, cmd = m.modelList.Update(msg)

	if msg.String() == "esc" {
		m.state = stateProviderPick
		return m, nil
	}
	if msg.String() != "enter" {
		return m, cmd
	}

	selected, ok := m.modelList.SelectedItem().(item)
	if !ok {
		return m, nil
	}
	m.modelName = selected.title

	if err := config.Save(&config.Config{
		Provider:      m.provider,
		Model:         m.modelName,
		CustomBaseURL: m.customURL,
	}); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}

	m.state = stateChat
	return m, nil
}

func (m model) updateError(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up":
		m.errChoice--
		if m.errChoice < 0 {
			m.errChoice = len(errorActions) - 1
		}
	case "down":
		m.errChoice++
		if m.errChoice >= len(errorActions) {
			m.errChoice = 0
		}
	case "enter":
		switch errorAction(m.errChoice) {
		case errorRetry:
			m.state = stateModelFetch
			return m, tea.Batch(
				m.spinner.Tick,
				fetchModelsCmd(m.provider, m.apiKey, m.customURL),
			)
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
	}
	return m, nil
}

func (m model) updateManualModel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.manualInput, cmd = m.manualInput.Update(msg)

	if msg.String() != "enter" {
		return m, cmd
	}

	name := strings.TrimSpace(m.manualInput.Value())
	if name == "" {
		return m, nil
	}
	m.modelName = name

	if err := config.Save(&config.Config{
		Provider:      m.provider,
		Model:         m.modelName,
		CustomBaseURL: m.customURL,
	}); err != nil {
		m.err = err
		m.errChoice = 0
		m.state = stateError
		return m, nil
	}

	m.state = stateChat
	return m, nil
}

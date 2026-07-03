package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type state int

const (
	stateWelcome state = iota
	stateModelPick
	stateChat
)

type model struct {
	state    state
	yolo     bool
	modelArg string
	styles   styles
}

type styles struct {
	header lipgloss.Style
	dim    lipgloss.Style
}

func initialModel(yolo bool, modelArg string) model {
	return model{
		state:    stateWelcome,
		yolo:     yolo,
		modelArg: modelArg,
		styles: styles{
			header: lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				Padding(0, 1),
			dim: lipgloss.NewStyle().
				Foreground(lipgloss.Color("240")),
		},
	}
}

func (m model) Init() tea.Cmd {
	if m.modelArg != "" {
		m.state = stateChat
		return nil
	}
	return tea.WindowSize()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.state == stateWelcome {
				m.state = stateModelPick
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	switch m.state {
	case stateWelcome:
		return m.welcomeView()
	case stateModelPick:
		return m.modelPickView()
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

func (m model) modelPickView() string {
	return m.styles.header.Render("Pick a model") + "\n\n" +
		"(picker goes here)" + "\n\n" +
		m.styles.dim.Render("ctrl+c quit")
}

func (m model) chatView() string {
	return m.styles.header.Render("gurtcli") + "\n" +
		"─"+strings.Repeat("─", 40)+"\n\n"+
		"Chat interface goes here." + "\n\n" +
		"> _"
}

func main() {
	yolo := flag.Bool("yolo", false, "skip permission prompts")
	dangerous := flag.Bool("dangerously-skip-permissions", false, "skip permission prompts")
	modelArg := flag.String("model", "", "model to use")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(VersionString())
		os.Exit(0)
	}

	skipPerms := *yolo || *dangerous

	p := tea.NewProgram(
		initialModel(skipPerms, *modelArg),
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

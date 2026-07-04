package ui

import "github.com/charmbracelet/lipgloss"

// Catppuccin Mocha palette
const (
	ColorMauve    = "#cba6f7"
	ColorLavender = "#b4befe"
	ColorPink     = "#f5c2e7"
	ColorBlue     = "#89b4fa"
	ColorGreen    = "#a6e3a1"
	ColorYellow   = "#f9e2af"
	ColorPeach    = "#fab387"
	ColorRed      = "#f38ba8"
	ColorTeal     = "#94e2d5"
	ColorText     = "#cdd6f4"
	ColorSubtext1 = "#bac2de"
	ColorSubtext0 = "#a6adc8"
	ColorOverlay2 = "#9399b2"
	ColorOverlay1 = "#7f849c"
	ColorOverlay0 = "#6c7086"
	ColorSurface2 = "#585b70"
	ColorSurface1 = "#45475a"
	ColorSurface0 = "#313244"
	ColorBase     = "#1e1e2e"
	ColorCrust    = "#181825"
)

// ToolAccent holds display metadata for a tool type.
type ToolAccent struct {
	Icon  string
	Color string
	Label string
}

// Theme holds all lipgloss styles for the TUI.
type Theme struct {
	Brand            lipgloss.Style
	Dim              lipgloss.Style
	Muted            lipgloss.Style
	Error            lipgloss.Style
	ReasoningToggle  lipgloss.Style
	ReasoningContent lipgloss.Style
	Divider          lipgloss.Style
	UserLabel        lipgloss.Style
	UserContent      lipgloss.Style
	AssistantLabel   lipgloss.Style
	AssistantContent lipgloss.Style
	InputPrompt      lipgloss.Style
	DiffAdd          lipgloss.Style
	DiffAddHighlight lipgloss.Style
	DiffDel          lipgloss.Style
	DiffDelHighlight lipgloss.Style
	DiffContext      lipgloss.Style
	ReasoningHeader  lipgloss.Style
	ReasoningBox     lipgloss.Style
	ReasoningText    lipgloss.Style
	PermPrompt       lipgloss.Style
	PermKey          lipgloss.Style
	StatusBar        lipgloss.Style
	ContextBar       lipgloss.Style
	ToolPath         lipgloss.Style
	ToolMeta         lipgloss.Style
	ToolCode         lipgloss.Style
	ToolResultOK     lipgloss.Style
	ToolResultErr    lipgloss.Style
	ToolResultBody   lipgloss.Style
	EmptyState       lipgloss.Style
	UpdateBanner     lipgloss.Style

	// Legacy aliases used across views
	Header    lipgloss.Style
	ToolLabel lipgloss.Style
}

var toolAccents = map[string]ToolAccent{
	"read_file":   {Icon: "◈", Color: ColorBlue, Label: "Read"},
	"write_file":  {Icon: "✎", Color: ColorGreen, Label: "Write"},
	"edit_file":   {Icon: "◆", Color: ColorYellow, Label: "Edit"},
	"delete_file": {Icon: "✕", Color: ColorRed, Label: "Delete"},
	"run_bash":    {Icon: "$", Color: ColorLavender, Label: "Shell"},
}

// ToolAccentFor returns display metadata for a tool name.
func ToolAccentFor(name string) ToolAccent {
	if a, ok := toolAccents[name]; ok {
		return a
	}
	return ToolAccent{Icon: "⚙", Color: ColorOverlay1, Label: name}
}

// DefaultTheme returns the default gurtcli theme.
func DefaultTheme() Theme {
	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorMauve))
	return Theme{
		Brand:            brand,
		Header:           brand,
		Dim:              lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOverlay1)),
		Muted:            lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOverlay0)),
		Error:            lipgloss.NewStyle().Foreground(lipgloss.Color(ColorRed)).Bold(true),
		ReasoningToggle: lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtext1)).Italic(true),
		ReasoningHeader: lipgloss.NewStyle().Foreground(lipgloss.Color(ColorLavender)).Bold(true),
		ReasoningBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ColorSurface2)).
			Padding(0, 1).
			Margin(0, 0, 1, 0),
		ReasoningText: lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOverlay0)).Padding(0, 1),
		ReasoningContent: lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOverlay0)).Padding(0, 2),
		Divider:          lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSurface2)),
		UserLabel:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorLavender)),
		UserContent:      lipgloss.NewStyle().Foreground(lipgloss.Color(ColorText)).PaddingLeft(2),
		AssistantLabel:   brand,
		AssistantContent: lipgloss.NewStyle().Foreground(lipgloss.Color(ColorText)).PaddingLeft(2),
		InputPrompt:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorMauve)),
		ToolLabel:        lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color(ColorPink)),
		DiffAdd: lipgloss.NewStyle().
			Background(lipgloss.Color(ColorDiffAddBG)).
			Foreground(lipgloss.Color(ColorDiffAddText)),
		DiffAddHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color(ColorDiffAddHighlight)).
			Foreground(lipgloss.Color(ColorDiffAddChange)).
			Bold(true),
		DiffDel: lipgloss.NewStyle().
			Background(lipgloss.Color(ColorDiffDelBG)).
			Foreground(lipgloss.Color(ColorDiffDelText)),
		DiffDelHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color(ColorDiffDelHighlight)).
			Foreground(lipgloss.Color(ColorDiffDelChange)).
			Bold(true),
		DiffContext: lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOverlay1)).Italic(true),
		PermPrompt:       lipgloss.NewStyle().Foreground(lipgloss.Color(ColorText)),
		PermKey:          lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorMauve)),
		StatusBar:        lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtext0)),
		ContextBar:       lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOverlay0)).Padding(0, 1),
		ToolPath:         lipgloss.NewStyle().Foreground(lipgloss.Color(ColorBlue)).Bold(true),
		ToolMeta:         lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOverlay2)),
		ToolCode:         lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSubtext0)).Background(lipgloss.Color(ColorSurface0)).Padding(0, 1),
		ToolResultOK:     lipgloss.NewStyle().Foreground(lipgloss.Color(ColorGreen)),
		ToolResultErr:    lipgloss.NewStyle().Foreground(lipgloss.Color(ColorRed)),
		ToolResultBody:   lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOverlay1)).PaddingLeft(4),
		EmptyState:       lipgloss.NewStyle().Foreground(lipgloss.Color(ColorOverlay0)).Italic(true).PaddingLeft(2),
		UpdateBanner:     lipgloss.NewStyle().Foreground(lipgloss.Color(ColorPeach)).PaddingLeft(1),
	}
}

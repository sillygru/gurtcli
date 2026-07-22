package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Palette holds raw hex colors for a theme.
type Palette struct {
	Base    string
	Crust   string
	Text    string
	Subtext string
	Mauve   string
	Lavender string
	Pink    string
	Blue    string
	Green   string
	Yellow  string
	Peach   string
	Red     string
	Teal    string
	Overlay0 string
	Overlay1 string
	Overlay2 string
	Surface0 string
	Surface1 string
	Surface2 string
	DiffAddBG     string
	DiffAddHiBG   string
	DiffDelBG     string
	DiffDelHiBG   string
}

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
	UserBox          lipgloss.Style
	UserBoxLabel     lipgloss.Style
	AssistantLabel   lipgloss.Style
	AssistantContent lipgloss.Style
	InputPrompt      lipgloss.Style
	DiffAdd          lipgloss.Style
	DiffAddHighlight lipgloss.Style
	DiffDel          lipgloss.Style
	DiffDelHighlight lipgloss.Style
	DiffContext      lipgloss.Style
	DiffPanelLabel   lipgloss.Style
	DiffEmptyLine    lipgloss.Style
	ReasoningHeader  lipgloss.Style
	ReasoningGutter  lipgloss.Style
	ReasoningText    lipgloss.Style
	PermPrompt       lipgloss.Style
	PermKey          lipgloss.Style
	StatusBar        lipgloss.Style
	ContextBar       lipgloss.Style
	ToolPath         lipgloss.Style
	ToolMeta         lipgloss.Style
	ToolTitle        lipgloss.Style
	ToolCode         lipgloss.Style
	ToolResultOK     lipgloss.Style
	ToolResultErr    lipgloss.Style
	ToolResultBody   lipgloss.Style
	EmptyState       lipgloss.Style
	UpdateBanner     lipgloss.Style
	WorkingStatus    lipgloss.Style
	QueuedMessage    lipgloss.Style
	Toast            lipgloss.Style
	TableBorder      lipgloss.Style
	TableHeader      lipgloss.Style
	TableCell        lipgloss.Style
	FileRef          lipgloss.Style
	CmdRef           lipgloss.Style
	Heading1         lipgloss.Style
	Heading2         lipgloss.Style
	Heading3         lipgloss.Style
	Bold             lipgloss.Style
	Italic           lipgloss.Style
	InlineCode       lipgloss.Style
	CodeBlock        lipgloss.Style
	ListBullet       lipgloss.Style
	ListNumber       lipgloss.Style
	Rule             lipgloss.Style
	CheckboxOn       lipgloss.Style
	CheckboxOff      lipgloss.Style

	// Legacy aliases used across views
	Header    lipgloss.Style
	ToolLabel lipgloss.Style

	// Base palette colors (for ad-hoc style construction outside Theme styles)
	Base      string
	Crust     string
	Text      string
	Subtext   string
	Mauve     string
	Blue      string
	Green     string
	Yellow    string
	Red       string
	Lavender  string
	Peach     string
	Teal      string
	Pink      string
	Overlay0  string
	Overlay1  string
	Overlay2  string
	Surface0  string
	Surface1  string
	Surface2  string
}

// ThemeEntry describes an available theme.
type ThemeEntry struct {
	Name    string
	NewFunc func() Theme
}

// ThemeRegistry lists every theme available to the user.
var ThemeRegistry = []ThemeEntry{
	{Name: "Catppuccin Mocha", NewFunc: CatppuccinMochaTheme},
	{Name: "Tokyo Night", NewFunc: TokyoNightTheme},
	{Name: "Gruvbox", NewFunc: GruvboxTheme},
	{Name: "One Dark", NewFunc: OneDarkTheme},
}

// ToolAccentFor returns display metadata for a tool name using the theme's colors.
func (t Theme) ToolAccentFor(name string) ToolAccent {
	switch name {
	case "read_file":
		return ToolAccent{Icon: "◈", Color: t.Blue, Label: "Read"}
	case "write_file":
		return ToolAccent{Icon: "✎", Color: t.Green, Label: "Write"}
	case "edit_file":
		return ToolAccent{Icon: "◆", Color: t.Yellow, Label: "Edit"}
	case "delete_file":
		return ToolAccent{Icon: "✕", Color: t.Red, Label: "Delete"}
	case "run_bash":
		return ToolAccent{Icon: "$", Color: t.Lavender, Label: "Shell"}
	default:
		return ToolAccent{Icon: "⚙", Color: t.Overlay1, Label: name}
	}
}

// DefaultTheme returns the default (Catppuccin Mocha) theme.
func DefaultTheme() Theme {
	return CatppuccinMochaTheme()
}

func buildTheme(p Palette) Theme {
	base := lipgloss.Color(p.Base)
	surf0 := lipgloss.Color(p.Surface0)
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Background(base).Foreground(c) }
	fgSurf := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Background(surf0).Foreground(c) }

	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.Mauve)).Background(base)

	return Theme{
		Brand:            brand,
		Header:           brand,
		Dim:              fg(lipgloss.Color(p.Overlay1)),
		Muted:            fg(lipgloss.Color(p.Overlay0)),
		Error:            fg(lipgloss.Color(p.Red)).Bold(true),
		ReasoningToggle:  fg(lipgloss.Color(p.Subtext)).Italic(true),
		ReasoningHeader:  fg(lipgloss.Color(p.Lavender)).Bold(true),
		// Surface2 is what the block's border used to be drawn in, so the rule
		// reads as the same faint structure without boxing the content.
		ReasoningGutter:  fg(lipgloss.Color(p.Surface2)),
		ReasoningText:    fg(lipgloss.Color(p.Overlay0)),
		ReasoningContent: fg(lipgloss.Color(p.Overlay0)).Padding(0, 2),
		Divider:          fg(lipgloss.Color(p.Surface2)),
		Rule: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Surface2)).
			Background(base),
		UserLabel:   fg(lipgloss.Color(p.Lavender)).Bold(true),
		UserContent: fgSurf(lipgloss.Color(p.Text)),
		UserBox: lipgloss.NewStyle().
			Background(surf0).
			Padding(0, 1).
			Margin(0, 0, 1, 0),
		UserBoxLabel: fgSurf(lipgloss.Color(p.Lavender)).Bold(true),
		AssistantLabel:   brand,
		AssistantContent: fg(lipgloss.Color(p.Text)),
		InputPrompt:      fg(lipgloss.Color(p.Mauve)).Bold(true),
		ToolLabel:        fg(lipgloss.Color(p.Pink)).Italic(true),
		ToolTitle:        fg(lipgloss.Color(p.Mauve)).Bold(true),
		DiffAdd: lipgloss.NewStyle().
			Background(lipgloss.Color(p.DiffAddBG)).
			Foreground(lipgloss.Color(p.Text)),
		DiffAddHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color(p.DiffAddHiBG)).
			Foreground(lipgloss.Color(p.Green)).
			Bold(true),
		DiffDel: lipgloss.NewStyle().
			Background(lipgloss.Color(p.DiffDelBG)).
			Foreground(lipgloss.Color(p.Text)),
		DiffDelHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color(p.DiffDelHiBG)).
			Foreground(lipgloss.Color(p.Red)).
			Bold(true),
		DiffContext:    fg(lipgloss.Color(p.Overlay1)).Italic(true),
		DiffPanelLabel: fg(lipgloss.Color(p.Overlay2)).Bold(true),
		DiffEmptyLine:  fg(lipgloss.Color(p.Surface1)).Italic(true),
		PermPrompt:     fg(lipgloss.Color(p.Text)),
		PermKey:        fg(lipgloss.Color(p.Mauve)).Bold(true),
		StatusBar:      fg(lipgloss.Color(p.Subtext)),
		ContextBar:     fg(lipgloss.Color(p.Overlay0)).Padding(0, 1),
		ToolPath:       fg(lipgloss.Color(p.Blue)).Bold(true),
		ToolMeta:       fg(lipgloss.Color(p.Overlay2)),
		ToolCode: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Subtext)).
			Background(lipgloss.Color(p.Surface0)).
			Padding(0, 1),
		ToolResultOK:   fg(lipgloss.Color(p.Green)),
		ToolResultErr:  fg(lipgloss.Color(p.Red)),
		ToolResultBody: fg(lipgloss.Color(p.Overlay1)).PaddingLeft(4),
		EmptyState:     fg(lipgloss.Color(p.Overlay0)).Italic(true).PaddingLeft(2),
		UpdateBanner:   fg(lipgloss.Color(p.Peach)).PaddingLeft(1),
		WorkingStatus:  fg(lipgloss.Color(p.Teal)),
		QueuedMessage:  fg(lipgloss.Color(p.Yellow)).Italic(true),
		Toast: lipgloss.NewStyle().
			Background(lipgloss.Color(p.Green)).
			Foreground(lipgloss.Color(p.Crust)).
			Bold(true).
			Padding(0, 1),
		TableBorder: fg(lipgloss.Color(p.Surface2)),
		TableHeader: fg(lipgloss.Color(p.Mauve)).Bold(true),
		TableCell:   fg(lipgloss.Color(p.Text)),
		FileRef:     fg(lipgloss.Color(p.Mauve)).Bold(true),
		CmdRef:      fg(lipgloss.Color(p.Mauve)).Bold(true),
		Heading1: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Mauve)).
			Background(base).
			Bold(true),
		Heading2: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Lavender)).
			Background(base).
			Bold(true),
		Heading3: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Blue)).
			Background(base).
			Bold(true),
		Bold: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Text)).
			Background(base).
			Bold(true),
		Italic: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Subtext)).
			Background(base).
			Italic(true),
		InlineCode: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Peach)).
			Background(lipgloss.Color(p.Surface0)).
			Padding(0, 1),
		CodeBlock: lipgloss.NewStyle().
			Foreground(lipgloss.Color(p.Subtext)).
			Background(lipgloss.Color(p.Surface0)).
			Padding(0, 1),
		ListBullet: fg(lipgloss.Color(p.Mauve)).Bold(true),
		ListNumber: fg(lipgloss.Color(p.Mauve)).Bold(true),
		CheckboxOn:  fg(lipgloss.Color(p.Green)).Bold(true),
		CheckboxOff: fg(lipgloss.Color(p.Overlay1)),

		Base:     p.Base,
		Crust:    p.Crust,
		Text:     p.Text,
		Subtext:  p.Subtext,
		Mauve:    p.Mauve,
		Blue:     p.Blue,
		Green:    p.Green,
		Yellow:   p.Yellow,
		Red:      p.Red,
		Lavender: p.Lavender,
		Peach:    p.Peach,
		Teal:     p.Teal,
		Pink:     p.Pink,
		Overlay0: p.Overlay0,
		Overlay1: p.Overlay1,
		Overlay2: p.Overlay2,
		Surface0: p.Surface0,
		Surface1: p.Surface1,
		Surface2: p.Surface2,
	}
}

// CatppuccinMochaTheme returns the dark Catppuccin Mocha theme.
func CatppuccinMochaTheme() Theme {
	return buildTheme(Palette{
		Base: "#1e1e2e", Crust: "#181825", Text: "#cdd6f4", Subtext: "#bac2de",
		Mauve: "#cba6f7", Lavender: "#b4befe", Pink: "#f5c2e7", Blue: "#89b4fa",
		Green: "#a6e3a1", Yellow: "#f9e2af", Peach: "#fab387", Red: "#f38ba8", Teal: "#94e2d5",
		Overlay0: "#6c7086", Overlay1: "#7f849c", Overlay2: "#9399b2",
		Surface0: "#313244", Surface1: "#45475a", Surface2: "#585b70",
		DiffAddBG: "#1a2e28", DiffAddHiBG: "#2a4538", DiffDelBG: "#3d2028", DiffDelHiBG: "#6e3040",
	})
}

// TokyoNightTheme returns the dark Tokyo Night theme.
func TokyoNightTheme() Theme {
	return buildTheme(Palette{
		Base: "#1a1b26", Crust: "#16161e", Text: "#c0caf5", Subtext: "#a9b1d6",
		Mauve: "#bb9af7", Lavender: "#c0caf5", Pink: "#f7768e", Blue: "#7aa2f7",
		Green: "#9ece6a", Yellow: "#e0af68", Peach: "#ff9e64", Red: "#f7768e", Teal: "#1abc9c",
		Overlay0: "#2f354a", Overlay1: "#3b4261", Overlay2: "#565f89",
		Surface0: "#24283b", Surface1: "#2f354a", Surface2: "#363b54",
		DiffAddBG: "#1f3a2b", DiffAddHiBG: "#2c4d39", DiffDelBG: "#3a1f2b", DiffDelHiBG: "#4d2c39",
	})
}

// GruvboxTheme returns the Gruvbox dark theme.
func GruvboxTheme() Theme {
	return buildTheme(Palette{
		Base: "#282828", Crust: "#1d2021", Text: "#ebdbb2", Subtext: "#d5c4a1",
		Mauve: "#d3869b", Lavender: "#83a598", Pink: "#fb4934", Blue: "#83a598",
		Green: "#b8bb26", Yellow: "#fabd2f", Peach: "#fe8019", Red: "#fb4934", Teal: "#8ec07c",
		Overlay0: "#3c3836", Overlay1: "#504945", Overlay2: "#928374",
		Surface0: "#333030", Surface1: "#3c3836", Surface2: "#504945",
		DiffAddBG: "#2e3328", DiffAddHiBG: "#434f38", DiffDelBG: "#3d2828", DiffDelHiBG: "#543333",
	})
}

// OneDarkTheme returns the One Dark (Atom) theme.
func OneDarkTheme() Theme {
	return buildTheme(Palette{
		Base: "#282c34", Crust: "#21252b", Text: "#abb2bf", Subtext: "#8a8fa8",
		Mauve: "#c678dd", Lavender: "#61afef", Pink: "#e06c75", Blue: "#61afef",
		Green: "#98c379", Yellow: "#e5c07b", Peach: "#d19a66", Red: "#e06c75", Teal: "#56b6c2",
		Overlay0: "#333842", Overlay1: "#3e4451", Overlay2: "#5c6370",
		Surface0: "#2c313a", Surface1: "#333842", Surface2: "#3e4451",
		DiffAddBG: "#2a3a2a", DiffAddHiBG: "#3a5a3a", DiffDelBG: "#3a2a2c", DiffDelHiBG: "#5a3a3c",
	})
}

// ThemeByName returns the theme matching name, or the default if not found.
func ThemeByName(name string) Theme {
	for _, e := range ThemeRegistry {
		if e.Name == name {
			return e.NewFunc()
		}
	}
	return DefaultTheme()
}

// RenderRule renders a subtle centered horizontal rule.
func RenderRule(t Theme, layout Layout) string {
	w := layout.RuleWidth()
	pad := (layout.ContentWidth() - w) / 2
	if pad < 0 {
		pad = 0
	}
	rule := t.Rule.Render(strings.Repeat("─", w))
	return strings.Repeat(" ", pad) + rule
}

// RenderScreenHeader renders a screen title with optional subtitle.
func RenderScreenHeader(t Theme, title, subtitle string) string {
	var b strings.Builder
	b.WriteString(t.Brand.Render("  " + title))
	if subtitle != "" {
		b.WriteString("\n")
		b.WriteString(t.Dim.Render("  " + subtitle))
	}
	return b.String()
}

// RenderFooterHelp renders dim footer help text.
func RenderFooterHelp(t Theme, text string) string {
	return t.Dim.Render(text)
}

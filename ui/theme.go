package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

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
	WorkingStatus    lipgloss.Style
	QueuedMessage    lipgloss.Style
	Toast            lipgloss.Style
	TableBorder      lipgloss.Style
	TableHeader      lipgloss.Style
	TableCell        lipgloss.Style
	FileRef          lipgloss.Style
	CmdRef           lipgloss.Style

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

// CatppuccinMochaTheme returns the dark Catppuccin Mocha theme.
func CatppuccinMochaTheme() Theme {
	base := lipgloss.Color(ColorBase)
	bg := func() lipgloss.Style { return lipgloss.NewStyle().Background(base) }
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Background(base).Foreground(c) }

	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(ColorMauve)).Background(base)
	return Theme{
		Brand:            brand,
		Header:           brand,
		Dim:              fg(lipgloss.Color(ColorOverlay1)),
		Muted:            fg(lipgloss.Color(ColorOverlay0)),
		Error:            fg(lipgloss.Color(ColorRed)).Bold(true),
		ReasoningToggle:  fg(lipgloss.Color(ColorSubtext1)).Italic(true),
		ReasoningHeader:  fg(lipgloss.Color(ColorLavender)).Bold(true),
		ReasoningBox: bg().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(ColorSurface2)).
			Padding(0, 1).
			Margin(0, 0, 1, 0),
		ReasoningText:    fg(lipgloss.Color(ColorOverlay0)).Padding(0, 1),
		ReasoningContent: fg(lipgloss.Color(ColorOverlay0)).Padding(0, 2),
		Divider:          fg(lipgloss.Color(ColorSurface2)),
		UserLabel:        fg(lipgloss.Color(ColorLavender)).Bold(true),
		UserContent:      fg(lipgloss.Color(ColorText)),
		AssistantLabel:   brand,
		AssistantContent: fg(lipgloss.Color(ColorText)),
		InputPrompt:      fg(lipgloss.Color(ColorMauve)).Bold(true),
		ToolLabel:        fg(lipgloss.Color(ColorPink)).Italic(true),
		DiffAdd: lipgloss.NewStyle().
			Background(lipgloss.Color("#1a2e28")).
			Foreground(lipgloss.Color("#cdd6f4")),
		DiffAddHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color("#2a4538")).
			Foreground(lipgloss.Color("#a6e3a1")).
			Bold(true),
		DiffDel: lipgloss.NewStyle().
			Background(lipgloss.Color("#3d2028")).
			Foreground(lipgloss.Color("#cdd6f4")),
		DiffDelHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color("#6e3040")).
			Foreground(lipgloss.Color("#f38ba8")).
			Bold(true),
		DiffContext: fg(lipgloss.Color(ColorOverlay1)).Italic(true),
		PermPrompt:  fg(lipgloss.Color(ColorText)),
		PermKey:     fg(lipgloss.Color(ColorMauve)).Bold(true),
		StatusBar:   fg(lipgloss.Color(ColorSubtext0)),
		ContextBar:  fg(lipgloss.Color(ColorOverlay0)).Padding(0, 1),
		ToolPath:    fg(lipgloss.Color(ColorBlue)).Bold(true),
		ToolMeta:    fg(lipgloss.Color(ColorOverlay2)),
		ToolCode: lipgloss.NewStyle().
			Foreground(lipgloss.Color(ColorSubtext0)).
			Background(lipgloss.Color(ColorSurface0)).
			Padding(0, 1),
		ToolResultOK:   fg(lipgloss.Color(ColorGreen)),
		ToolResultErr:  fg(lipgloss.Color(ColorRed)),
		ToolResultBody: fg(lipgloss.Color(ColorOverlay1)).PaddingLeft(4),
		EmptyState:     fg(lipgloss.Color(ColorOverlay0)).Italic(true).PaddingLeft(2),
		UpdateBanner:   fg(lipgloss.Color(ColorPeach)).PaddingLeft(1),
		WorkingStatus:  fg(lipgloss.Color(ColorTeal)),
		QueuedMessage:  fg(lipgloss.Color(ColorYellow)).Italic(true),
		Toast: lipgloss.NewStyle().
			Background(lipgloss.Color(ColorGreen)).
			Foreground(lipgloss.Color(ColorCrust)).
			Bold(true).
			Padding(0, 1),
		TableBorder: fg(lipgloss.Color(ColorSurface2)),
		TableHeader: fg(lipgloss.Color(ColorSubtext1)).Bold(true),
		TableCell:   fg(lipgloss.Color(ColorText)),
		FileRef:     fg(lipgloss.Color(ColorMauve)).Bold(true),
		CmdRef:      fg(lipgloss.Color(ColorMauve)).Bold(true),

		Base:      ColorBase,
		Crust:     ColorCrust,
		Text:      ColorText,
		Subtext:   ColorSubtext1,
		Mauve:     ColorMauve,
		Blue:      ColorBlue,
		Green:     ColorGreen,
		Yellow:    ColorYellow,
		Red:       ColorRed,
		Lavender:  ColorLavender,
		Peach:     ColorPeach,
		Teal:      ColorTeal,
		Pink:      ColorPink,
		Overlay0:  ColorOverlay0,
		Overlay1:  ColorOverlay1,
		Overlay2:  ColorOverlay2,
		Surface0:  ColorSurface0,
		Surface1:  ColorSurface1,
		Surface2:  ColorSurface2,
	}
}

// TokyoNightTheme returns the dark Tokyo Night theme.
func TokyoNightTheme() Theme {
	const (
		base    = "#1a1b26"
		crust   = "#16161e"
		text    = "#c0caf5"
		subtext = "#a9b1d6"
		muted   = "#9aa5ce"
		mauve   = "#bb9af7"
		lav     = "#c0caf5"
		pink    = "#f7768e"
		blue    = "#7aa2f7"
		green   = "#9ece6a"
		yellow  = "#e0af68"
		peach   = "#ff9e64"
		red     = "#f7768e"
		teal    = "#1abc9c"
		ov2     = "#565f89"
		ov1     = "#3b4261"
		ov0     = "#2f354a"
		surf2   = "#363b54"
		surf1   = "#2f354a"
		surf0   = "#24283b"
	)

	bc := lipgloss.Color(base)
	bg := func() lipgloss.Style { return lipgloss.NewStyle().Background(bc) }
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Background(bc).Foreground(c) }

	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(mauve)).Background(bc)
	return Theme{
		Brand:            brand,
		Header:           brand,
		Dim:              fg(lipgloss.Color(ov1)),
		Muted:            fg(lipgloss.Color(ov0)),
		Error:            fg(lipgloss.Color(red)).Bold(true),
		ReasoningToggle:  fg(lipgloss.Color(subtext)).Italic(true),
		ReasoningHeader:  fg(lipgloss.Color(lav)).Bold(true),
		ReasoningBox: bg().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(surf2)).
			Padding(0, 1).
			Margin(0, 0, 1, 0),
		ReasoningText:    fg(lipgloss.Color(ov0)).Padding(0, 1),
		ReasoningContent: fg(lipgloss.Color(ov0)).Padding(0, 2),
		Divider:          fg(lipgloss.Color(surf2)),
		UserLabel:        fg(lipgloss.Color(lav)).Bold(true),
		UserContent:      fg(lipgloss.Color(text)),
		AssistantLabel:   brand,
		AssistantContent: fg(lipgloss.Color(text)),
		InputPrompt:      fg(lipgloss.Color(mauve)).Bold(true),
		ToolLabel:        fg(lipgloss.Color(pink)).Italic(true),
		DiffAdd: lipgloss.NewStyle().
			Background(lipgloss.Color("#1f3a2b")).
			Foreground(lipgloss.Color(text)),
		DiffAddHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color("#2c4d39")).
			Foreground(lipgloss.Color(green)).
			Bold(true),
		DiffDel: lipgloss.NewStyle().
			Background(lipgloss.Color("#3a1f2b")).
			Foreground(lipgloss.Color(text)),
		DiffDelHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color("#4d2c39")).
			Foreground(lipgloss.Color(red)).
			Bold(true),
		DiffContext: fg(lipgloss.Color(ov1)).Italic(true),
		PermPrompt:  fg(lipgloss.Color(text)),
		PermKey:     fg(lipgloss.Color(mauve)).Bold(true),
		StatusBar:   fg(lipgloss.Color(muted)),
		ContextBar:  fg(lipgloss.Color(ov0)).Padding(0, 1),
		ToolPath:    fg(lipgloss.Color(blue)).Bold(true),
		ToolMeta:    fg(lipgloss.Color(ov2)),
		ToolCode: lipgloss.NewStyle().
			Foreground(lipgloss.Color(subtext)).
			Background(lipgloss.Color(surf0)).
			Padding(0, 1),
		ToolResultOK:   fg(lipgloss.Color(green)),
		ToolResultErr:  fg(lipgloss.Color(red)),
		ToolResultBody: fg(lipgloss.Color(ov1)).PaddingLeft(4),
		EmptyState:     fg(lipgloss.Color(ov0)).Italic(true).PaddingLeft(2),
		UpdateBanner:   fg(lipgloss.Color(peach)).PaddingLeft(1),
		WorkingStatus:  fg(lipgloss.Color(teal)),
		QueuedMessage:  fg(lipgloss.Color(yellow)).Italic(true),
		Toast: lipgloss.NewStyle().
			Background(lipgloss.Color(green)).
			Foreground(lipgloss.Color(crust)).
			Bold(true).
			Padding(0, 1),
		TableBorder: fg(lipgloss.Color(surf2)),
		TableHeader: fg(lipgloss.Color(subtext)).Bold(true),
		TableCell:   fg(lipgloss.Color(text)),
		FileRef:     fg(lipgloss.Color(mauve)).Bold(true),
		CmdRef:      fg(lipgloss.Color(mauve)).Bold(true),

		Base:      base,
		Crust:     crust,
		Text:      text,
		Subtext:   subtext,
		Mauve:     mauve,
		Blue:      blue,
		Green:     green,
		Yellow:    yellow,
		Red:       red,
		Lavender:  lav,
		Peach:     peach,
		Teal:      teal,
		Pink:      pink,
		Overlay0:  ov0,
		Overlay1:  ov1,
		Overlay2:  ov2,
		Surface0:  surf0,
		Surface1:  surf1,
		Surface2:  surf2,
	}
}

// GruvboxTheme returns the Gruvbox dark theme.
func GruvboxTheme() Theme {
	const (
		base    = "#282828"
		crust   = "#1d2021"
		text    = "#ebdbb2"
		subtext = "#d5c4a1"
		muted   = "#928374"
		mauve   = "#d3869b"
		lav     = "#83a598"
		pink    = "#fb4934"
		blue    = "#83a598"
		green   = "#b8bb26"
		yellow  = "#fabd2f"
		peach   = "#fe8019"
		red     = "#fb4934"
		teal    = "#8ec07c"
		ov2     = "#928374"
		ov1     = "#504945"
		ov0     = "#3c3836"
		surf2   = "#504945"
		surf1   = "#3c3836"
		surf0   = "#333030"
	)

	bc := lipgloss.Color(base)
	bg := func() lipgloss.Style { return lipgloss.NewStyle().Background(bc) }
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Background(bc).Foreground(c) }

	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(yellow)).Background(bc)
	return Theme{
		Brand:            brand,
		Header:           brand,
		Dim:              fg(lipgloss.Color(ov1)),
		Muted:            fg(lipgloss.Color(ov0)),
		Error:            fg(lipgloss.Color(red)).Bold(true),
		ReasoningToggle:  fg(lipgloss.Color(subtext)).Italic(true),
		ReasoningHeader:  fg(lipgloss.Color(peach)).Bold(true),
		ReasoningBox: bg().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(surf2)).
			Padding(0, 1).
			Margin(0, 0, 1, 0),
		ReasoningText:    fg(lipgloss.Color(ov0)).Padding(0, 1),
		ReasoningContent: fg(lipgloss.Color(ov0)).Padding(0, 2),
		Divider:          fg(lipgloss.Color(surf2)),
		UserLabel:        fg(lipgloss.Color(blue)).Bold(true),
		UserContent:      fg(lipgloss.Color(text)),
		AssistantLabel:   brand,
		AssistantContent: fg(lipgloss.Color(text)),
		InputPrompt:      fg(lipgloss.Color(green)).Bold(true),
		ToolLabel:        fg(lipgloss.Color(pink)).Italic(true),
		DiffAdd: lipgloss.NewStyle().
			Background(lipgloss.Color("#2e3328")).
			Foreground(lipgloss.Color(text)),
		DiffAddHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color("#434f38")).
			Foreground(lipgloss.Color(green)).
			Bold(true),
		DiffDel: lipgloss.NewStyle().
			Background(lipgloss.Color("#3d2828")).
			Foreground(lipgloss.Color(text)),
		DiffDelHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color("#543333")).
			Foreground(lipgloss.Color(red)).
			Bold(true),
		DiffContext: fg(lipgloss.Color(ov1)).Italic(true),
		PermPrompt:  fg(lipgloss.Color(text)),
		PermKey:     fg(lipgloss.Color(yellow)).Bold(true),
		StatusBar:   fg(lipgloss.Color(muted)),
		ContextBar:  fg(lipgloss.Color(ov0)).Padding(0, 1),
		ToolPath:    fg(lipgloss.Color(blue)).Bold(true),
		ToolMeta:    fg(lipgloss.Color(ov2)),
		ToolCode: lipgloss.NewStyle().
			Foreground(lipgloss.Color(subtext)).
			Background(lipgloss.Color(surf0)).
			Padding(0, 1),
		ToolResultOK:   fg(lipgloss.Color(green)),
		ToolResultErr:  fg(lipgloss.Color(red)),
		ToolResultBody: fg(lipgloss.Color(ov1)).PaddingLeft(4),
		EmptyState:     fg(lipgloss.Color(ov0)).Italic(true).PaddingLeft(2),
		UpdateBanner:   fg(lipgloss.Color(peach)).PaddingLeft(1),
		WorkingStatus:  fg(lipgloss.Color(teal)),
		QueuedMessage:  fg(lipgloss.Color(yellow)).Italic(true),
		Toast: lipgloss.NewStyle().
			Background(lipgloss.Color(green)).
			Foreground(lipgloss.Color(crust)).
			Bold(true).
			Padding(0, 1),
		TableBorder: fg(lipgloss.Color(surf2)),
		TableHeader: fg(lipgloss.Color(subtext)).Bold(true),
		TableCell:   fg(lipgloss.Color(text)),
		FileRef:     fg(lipgloss.Color(mauve)).Bold(true),
		CmdRef:      fg(lipgloss.Color(mauve)).Bold(true),

		Base:      base,
		Crust:     crust,
		Text:      text,
		Subtext:   subtext,
		Mauve:     mauve,
		Blue:      blue,
		Green:     green,
		Yellow:    yellow,
		Red:       red,
		Lavender:  lav,
		Peach:     peach,
		Teal:      teal,
		Pink:      pink,
		Overlay0:  ov0,
		Overlay1:  ov1,
		Overlay2:  ov2,
		Surface0:  surf0,
		Surface1:  surf1,
		Surface2:  surf2,
	}
}

// OneDarkTheme returns the One Dark (Atom) theme.
func OneDarkTheme() Theme {
	const (
		base    = "#282c34"
		crust   = "#21252b"
		text    = "#abb2bf"
		subtext = "#8a8fa8"
		muted   = "#636d83"
		mauve   = "#c678dd"
		lav     = "#61afef"
		pink    = "#e06c75"
		blue    = "#61afef"
		green   = "#98c379"
		yellow  = "#e5c07b"
		peach   = "#d19a66"
		red     = "#e06c75"
		teal    = "#56b6c2"
		ov2     = "#5c6370"
		ov1     = "#3e4451"
		ov0     = "#333842"
		surf2   = "#3e4451"
		surf1   = "#333842"
		surf0   = "#2c313a"
	)

	bc := lipgloss.Color(base)
	bg := func() lipgloss.Style { return lipgloss.NewStyle().Background(bc) }
	fg := func(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Background(bc).Foreground(c) }

	brand := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(mauve)).Background(bc)
	return Theme{
		Brand:            brand,
		Header:           brand,
		Dim:              fg(lipgloss.Color(ov1)),
		Muted:            fg(lipgloss.Color(ov0)),
		Error:            fg(lipgloss.Color(red)).Bold(true),
		ReasoningToggle:  fg(lipgloss.Color(subtext)).Italic(true),
		ReasoningHeader:  fg(lipgloss.Color(lav)).Bold(true),
		ReasoningBox: bg().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(surf2)).
			Padding(0, 1).
			Margin(0, 0, 1, 0),
		ReasoningText:    fg(lipgloss.Color(ov0)).Padding(0, 1),
		ReasoningContent: fg(lipgloss.Color(ov0)).Padding(0, 2),
		Divider:          fg(lipgloss.Color(surf2)),
		UserLabel:        fg(lipgloss.Color(green)).Bold(true),
		UserContent:      fg(lipgloss.Color(text)),
		AssistantLabel:   brand,
		AssistantContent: fg(lipgloss.Color(text)),
		InputPrompt:      fg(lipgloss.Color(green)).Bold(true),
		ToolLabel:        fg(lipgloss.Color(pink)).Italic(true),
		DiffAdd: lipgloss.NewStyle().
			Background(lipgloss.Color("#2a3a2a")).
			Foreground(lipgloss.Color(text)),
		DiffAddHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color("#3a5a3a")).
			Foreground(lipgloss.Color(green)).
			Bold(true),
		DiffDel: lipgloss.NewStyle().
			Background(lipgloss.Color("#3a2a2c")).
			Foreground(lipgloss.Color(text)),
		DiffDelHighlight: lipgloss.NewStyle().
			Background(lipgloss.Color("#5a3a3c")).
			Foreground(lipgloss.Color(red)).
			Bold(true),
		DiffContext: fg(lipgloss.Color(ov1)).Italic(true),
		PermPrompt:  fg(lipgloss.Color(text)),
		PermKey:     fg(lipgloss.Color(mauve)).Bold(true),
		StatusBar:   fg(lipgloss.Color(muted)),
		ContextBar:  fg(lipgloss.Color(ov0)).Padding(0, 1),
		ToolPath:    fg(lipgloss.Color(blue)).Bold(true),
		ToolMeta:    fg(lipgloss.Color(ov2)),
		ToolCode: lipgloss.NewStyle().
			Foreground(lipgloss.Color(subtext)).
			Background(lipgloss.Color(surf0)).
			Padding(0, 1),
		ToolResultOK:   fg(lipgloss.Color(green)),
		ToolResultErr:  fg(lipgloss.Color(red)),
		ToolResultBody: fg(lipgloss.Color(ov1)).PaddingLeft(4),
		EmptyState:     fg(lipgloss.Color(ov0)).Italic(true).PaddingLeft(2),
		UpdateBanner:   fg(lipgloss.Color(peach)).PaddingLeft(1),
		WorkingStatus:  fg(lipgloss.Color(teal)),
		QueuedMessage:  fg(lipgloss.Color(yellow)).Italic(true),
		Toast: lipgloss.NewStyle().
			Background(lipgloss.Color(green)).
			Foreground(lipgloss.Color(crust)).
			Bold(true).
			Padding(0, 1),
		TableBorder: fg(lipgloss.Color(surf2)),
		TableHeader: fg(lipgloss.Color(subtext)).Bold(true),
		TableCell:   fg(lipgloss.Color(text)),
		FileRef:     fg(lipgloss.Color(mauve)).Bold(true),
		CmdRef:      fg(lipgloss.Color(mauve)).Bold(true),

		Base:      base,
		Crust:     crust,
		Text:      text,
		Subtext:   subtext,
		Mauve:     mauve,
		Blue:      blue,
		Green:     green,
		Yellow:    yellow,
		Red:       red,
		Lavender:  lav,
		Peach:     peach,
		Teal:      teal,
		Pink:      pink,
		Overlay0:  ov0,
		Overlay1:  ov1,
		Overlay2:  ov2,
		Surface0:  surf0,
		Surface1:  surf1,
		Surface2:  surf2,
	}
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

package ui

const (
	contentMargin      = 4
	cardMargin         = 6
	minCardWidth       = 36
	minDiffPanelWidth  = 28
	minSideBySideWidth = 100
	diffGutter         = 3
	maxPopupWidth      = 80
)

// Breakpoint describes the terminal width class.
type Breakpoint int

const (
	BreakpointNarrow Breakpoint = iota
	BreakpointMedium
	BreakpointWide
)

// DiffLayout describes how edit diffs are rendered.
type DiffLayout int

const (
	DiffSideBySide DiffLayout = iota
	DiffStacked
)

// Layout holds terminal dimensions for responsive rendering.
type Layout struct {
	Width  int
	Height int
}

// NewLayout returns a layout for the given terminal size.
func NewLayout(width, height int) Layout {
	return Layout{Width: width, Height: height}
}

// ContentWidth returns the inner content width after margins.
func (l Layout) ContentWidth() int {
	w := l.Width - contentMargin
	if w < minCardWidth {
		return minCardWidth
	}
	return w
}

// CardWidth returns the width for bordered cards (no upper cap).
func (l Layout) CardWidth() int {
	w := l.Width - cardMargin
	if w < minCardWidth {
		return minCardWidth
	}
	return w
}

// PopupWidth returns a scaled popup width capped at maxPopupWidth.
func (l Layout) PopupWidth() int {
	w := l.CardWidth() - 2
	if w < 30 {
		return 30
	}
	if w > maxPopupWidth {
		return maxPopupWidth
	}
	return w
}

// InputWidth returns the chat input area width.
func (l Layout) InputWidth() int {
	w := l.Width - contentMargin
	if w < 10 {
		return 10
	}
	return w
}

// Breakpoint returns the width breakpoint for the terminal.
func (l Layout) Breakpoint() Breakpoint {
	cw := l.ContentWidth()
	switch {
	case cw >= 100:
		return BreakpointWide
	case cw >= 60:
		return BreakpointMedium
	default:
		return BreakpointNarrow
	}
}

// DiffMode returns side-by-side or stacked diff layout.
func (l Layout) DiffMode() DiffLayout {
	cw := l.ContentWidth()
	panelW := (cw - diffGutter) / 2
	if cw >= minSideBySideWidth && panelW >= minDiffPanelWidth {
		return DiffSideBySide
	}
	return DiffStacked
}

// DiffPanelWidth returns the width of each diff panel in side-by-side mode.
func (l Layout) DiffPanelWidth() int {
	w := (l.CardWidth() - diffGutter) / 2
	if w < minDiffPanelWidth {
		return minDiffPanelWidth
	}
	return w
}

// RuleWidth returns a subtle divider width (centered, capped).
func (l Layout) RuleWidth() int {
	w := l.ContentWidth()
	if w > 60 {
		return 60
	}
	if w < 20 {
		return 20
	}
	return w
}

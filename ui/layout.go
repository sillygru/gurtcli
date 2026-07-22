package ui

const (
	minDiffPanelWidth  = 28
	minSideBySideWidth = 100
	diffGutter         = 3
	maxPopupWidth      = 80
	maxRuleWidth       = 60

	// minUsableWidth is the narrowest anything is ever rendered at. Every
	// accessor clamps *down* to the terminal width first and only then up to
	// this, so the ideal widths act as targets rather than as floors that
	// overflow a small screen.
	minUsableWidth = 10

	// shortHeight is the point below which optional chrome is dropped to keep
	// the transcript and the prompt on screen.
	shortHeight = 16

	// Cells drawn around an input that its own width does not account for:
	// "  ❯ " in front of the chat input, and "> " plus a trailing cursor cell
	// around a bubbles textinput.
	chatPromptWidth      = 4
	textInputPromptWidth = 3
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
//
// The invariant every width accessor upholds: the value returned is never
// greater than Width. Phone-sized terminals reached over SSH can be 30 columns
// or fewer, and anything wider than the screen soft-wraps, which pushes the
// rows below it down and scrolls the bottom of the UI away.
type Layout struct {
	Width  int
	Height int

	// contentOnly marks a layout built from an already-inset content width, so
	// margins are not deducted a second time. See LayoutForContent.
	contentOnly bool
}

// NewLayout returns a layout for the given terminal size.
func NewLayout(width, height int) Layout {
	return Layout{Width: width, Height: height}
}

// LayoutForContent returns a layout for a caller that already knows its content
// width rather than the terminal's. Margins have been taken off upstream, so
// they are not taken off again — but the clamp still applies, so nothing this
// layout hands back can exceed the space the caller actually has.
func LayoutForContent(contentWidth int) Layout {
	return Layout{Width: contentWidth, contentOnly: true}
}

// clamp fits w into [minUsableWidth, terminal width], with the terminal width
// winning when the two conflict.
func (l Layout) clamp(w int) int {
	// Width is 0 until the first WindowSizeMsg arrives and a frame can render
	// before then, so fall back to the floor rather than to a negative width.
	if l.Width <= 0 {
		return minUsableWidth
	}
	if w > l.Width {
		w = l.Width
	}
	if w < minUsableWidth {
		if l.Width < minUsableWidth {
			return l.Width
		}
		return minUsableWidth
	}
	return w
}

// margin returns the horizontal breathing room to reserve. Narrow terminals
// give theirs up rather than push content off the right edge.
func (l Layout) margin(full int) int {
	if l.contentOnly {
		return 0
	}
	switch {
	case l.Width >= 60:
		return full
	case l.Width >= 40:
		return 2
	default:
		return 0
	}
}

// ContentWidth returns the inner content width after margins.
func (l Layout) ContentWidth() int {
	return l.clamp(l.Width - l.margin(4))
}

// CardWidth returns the width for bordered cards.
func (l Layout) CardWidth() int {
	return l.clamp(l.Width - l.margin(6))
}

// PopupWidth returns a scaled popup width capped at maxPopupWidth.
func (l Layout) PopupWidth() int {
	w := l.CardWidth() - 2
	if w > maxPopupWidth {
		w = maxPopupWidth
	}
	return l.clamp(w)
}

// InputWidth returns the width available to an input's text area.
func (l Layout) InputWidth() int {
	return l.clamp(l.Width - l.margin(4))
}

// ChatInputWidth returns the text width of the chat input, less the "  ❯ "
// gutter chatView draws to its left. Wide terminals absorb that in their margin
// but narrow ones have no margin to give, so it has to come off explicitly or
// the prompt row overflows.
func (l Layout) ChatInputWidth() int {
	return l.clamp(l.InputWidth() - chatPromptWidth)
}

// SetupInputWidth returns the text width of a setup-flow textinput, less the
// "> " prompt bubbles draws in front of it.
func (l Layout) SetupInputWidth() int {
	return l.clamp(l.InputWidth() - textInputPromptWidth)
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
// The minDiffPanelWidth target only matters when the card can hold two panels;
// below that DiffMode has already switched to stacked, so a floor wider than
// the screen would buy nothing and overflow.
func (l Layout) DiffPanelWidth() int {
	w := (l.CardWidth() - diffGutter) / 2
	if w < minDiffPanelWidth {
		w = minDiffPanelWidth
	}
	return l.clamp(w)
}

// RuleWidth returns a subtle divider width (centered, capped).
func (l Layout) RuleWidth() int {
	w := l.ContentWidth()
	if w > maxRuleWidth {
		w = maxRuleWidth
	}
	return l.clamp(w)
}

// ListHeight returns the height available to a full-screen list, after its
// title, help line and surrounding chrome.
func (l Layout) ListHeight() int {
	h := l.Height - 10
	if h < 1 {
		return 1
	}
	return h
}

// IsShort reports whether the terminal is too short to afford optional chrome
// such as the chat title row.
func (l Layout) IsShort() bool {
	return l.Height > 0 && l.Height < shortHeight
}

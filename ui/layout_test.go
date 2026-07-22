package ui

import "testing"

func TestCardWidthNoCap(t *testing.T) {
	t.Parallel()
	layout := NewLayout(160, 40)
	cw := layout.CardWidth()
	if cw <= 68 {
		t.Fatalf("card width should exceed old 68 cap at wide terminal, got %d", cw)
	}
	if cw != 154 {
		t.Fatalf("expected card width 154, got %d", cw)
	}
}

// A terminal narrower than any ideal width must still be respected: returning
// something wider soft-wraps and breaks the vertical layout. This is the whole
// point of the clamp, so it is checked for every accessor at every size.
func TestWidthsNeverExceedTerminal(t *testing.T) {
	t.Parallel()
	accessors := map[string]func(Layout) int{
		"ContentWidth":   Layout.ContentWidth,
		"CardWidth":      Layout.CardWidth,
		"PopupWidth":     Layout.PopupWidth,
		"InputWidth":     Layout.InputWidth,
		"RuleWidth":      Layout.RuleWidth,
		"DiffPanelWidth": Layout.DiffPanelWidth,
	}
	for _, width := range []int{10, 20, 24, 30, 40, 60, 80, 120, 200} {
		for _, height := range []int{5, 10, 24, 60} {
			layout := NewLayout(width, height)
			for name, fn := range accessors {
				got := fn(layout)
				if got > width {
					t.Errorf("%s at %dx%d = %d, exceeds terminal width", name, width, height, got)
				}
				if got < 1 {
					t.Errorf("%s at %dx%d = %d, must be positive", name, width, height, got)
				}
			}
		}
	}
}

// Before the first WindowSizeMsg the terminal size is unknown; a frame can
// still render, and must not do so at a negative width.
func TestWidthsBeforeFirstResize(t *testing.T) {
	t.Parallel()
	layout := NewLayout(0, 0)
	if got := layout.ContentWidth(); got != minUsableWidth {
		t.Fatalf("ContentWidth at zero size = %d, want %d", got, minUsableWidth)
	}
	if got := layout.PopupWidth(); got != minUsableWidth {
		t.Fatalf("PopupWidth at zero size = %d, want %d", got, minUsableWidth)
	}
}

// LayoutForContent is handed a width that margins were already taken off of,
// so it must not deduct them a second time.
func TestLayoutForContentKeepsWidth(t *testing.T) {
	t.Parallel()
	if got := LayoutForContent(76).ContentWidth(); got != 76 {
		t.Fatalf("LayoutForContent(76).ContentWidth() = %d, want 76", got)
	}
	if got := LayoutForContent(20).ContentWidth(); got != 20 {
		t.Fatalf("LayoutForContent(20).ContentWidth() = %d, want 20", got)
	}
}

func TestListHeightAndShort(t *testing.T) {
	t.Parallel()
	if got := NewLayout(80, 40).ListHeight(); got != 30 {
		t.Fatalf("ListHeight at height 40 = %d, want 30", got)
	}
	if got := NewLayout(80, 4).ListHeight(); got != 1 {
		t.Fatalf("ListHeight must stay positive on a tiny terminal, got %d", got)
	}
	if !NewLayout(80, 10).IsShort() {
		t.Fatal("height 10 should count as short")
	}
	if NewLayout(80, 40).IsShort() {
		t.Fatal("height 40 should not count as short")
	}
}

func TestDiffModeSideBySide(t *testing.T) {
	t.Parallel()
	layout := NewLayout(120, 40)
	if layout.DiffMode() != DiffSideBySide {
		t.Fatal("expected side-by-side diff at width 120")
	}
}

func TestDiffModeStacked(t *testing.T) {
	t.Parallel()
	layout := NewLayout(60, 40)
	if layout.DiffMode() != DiffStacked {
		t.Fatal("expected stacked diff at width 60")
	}
}

func TestBreakpoint(t *testing.T) {
	t.Parallel()
	if NewLayout(40, 10).Breakpoint() != BreakpointNarrow {
		t.Fatal("expected narrow breakpoint")
	}
	if NewLayout(80, 10).Breakpoint() != BreakpointMedium {
		t.Fatal("expected medium breakpoint")
	}
	if NewLayout(120, 10).Breakpoint() != BreakpointWide {
		t.Fatal("expected wide breakpoint")
	}
}

func TestPopupWidth(t *testing.T) {
	t.Parallel()
	layout := NewLayout(200, 40)
	pw := layout.PopupWidth()
	if pw != maxPopupWidth {
		t.Fatalf("popup width should cap at %d, got %d", maxPopupWidth, pw)
	}
}

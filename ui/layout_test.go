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

func TestCardWidthMinimum(t *testing.T) {
	t.Parallel()
	layout := NewLayout(10, 10)
	if layout.CardWidth() < minCardWidth {
		t.Fatalf("card width below minimum: %d", layout.CardWidth())
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

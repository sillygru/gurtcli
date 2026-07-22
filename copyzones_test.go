package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sillygru/gurtcli/llm"
)

// cellIndexOf returns the column a substring starts at once a rendered row is
// stripped of styling — where the user actually sees it.
func cellIndexOf(t *testing.T, rendered, want string) int {
	t.Helper()
	plain := stripANSI(rendered)
	i := strings.Index(plain, want)
	if i < 0 {
		t.Fatalf("%q not found in rendered row %q", want, plain)
	}
	return ansi.StringWidth(plain[:i])
}

func testChatModelWithStatus() model {
	m := testChatModel()
	m.sessionName = "refactor selection"
	m.provider = llm.ProviderAnthropic
	m.modelName = "claude-opus-4-8"
	m.workspaceRoot = "/home/dev/gurtcli"
	m.cwdDisplay = "~/gurtcli"
	m.contextInputTokens = 18000
	m.contextOutputTokens = 200
	m.contextCacheTokens = 9000
	m.maxInputTokens = 200000
	return m
}

func TestCopyZoneHitsModelNameInHeader(t *testing.T) {
	m := testChatModelWithStatus()
	zone, ok := hitTestCopyZone(m, 3, 0)
	if !ok {
		t.Fatal("no zone on the title row")
	}
	if zone.text != m.modelDisplayName() {
		t.Errorf("text = %q, want %q", zone.text, m.modelDisplayName())
	}
	if _, ok := hitTestCopyZone(m, 0, 0); ok {
		t.Error("the two-space indent should not be a copy target")
	}
}

func TestCopyZonesAlignWithStatusBar(t *testing.T) {
	m := testChatModelWithStatus()
	row := m.helpWithStatus()
	lastRow := m.height - 1

	for _, want := range []struct {
		display string
		text    string
	}{
		{display: m.sessionName, text: m.sessionName},
		{display: m.providerLabel(), text: m.providerLabel()},
		{display: m.modelDisplayName(), text: m.modelDisplayName()},
		{display: m.cwdDisplay, text: m.workspaceRoot},
		{display: VersionString(), text: VersionString()},
	} {
		col := cellIndexOf(t, row, want.display)
		zone, ok := hitTestCopyZone(m, col, lastRow)
		if !ok {
			t.Errorf("no zone under %q at column %d", want.display, col)
			continue
		}
		if zone.text != want.text {
			t.Errorf("zone under %q copies %q, want %q", want.display, zone.text, want.text)
		}
		// The far end of the segment must belong to the same zone.
		end := col + ansi.StringWidth(want.display) - 1
		if z, ok := hitTestCopyZone(m, end, lastRow); !ok || z.text != want.text {
			t.Errorf("zone under %q does not cover its last cell", want.display)
		}
	}
}

// The bottom bar is budgeted as exactly one row by adjustViewportHeight, so at
// any width it must fit on one — segments get dropped, never wrapped.
func TestBottomBarFitsOnOneRow(t *testing.T) {
	for _, width := range []int{20, 30, 40, 50, 60, 80, 120} {
		m := testChatModelWithStatus()
		m.width = width
		row := m.helpWithStatus()
		if strings.Contains(row, "\n") {
			t.Errorf("width %d: bottom bar spans multiple rows", width)
		}
		if got := ansi.StringWidth(stripANSI(row)); got > width {
			t.Errorf("width %d: bottom bar is %d cells wide", width, got)
		}
	}
}

// The model name is the one thing worth keeping when space runs out.
func TestBottomBarKeepsModelName(t *testing.T) {
	m := testChatModelWithStatus()
	m.width = 30
	_, helpSegs, statusSegs := m.fitBottomBar()
	if len(statusSegs) == 0 {
		t.Fatal("status segments were emptied")
	}
	last := statusSegs[len(statusSegs)-1]
	if last.text != m.modelDisplayName() {
		t.Errorf("last surviving status segment is %q, want the model name", last.text)
	}
	if len(helpSegs) != 0 {
		t.Errorf("help segments should be dropped first at width 30, got %d", len(helpSegs))
	}
}

// Zones are re-derived rather than recorded, so they have to be fitted the same
// way the renderer fitted them or a click lands on the wrong text.
func TestCopyZonesFollowDroppedSegments(t *testing.T) {
	m := testChatModelWithStatus()
	m.width = 30
	row := m.helpWithStatus()
	lastRow := m.height - 1

	name := m.modelDisplayName()
	col := cellIndexOf(t, row, name)
	zone, ok := hitTestCopyZone(m, col, lastRow)
	if !ok {
		t.Fatalf("no zone under the model name at column %d", col)
	}
	if zone.text != name {
		t.Errorf("zone under the model name copies %q, want %q", zone.text, name)
	}
	// The version was dropped from the row, so nothing may still claim to copy it.
	for _, z := range chatCopyZones(m) {
		if z.row == lastRow && z.text == VersionString() {
			t.Error("a copy zone survives for a segment that was not rendered")
		}
	}
}

func TestCopyZoneHitsContextBar(t *testing.T) {
	m := testChatModelWithStatus()
	spacerRow := computeViewportStartRow(m) + m.chatViewport.Height()

	// The spacer row is right-aligned, so the bar occupies the trailing cells.
	row := stripANSI(m.renderSpacerLine())
	bar := stripANSI(m.renderContextBar())
	if !strings.HasSuffix(row, bar) {
		t.Fatalf("spacer row %q does not end with the context bar %q", row, bar)
	}
	start := ansi.StringWidth(row) - ansi.StringWidth(bar)

	zone, ok := hitTestCopyZone(m, start, spacerRow)
	if !ok {
		t.Fatalf("no zone at the start of the context bar (column %d)", start)
	}
	if _, ok := hitTestCopyZone(m, m.width-1, spacerRow); !ok {
		t.Error("zone does not reach the right edge of the screen")
	}
	if zone.label != "context usage" {
		t.Fatalf("label = %q, want %q", zone.label, "context usage")
	}
	// The pasted text has to be readable prose, not the bar's box glyphs.
	if strings.ContainsAny(zone.text, "━─") {
		t.Errorf("context summary contains bar glyphs: %q", zone.text)
	}
	for _, want := range []string{"18K", "200K", "9%", "50% cached"} {
		if !strings.Contains(zone.text, want) {
			t.Errorf("context summary %q missing %q", zone.text, want)
		}
	}

	if _, ok := hitTestCopyZone(m, start-1, spacerRow); ok {
		t.Error("the gap left of the context bar should not be a copy target")
	}
}

func TestCopyZonesEmptyWithoutContext(t *testing.T) {
	m := testChatModel()
	spacerRow := computeViewportStartRow(m) + m.chatViewport.Height()
	if _, ok := hitTestCopyZone(m, m.width-1, spacerRow); ok {
		t.Error("context zone offered with no tokens counted yet")
	}
}

func TestPermCopyPayload(t *testing.T) {
	tests := []struct {
		name      string
		call      llm.ToolCall
		wantText  string
		wantLabel string
	}{
		{
			name:      "bash command",
			call:      toolCallFor("run_bash", `{"command":"go test ./..."}`),
			wantText:  "go test ./...",
			wantLabel: "command",
		},
		{
			name:      "file path",
			call:      toolCallFor("write_file", `{"path":"main.go","content":"package main"}`),
			wantText:  "main.go",
			wantLabel: "path",
		},
		{
			name:      "falls back to arguments",
			call:      toolCallFor("list_dir", `{"depth":2}`),
			wantText:  `{"depth":2}`,
			wantLabel: "tool arguments",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			text, label := permCopyPayload(tc.call)
			if text != tc.wantText || label != tc.wantLabel {
				t.Errorf("= (%q, %q), want (%q, %q)", text, label, tc.wantText, tc.wantLabel)
			}
		})
	}
}

func toolCallFor(name, args string) llm.ToolCall {
	var tc llm.ToolCall
	tc.Function.Name = name
	tc.Function.Arguments = args
	return tc
}

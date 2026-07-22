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

// The bar wraps onto extra rows rather than cutting a segment in half, but no
// row of it may ever be wider than the screen, and it stays within the height
// adjustViewportHeight budgets for it.
func TestBottomBarFitsWidth(t *testing.T) {
	for _, width := range []int{20, 30, 40, 50, 60, 80, 120} {
		m := testChatModelWithStatus()
		m.width = width
		bar := m.fitBottomBar()
		for i, row := range bar.rows {
			if got := ansi.StringWidth(stripANSI(row)); got > width {
				t.Errorf("width %d: bar row %d is %d cells wide: %q", width, i, got, stripANSI(row))
			}
		}
		if len(bar.rows) > maxBottomBarRows {
			t.Errorf("width %d: bar took %d rows, more than the %d it is allowed",
				width, len(bar.rows), maxBottomBarRows)
		}
		if strings.Contains(stripANSI(strings.Join(bar.rows, "")), "…") {
			t.Errorf("width %d: bar truncated a segment instead of wrapping it", width)
		}
	}
}

// Wrapping buys the bar a lot of room, but a phone-width terminal runs out of
// it anyway. What survives then is the model name, and what goes first is the
// version.
func TestBottomBarKeepsModelName(t *testing.T) {
	m := testChatModelWithStatus()
	m.width = 20
	bar := m.fitBottomBar()

	kept := map[string]bool{}
	for _, p := range bar.placed {
		kept[p.seg.text] = true
	}
	if !kept[m.modelDisplayName()] {
		t.Errorf("the model name was dropped at width 20; kept %v", kept)
	}
	if kept[VersionString()] {
		t.Error("the version should be the first thing dropped at width 20")
	}

	// Down to 30 columns everything still fits somewhere — nothing is dropped.
	m.width = 30
	kept = map[string]bool{}
	for _, p := range m.fitBottomBar().placed {
		kept[p.seg.text] = true
	}
	for _, want := range []string{VersionString(), m.workspaceRoot, m.sessionName, m.modelDisplayName()} {
		if !kept[want] {
			t.Errorf("%q was dropped at width 30 even though the bar could have wrapped it", want)
		}
	}
}

// Zones are re-derived rather than recorded, so they have to be fitted the same
// way the renderer fitted them or a click lands on the wrong text.
func TestCopyZonesFollowDroppedSegments(t *testing.T) {
	m := testChatModelWithStatus()
	m.width = 20
	bar := m.fitBottomBar()
	lastRow := m.height - 1

	name := m.modelDisplayName()
	nameRow, nameCol := lastRow, 0
	for i, row := range bar.rows {
		if strings.Contains(stripANSI(row), name) {
			nameRow = m.height - len(bar.rows) + i
			nameCol = cellIndexOf(t, row, name)
			break
		}
	}
	col := nameCol
	zone, ok := hitTestCopyZone(m, col, nameRow)
	if !ok {
		t.Fatalf("no zone under the model name at column %d", col)
	}
	if zone.text != name {
		t.Errorf("zone under the model name copies %q, want %q", zone.text, name)
	}
	// The version was dropped from the bar, so nothing may still claim to copy it.
	for _, z := range chatCopyZones(m) {
		if z.row >= m.height-len(bar.rows) && z.text == VersionString() {
			t.Error("a copy zone survives for a segment that was not rendered")
		}
	}
}

func TestCopyZoneHitsContextBar(t *testing.T) {
	m := testChatModelWithStatus()
	spacerRow := computeViewportStartRow(m) + m.chatViewport.Height()

	// The spacer row is right-aligned, so the bar occupies the trailing cells.
	row := stripANSI(m.spacerRows()[0])
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

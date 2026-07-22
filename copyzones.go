package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/tools"
	"github.com/sillygru/gurtcli/ui"
)

// Click-to-copy zones
//
// The transcript is drag-selectable, but most of what people want to quote out
// of a TUI lives in the chrome around it: the model name, the context meter,
// the working directory, the command a permission prompt is asking about. Those
// are single rows of derived text, so instead of making them selectable they
// are click targets — one click copies a clean plain-text rendering of the
// element, not the box-drawing glyphs it is painted with.
//
// Zones are recomputed from the model on demand rather than recorded during
// rendering: View has a value receiver and cannot hand anything back, and the
// layout is a pure function of the model, so re-deriving it is both possible
// and impossible to get out of sync with what the user sees.

// copyZone is a clickable region of one terminal row.
type copyZone struct {
	row   int    // terminal row, zero-based
	start int    // first cell, inclusive
	end   int    // last cell, exclusive
	label string // named in the toast: "Copied model name"
	text  string // what lands on the clipboard
}

// contains reports whether a terminal position falls inside the zone.
func (z copyZone) contains(x, y int) bool {
	return y == z.row && x >= z.start && x < z.end
}

// hitTestCopyZone returns the zone under a terminal position, if any.
func hitTestCopyZone(m model, x, y int) (copyZone, bool) {
	for _, z := range chatCopyZones(m) {
		if z.contains(x, y) {
			return z, true
		}
	}
	return copyZone{}, false
}

// chatCopyZones lists every click-to-copy region of the chat screen. The row
// arithmetic mirrors chatView: header, rule, viewport, spacer, toast, rule,
// then the bottom section anchored to the last row of the screen.
func chatCopyZones(m model) []copyZone {
	if m.state != stateChat || m.width <= 0 || m.height <= 0 {
		return nil
	}

	var zones []copyZone
	zones = appendHeaderZone(m, zones)
	zones = appendSpacerZones(m, zones)

	switch {
	case m.pendingPerm != nil:
		zones = appendPermZones(m, zones)
	case m.showThemePicker:
		// The picker owns its rows and is keyboard-driven.
	default:
		zones = appendBottomZones(m, zones)
	}
	return zones
}

// appendHeaderZone adds the model name in the title row.
func appendHeaderZone(m model, zones []copyZone) []copyZone {
	name := m.modelDisplayName()
	if name == "" {
		return zones
	}
	return append(zones, copyZone{
		row:   0,
		start: 2,
		end:   2 + lipgloss.Width(name),
		label: "model name",
		text:  name,
	})
}

// appendSpacerZones adds the working status, the debug readout and the context
// meter that share the row just below the transcript. The right-hand block is
// flush with the right edge of the screen, so it is measured backwards from
// there exactly as renderSpacerLine lays it out.
func appendSpacerZones(m model, zones []copyZone) []copyZone {
	row := computeViewportStartRow(m) + m.chatViewport.Height()
	if row >= m.height {
		return zones
	}

	// Read what the row actually kept: on a narrow terminal the right-hand block
	// is dropped, and zones for it would sit over blank cells.
	left, right := m.spacerParts()

	if status := m.workingStatusText(); status != "" && left != "" {
		zones = append(zones, copyZone{
			row:   row,
			start: 0,
			end:   lipgloss.Width(status) + 2, // the spinner glyph and its space
			label: "status",
			text:  status,
		})
	}
	if right == "" {
		return zones
	}

	ctxWidth := lipgloss.Width(m.renderContextBar())
	debugWidth := lipgloss.Width(m.renderDebugBar())
	rightEnd := m.width

	if ctxWidth > 0 {
		if summary := m.contextSummary(); summary != "" {
			zones = append(zones, copyZone{
				row:   row,
				start: rightEnd - ctxWidth,
				end:   rightEnd,
				label: "context usage",
				text:  summary,
			})
		}
		rightEnd -= ctxWidth + 2 // the two-space gutter between the two readouts
	}

	if debugWidth > 0 {
		zones = append(zones, copyZone{
			row:   row,
			start: rightEnd - debugWidth,
			end:   rightEnd,
			label: "resource usage",
			text:  m.debugSummary(),
		})
	}
	return zones
}

// appendBottomZones adds the queued message, the help line and the status line.
// The bottom section ends on the last row of the screen, so it is laid out
// upwards from there.
func appendBottomZones(m model, zones []copyZone) []copyZone {
	helpRow := m.height - 1
	if helpRow < 0 {
		return zones
	}

	if m.queuedMessage != "" {
		queuedRow := helpRow - m.chatInput.Height() - 1
		if queuedRow > 0 {
			zones = append(zones, copyZone{
				row:   queuedRow,
				start: 0,
				end:   m.width,
				label: "queued message",
				text:  m.queuedMessage,
			})
		}
	}

	// The same fitting the renderer used, so the zones land on the columns that
	// actually got drawn rather than on segments that were dropped.
	_, helpSegs, statusSegs := m.fitBottomBar()
	status := joinSegments(statusSegs)

	zones = appendSegmentZones(zones, helpRow, 0, helpSegs)
	statusStart := m.width - lipgloss.Width(status)
	return appendSegmentZones(zones, helpRow, statusStart, statusSegs)
}

// appendSegmentZones turns a joined run of segments into one zone each, so
// clicking the model name in the status bar copies the model name rather than
// the whole line.
func appendSegmentZones(zones []copyZone, row, start int, segments []segment) []copyZone {
	sepWidth := lipgloss.Width(segmentSeparator)
	col := start
	for _, seg := range segments {
		w := lipgloss.Width(seg.display)
		if w > 0 && seg.text != "" {
			zones = append(zones, copyZone{
				row:   row,
				start: col,
				end:   col + w,
				label: seg.label,
				text:  seg.text,
			})
		}
		col += w + sepWidth
	}
	return zones
}

// appendPermZones makes the pending permission prompt copyable: one click
// anywhere in the box puts the command — or the path — it is asking about on
// the clipboard, which is the whole reason people squint at that box.
func appendPermZones(m model, zones []copyZone) []copyZone {
	text, label := permCopyPayload(m.pendingPerm.toolCall)
	if text == "" {
		return zones
	}
	_, boxHeight, _, _ := m.renderPermOverlay()
	top := m.height - boxHeight
	if top < 0 {
		top = 0
	}
	width := ui.NewLayout(m.width, m.height).PopupWidth() + 2 // borders
	if width > m.width {
		width = m.width
	}
	for row := top; row < m.height; row++ {
		zones = append(zones, copyZone{
			row:   row,
			start: 0,
			end:   width,
			label: label,
			text:  text,
		})
	}
	return zones
}

// permCopyPayload returns the most useful thing to copy out of a tool call the
// user is being asked to approve.
func permCopyPayload(tc llm.ToolCall) (text, label string) {
	if tc.Function.Name == "run_bash" {
		if cmd, err := tools.ExtractBashCommand(json.RawMessage(tc.Function.Arguments)); err == nil && cmd != "" {
			return cmd, "command"
		}
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err == nil {
		for _, key := range []string{"path", "file_path", "filename", "url", "command", "pattern"} {
			if v, ok := args[key].(string); ok && v != "" {
				return v, key
			}
		}
	}
	if tc.Function.Arguments != "" {
		return tc.Function.Arguments, "tool arguments"
	}
	return "", ""
}

// workingStatusText returns the plain text of the spinner line, if one is up.
func (m model) workingStatusText() string {
	if m.toolExec != nil && m.toolExec.active {
		if m.toolExec.label != "" {
			return m.toolExec.label
		}
		return m.toolExec.toolName
	}
	if m.isStreaming {
		return m.workingMsg
	}
	return ""
}

// contextSummary renders the context meter as prose. The bar itself is drawn
// out of box-drawing characters that mean nothing once pasted elsewhere.
func (m model) contextSummary() string {
	used := m.contextInputTokens + m.contextOutputTokens
	if used <= 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Context: %s tokens", formatTokens(used))
	if m.maxInputTokens > 0 {
		pct := float64(used) / float64(m.maxInputTokens) * 100
		fmt.Fprintf(&b, " / %s (%.0f%%)", formatTokens(m.maxInputTokens), pct)
	}
	if m.contextInputTokens > 0 && m.contextCacheTokens > 0 {
		cachePct := float64(m.contextCacheTokens) / float64(m.contextInputTokens) * 100
		fmt.Fprintf(&b, " · %.0f%% cached", cachePct)
	}
	fmt.Fprintf(&b, " · %s", m.modelDisplayName())
	return b.String()
}

// debugSummary renders the debug readout as prose.
func (m model) debugSummary() string {
	return fmt.Sprintf("CPU %.1f%% · RAM %.1fMB", m.debugStats.cpuPercent, m.debugStats.memMB)
}

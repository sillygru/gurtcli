package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/ui"
)

// terminalSizes covers the shapes gurtcli is actually reached at, including the
// ones that only turn up over SSH: a phone in portrait, a phone in landscape,
// and a split pane.
// minSupportedRows is the shortest terminal every screen is expected to fit
// inside vertically. Narrower is always handled; shorter than this is only
// promised not to corrupt the display.
const minSupportedRows = 12

var terminalSizes = []struct{ w, h int }{
	{20, 10},  // absurdly small — must still not corrupt the screen
	{30, 60},  // phone, portrait
	{40, 80},  // phone, portrait, roomier
	{45, 20},  // narrow split pane
	{60, 24},  // classic small terminal
	{80, 24},  // classic
	{120, 12}, // wide and short — phone in landscape
	{200, 60}, // large
}

// sizedChatModel returns a chat model laid out for a terminal, the way
// WindowSizeMsg would have laid it out.
func sizedChatModel(w, h int) model {
	m := testChatModelWithStatus()
	m.width, m.height = w, h
	layout := ui.NewLayout(w, h)
	m = m.resizeInputs(layout)
	m.chatViewport.SetWidth(layout.ContentWidth())
	m = m.adjustViewportHeight()
	return m
}

// assertFitsScreen checks a view against the terminal it is being drawn into,
// before WrapScreen gets to it.
//
// Checking the raw view output is the point: WrapScreen truncates as a
// backstop, so anything measured after it fits by construction and proves
// nothing. A view that produces an over-wide line here is a view whose content
// is about to be silently cut off — and, without the backstop, one that would
// soft-wrap and push the bottom of the UI off the screen.
func assertFitsScreen(t *testing.T, name string, m model, content string) {
	t.Helper()
	rows := strings.Split(content, "\n")
	for i, r := range rows {
		if got := ansi.StringWidth(stripANSI(r)); got > m.width {
			t.Errorf("%s at %dx%d: row %d is %d cells wide, overflows by %d: %q",
				name, m.width, m.height, i, got, got-m.width, stripANSI(r))
		}
	}
	// Below minSupportedRows the densest prompt screens genuinely need more rows
	// than exist — a wrapped error plus three wrapped options does not fit in
	// ten rows at twenty columns, and there is no honest layout that makes it.
	// WrapScreen still keeps the screen intact there; content is clipped.
	if m.height >= minSupportedRows && len(rows) > m.height {
		t.Errorf("%s at %dx%d: produced %d rows, %d more than the screen holds",
			name, m.width, m.height, len(rows), len(rows)-m.height)
	}

	// And the backstop still has to hold: exactly height rows out.
	screen := strings.Split(ui.WrapScreen(content, m.width, m.height, m.theme.Base), "\n")
	if len(screen) != m.height {
		t.Errorf("%s at %dx%d: WrapScreen emitted %d rows, want %d",
			name, m.width, m.height, len(screen), m.height)
	}
}

// busyTranscript is a conversation with the things that render widest: a tool
// card, an edit diff, a fenced code block and an unbroken path.
var busyTranscript = []llm.Message{
	{Role: "user", Content: "explain the transcript render cache and edit ui/layout.go for me"},
	{
		Role:    "assistant",
		Content: "Reading the file first.\n\n```go\nfunc (l Layout) ContentWidth() int { return l.clamp(l.Width - l.margin(4)) }\n```",
		ToolCalls: []llm.ToolCall{{
			ID: "call_1",
			Function: llm.ToolCallFunction{
				Name:      "edit_file",
				Arguments: `{"path":"/Volumes/KINGSTON/code/projects/gurtcli/ui/layout.go","old_string":"if w < minCardWidth {","new_string":"if w < minUsableWidth {"}`,
			},
		}},
	},
	{Role: "tool", ToolCallID: "call_1", Content: "Edited /Volumes/KINGSTON/code/projects/gurtcli/ui/layout.go (1 replacement)"},
	{Role: "assistant", Content: "Done — the floor now clamps down to the terminal width instead of up to a fixed minimum."},
}

func TestChatViewFitsEveryTerminalSize(t *testing.T) {
	for _, size := range terminalSizes {
		m := sizedChatModel(size.w, size.h)
		m.messages = busyTranscript
		m.stableContent = buildChatContent(m)
		m.chatViewport.SetContent(m.stableContent)
		assertFitsScreen(t, "chatView", m, m.chatView())
	}
}

// The transient chrome — toast, suggestion list, queued-message notice — all
// shares the bottom of the screen with the prompt.
func TestChatViewChromeFitsEveryTerminalSize(t *testing.T) {
	for _, size := range terminalSizes {
		m := sizedChatModel(size.w, size.h)
		m.messages = busyTranscript
		m.toast = &toastMsg{text: "Copied working directory to the clipboard"}
		m.queuedMessage = "and then run the whole test suite against the narrow layouts"
		m.suggestions = suggestionState{
			active: true,
			items: []suggestionItem{
				{name: "model", description: "switch the model this session uses"},
				{name: "allow", description: "manage always-allowed tools and command prefixes"},
			},
		}
		m.isStreaming = true
		m.workingMsg = "Reticulating splines"
		m.stableContent = buildChatContent(m)
		m.chatViewport.SetContent(m.stableContent)
		m = m.adjustViewportHeight()
		assertFitsScreen(t, "chatView with chrome", m, m.chatView())
	}
}

// The transcript must keep at least one row at every size, or there is nowhere
// for the conversation to appear.
func TestViewportKeepsARowAtEverySize(t *testing.T) {
	for _, size := range terminalSizes {
		m := sizedChatModel(size.w, size.h)
		if got := m.chatViewport.Height(); got < 1 {
			t.Errorf("viewport height at %dx%d = %d", size.w, size.h, got)
		}
	}
}

// Every setup screen is reachable over SSH before the user has a working
// config, so each one has to survive a phone-sized terminal too.
func TestSetupViewsFitEveryTerminalSize(t *testing.T) {
	views := map[state]func(model) string{
		stateWelcome:         model.welcomeView,
		stateCustomURL:       model.customURLView,
		stateAPIKeyInput:     model.apiKeyView,
		stateCustomName:      model.customNameView,
		stateManualModel:     model.manualModelView,
		stateModelFetch:      model.modelFetchView,
		stateDotenvPrompt:    model.dotenvPromptView,
		stateDotenvKeyName:   model.dotenvKeyNameView,
		stateDotenvKeyExists: model.dotenvKeyExistsView,
		stateReasoningConfig: model.reasoningConfigView,
		stateAllowManage:     model.allowManageView,
	}

	for _, size := range terminalSizes {
		for st, render := range views {
			m := sizedChatModel(size.w, size.h)
			m.state = st
			m.provider = llm.ProviderAnthropic
			m.customURL = "https://api.example.com/v1/very/long/path/to/an/endpoint"
			m.dotenvKeyName = "SOME_QUITE_LONG_API_KEY_NAME"
			m.err = errStub{}
			m.alwaysAllowCommandPrefixes = []string{"git", "npm", "go build", "cargo check", "kubectl get pods"}
			assertFitsScreen(t, stateName(st), m, render(m))
		}
	}
}

// The permission overlay is the densest thing rendered and is anchored to the
// bottom, so it is the most likely to run off a small screen.
func TestPermissionOverlayFitsEveryTerminalSize(t *testing.T) {
	for _, size := range terminalSizes {
		m := sizedChatModel(size.w, size.h)
		m.pendingPerm = &pendingPerm{
			toolCall: llm.ToolCall{
				ID: "call_1",
				Function: llm.ToolCallFunction{
					Name:      "run_bash",
					Arguments: `{"command":"find . -name '*.go' -exec grep -l TODO {} \\; | head -50"}`,
				},
			},
		}
		m = m.adjustViewportHeight()
		assertFitsScreen(t, "permission overlay", m, m.chatView())
	}
}

type errStub struct{}

func (errStub) Error() string {
	return "the configured endpoint refused the connection after three attempts"
}

func stateName(s state) string {
	switch s {
	case stateWelcome:
		return "welcomeView"
	case stateCustomURL:
		return "customURLView"
	case stateAPIKeyInput:
		return "apiKeyView"
	case stateCustomName:
		return "customNameView"
	case stateManualModel:
		return "manualModelView"
	case stateModelFetch:
		return "modelFetchView"
	case stateDotenvPrompt:
		return "dotenvPromptView"
	case stateDotenvKeyName:
		return "dotenvKeyNameView"
	case stateDotenvKeyExists:
		return "dotenvKeyExistsView"
	case stateReasoningConfig:
		return "reasoningConfigView"
	case stateAllowManage:
		return "allowManageView"
	}
	return "view"
}

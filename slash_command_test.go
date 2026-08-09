package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sillygru/gurtcli/llm"
)

// enterChatInput sets the chat input to text and presses enter, the way a user
// would submit a message.
func enterChatInput(m model, text string) model {
	m.chatInput.SetValue(text)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	return updated.(model)
}

// An unknown slash command must not reach the LLM or the queue: it is dropped,
// the input clears, and a toast says so.
func TestUnknownSlashCommandIsNotSent(t *testing.T) {
	m := testChatModel()
	m.provider = llm.ProviderAnthropic
	m.modelName = "claude-opus-4-8"
	m.workspaceRoot = "/tmp/ws"

	before := len(m.messages)
	m = enterChatInput(m, "/frobnicate the widgets")

	if m.isStreaming {
		t.Error("isStreaming = true, want false: unknown command must not start an LLM stream")
	}
	if len(m.messages) != before {
		t.Errorf("messages grew from %d to %d: unknown command must not be added to the transcript", before, len(m.messages))
	}
	if m.queuedMessage != "" {
		t.Errorf("queuedMessage = %q, want empty: unknown command must not be queued", m.queuedMessage)
	}
	if got := m.chatInput.Value(); got != "" {
		t.Errorf("chat input = %q, want empty (cleared)", got)
	}
	if m.toast == nil || !strings.Contains(m.toast.text, "Unknown command: /frobnicate") {
		t.Errorf("toast = %+v, want an unknown-command notice for /frobnicate", m.toast)
	}
}

// A bare slash is not a command either — it must be dropped, not sent.
func TestBareSlashIsNotSent(t *testing.T) {
	m := testChatModel()

	m = enterChatInput(m, "/")

	if m.isStreaming || m.queuedMessage != "" || len(m.messages) != 0 {
		t.Errorf("bare slash started a stream (isStreaming=%v, queued=%q, messages=%d)",
			m.isStreaming, m.queuedMessage, len(m.messages))
	}
}

// While streaming, an unknown slash command is neither queued nor sent.
func TestUnknownSlashCommandWhileStreamingIsNotQueued(t *testing.T) {
	m := testChatModel()
	m.isStreaming = true

	m = enterChatInput(m, "/nope")

	if m.queuedMessage != "" {
		t.Errorf("queuedMessage = %q, want empty: unknown command must not be queued while streaming", m.queuedMessage)
	}
	if !m.isStreaming {
		t.Error("isStreaming = false, want true: the running stream must be untouched")
	}
	if m.toast == nil || !strings.Contains(m.toast.text, "Unknown command: /nope") {
		t.Errorf("toast = %+v, want an unknown-command notice", m.toast)
	}
}

// A known slash command still routes to the command handler, not the LLM.
func TestKnownSlashCommandStillRouted(t *testing.T) {
	m := testChatModel()
	m.state = stateChat
	m.provider = llm.ProviderAnthropic
	m.modelName = "claude-opus-4-8"

	updated, cmd := m.handleKeyPressEnter("/help")

	mm := updated.(model)
	if cmd != nil {
		// /help is handled synchronously with no command.
		t.Errorf("cmd = %v, want nil for /help", cmd)
	}
	if mm.isStreaming {
		t.Error("isStreaming = true, want false: /help is handled locally")
	}
	joined := ""
	for _, msg := range mm.messages {
		joined += msg.Content
	}
	if !strings.Contains(joined, "Available commands") {
		t.Errorf("/help output missing from transcript, got %q", joined)
	}
	if mm.toast != nil {
		t.Errorf("toast = %+v, want nil for a known command", mm.toast)
	}
}

// helper to drive the real enter path used by the chat state
func (m model) handleKeyPressEnter(text string) (tea.Model, tea.Cmd) {
	m.chatInput.SetValue(text)
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// A queued unknown slash command is discarded on replay, never sent.
func TestReplayQueuedUnknownCommandIsDropped(t *testing.T) {
	m := testChatModel()
	m.provider = llm.ProviderAnthropic
	m.modelName = "claude-opus-4-8"
	m.workspaceRoot = "/tmp/ws"
	m.queuedMessage = "/frobnicate"

	before := len(m.messages)
	updated, _ := m.replayQueuedMessage()
	mm := updated.(model)

	if mm.isStreaming {
		t.Error("isStreaming = true, want false: unknown queued command must not start a stream")
	}
	if len(mm.messages) != before {
		t.Errorf("messages grew from %d to %d", before, len(mm.messages))
	}
	if mm.queuedMessage != "" {
		t.Errorf("queuedMessage = %q, want empty after replay", mm.queuedMessage)
	}
	if mm.toast == nil || !strings.Contains(mm.toast.text, "Unknown command: /frobnicate") {
		t.Errorf("toast = %+v, want unknown-command notice", mm.toast)
	}
}

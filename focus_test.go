package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Starting straight on the API key screen — a config with a provider but no
// stored key, which is what a machine with no OS keychain leaves behind —
// used to swallow every keypress: nothing on that path focused the input, and
// bubbles' textinput drops keys when unfocused.
func TestAPIKeyScreenAcceptsTypingOnColdStart(t *testing.T) {
	m := testChatModel()
	m.state = stateAPIKeyInput
	m.keyInput.Blur() // the state initialModel used to hand back

	var tm tea.Model = m
	for _, r := range "sk-test-123" {
		tm, _ = tm.(model).Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := tm.(model).keyInput.Value(); got != "sk-test-123" {
		t.Fatalf("typed key = %q, want %q", got, "sk-test-123")
	}
}

// Every state that shows an input must accept typing however it was entered.
func TestInputStatesAcceptTyping(t *testing.T) {
	cases := []struct {
		state state
		value func(model) string
	}{
		{stateAPIKeyInput, func(m model) string { return m.keyInput.Value() }},
		{stateCustomURL, func(m model) string { return m.urlInput.Value() }},
		{stateCustomName, func(m model) string { return m.nameInput.Value() }},
		{stateManualModel, func(m model) string { return m.manualInput.Value() }},
		{stateDotenvKeyName, func(m model) string { return m.dotenvInput.Value() }},
	}

	for _, tc := range cases {
		m := testChatModel()
		m.state = tc.state
		var tm tea.Model = m
		for _, r := range "abc" {
			tm, _ = tm.(model).Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		if got := tc.value(tm.(model)); got != "abc" {
			t.Errorf("state %d: typed %q, want \"abc\"", tc.state, got)
		}
	}
}

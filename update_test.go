package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sillygru/gurtcli/llm"
)

func TestCollectFileAttachments(t *testing.T) {
	tmpDir := t.TempDir()

	fooContent := "package foo\n\nfunc Foo() int { return 1 }"
	barContent := "# Bar\n\nThis is bar.md"
	if err := os.WriteFile(filepath.Join(tmpDir, "foo.go"), []byte(fooContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bar.md"), []byte(barContent), 0644); err != nil {
		t.Fatal(err)
	}

	m := model{workspaceRoot: tmpDir}

	tests := []struct {
		name  string
		model model
		msgs  []llm.Message
		want  string
	}{
		{
			name:  "no at refs",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "hello world"}},
			want:  "",
		},
		{
			name:  "single valid go file",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@foo.go fix the bug"}},
			want:  "Contents of foo.go:\n```go\n" + fooContent + "\n```\n\n",
		},
		{
			name:  "markdown file",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@bar.md read this"}},
			want:  "Contents of bar.md:\n```md\n" + barContent + "\n```\n\n",
		},
		{
			name:  "nonexistent file",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@nonexistent.md hello"}},
			want:  "",
		},
		{
			name:  "empty messages",
			model: m,
			msgs:  []llm.Message{},
			want:  "",
		},
		{
			name:  "non-user last message",
			model: m,
			msgs:  []llm.Message{{Role: "assistant", Content: "@foo.go"}},
			want:  "",
		},
		{
			name:  "multiple files",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@foo.go @bar.md compare"}},
			want:  "Contents of foo.go:\n```go\n" + fooContent + "\n```\n\nContents of bar.md:\n```md\n" + barContent + "\n```\n\n",
		},
		{
			name:  "no workspace root",
			model: model{workspaceRoot: ""},
			msgs:  []llm.Message{{Role: "user", Content: "@foo.go"}},
			want:  "",
		},
		{
			name:  "partial match one valid one missing",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@foo.go @missing.go compare"}},
			want:  "Contents of foo.go:\n```go\n" + fooContent + "\n```\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectFileAttachments(tt.model, tt.msgs)
			if tt.want == "" && got != "" {
				t.Errorf("expected empty, got %q", got)
			} else if tt.want != "" && got != tt.want {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.want, got)
			}
		})
	}
}

func TestCustomProviderAPIKeySkipOnEnter(t *testing.T) {
	t.Run("without modelName goes to stateModelFetch", func(t *testing.T) {
		m := testChatModel()
		m.state = stateAPIKeyInput
		m.provider = llm.ProviderCustom
		m.customURL = "http://localhost:11434/v1"
		m.modelName = ""
		m.keyInput.SetValue("")

		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		um := updated.(model)

		if um.state != stateModelFetch {
			t.Errorf("state = %v, want stateModelFetch (%v)", um.state, stateModelFetch)
		}
		if um.apiKey != "" {
			t.Errorf("apiKey = %q, want empty", um.apiKey)
		}
	})

	t.Run("with modelName enters chat", func(t *testing.T) {
		m := testChatModel()
		m.state = stateAPIKeyInput
		m.provider = llm.ProviderCustom
		m.customURL = "http://localhost:11434/v1"
		m.modelName = "llama3"
		m.keyInput.SetValue("")

		updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		um := updated.(model)

		if um.state != stateChat {
			t.Errorf("state = %v, want stateChat (%v)", um.state, stateChat)
		}
		if um.apiKey != "" {
			t.Errorf("apiKey = %q, want empty", um.apiKey)
		}
	})
}

func TestNonCustomProviderRequiresAPIKey(t *testing.T) {
	m := testChatModel()
	m.state = stateAPIKeyInput
	m.provider = llm.ProviderOpenAI
	m.keyInput.SetValue("")

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	um := updated.(model)

	if um.state != stateAPIKeyInput {
		t.Errorf("state = %v, want stateAPIKeyInput (%v)", um.state, stateAPIKeyInput)
	}
}

func TestAPIKeyViewCustomProviderMentionsSkip(t *testing.T) {
	m := testChatModel()
	m.state = stateAPIKeyInput
	m.provider = llm.ProviderCustom
	m.customURL = "http://localhost:11434/v1"

	view := m.apiKeyView()
	if !strings.Contains(view, "press enter to skip") {
		t.Errorf("apiKeyView() should mention 'press enter to skip', got:\n%s", view)
	}
}

func TestChatViewportHomeEndPageUpDown(t *testing.T) {
	m := testChatModel()
	var lines []string
	for i := 0; i < 50; i++ {
		lines = append(lines, strings.Repeat("line ", 10))
	}
	content := strings.Join(lines, "\n")
	m.chatViewport.SetContent(content)
	m.chatViewport.GotoTop()

	if m.chatViewport.YOffset() != 0 {
		t.Fatalf("expected initial YOffset 0, got %d", m.chatViewport.YOffset())
	}

	// Test PageDown
	res, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	m = res.(model)
	if m.chatViewport.YOffset() == 0 {
		t.Errorf("expected YOffset > 0 after KeyPgDown, got %d", m.chatViewport.YOffset())
	}

	// Test End (go to bottom)
	res, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	m = res.(model)
	if !m.chatViewport.AtBottom() {
		t.Errorf("expected viewport to be at bottom after KeyEnd")
	}
	if !m.stickToBottom {
		t.Errorf("expected stickToBottom to be true after KeyEnd")
	}

	// Test PageUp
	res, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = res.(model)
	if m.chatViewport.AtBottom() {
		t.Errorf("expected viewport not at bottom after KeyPgUp from bottom")
	}

	// Test Home (go to top)
	res, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	m = res.(model)
	if m.chatViewport.YOffset() != 0 {
		t.Errorf("expected YOffset 0 after KeyHome, got %d", m.chatViewport.YOffset())
	}
}

func TestKeyHelpers(t *testing.T) {
	tests := []struct {
		msg    tea.KeyPressMsg
		isHome bool
		isEnd  bool
		isPgUp bool
		isPgDn bool
	}{
		{msg: tea.KeyPressMsg{Code: tea.KeyHome}, isHome: true},
		{msg: tea.KeyPressMsg{Code: tea.KeyKpHome}, isHome: true},
		{msg: tea.KeyPressMsg{Code: tea.KeyEnd}, isEnd: true},
		{msg: tea.KeyPressMsg{Code: tea.KeyKpEnd}, isEnd: true},
		{msg: tea.KeyPressMsg{Code: tea.KeyPgUp}, isPgUp: true},
		{msg: tea.KeyPressMsg{Code: tea.KeyKpPgUp}, isPgUp: true},
		{msg: tea.KeyPressMsg{Code: tea.KeyPgDown}, isPgDn: true},
		{msg: tea.KeyPressMsg{Code: tea.KeyKpPgDown}, isPgDn: true},
	}

	for _, tt := range tests {
		if got := isHomeKey(tt.msg); got != tt.isHome {
			t.Errorf("isHomeKey(%v) = %v, want %v", tt.msg, got, tt.isHome)
		}
		if got := isEndKey(tt.msg); got != tt.isEnd {
			t.Errorf("isEndKey(%v) = %v, want %v", tt.msg, got, tt.isEnd)
		}
		if got := isPgUpKey(tt.msg); got != tt.isPgUp {
			t.Errorf("isPgUpKey(%v) = %v, want %v", tt.msg, got, tt.isPgUp)
		}
		if got := isPgDownKey(tt.msg); got != tt.isPgDn {
			t.Errorf("isPgDownKey(%v) = %v, want %v", tt.msg, got, tt.isPgDn)
		}
	}
}

package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/tools"
	"github.com/sillygru/gurtcli/ui"
)

func TestCdCommandPermissionsCurrentDir(t *testing.T) {
	ws := t.TempDir()
	cmd := "cd " + ws + " && npm test"
	rawArgs, err := json.Marshal(tools.RunBashArgs{Command: cmd, Title: "Test"})
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	tc := llm.ToolCall{
		ID: "call_1",
		Function: llm.ToolCallFunction{
			Name:      "run_bash",
			Arguments: string(rawArgs),
		},
	}

	m := model{
		workspaceRoot:               ws,
		allowedExternalPathsSession: make(map[string]bool),
		allowedBashPrefixesSession:  make(map[string]bool),
		theme:                       ui.ThemeRegistry[0].NewFunc(),
		chatViewport:                viewport.New(),
		permPatternInput:            textinput.New(),
	}

	resModel, _ := m.processToolCalls([]llm.ToolCall{tc})
	res := resModel.(model)

	if res.pendingPerm == nil {
		t.Fatalf("expected pendingPerm for run_bash, got nil")
	}

	if res.pendingPerm.externalPath != "" {
		t.Errorf("expected empty externalPath for current directory cd, got %q", res.pendingPerm.externalPath)
	}

	// Verify command displayed in prompt is stripped of cd
	dispCmd, err := tools.ExtractBashCommand(json.RawMessage(res.pendingPerm.toolCall.Function.Arguments))
	if err != nil {
		t.Fatalf("extract error: %v", err)
	}
	if dispCmd != "npm test" {
		t.Errorf("display command = %q, want %q", dispCmd, "npm test")
	}

	// Verify original tool call retains full command
	origCmd, err := tools.ExtractBashCommand(json.RawMessage(res.pendingPerm.origToolCall.Function.Arguments))
	if err != nil {
		t.Fatalf("extract orig error: %v", err)
	}
	if origCmd != cmd {
		t.Errorf("orig command = %q, want %q", origCmd, cmd)
	}

	if res.permPatternInput.Value() != "npm test *" {
		t.Errorf("pattern input = %q, want %q", res.permPatternInput.Value(), "npm test *")
	}
}

func TestCdCommandPermissionsOutsideDir(t *testing.T) {
	ws := t.TempDir()
	extDir := filepath.Join(t.TempDir(), "external")
	cmd := "cd " + extDir + " && git status"
	rawArgs, err := json.Marshal(tools.RunBashArgs{Command: cmd, Title: "Git Status"})
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	tc := llm.ToolCall{
		ID: "call_2",
		Function: llm.ToolCallFunction{
			Name:      "run_bash",
			Arguments: string(rawArgs),
		},
	}

	m := model{
		workspaceRoot:               ws,
		allowedExternalPathsSession: make(map[string]bool),
		allowedBashPrefixesSession:  make(map[string]bool),
		theme:                       ui.ThemeRegistry[0].NewFunc(),
		chatViewport:                viewport.New(),
		permPatternInput:            textinput.New(),
	}

	// First pass: should prompt for external directory permission first
	resModel, _ := m.processToolCalls([]llm.ToolCall{tc})
	res := resModel.(model)

	if res.pendingPerm == nil {
		t.Fatalf("expected pendingPerm for external dir prompt, got nil")
	}

	if res.pendingPerm.externalPath != extDir {
		t.Errorf("expected externalPath = %q, got %q", extDir, res.pendingPerm.externalPath)
	}

	// Simulate user allowing directory for session (option 1)
	res.allowedExternalPathsSession[filepath.Clean(extDir)] = true
	rem := res.pendingPerm.remaining
	res.pendingPerm = nil

	// Second pass: should now prompt for bash command permission (or run if matched)
	resModel2, _ := res.processToolCalls(rem)
	res2 := resModel2.(model)

	if res2.pendingPerm == nil {
		t.Fatalf("expected pendingPerm for bash command after external dir allowed, got nil")
	}

	if res2.pendingPerm.externalPath != "" {
		t.Errorf("expected empty externalPath on second pass, got %q", res2.pendingPerm.externalPath)
	}

	dispCmd, _ := tools.ExtractBashCommand(json.RawMessage(res2.pendingPerm.toolCall.Function.Arguments))
	if dispCmd != "git status" {
		t.Errorf("display command on second pass = %q, want %q", dispCmd, "git status")
	}
}

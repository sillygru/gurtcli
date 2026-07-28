package main

import (
	"encoding/json"
	"testing"

	"github.com/sillygru/gurtcli/config"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/tools"
)

func TestBashPatternPermissionPersistence(t *testing.T) {
	ws := t.TempDir()

	cmd := "rg search_term"
	rawArgs, err := json.Marshal(tools.RunBashArgs{Command: cmd})
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	tc := llm.ToolCall{
		ID: "call_rg_1",
		Function: llm.ToolCallFunction{
			Name:      "run_bash",
			Arguments: string(rawArgs),
		},
	}

	// 1. Setup config in temp dir with AllowedBashPrefixes
	cfgDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("HOME", cfgDir)

	cfg := &config.Config{
		AllowedBashPrefixes:        []string{"rg *"},
		AlwaysAllowCommandPrefixes: []string{"git status"},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	// 2. Load model as if starting a new session
	m := initialModel(false, "", "", false, false, false)
	m.workspaceRoot = ws

	// Verify that allowedBashPrefixes contains "rg *" and alwaysAllowCommandPrefixes contains "rg *"
	if !m.allowedBashPrefixes["rg *"] {
		t.Errorf("expected m.allowedBashPrefixes to contain 'rg *'")
	}
	foundInCmds := false
	for _, p := range m.alwaysAllowCommandPrefixes {
		if p == "rg *" {
			foundInCmds = true
			break
		}
	}
	if !foundInCmds {
		t.Errorf("expected m.alwaysAllowCommandPrefixes to contain 'rg *'")
	}

	// Verify that processToolCalls matches "rg *" without creating a pendingPerm prompt
	matched := false
	for pat := range m.allowedBashPrefixes {
		if tools.MatchCommandPattern(pat, cmd) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("expected command %q to match allowed pattern 'rg *'", cmd)
	}

	// 3. Test allowBashPattern saves to both AllowedBashPrefixes and AlwaysAllowCommandPrefixes
	m.allowBashPattern("kubectl *", tc, nil)
	cfgLoaded, err := config.Load()
	if err != nil {
		t.Fatalf("failed to load updated config: %v", err)
	}
	hasKubectlCmds := false
	for _, p := range cfgLoaded.AlwaysAllowCommandPrefixes {
		if p == "kubectl *" {
			hasKubectlCmds = true
			break
		}
	}
	if !hasKubectlCmds {
		t.Errorf("expected cfgLoaded.AlwaysAllowCommandPrefixes to contain 'kubectl *'")
	}
}

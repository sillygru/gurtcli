package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	dir, err := Dir()
	if err != nil {
		t.Fatalf("Dir() returned error: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "gurtcli")
	if dir != want {
		t.Errorf("Dir() = %q, want %q", dir, want)
	}
}

func TestPath(t *testing.T) {
	p, err := Path()
	if err != nil {
		t.Fatalf("Path() returned error: %v", err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "gurtcli", "config.json")
	if p != want {
		t.Errorf("Path() = %q, want %q", p, want)
	}
}

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	configDirOverride = tmp
	defer func() { configDirOverride = "" }()

	cfg := &Config{
		Provider:      "openai",
		Model:         "gpt-5.4",
		CustomBaseURL: "",
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if loaded == nil {
		t.Fatal("Load() returned nil")
	}
	if loaded.Provider != "openai" {
		t.Errorf("Provider = %q, want %q", loaded.Provider, "openai")
	}
	if loaded.Model != "gpt-5.4" {
		t.Errorf("Model = %q, want %q", loaded.Model, "gpt-5.4")
	}
}

func TestSaveAndLoadCustom(t *testing.T) {
	tmp := t.TempDir()
	configDirOverride = tmp
	defer func() { configDirOverride = "" }()

	cfg := &Config{
		Provider:      "custom",
		Model:         "my-model",
		CustomBaseURL: "https://example.com/v1",
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if loaded.CustomBaseURL != "https://example.com/v1" {
		t.Errorf("CustomBaseURL = %q, want %q", loaded.CustomBaseURL, "https://example.com/v1")
	}
}

func TestLoadNoFile(t *testing.T) {
	tmp := t.TempDir()
	configDirOverride = tmp
	defer func() { configDirOverride = "" }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg != nil {
		t.Errorf("Load() = %v, want nil", cfg)
	}
}

func TestOverwriteSave(t *testing.T) {
	tmp := t.TempDir()
	configDirOverride = tmp
	defer func() { configDirOverride = "" }()

	cfg1 := &Config{Provider: "openai", Model: "gpt-5.4"}
	if err := Save(cfg1); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	cfg2 := &Config{Provider: "anthropic", Model: "claude-sonnet-5"}
	if err := Save(cfg2); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if loaded.Provider != "anthropic" {
		t.Errorf("Provider = %q, want %q", loaded.Provider, "anthropic")
	}
	if loaded.Model != "claude-sonnet-5" {
		t.Errorf("Model = %q, want %q", loaded.Model, "claude-sonnet-5")
	}
}

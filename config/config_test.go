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

func TestSavedEndpoints(t *testing.T) {
	tmp := t.TempDir()
	configDirOverride = tmp
	defer func() { configDirOverride = "" }()

	cfg := &Config{
		Provider: "custom",
		Model:    "gpt-4",
	}

	if err := cfg.AddSavedEndpoint("My Groq", "https://api.groq.com/v1"); err != nil {
		t.Fatalf("AddSavedEndpoint() returned error: %v", err)
	}
	if err := cfg.AddSavedEndpoint("My DeepSeek", "https://api.deepseek.com/v1"); err != nil {
		t.Fatalf("AddSavedEndpoint() returned error: %v", err)
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if len(loaded.SavedEndpoints) != 2 {
		t.Fatalf("len(SavedEndpoints) = %d, want 2", len(loaded.SavedEndpoints))
	}
	if loaded.SavedEndpoints[0].Name != "My Groq" {
		t.Errorf("SavedEndpoints[0].Name = %q, want %q", loaded.SavedEndpoints[0].Name, "My Groq")
	}
	if loaded.SavedEndpoints[1].BaseURL != "https://api.deepseek.com/v1" {
		t.Errorf("SavedEndpoints[1].BaseURL = %q, want %q", loaded.SavedEndpoints[1].BaseURL, "https://api.deepseek.com/v1")
	}

	ep, ok := loaded.GetSavedEndpoint("My Groq")
	if !ok {
		t.Fatal("GetSavedEndpoint('My Groq') returned false")
	}
	if ep.BaseURL != "https://api.groq.com/v1" {
		t.Errorf("GetSavedEndpoint BaseURL = %q, want %q", ep.BaseURL, "https://api.groq.com/v1")
	}

	err = cfg.AddSavedEndpoint("My Groq", "https://api.groq.com/v2")
	if err == nil {
		t.Error("AddSavedEndpoint duplicate should return error")
	}

	if !loaded.RemoveSavedEndpoint("My Groq") {
		t.Fatal("RemoveSavedEndpoint('My Groq') returned false")
	}
	if len(loaded.SavedEndpoints) != 1 {
		t.Fatalf("len(SavedEndpoints) after remove = %d, want 1", len(loaded.SavedEndpoints))
	}
	if loaded.RemoveSavedEndpoint("nonexistent") {
		t.Error("RemoveSavedEndpoint('nonexistent') should return false")
	}
}

func TestSavedEndpointConfigPersistence(t *testing.T) {
	tmp := t.TempDir()
	configDirOverride = tmp
	defer func() { configDirOverride = "" }()

	cfg := &Config{
		Provider:          "custom",
		Model:             "my-model",
		SavedEndpointName: "My Groq",
	}
	cfg.AddSavedEndpoint("My Groq", "https://api.groq.com/v1")

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() returned error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if loaded.SavedEndpointName != "My Groq" {
		t.Errorf("SavedEndpointName = %q, want %q", loaded.SavedEndpointName, "My Groq")
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

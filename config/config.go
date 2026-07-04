package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var configDirOverride string

func dir() (string, error) {
	if configDirOverride != "" {
		return configDirOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return filepath.Join(home, ".config", "gurtcli"), nil
}

type SavedEndpoint struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
}

func (c *Config) AddSavedEndpoint(name, baseURL string) error {
	for _, ep := range c.SavedEndpoints {
		if ep.Name == name {
			return fmt.Errorf("endpoint %q already exists", name)
		}
	}
	c.SavedEndpoints = append(c.SavedEndpoints, SavedEndpoint{Name: name, BaseURL: baseURL})
	return nil
}

func (c *Config) RemoveSavedEndpoint(name string) bool {
	for i, ep := range c.SavedEndpoints {
		if ep.Name == name {
			c.SavedEndpoints = append(c.SavedEndpoints[:i], c.SavedEndpoints[i+1:]...)
			return true
		}
	}
	return false
}

func (c *Config) GetSavedEndpoint(name string) (SavedEndpoint, bool) {
	for _, ep := range c.SavedEndpoints {
		if ep.Name == name {
			return ep, true
		}
	}
	return SavedEndpoint{}, false
}

type Config struct {
	Provider           string          `json:"provider"`
	Model              string          `json:"model"`
	CustomBaseURL      string          `json:"custom_base_url,omitempty"`
	SavedEndpointName  string          `json:"saved_endpoint_name,omitempty"`
	SavedEndpoints     []SavedEndpoint `json:"saved_endpoints,omitempty"`
	ReasoningVisible   bool            `json:"reasoning_visible,omitempty"`
	ThinkingType       string          `json:"thinking_type,omitempty"`
	EffortLevel        string          `json:"effort_level,omitempty"`
	MaxTokens          int             `json:"max_tokens,omitempty"`

	UpdateVersion    string `json:"update_version,omitempty"`
	UpdateTempBinary string `json:"update_temp_binary,omitempty"`
}

func Dir() (string, error) {
	return dir()
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("opening config: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}
	return &cfg, nil
}

func Save(cfg *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	p := filepath.Join(dir, "config.json")
	f, err := os.Create(p)
	if err != nil {
		return fmt.Errorf("creating config file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	return nil
}

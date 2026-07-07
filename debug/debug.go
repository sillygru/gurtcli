package debug

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sillygru/gurtcli/config"
)

const dirName = "debug"

type StreamEvent struct {
	Type         string `json:"type"`
	Content      string `json:"content,omitempty"`
	InputTokens  int    `json:"input_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
	ToolCalls    int    `json:"tool_calls,omitempty"`
	Error        string `json:"error,omitempty"`
}

type StreamRecord struct {
	SessionID string        `json:"session_id"`
	Model     string        `json:"model"`
	Timestamp time.Time     `json:"timestamp"`
	Request   interface{}   `json:"request"`
	Events    []StreamEvent `json:"events"`
}

func Dir() (string, error) {
	cfgDir, err := config.Dir()
	if err != nil {
		return "", fmt.Errorf("getting config dir: %w", err)
	}
	d := filepath.Join(cfgDir, dirName)
	if err := os.MkdirAll(d, 0700); err != nil {
		return "", fmt.Errorf("creating debug dir: %w", err)
	}
	return d, nil
}

func SaveRecord(sessionID, modelName string, request interface{}, events []StreamEvent) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	now := time.Now()
	name := fmt.Sprintf("%s_%d.json", sessionID, now.UnixNano())
	p := filepath.Join(dir, name)
	rec := StreamRecord{
		SessionID: sessionID,
		Model:     modelName,
		Timestamp: now,
		Request:   request,
		Events:    events,
	}
	f, err := os.Create(p)
	if err != nil {
		return fmt.Errorf("creating debug file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rec); err != nil {
		return fmt.Errorf("encoding debug record: %w", err)
	}
	return nil
}

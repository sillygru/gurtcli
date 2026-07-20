package telemetry

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	endpoint    = "https://api.gru0.dev/telemetry/api/v1/event"
	projectID   = "gurtcli"
	uuidFile    = "telemetry-id"
	requestTimeout = 5 * time.Second
)

type Event struct {
	UUID      string `json:"uuid"`
	ProjectID string `json:"project_id"`
	Version   string `json:"version,omitempty"`
	EventType string `json:"event_type,omitempty"`
	OS        string `json:"os,omitempty"`
	Arch      string `json:"arch,omitempty"`
	Sig       string `json:"sig"`
}

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func ComputeSig(uuid, projectID, secret string) string {
	if secret == "" {
		return ""
	}
	today := time.Now().UTC().Format("2006-01-02")
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%s:%s:%s", projectID, uuid, today)
	return hex.EncodeToString(mac.Sum(nil))
}

// isValidUUID checks if s is a valid UUID v4 string (36 characters, no trailing junk).
func isValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	// Basic format: 8-4-4-4-12 hex with hyphens
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

func LoadOrCreateUUID(configDir string) string {
	if configDir == "" {
		return ""
	}
	p := filepath.Join(configDir, uuidFile)

	data, err := os.ReadFile(p)
	if err == nil {
		id := strings.TrimSpace(string(data))
		if isValidUUID(id) {
			return id
		}
	}

	id := newUUID()
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return id
	}
	// Atomic write: write to temp file then rename to avoid partial writes
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(id), 0600); err != nil {
		return id
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return id
	}
	return id
}

func SendEvent(uuidStr, version, eventType, secret string) {
	if secret == "" || uuidStr == "" {
		return
	}

	sig := ComputeSig(uuidStr, projectID, secret)
	if sig == "" {
		return
	}

	evt := Event{
		UUID:      uuidStr,
		ProjectID: projectID,
		Version:   version,
		EventType: eventType,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Sig:       sig,
	}

	body, err := json.Marshal(evt)
	if err != nil {
		return
	}

	go func() {
		client := &http.Client{Timeout: requestTimeout}
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		client.Do(req)
	}()
}

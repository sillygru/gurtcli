package sessions

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sillygru/gurtcli/llm"
)

// Session represents a saved chat session.
type Session struct {
	ID               string       `json:"id"`
	Name             string       `json:"name"`
	CreatedAt        time.Time    `json:"created_at"`
	UpdatedAt        time.Time    `json:"updated_at"`
	Provider         string       `json:"provider"`
	Model            string       `json:"model"`
	CustomURL        string       `json:"custom_url,omitempty"`
	SavedEndpointName string      `json:"saved_endpoint_name,omitempty"`
	ThinkingType     string       `json:"thinking_type,omitempty"`
	EffortLevel      string       `json:"effort_level,omitempty"`
	ReasoningVisible bool         `json:"reasoning_visible,omitempty"`
	WorkspaceRoot    string       `json:"workspace_root"`
	Messages         []llm.Message `json:"messages"`
}

// Metadata is a lightweight summary for listing sessions without loading all messages.
type Metadata struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Provider      string    `json:"provider"`
	Model         string    `json:"model"`
	WorkspaceRoot string    `json:"workspace_root"`
	MessageCount  int       `json:"message_count"`
}

var dirOverride string

// SetDirForTesting overrides the sessions directory (for tests only).
func SetDirForTesting(dir string) {
	dirOverride = dir
}

// sessionsDir returns the sessions directory path.
func sessionsDir() (string, error) {
	if dirOverride != "" {
		return dirOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return filepath.Join(home, ".config", "gurtcli", "sessions"), nil
}

// workspaceHash returns a short hash of the workspace root for directory scoping.
func workspaceHash(workspace string) string {
	h := sha256.Sum256([]byte(workspace))
	return fmt.Sprintf("%x", h[:8])
}

// workspaceDir returns the session directory for a specific workspace.
func workspaceDir(workspace string) (string, error) {
	base, err := sessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, workspaceHash(workspace)), nil
}

// GenerateID creates a new unique session ID.
func GenerateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// Save persists a session to disk. Creates the directory if needed.
func Save(s *Session) error {
	dir, err := workspaceDir(s.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("getting workspace dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating sessions dir: %w", err)
	}

	s.UpdatedAt = time.Now()
	if s.Name == "" {
		s.Name = generateName(s.Messages)
	}

	path := filepath.Join(dir, s.ID+".json")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating session file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(s); err != nil {
		return fmt.Errorf("encoding session: %w", err)
	}
	return nil
}

// Load reads a session from disk by ID and workspace.
func Load(workspace, id string) (*Session, error) {
	dir, err := workspaceDir(workspace)
	if err != nil {
		return nil, fmt.Errorf("getting workspace dir: %w", err)
	}

	path := filepath.Join(dir, id+".json")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening session: %w", err)
	}
	defer f.Close()

	var s Session
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return nil, fmt.Errorf("decoding session: %w", err)
	}
	return &s, nil
}

// List returns metadata for all sessions in a workspace, sorted by most recently updated.
func List(workspace string) ([]Metadata, error) {
	dir, err := workspaceDir(workspace)
	if err != nil {
		return nil, fmt.Errorf("getting workspace dir: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading sessions dir: %w", err)
	}

	var metas []Metadata
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		var s Session
		if err := json.NewDecoder(f).Decode(&s); err != nil {
			f.Close()
			continue
		}
		f.Close()

		metas = append(metas, Metadata{
			ID:            s.ID,
			Name:          s.Name,
			CreatedAt:     s.CreatedAt,
			UpdatedAt:     s.UpdatedAt,
			Provider:      s.Provider,
			Model:         s.Model,
			WorkspaceRoot: s.WorkspaceRoot,
			MessageCount:  len(s.Messages),
		})
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].UpdatedAt.After(metas[j].UpdatedAt)
	})

	return metas, nil
}

// Delete removes a session from disk.
func Delete(workspace, id string) error {
	dir, err := workspaceDir(workspace)
	if err != nil {
		return fmt.Errorf("getting workspace dir: %w", err)
	}
	path := filepath.Join(dir, id+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// SetActiveSession stores the active session ID for a workspace in config.
func SetActiveSession(workspace, sessionID string) error {
	dir, err := workspaceDir(workspace)
	if err != nil {
		return fmt.Errorf("getting workspace dir: %w", err)
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating sessions dir: %w", err)
	}
	path := filepath.Join(dir, "active.txt")
	if sessionID == "" {
		os.Remove(path)
		return nil
	}
	return os.WriteFile(path, []byte(sessionID), 0644)
}

// GetActiveSession reads the active session ID for a workspace.
func GetActiveSession(workspace string) (string, error) {
	dir, err := workspaceDir(workspace)
	if err != nil {
		return "", fmt.Errorf("getting workspace dir: %w", err)
	}
	path := filepath.Join(dir, "active.txt")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading active session: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// NameForMessages returns a display name derived from the first user message.
func NameForMessages(msgs []llm.Message) string {
	return generateName(msgs)
}

// generateName creates a session name from the first user message.
func generateName(msgs []llm.Message) string {
	for _, msg := range msgs {
		if msg.Role == "user" && msg.Content != "" {
			name := msg.Content
			// Take first line
			if idx := strings.Index(name, "\n"); idx != -1 {
				name = name[:idx]
			}
			// Truncate
			if len(name) > 80 {
				name = name[:77] + "..."
			}
			return name
		}
	}
	return "Empty session"
}

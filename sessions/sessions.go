package sessions

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sillygru/gurtcli/llm"
	_ "modernc.org/sqlite"
)

var (
	mu          sync.Mutex
	dbConn      *sql.DB
	dbDir       string
	dirOverride string
)

func lockDB(cfgDir string, readOnly bool) (func(), error) {
	path := filepath.Join(cfgDir, "sessions.db.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}
	kind := syscall.LOCK_EX
	if readOnly {
		kind = syscall.LOCK_SH
	}
	if err := syscall.Flock(int(f.Fd()), kind); err != nil {
		f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

func SetDirForTesting(dir string) {
	mu.Lock()
	defer mu.Unlock()
	dirOverride = dir
}

func Close() {
	mu.Lock()
	defer mu.Unlock()
	if dbConn != nil {
		dbConn.Close()
		dbConn = nil
		dbDir = ""
	}
}

func configDir() (string, error) {
	if dirOverride != "" {
		return dirOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home dir: %w", err)
	}
	return filepath.Join(home, ".config", "gurtcli"), nil
}

func openDB(cfgDir string) (*sql.DB, error) {
	if dbConn != nil && dbDir == cfgDir {
		return dbConn, nil
	}

	if dbConn != nil {
		dbConn.Close()
		dbConn = nil
	}

	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		return nil, fmt.Errorf("creating config dir: %w", err)
	}
	path := filepath.Join(cfgDir, "sessions.db")
	var err error
	dbConn, err = sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening db: %w", err)
	}
	dbDir = cfgDir

	if err := migrate(dbConn); err != nil {
		dbConn.Close()
		dbConn = nil
		return nil, err
	}

	removeOldSessionsDir(cfgDir)

	return dbConn, nil
}

func removeOldSessionsDir(cfgDir string) {
	old := filepath.Join(cfgDir, "sessions")
	if _, err := os.Stat(old); err == nil {
		os.RemoveAll(old)
	}
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("reading schema version: %w", err)
	}

	if version < 1 {
		_, err := db.Exec(`
			CREATE TABLE IF NOT EXISTS sessions (
				id TEXT PRIMARY KEY,
				name TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				provider TEXT NOT NULL,
				model TEXT NOT NULL,
				custom_url TEXT NOT NULL DEFAULT '',
				saved_endpoint_name TEXT NOT NULL DEFAULT '',
				thinking_type TEXT NOT NULL DEFAULT '',
				effort_level TEXT NOT NULL DEFAULT '',
				reasoning_visible INTEGER NOT NULL DEFAULT 0,
				workspace_root TEXT NOT NULL,
				messages TEXT NOT NULL DEFAULT '[]'
			);
			CREATE INDEX IF NOT EXISTS idx_sessions_workspace ON sessions(workspace_root);
			PRAGMA user_version = 1;
		`)
		if err != nil {
			return fmt.Errorf("migrate v1: %w", err)
		}
		version = 1
	}

	if version < 2 {
		_, err := db.Exec(`
			ALTER TABLE sessions ADD COLUMN input_tokens INTEGER NOT NULL DEFAULT 0;
			ALTER TABLE sessions ADD COLUMN output_tokens INTEGER NOT NULL DEFAULT 0;
			PRAGMA user_version = 2;
		`)
		if err != nil {
			return fmt.Errorf("migrate v2: %w", err)
		}
	}

	return nil
}

type Session struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	CreatedAt         time.Time     `json:"created_at"`
	UpdatedAt         time.Time     `json:"updated_at"`
	Provider          string        `json:"provider"`
	Model             string        `json:"model"`
	CustomURL         string        `json:"custom_url,omitempty"`
	SavedEndpointName string        `json:"saved_endpoint_name,omitempty"`
	ThinkingType      string        `json:"thinking_type,omitempty"`
	EffortLevel       string        `json:"effort_level,omitempty"`
	ReasoningVisible  bool          `json:"reasoning_visible,omitempty"`
	WorkspaceRoot     string        `json:"workspace_root"`
	Messages          []llm.Message `json:"messages"`
	InputTokens       int           `json:"input_tokens,omitempty"`
	OutputTokens      int           `json:"output_tokens,omitempty"`
}

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

func GenerateID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x_%d", b, time.Now().UnixNano())
}

func Query(fn func(*sql.DB) error) error {
	mu.Lock()
	defer mu.Unlock()
	cfgDir, err := configDir()
	if err != nil {
		return fmt.Errorf("session db: %w", err)
	}
	unlock, err := lockDB(cfgDir, true)
	if err != nil {
		return fmt.Errorf("session db: %w", err)
	}
	defer unlock()
	db, err := openDB(cfgDir)
	if err != nil {
		return err
	}
	return fn(db)
}

func Save(s *Session) error {
	mu.Lock()
	defer mu.Unlock()
	cfgDir, err := configDir()
	if err != nil {
		return fmt.Errorf("session db: %w", err)
	}
	unlock, err := lockDB(cfgDir, false)
	if err != nil {
		return fmt.Errorf("session db: %w", err)
	}
	defer unlock()

	db, err := openDB(cfgDir)
	if err != nil {
		return err
	}

	s.UpdatedAt = time.Now()
	if s.Name == "" {
		s.Name = generateName(s.Messages)
	}

	msgs, err := json.Marshal(s.Messages)
	if err != nil {
		return fmt.Errorf("encoding messages: %w", err)
	}

	_, err = db.Exec(`
		INSERT OR REPLACE INTO sessions
			(id, name, created_at, updated_at, provider, model, custom_url,
			 saved_endpoint_name, thinking_type, effort_level, reasoning_visible,
			 workspace_root, messages, input_tokens, output_tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		s.ID, s.Name,
		s.CreatedAt.UTC().Format(time.RFC3339),
		s.UpdatedAt.UTC().Format(time.RFC3339),
		s.Provider, s.Model, s.CustomURL,
		s.SavedEndpointName, s.ThinkingType, s.EffortLevel,
		boolToInt(s.ReasoningVisible),
		s.WorkspaceRoot, string(msgs),
		s.InputTokens, s.OutputTokens,
	)
	if err != nil {
		return fmt.Errorf("saving session: %w", err)
	}
	return nil
}

func Load(workspace, id string) (*Session, error) {
	mu.Lock()
	defer mu.Unlock()
	cfgDir, err := configDir()
	if err != nil {
		return nil, fmt.Errorf("session db: %w", err)
	}
	unlock, err := lockDB(cfgDir, true)
	if err != nil {
		return nil, fmt.Errorf("session db: %w", err)
	}
	defer unlock()

	db, err := openDB(cfgDir)
	if err != nil {
		return nil, err
	}

	row := db.QueryRow(`
		SELECT id, name, created_at, updated_at, provider, model, custom_url,
		       saved_endpoint_name, thinking_type, effort_level, reasoning_visible,
		       workspace_root, messages, input_tokens, output_tokens
		FROM sessions WHERE id = ? AND workspace_root = ?
	`, id, workspace)

	var s Session
	var createdAt, updatedAt string
	var msgsJSON string
	var reasoningVisible int

	if err := row.Scan(
		&s.ID, &s.Name, &createdAt, &updatedAt,
		&s.Provider, &s.Model, &s.CustomURL,
		&s.SavedEndpointName, &s.ThinkingType, &s.EffortLevel,
		&reasoningVisible, &s.WorkspaceRoot, &msgsJSON,
		&s.InputTokens, &s.OutputTokens,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		return nil, fmt.Errorf("loading session: %w", err)
	}

	s.ReasoningVisible = reasoningVisible != 0

	s.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parsing created_at: %w", err)
	}
	s.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parsing updated_at: %w", err)
	}

	if err := json.Unmarshal([]byte(msgsJSON), &s.Messages); err != nil {
		return nil, fmt.Errorf("decoding messages: %w", err)
	}

	return &s, nil
}

func List(workspace string) ([]Metadata, error) {
	mu.Lock()
	defer mu.Unlock()
	cfgDir, err := configDir()
	if err != nil {
		return nil, fmt.Errorf("session db: %w", err)
	}
	unlock, err := lockDB(cfgDir, true)
	if err != nil {
		return nil, fmt.Errorf("session db: %w", err)
	}
	defer unlock()

	db, err := openDB(cfgDir)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`
		SELECT id, name, created_at, updated_at, provider, model,
		       workspace_root, (SELECT count(*) FROM json_each(messages) WHERE json_extract(value, '$.role') = 'user') AS message_count
		FROM sessions WHERE workspace_root = ?
		ORDER BY updated_at DESC
	`, workspace)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}
	defer rows.Close()

	var metas []Metadata
	for rows.Next() {
		var m Metadata
		var createdAt, updatedAt string
		if err := rows.Scan(
			&m.ID, &m.Name, &createdAt, &updatedAt,
			&m.Provider, &m.Model, &m.WorkspaceRoot, &m.MessageCount,
		); err != nil {
			continue
		}
		m.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		m.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		metas = append(metas, m)
	}

	return metas, nil
}

func Delete(workspace, id string) error {
	mu.Lock()
	defer mu.Unlock()
	cfgDir, err := configDir()
	if err != nil {
		return fmt.Errorf("session db: %w", err)
	}
	unlock, err := lockDB(cfgDir, false)
	if err != nil {
		return fmt.Errorf("session db: %w", err)
	}
	defer unlock()

	db, err := openDB(cfgDir)
	if err != nil {
		return err
	}

	_, err = db.Exec("DELETE FROM sessions WHERE id = ? AND workspace_root = ?", id, workspace)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

func EnsureDB() (*sql.DB, error) {
	mu.Lock()
	defer mu.Unlock()
	cfgDir, err := configDir()
	if err != nil {
		return nil, fmt.Errorf("config dir: %w", err)
	}
	unlock, err := lockDB(cfgDir, false)
	if err != nil {
		return nil, fmt.Errorf("session db: %w", err)
	}
	defer unlock()
	return openDB(cfgDir)
}

func NameForMessages(msgs []llm.Message) string {
	return generateName(msgs)
}

func generateName(msgs []llm.Message) string {
	for _, msg := range msgs {
		if msg.Role == "user" && msg.Content != "" {
			name := msg.Content
			if idx := strings.Index(name, "\n"); idx != -1 {
				name = name[:idx]
			}
			if len(name) > 80 {
				name = name[:77] + "..."
			}
			return name
		}
	}
	return "Empty session"
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

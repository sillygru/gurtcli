package sessions

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/sillygru/gurtcli/llm"
)

func TestSaveLoadListRoundTrip(t *testing.T) {
	dir := t.TempDir()
	SetDirForTesting(dir)
	t.Cleanup(func() { Close(); SetDirForTesting("") })

	workspace := "/tmp/test-workspace"
	now := time.Now().UTC().Truncate(time.Second)

	s := &Session{
		ID:               "test-session-1",
		Name:             "Hello world",
		CreatedAt:        now,
		UpdatedAt:        now,
		Provider:         "openai",
		Model:            "gpt-5.5",
		WorkspaceRoot:    workspace,
		ReasoningVisible: true,
		ReasoningMode:    "auto",
		InputTokens:      142,
		OutputTokens:     57,
		Messages: []llm.Message{
			{Role: "user", Content: "Hello world"},
			{Role: "assistant", Content: "Hi there"},
		},
	}

	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(workspace, s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != s.Name {
		t.Fatalf("name: got %q want %q", loaded.Name, s.Name)
	}
	if len(loaded.Messages) != 2 {
		t.Fatalf("messages: got %d want 2", len(loaded.Messages))
	}
	if loaded.InputTokens != 142 {
		t.Fatalf("input_tokens: got %d want 142", loaded.InputTokens)
	}
	if loaded.OutputTokens != 57 {
		t.Fatalf("output_tokens: got %d want 57", loaded.OutputTokens)
	}
	if loaded.ReasoningMode != "auto" {
		t.Fatalf("reasoning_mode: got %q want %q", loaded.ReasoningMode, "auto")
	}

	metas, err := List(workspace)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("list count: got %d want 1", len(metas))
	}
	if metas[0].MessageCount != 1 {
		t.Fatalf("message count: got %d want 1", metas[0].MessageCount)
	}
}

func TestSessionListScopedByWorkspace(t *testing.T) {
	dir := t.TempDir()
	SetDirForTesting(dir)
	t.Cleanup(func() { Close(); SetDirForTesting("") })

	now := time.Now().UTC().Truncate(time.Second)

	s1 := &Session{
		ID:            "s1",
		Name:          "Project A session",
		CreatedAt:     now,
		UpdatedAt:     now,
		Provider:      "openai",
		Model:         "gpt-5.5",
		WorkspaceRoot: "/project-a",
		Messages:      []llm.Message{{Role: "user", Content: "hi"}},
	}
	s2 := &Session{
		ID:            "s2",
		Name:          "Project B session",
		CreatedAt:     now,
		UpdatedAt:     now,
		Provider:      "anthropic",
		Model:         "fable-5",
		WorkspaceRoot: "/project-b",
		Messages:      []llm.Message{{Role: "user", Content: "hello"}},
	}

	if err := Save(s1); err != nil {
		t.Fatalf("Save s1: %v", err)
	}
	if err := Save(s2); err != nil {
		t.Fatalf("Save s2: %v", err)
	}

	metas, err := List("/project-a")
	if err != nil {
		t.Fatalf("List /project-a: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 session for project-a, got %d", len(metas))
	}
	if metas[0].ID != "s1" {
		t.Fatalf("expected s1, got %s", metas[0].ID)
	}

	metas, err = List("/project-b")
	if err != nil {
		t.Fatalf("List /project-b: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("expected 1 session for project-b, got %d", len(metas))
	}
	if metas[0].ID != "s2" {
		t.Fatalf("expected s2, got %s", metas[0].ID)
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	SetDirForTesting(dir)
	t.Cleanup(func() { Close(); SetDirForTesting("") })

	workspace := "/tmp/delete-test"
	s := &Session{
		ID:            "delete-me",
		Name:          "To delete",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Provider:      "openai",
		Model:         "gpt-5.5",
		WorkspaceRoot: workspace,
		Messages:      []llm.Message{{Role: "user", Content: "bye"}},
	}
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Delete(workspace, "delete-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	metas, err := List(workspace)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(metas) != 0 {
		t.Fatalf("expected 0 sessions after delete, got %d", len(metas))
	}
}

func TestGenerateName(t *testing.T) {
	name := generateName([]llm.Message{
		{Role: "assistant", Content: "ignored"},
		{Role: "user", Content: "Fix the bug\nin main.go"},
	})
	if name != "Fix the bug" {
		t.Fatalf("name: got %q want %q", name, "Fix the bug")
	}
}

func TestConcurrentSaveLoad(t *testing.T) {
	dir := t.TempDir()
	SetDirForTesting(dir)
	t.Cleanup(func() { Close(); SetDirForTesting("") })

	workspace := "/tmp/concurrent-test"
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-%d", n)
			s := &Session{
				ID:            id,
				Name:          fmt.Sprintf("Session %d", n),
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Provider:      "test",
				Model:         "test-model",
				WorkspaceRoot: workspace,
				Messages:      []llm.Message{{Role: "user", Content: fmt.Sprintf("hello %d", n)}},
			}
			if err := Save(s); err != nil {
				t.Errorf("Save(%d): %v", n, err)
				return
			}
			loaded, err := Load(workspace, id)
			if err != nil {
				t.Errorf("Load(%d): %v", n, err)
				return
			}
			if loaded.Name != s.Name {
				t.Errorf("Load(%d): got name %q want %q", n, loaded.Name, s.Name)
			}
		}(i)
	}
	wg.Wait()

	metas, err := List(workspace)
	if err != nil {
		t.Fatalf("List after concurrent: %v", err)
	}
	if len(metas) != 10 {
		t.Fatalf("expected 10 sessions, got %d", len(metas))
	}
}

func TestConcurrentSaveList(t *testing.T) {
	dir := t.TempDir()
	SetDirForTesting(dir)
	t.Cleanup(func() { Close(); SetDirForTesting("") })

	workspace := "/tmp/stress-test"

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s := &Session{
				ID:            fmt.Sprintf("stress-%d", n),
				Name:          fmt.Sprintf("Stress %d", n),
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Provider:      "test",
				Model:         "test-model",
				WorkspaceRoot: workspace,
				Messages:      []llm.Message{{Role: "user", Content: fmt.Sprintf("msg %d", n)}},
			}
			if err := Save(s); err != nil {
				t.Errorf("Save(%d): %v", n, err)
			}
		}(i)
	}
	wg.Wait()

	metas, err := List(workspace)
	if err != nil {
		t.Fatalf("List after batch: %v", err)
	}
	if len(metas) != 20 {
		t.Fatalf("expected 20 sessions, got %d", len(metas))
	}
}

func TestConcurrentReadWriteMix(t *testing.T) {
	dir := t.TempDir()
	SetDirForTesting(dir)
	t.Cleanup(func() { Close(); SetDirForTesting("") })

	workspace := "/tmp/mix-test"

	// Seed with 5 sessions
	for i := range 5 {
		s := &Session{
			ID:            fmt.Sprintf("mix-%d", i),
			Name:          fmt.Sprintf("Base %d", i),
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Provider:      "test",
			Model:         "test-model",
			WorkspaceRoot: workspace,
			Messages:      []llm.Message{{Role: "user", Content: fmt.Sprintf("base %d", i)}},
		}
		if err := Save(s); err != nil {
			t.Fatalf("seed Save(%d): %v", i, err)
		}
	}

	var wg sync.WaitGroup

	// 5 concurrent writers
	for i := range 5 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s := &Session{
				ID:            fmt.Sprintf("writer-%d", n),
				Name:          fmt.Sprintf("Writer %d", n),
				CreatedAt:     time.Now(),
				UpdatedAt:     time.Now(),
				Provider:      "test",
				Model:         "test-model",
				WorkspaceRoot: workspace,
				Messages:      []llm.Message{{Role: "user", Content: fmt.Sprintf("write %d", n)}},
			}
			if err := Save(s); err != nil {
				t.Errorf("writer Save(%d): %v", n, err)
			}
		}(i)
	}

	// 5 concurrent readers
	for i := range 5 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			metas, err := List(workspace)
			if err != nil {
				t.Errorf("reader List(%d): %v", n, err)
				return
			}
			if len(metas) < 5 {
				t.Errorf("reader List(%d): got %d metas, expected at least 5", n, len(metas))
			}
		}(i)
	}

	wg.Wait()
}

func TestQuery(t *testing.T) {
	dir := t.TempDir()
	SetDirForTesting(dir)
	t.Cleanup(func() { Close(); SetDirForTesting("") })

	workspace := "/tmp/query-test"
	s := &Session{
		ID:            "query-session",
		Name:          "Query test",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Provider:      "openai",
		Model:         "gpt-5",
		WorkspaceRoot: workspace,
		Messages:      []llm.Message{{Role: "user", Content: "test query"}},
	}
	if err := Save(s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var count int
	err := Query(func(db *sql.DB) error {
		return db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count)
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 session, got %d", count)
	}
}

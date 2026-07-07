package sessions

import (
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

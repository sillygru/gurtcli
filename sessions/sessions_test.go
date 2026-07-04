package sessions

import (
	"testing"
	"time"

	"github.com/sillygru/gurtcli/llm"
)

func TestSaveLoadListRoundTrip(t *testing.T) {
	dir := t.TempDir()
	SetDirForTesting(dir)
	t.Cleanup(func() { SetDirForTesting("") })

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

	metas, err := List(workspace)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 1 {
		t.Fatalf("list count: got %d want 1", len(metas))
	}
	if metas[0].MessageCount != 2 {
		t.Fatalf("message count: got %d want 2", metas[0].MessageCount)
	}
}

func TestActiveSession(t *testing.T) {
	dir := t.TempDir()
	SetDirForTesting(dir)
	t.Cleanup(func() { SetDirForTesting("") })

	workspace := "/tmp/active-workspace"

	if id, err := GetActiveSession(workspace); err != nil || id != "" {
		t.Fatalf("initial active: id=%q err=%v", id, err)
	}

	if err := SetActiveSession(workspace, "session-abc"); err != nil {
		t.Fatalf("SetActiveSession: %v", err)
	}

	id, err := GetActiveSession(workspace)
	if err != nil {
		t.Fatalf("GetActiveSession: %v", err)
	}
	if id != "session-abc" {
		t.Fatalf("active id: got %q want session-abc", id)
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

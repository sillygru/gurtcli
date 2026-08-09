package stats

import (
	"database/sql"
	"testing"

	"github.com/sillygru/gurtcli/sessions"
)

// seedStatsDB points the sessions package at a temp dir, seeds the given SQL
// inside a single sessions.Query call, and cleans up afterwards. Compute must
// be called after seeding returns: it re-enters sessions.Query, which would
// deadlock on the package lock if run from inside another Query callback.
func seedStatsDB(t *testing.T, seedSQL string) {
	t.Helper()
	dir := t.TempDir()
	sessions.SetDirForTesting(dir)
	t.Cleanup(func() {
		sessions.Close()
		sessions.SetDirForTesting("")
	})
	err := sessions.Query(func(db *sql.DB) error {
		_, err := db.Exec(seedSQL)
		return err
	})
	if err != nil {
		t.Fatalf("seeding sessions: %v", err)
	}
}

// TestComputeTokenSums verifies that Compute aggregates the per-session token
// columns (including the cache-write counter) across the whole database.
func TestComputeTokenSums(t *testing.T) {
	seedStatsDB(t, `
		INSERT INTO sessions (id, name, created_at, updated_at, provider, model, workspace_root, messages,
			input_tokens, output_tokens, reasoning_tokens, cache_hit_tokens, cache_write_tokens)
		VALUES
			('t1', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'anthropic', 'm', '/tmp', '[]',
			 10000, 500, 100, 8000, 1000),
			('t2', '', '2026-01-02T00:00:00Z', '2026-01-02T00:00:00Z', 'openai', 'm', '/tmp', '[]',
			 7000, 300, 0, 6000, 0)
	`)

	got, err := Compute()
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got.InputTokens != 17000 {
		t.Errorf("InputTokens = %d, want 17000", got.InputTokens)
	}
	if got.OutputTokens != 800 {
		t.Errorf("OutputTokens = %d, want 800", got.OutputTokens)
	}
	if got.ReasoningTokens != 100 {
		t.Errorf("ReasoningTokens = %d, want 100", got.ReasoningTokens)
	}
	if got.CacheHitTokens != 14000 {
		t.Errorf("CacheHitTokens = %d, want 14000", got.CacheHitTokens)
	}
	if got.CacheWriteTokens != 1000 {
		t.Errorf("CacheWriteTokens = %d, want 1000", got.CacheWriteTokens)
	}
}

// TestComputeTokenSumsZeroMessages ensures the token sums do not depend on the
// messages JSON: a session with zero recorded messages still counts its token
// columns.
func TestComputeTokenSumsZeroMessages(t *testing.T) {
	seedStatsDB(t, `
		INSERT INTO sessions (id, name, created_at, updated_at, provider, model, workspace_root, messages,
			input_tokens, cache_write_tokens)
		VALUES ('t3', '', '2026-01-03T00:00:00Z', '2026-01-03T00:00:00Z', 'anthropic', 'm', '/tmp', '[]',
			555, 44)
	`)

	got, err := Compute()
	if err != nil {
		t.Fatalf("Compute: %v", err)
	}
	if got.InputTokens != 555 || got.CacheWriteTokens != 44 {
		t.Errorf("InputTokens/CacheWriteTokens = %d/%d, want 555/44", got.InputTokens, got.CacheWriteTokens)
	}
	if got.APICalls != 0 || got.UserMessages != 0 {
		t.Errorf("APICalls/UserMessages = %d/%d, want 0/0 for empty transcripts",
			got.APICalls, got.UserMessages)
	}
}

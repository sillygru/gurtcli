package sessions

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Simulates a database created by an older release (schema v4) and checks that
// opening it under the new code walks the whole remaining ladder in place.
func TestMigrateV4ToLatest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL, provider TEXT NOT NULL, model TEXT NOT NULL,
			custom_url TEXT NOT NULL DEFAULT '', saved_endpoint_name TEXT NOT NULL DEFAULT '',
			thinking_type TEXT NOT NULL DEFAULT '', effort_level TEXT NOT NULL DEFAULT '',
			reasoning_visible INTEGER NOT NULL DEFAULT 0, workspace_root TEXT NOT NULL,
			messages TEXT NOT NULL DEFAULT '[]',
			input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0, cache_hit_tokens INTEGER NOT NULL DEFAULT 0
		);
		INSERT INTO sessions (id, name, created_at, updated_at, provider, model, workspace_root, input_tokens)
			VALUES ('old1', 'legacy', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'anthropic', 'm', '/w', 999);
		PRAGMA user_version = 4;
	`); err != nil {
		t.Fatal(err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 6 {
		t.Fatalf("user_version = %d, want 6", v)
	}

	// Existing row survives, new columns take their defaults.
	var name, mode string
	var in, ctx, ctxCache int
	if err := db.QueryRow(`SELECT name, input_tokens, context_tokens, context_cache_tokens, reasoning_mode FROM sessions WHERE id='old1'`).
		Scan(&name, &in, &ctx, &ctxCache, &mode); err != nil {
		t.Fatal(err)
	}
	if name != "legacy" || in != 999 || ctx != 0 || ctxCache != 0 || mode != "" {
		t.Fatalf("got %q %d %d %d %q", name, in, ctx, ctxCache, mode)
	}

	// Migrating again is a no-op, not an error.
	if err := migrate(db); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

// A v5 database is what the release before thinking modes left behind, and it
// is the step most likely to be skipped: v5 was the first migration that did
// not bump the local version variable, so a v6 step appended after it would
// silently never run.
func TestMigrateV5ToV6(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v5.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`
		CREATE TABLE sessions (
			id TEXT PRIMARY KEY, name TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL, provider TEXT NOT NULL, model TEXT NOT NULL,
			custom_url TEXT NOT NULL DEFAULT '', saved_endpoint_name TEXT NOT NULL DEFAULT '',
			thinking_type TEXT NOT NULL DEFAULT '', effort_level TEXT NOT NULL DEFAULT '',
			reasoning_visible INTEGER NOT NULL DEFAULT 1, workspace_root TEXT NOT NULL,
			messages TEXT NOT NULL DEFAULT '[]',
			input_tokens INTEGER NOT NULL DEFAULT 0, output_tokens INTEGER NOT NULL DEFAULT 0,
			reasoning_tokens INTEGER NOT NULL DEFAULT 0, cache_hit_tokens INTEGER NOT NULL DEFAULT 0,
			context_tokens INTEGER NOT NULL DEFAULT 0, context_cache_tokens INTEGER NOT NULL DEFAULT 0
		);
		INSERT INTO sessions (id, name, created_at, updated_at, provider, model, workspace_root)
			VALUES ('old5', 'legacy', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', 'anthropic', 'm', '/w');
		PRAGMA user_version = 5;
	`); err != nil {
		t.Fatal(err)
	}

	if err := migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 6 {
		t.Fatalf("user_version = %d, want 6", v)
	}

	// The empty mode is what makes the old boolean the fallback on load.
	var mode string
	var visible int
	if err := db.QueryRow(`SELECT reasoning_mode, reasoning_visible FROM sessions WHERE id='old5'`).
		Scan(&mode, &visible); err != nil {
		t.Fatal(err)
	}
	if mode != "" || visible != 1 {
		t.Fatalf("reasoning_mode = %q, reasoning_visible = %d; want \"\" and 1", mode, visible)
	}
}

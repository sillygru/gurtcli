package sessions

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Simulates a database created by the previous release (schema v4) and checks
// that opening it under the new code adds the context columns in place.
func TestMigrateV4ToV5(t *testing.T) {
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
	if v != 5 {
		t.Fatalf("user_version = %d, want 5", v)
	}

	// Existing row survives, new columns default to 0.
	var name string
	var in, ctx, ctxCache int
	if err := db.QueryRow(`SELECT name, input_tokens, context_tokens, context_cache_tokens FROM sessions WHERE id='old1'`).
		Scan(&name, &in, &ctx, &ctxCache); err != nil {
		t.Fatal(err)
	}
	if name != "legacy" || in != 999 || ctx != 0 || ctxCache != 0 {
		t.Fatalf("got %q %d %d %d", name, in, ctx, ctxCache)
	}

	// Migrating again is a no-op, not an error.
	if err := migrate(db); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}
}

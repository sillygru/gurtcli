package stats

import (
	"database/sql"
	"testing"
	"time"

	"github.com/sillygru/gurtcli/sessions"
)

// withTestDB points the sessions package at a temp dir, runs fn with the open
// DB, and cleans up afterwards.
func withTestDB(t *testing.T, fn func(db *sql.DB)) {
	t.Helper()
	dir := t.TempDir()
	sessions.SetDirForTesting(dir)
	t.Cleanup(func() {
		sessions.Close()
		sessions.SetDirForTesting("")
	})
	err := sessions.Query(func(db *sql.DB) error {
		fn(db)
		return nil
	})
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
}

// insertSession inserts a minimal session row with the given created_at.
func insertSession(t *testing.T, db *sql.DB, id, createdAt string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO sessions (id, name, created_at, updated_at, provider, model, workspace_root, messages)
		VALUES (?, '', ?, ?, 'test', 'test', '/tmp', '[]')
	`, id, createdAt, createdAt)
	if err != nil {
		t.Fatalf("inserting session %s: %v", id, err)
	}
}

// TestComputeStreaks seeds a sessions DB with known created_at dates and
// verifies current and highest consecutive-day streaks.
func TestComputeStreaks(t *testing.T) {
	withTestDB(t, func(db *sql.DB) {
		// Runs of 3 (Jan 3-5) and 4 (Jan 7-10).
		days := []string{
			"2026-01-01T10:00:00Z",
			"2026-01-03T10:00:00Z",
			"2026-01-04T10:00:00Z",
			"2026-01-05T10:00:00Z",
			"2026-01-07T10:00:00Z",
			"2026-01-08T10:00:00Z",
			"2026-01-09T10:00:00Z",
			"2026-01-10T10:00:00Z",
		}
		for i, d := range days {
			insertSession(t, db, "s"+string(rune('a'+i)), d)
		}

		cur, high, err := computeStreaks(db)
		if err != nil {
			t.Fatalf("computeStreaks: %v", err)
		}
		// "Today" in the test environment is the real current date, so the last
		// seeded day (Jan 10) is in the past and the current streak is 0.
		if high != 4 {
			t.Fatalf("highest streak = %d, want 4", high)
		}
		if cur != 0 {
			t.Fatalf("current streak = %d, want 0 (last activity is in the past)", cur)
		}
	})
}

func TestComputeStreaksCurrentEndsToday(t *testing.T) {
	withTestDB(t, func(db *sql.DB) {
		day := func(offset time.Duration) string {
			return time.Now().UTC().Add(offset).Format("2006-01-02") + "T10:00:00Z"
		}
		insertSession(t, db, "cur1", day(-24*time.Hour))
		insertSession(t, db, "cur2", day(0))

		cur, high, err := computeStreaks(db)
		if err != nil {
			t.Fatalf("computeStreaks: %v", err)
		}
		if cur != 2 || high != 2 {
			t.Fatalf("current=%d highest=%d, want 2/2", cur, high)
		}

		// Extend the run backwards: -96h, -72h, -48h, -24h, and today are all
		// consecutive days, so both streaks become 5.
		insertSession(t, db, "old1", day(-48*time.Hour))
		insertSession(t, db, "old2", day(-72*time.Hour))
		insertSession(t, db, "old3", day(-96*time.Hour))

		cur, high, err = computeStreaks(db)
		if err != nil {
			t.Fatalf("computeStreaks (after older run): %v", err)
		}
		if cur != 5 || high != 5 {
			t.Fatalf("after older run: current=%d highest=%d, want 5/5", cur, high)
		}
	})
}

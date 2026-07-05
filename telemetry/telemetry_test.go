package telemetry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeSig(t *testing.T) {
	sig := ComputeSig("550e8400-e29b-41d4-a716-446655440000", "testproj", "mysecret")
	if sig == "" {
		t.Fatal("sig should not be empty")
	}
	if len(sig) != 64 {
		t.Fatalf("sig length = %d, want 64", len(sig))
	}
}

func TestComputeSigEmptySecret(t *testing.T) {
	sig := ComputeSig("any-uuid", "testproj", "")
	if sig != "" {
		t.Fatal("sig should be empty when secret is empty")
	}
}

func TestLoadOrCreateUUID(t *testing.T) {
	dir := t.TempDir()

	id1 := LoadOrCreateUUID(dir)
	if id1 == "" {
		t.Fatal("got empty uuid")
	}
	if len(id1) != 36 {
		t.Fatalf("uuid length = %d, want 36", len(id1))
	}

	id2 := LoadOrCreateUUID(dir)
	if id2 != id1 {
		t.Fatalf("second call returned different uuid: %q vs %q", id2, id1)
	}

	data, err := os.ReadFile(filepath.Join(dir, uuidFile))
	if err != nil {
		t.Fatalf("uuid file not written: %v", err)
	}
	if string(data) != id1 {
		t.Fatalf("file content = %q, want %q", string(data), id1)
	}
}

func TestLoadOrCreateUUIDEmptyDir(t *testing.T) {
	id := LoadOrCreateUUID("")
	if id != "" {
		t.Fatal("expected empty uuid for empty config dir")
	}
}

func TestUUIDIsValidV4(t *testing.T) {
	dir := t.TempDir()
	id := LoadOrCreateUUID(dir)

	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Fatalf("uuid has %d parts, want 5", len(parts))
	}

	if len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Fatal("uuid parts have wrong lengths")
	}
}

func TestSendEventEmptySecret(t *testing.T) {
	SendEvent("any-uuid", "1.0.0", "startup", "")
}

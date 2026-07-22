package config

import (
	"os"
	"testing"
)

// withTempConfigDir points the config directory at a temp dir for one test.
func withTempConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	got, err := Dir()
	if err != nil {
		t.Fatalf("Dir(): %v", err)
	}
	return got
}

func TestCredFileRoundTrip(t *testing.T) {
	withTempConfigDir(t)

	if _, ok := GetCredFileKey("anthropic"); ok {
		t.Fatal("found a key before anything was stored")
	}
	if err := SetCredFileKey("anthropic", "sk-ant-secret"); err != nil {
		t.Fatalf("SetCredFileKey: %v", err)
	}
	got, ok := GetCredFileKey("anthropic")
	if !ok || got != "sk-ant-secret" {
		t.Fatalf("GetCredFileKey = (%q, %v), want (%q, true)", got, ok, "sk-ant-secret")
	}

	// Accounts are independent, so a saved endpoint cannot shadow a provider.
	if err := SetCredFileKey("saved:groq", "gsk-other"); err != nil {
		t.Fatalf("SetCredFileKey: %v", err)
	}
	if got, _ := GetCredFileKey("anthropic"); got != "sk-ant-secret" {
		t.Errorf("first key changed to %q after storing a second", got)
	}

	if err := DeleteCredFileKey("anthropic"); err != nil {
		t.Fatalf("DeleteCredFileKey: %v", err)
	}
	if _, ok := GetCredFileKey("anthropic"); ok {
		t.Error("key survived deletion")
	}
	if _, ok := GetCredFileKey("saved:groq"); !ok {
		t.Error("deleting one account removed another")
	}
}

// The file holds an API key in plaintext, so the permissions are the only thing
// protecting it. They must be tight on the way out.
func TestCredFileIsOwnerOnly(t *testing.T) {
	withTempConfigDir(t)
	if err := SetCredFileKey("openai", "sk-secret"); err != nil {
		t.Fatalf("SetCredFileKey: %v", err)
	}
	p, err := CredFilePath()
	if err != nil {
		t.Fatalf("CredFilePath: %v", err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("credentials file mode = %04o, want 0600", perm)
	}
}

// And loose permissions must not be served from: a file anyone can read is not
// one to keep handing a secret out of.
func TestCredFileRefusesLoosePermissions(t *testing.T) {
	withTempConfigDir(t)
	if err := SetCredFileKey("openai", "sk-secret"); err != nil {
		t.Fatalf("SetCredFileKey: %v", err)
	}
	p, _ := CredFilePath()
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, ok := GetCredFileKey("openai"); ok {
		t.Error("key was served out of a world-readable file")
	}
}

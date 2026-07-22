package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const credFileName = "credentials.json"

// The credential file is a fallback for machines with no OS keychain — headless
// Linux boxes reached over SSH have no Secret Service, so keyring.Set fails and
// there is nowhere else to put the key but a file. It is plaintext protected
// only by file permissions, the same trade-off as ~/.aws/credentials or
// ~/.netrc, so it is only ever written when the user explicitly picks it, and
// it is only ever read back when the permissions are still tight.
const credFileMode os.FileMode = 0600

// CredFilePath returns the path of the fallback credential store.
func CredFilePath() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, credFileName), nil
}

// loadCredFile reads the credential store, refusing a file that anyone other
// than the owner can read. A botched chmod should cost the user their stored
// key, not silently keep serving it out of a world-readable file.
func loadCredFile() (map[string]string, error) {
	p, err := CredFilePath()
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if info.Mode().Perm()&0077 != 0 {
		logf("credentials file %s has permissions %04o, ignoring it (want 0600)", p, info.Mode().Perm())
		return nil, fmt.Errorf("credentials file %s is readable by other users; run: chmod 600 %s", p, p)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	creds := map[string]string{}
	if len(data) == 0 {
		return creds, nil
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", p, err)
	}
	return creds, nil
}

// saveCredFile writes the store via a 0600 temp file and an atomic rename, so a
// crash mid-write cannot leave a truncated file and the secret is never briefly
// visible at a wider mode.
func saveCredFile(creds map[string]string) error {
	p, err := CredFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	// Dir only computes the path; on a fresh machine nothing has created it yet.
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("creating temp credentials file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(credFileMode); err != nil {
		tmp.Close()
		return fmt.Errorf("setting credentials file permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing credentials file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing credentials file: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		return fmt.Errorf("saving credentials file: %w", err)
	}
	return nil
}

// GetCredFileKey returns the stored key for an account, if there is one.
func GetCredFileKey(account string) (string, bool) {
	creds, err := loadCredFile()
	if err != nil {
		logf("reading credentials file: %v", err)
		return "", false
	}
	key, ok := creds[account]
	return key, ok && key != ""
}

// SetCredFileKey stores a key for an account.
func SetCredFileKey(account, key string) error {
	creds, err := loadCredFile()
	if err != nil {
		// An unreadable or malformed store must not wedge the user out of saving
		// a key; start a fresh one rather than failing forever.
		logf("replacing unreadable credentials file: %v", err)
		creds = map[string]string{}
	}
	creds[account] = key
	return saveCredFile(creds)
}

// DeleteCredFileKey removes an account's key. A missing store is not an error.
func DeleteCredFileKey(account string) error {
	creds, err := loadCredFile()
	if err != nil {
		return nil
	}
	if _, ok := creds[account]; !ok {
		return nil
	}
	delete(creds, account)
	return saveCredFile(creds)
}

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const logFileName = "gurtcli.log"

// logf records a non-fatal diagnostic to ~/.config/gurtcli/gurtcli.log.
//
// These used to go to stderr, which paints over the alt-screen TUI and is
// unreadable over SSH. Nothing in this package may write to stdout or stderr
// once the program is running. Failures here are ignored on purpose: logging
// must never be the reason an operation fails.
func logf(format string, args ...interface{}) {
	dir, err := Dir()
	if err != nil {
		return
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, logFileName), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	debugMu   sync.Mutex
	debugFile *os.File
	debugOnce sync.Once
)

func LogDebug(format string, args ...any) {
	debugMu.Lock()
	defer debugMu.Unlock()

	debugOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		dir := filepath.Join(home, ".config", "gurtcli")
		os.MkdirAll(dir, 0700)
		f, err := os.OpenFile(filepath.Join(dir, "debug.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return
		}
		debugFile = f
	})

	if debugFile == nil {
		return
	}

	msg := fmt.Sprintf("[%s] %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
	debugFile.WriteString(msg)
}

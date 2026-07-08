package history

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sillygru/gurtcli/config"
)

const maxEntries = 1000

func filePath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", fmt.Errorf("getting config dir: %w", err)
	}
	return filepath.Join(dir, "history"), nil
}

func Load() ([]string, error) {
	p, err := filePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading history: %w", err)
	}

	if len(data) == 0 {
		return nil, nil
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var hist []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		entry, err := strconv.Unquote(line)
		if err != nil {
			entry = line
		}
		hist = append(hist, entry)
	}
	return hist, nil
}

func Add(hist []string, input string) []string {
	if len(hist) > 0 && hist[len(hist)-1] == input {
		return hist
	}
	hist = append(hist, input)
	if len(hist) > maxEntries {
		hist = hist[len(hist)-maxEntries:]
	}
	return hist
}

func Save(hist []string) error {
	p, err := filePath()
	if err != nil {
		return err
	}

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating history dir: %w", err)
	}

	lines := make([]string, len(hist))
	for i, entry := range hist {
		lines[i] = fmt.Sprintf("%q", entry)
	}
	data := strings.Join(lines, "\n")
	if err := os.WriteFile(p, []byte(data), 0600); err != nil {
		return fmt.Errorf("writing history: %w", err)
	}
	return nil
}

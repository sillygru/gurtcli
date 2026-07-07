package config

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func DotenvPath() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, ".env"), nil
}

func SaveDotenv(key, value string) error {
	p, err := DotenvPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}

	entry := key + "=" + value

	data, err := os.ReadFile(p)
	if err != nil {
		return os.WriteFile(p, []byte(entry+"\n"), 0600)
	}

	lines := strings.Split(string(data), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimLeftFunc(line, unicode.IsSpace)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if eq := strings.IndexByte(trimmed, '='); eq > 0 {
			k := strings.TrimSpace(trimmed[:eq])
			if k == key {
				lines[i] = entry
				found = true
				break
			}
		}
	}

	if !found {
		lines = append(lines, entry)
	}

	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	return os.WriteFile(p, []byte(output), 0600)
}

func GetDotenvKeys() (map[string]string, error) {
	p, err := DotenvPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if eq := strings.IndexByte(line, '='); eq > 0 {
			k := strings.TrimSpace(line[:eq])
			v := strings.TrimSpace(line[eq+1:])
			if k != "" {
				result[k] = v
			}
		}
	}
	return result, nil
}

func LoadDotenv() error {
	p, err := DotenvPath()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if eq := strings.IndexByte(line, '='); eq > 0 {
			k := strings.TrimSpace(line[:eq])
			v := strings.TrimSpace(line[eq+1:])
			if k != "" {
				os.Setenv(k, v)
			}
		}
	}
	return nil
}

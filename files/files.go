package files

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	MaxWalkFiles = 10000
	MaxFileSize  = 100 * 1024
	MaxFileLines = 2000
)

var mediaExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".bmp":  true,
	".webp": true,
	".svg":  true,
	".ico":  true,
	".tiff": true,
	".tif":  true,
	".mp4":  true,
	".avi":  true,
	".mkv":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".webm": true,
	".m4v":  true,
	".mpg":  true,
	".mpeg": true,
	".pdf":  true,
	".woff": true,
	".woff2": true,
	".eot":  true,
	".ttf":  true,
	".otf":  true,
}

var skippedDirs = map[string]bool{
	"node_modules": true,
	"vendor":       true,
	".next":        true,
	"dist":         true,
	"build":        true,
	".cache":       true,
	"target":       true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	"out":          true,
	".terraform":   true,
}

func isMediaFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return mediaExtensions[ext]
}

func isSkippedDir(name string) bool {
	if skippedDirs[name] {
		return true
	}
	return strings.HasPrefix(name, ".")
}

func IsHomeDir(root string) bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	cleanRoot := filepath.Clean(root)
	cleanHome := filepath.Clean(home)
	return cleanRoot == cleanHome
}

func WalkWorkspace(root string, maxFiles int) ([]string, error) {
	cleanRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		cleanRoot = root
	}

	var files []string

	err = filepath.WalkDir(cleanRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if len(files) >= maxFiles {
			return fs.SkipAll
		}

		rel, err := filepath.Rel(cleanRoot, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}

		if d.IsDir() {
			if isSkippedDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}

		if isMediaFile(d.Name()) {
			return nil
		}

		files = append(files, rel)
		return nil
	})

	return files, err
}

func ReadFileContents(root, relPath string) (string, error) {
	cleanRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		cleanRoot = root
	}

	fullPath := filepath.Join(cleanRoot, relPath)
	cleanPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(cleanPath, cleanRoot) {
		return "", nil
	}

	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", err
	}

	if info.Size() > MaxFileSize {
		return "", nil
	}

	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) > MaxFileLines {
		lines = lines[:MaxFileLines]
	}

	return strings.Join(lines, "\n"), nil
}

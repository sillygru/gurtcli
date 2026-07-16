package files

import (
	"bufio"
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

type gitignorePattern struct {
	pattern string
	negate  bool
	// baseDir is the relative path of the directory containing this .gitignore
	// (empty string for root). Used to scope patterns correctly.
	baseDir string
}

func loadGitignorePatterns(dirPath, baseDirRel string) ([]gitignorePattern, error) {
	f, err := os.Open(filepath.Join(dirPath, ".gitignore"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var patterns []gitignorePattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}
		// Strip trailing slash for directory-only patterns
		line = strings.TrimSuffix(line, "/")

		// Patterns from subdirectory .gitignore need to be scoped.
		// If the pattern contains a slash (after stripping leading ! and /**),
		// it's relative to the .gitignore location, so we prepend baseDir.
		// If it has no slash, it matches against the basename (works globally).
		effective := line
		if baseDirRel != "" && strings.Contains(line, "/") {
			effective = baseDirRel + "/" + line
		}
		patterns = append(patterns, gitignorePattern{pattern: effective, negate: negate, baseDir: baseDirRel})
	}
	return patterns, scanner.Err()
}

func matchesGitignorePattern(relPath string, patterns []gitignorePattern) bool {
	matched := false
	for _, p := range patterns {
		// Try matching against the full relative path
		if match, _ := filepath.Match(p.pattern, relPath); match {
			matched = !p.negate
			continue
		}
		// Try matching the basename (for patterns without a slash)
		if !strings.Contains(p.pattern, "/") {
			base := filepath.Base(relPath)
			if match, _ := filepath.Match(p.pattern, base); match {
				matched = !p.negate
				continue
			}
		}
		// Try matching with leading **/
		if strings.HasPrefix(p.pattern, "**/") {
			suffix := p.pattern[3:]
			if match, _ := filepath.Match(suffix, relPath); match {
				matched = !p.negate
				continue
			}
			// Also check basename with **/
			base := filepath.Base(relPath)
			if match, _ := filepath.Match(suffix, base); match {
				matched = !p.negate
				continue
			}
		}
		// Try matching a prefix (directory-only pattern, e.g. "build" matches "build/foo.o")
		if strings.HasPrefix(relPath, p.pattern+"/") || relPath == p.pattern {
			matched = !p.negate
			continue
		}
		// Try matching a pattern with a directory component (e.g. "build/*.o")
		baseName := filepath.Base(relPath)
		basePattern := filepath.Base(p.pattern)
		dirPattern := filepath.Dir(p.pattern)
		dirName := filepath.Dir(relPath)
		if dirPattern == "." {
			dirPattern = ""
		}
		if strings.HasPrefix(dirName, dirPattern) || dirPattern == "" {
			if match, _ := filepath.Match(basePattern, baseName); match {
				matched = !p.negate
				continue
			}
		}
	}
	return matched
}

// dirPatterns holds the gitignore patterns loaded from a specific directory.
type dirPatterns struct {
	dirRel   string // relative path of the directory (empty for root)
	patterns []gitignorePattern
}

func WalkWorkspace(root string, maxFiles int) ([]string, error) {
	cleanRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		cleanRoot = root
	}

	// Load root .gitignore
	rootPatterns, _ := loadGitignorePatterns(cleanRoot, "")
	var patternStack []dirPatterns
	if len(rootPatterns) > 0 {
		patternStack = append(patternStack, dirPatterns{dirRel: "", patterns: rootPatterns})
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

		// Pop patterns from the stack that are no longer ancestors of the current path
		for len(patternStack) > 1 {
			top := patternStack[len(patternStack)-1].dirRel
			if top == "" {
				break // root is always an ancestor
			}
			if rel == top || strings.HasPrefix(rel, top+"/") {
				break
			}
			patternStack = patternStack[:len(patternStack)-1]
		}

		// Build active patterns from the stack (root first, then nested)
		var activePatterns []gitignorePattern
		for _, dp := range patternStack {
			activePatterns = append(activePatterns, dp.patterns...)
		}

		if d.IsDir() {
			if isSkippedDir(d.Name()) {
				return fs.SkipDir
			}
			if matchesGitignorePattern(rel+"/", activePatterns) {
				return fs.SkipDir
			}
			// Load subdirectory .gitignore (if any) and push onto stack
			if subPatterns, err := loadGitignorePatterns(path, rel); err == nil && len(subPatterns) > 0 {
				patternStack = append(patternStack, dirPatterns{dirRel: rel, patterns: subPatterns})
			}
			return nil
		}

		if isMediaFile(d.Name()) {
			return nil
		}

		if matchesGitignorePattern(rel, activePatterns) {
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

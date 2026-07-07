package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/sillygru/gurtcli/llm"
)

type Options struct {
	WorkspaceRoot string
}

// safePath resolves path relative to workspace root and verifies it stays within.
func safePath(workspaceRoot, path string) (string, error) {
	if workspaceRoot == "" {
		return "", fmt.Errorf("workspace root is not set")
	}
	cleanRoot, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil {
		return "", fmt.Errorf("resolving workspace root: %w", err)
	}
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		cleanPath = filepath.Join(cleanRoot, cleanPath)
	}
	cleanPath = filepath.Clean(cleanPath)

	// Resolve symlinks. If the full path doesn't exist yet, try the parent.
	if resolved, err := filepath.EvalSymlinks(cleanPath); err == nil {
		cleanPath = resolved
	} else if parent, err := filepath.EvalSymlinks(filepath.Dir(cleanPath)); err == nil {
		cleanPath = filepath.Join(parent, filepath.Base(cleanPath))
	}

	if !strings.HasPrefix(cleanPath, cleanRoot) {
		return "", fmt.Errorf("path %q escapes workspace root", path)
	}
	return cleanPath, nil
}

type readFileArgs struct {
	FilePath string `json:"filePath"`
	Offset   int    `json:"offset"`
	Limit    int    `json:"limit"`
}

type writeFileArgs struct {
	FilePath string `json:"filePath"`
	Content  string `json:"content"`
}

type editFileArgs struct {
	FilePath  string `json:"filePath"`
	OldString string `json:"oldString"`
	NewString string `json:"newString"`
}

type deleteFileArgs struct {
	FilePath string `json:"filePath"`
}

type runBashArgs struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
	Title   string `json:"title"`
}

func Definitions() []llm.Tool {
	return []llm.Tool{
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "read_file",
				Description: "Read the contents of a file. Supports optional line offset and limit for reading specific sections.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"filePath": { "type": "string", "description": "Absolute path or path relative to workspace root" },
						"offset":   { "type": "integer", "description": "Starting line number (1-indexed)" },
						"limit":    { "type": "integer", "description": "Maximum number of lines to read" }
					},
					"required": ["filePath"]
				}`),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "write_file",
				Description: "Create a new file or overwrite an existing file with the given content. Creates parent directories if they don't exist.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"filePath": { "type": "string", "description": "Absolute path or path relative to workspace root" },
						"content":  { "type": "string", "description": "Full content to write to the file" }
					},
					"required": ["filePath", "content"]
				}`),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "edit_file",
				Description: "Replace an exact string match in a file with new text. Fails if the old string is not found or if it matches more than once. Prefer this over write_file for targeted changes.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"filePath":  { "type": "string", "description": "Absolute path or path relative to workspace root" },
						"oldString": { "type": "string", "description": "Exact text to search for and replace" },
						"newString": { "type": "string", "description": "Text to replace it with" }
					},
					"required": ["filePath", "oldString", "newString"]
				}`),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "delete_file",
				Description: "Delete a file from the filesystem. The path must be within the workspace root.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"filePath": { "type": "string", "description": "Absolute path or path relative to workspace root" }
					},
					"required": ["filePath"]
				}`),
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "run_bash",
				Description: "Execute a shell command and return its output. Captures both stdout and stderr. Use this to build, test, lint, or run shell utilities.",
				Parameters: json.RawMessage(`{
					"type": "object",
					"properties": {
						"command": { "type": "string", "description": "Shell command to execute" },
						"timeout":  { "type": "integer", "description": "Timeout in milliseconds (default 30000)" },
						"title":   { "type": "string", "description": "Brief human-readable description of what this command does (e.g. 'Install dependencies', 'Run tests')" }
					},
					"required": ["command", "title"]
				}`),
			},
		},
	}
}

// Execute dispatches a tool call to the appropriate implementation.
func Execute(ctx context.Context, name string, args json.RawMessage, opts Options) (string, error) {
	switch name {
	case "read_file":
		var a readFileArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid read_file arguments: %w", err)
		}
		return ReadFile(opts.WorkspaceRoot, a.FilePath, a.Offset, a.Limit)

	case "write_file":
		var a writeFileArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid write_file arguments: %w", err)
		}
		return WriteFile(opts.WorkspaceRoot, a.FilePath, a.Content)

	case "edit_file":
		var a editFileArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid edit_file arguments: %w", err)
		}
		return EditFile(opts.WorkspaceRoot, a.FilePath, a.OldString, a.NewString)

	case "delete_file":
		var a deleteFileArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid delete_file arguments: %w", err)
		}
		return DeleteFile(opts.WorkspaceRoot, a.FilePath)

	case "run_bash":
		var a runBashArgs
		if err := json.Unmarshal(args, &a); err != nil {
			return "", fmt.Errorf("invalid run_bash arguments: %w", err)
		}
		timeout := a.Timeout
		if timeout <= 0 {
			timeout = 30000
		}
		return RunBash(ctx, a.Command, timeout)

	default:
		return "", fmt.Errorf("unknown tool: %s", name)
	}
}

// IsDestructive returns true if the tool is inherently destructive (write, edit, delete, shell).
func IsDestructive(name string) bool {
	switch name {
	case "write_file", "edit_file", "delete_file", "run_bash":
		return true
	default:
		return false
	}
}

// DefaultSafeBashPrefixes returns the built-in set of safe (read-only) command prefixes.
func DefaultSafeBashPrefixes() []string {
	return []string{
		"cat", "ls", "grep", "find", "head", "tail", "echo", "pwd",
		"which", "whoami", "date", "env", "printenv", "wc", "sort",
		"uniq", "cut", "tr", "diff", "cmp", "file", "stat", "du", "df",
		"ps", "type", "man", "whatis", "apropos", "strings", "od",
		"xxd", "hexdump", "base64", "cksum", "tree", "dirname",
		"basename", "realpath", "readlink", "printf", "yes", "cal",
	}
}

// BashCommandPrefix extracts the first word from a shell command.
func BashCommandPrefix(command string) string {
	command = strings.TrimSpace(command)
	if idx := strings.IndexAny(command, " \t"); idx > 0 {
		return command[:idx]
	}
	return command
}

// ExtractBashCommand parses a run_bash tool call arguments and returns the command string.
func ExtractBashCommand(args json.RawMessage) (string, error) {
	var a runBashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("invalid run_bash arguments: %w", err)
	}
	return a.Command, nil
}



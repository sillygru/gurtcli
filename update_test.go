package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sillygru/gurtcli/llm"
)

func TestCollectFileAttachments(t *testing.T) {
	tmpDir := t.TempDir()

	fooContent := "package foo\n\nfunc Foo() int { return 1 }"
	barContent := "# Bar\n\nThis is bar.md"
	if err := os.WriteFile(filepath.Join(tmpDir, "foo.go"), []byte(fooContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bar.md"), []byte(barContent), 0644); err != nil {
		t.Fatal(err)
	}

	m := model{workspaceRoot: tmpDir}

	tests := []struct {
		name  string
		model model
		msgs  []llm.Message
		want  string
	}{
		{
			name:  "no at refs",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "hello world"}},
			want:  "",
		},
		{
			name:  "single valid go file",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@foo.go fix the bug"}},
			want:  "Contents of foo.go:\n```go\n" + fooContent + "\n```\n\n",
		},
		{
			name:  "markdown file",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@bar.md read this"}},
			want:  "Contents of bar.md:\n```md\n" + barContent + "\n```\n\n",
		},
		{
			name:  "nonexistent file",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@nonexistent.md hello"}},
			want:  "",
		},
		{
			name:  "empty messages",
			model: m,
			msgs:  []llm.Message{},
			want:  "",
		},
		{
			name:  "non-user last message",
			model: m,
			msgs:  []llm.Message{{Role: "assistant", Content: "@foo.go"}},
			want:  "",
		},
		{
			name:  "multiple files",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@foo.go @bar.md compare"}},
			want:  "Contents of foo.go:\n```go\n" + fooContent + "\n```\n\nContents of bar.md:\n```md\n" + barContent + "\n```\n\n",
		},
		{
			name:  "no workspace root",
			model: model{workspaceRoot: ""},
			msgs:  []llm.Message{{Role: "user", Content: "@foo.go"}},
			want:  "",
		},
		{
			name:  "partial match one valid one missing",
			model: m,
			msgs:  []llm.Message{{Role: "user", Content: "@foo.go @missing.go compare"}},
			want:  "Contents of foo.go:\n```go\n" + fooContent + "\n```\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectFileAttachments(tt.model, tt.msgs)
			if tt.want == "" && got != "" {
				t.Errorf("expected empty, got %q", got)
			} else if tt.want != "" && got != tt.want {
				t.Errorf("expected:\n%q\ngot:\n%q", tt.want, got)
			}
		})
	}
}

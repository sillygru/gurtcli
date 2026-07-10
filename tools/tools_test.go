package tools

import (
	"testing"
)

func TestBashCommandPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple command",
			input: "npm run build",
			want:  "npm",
		},
		{
			name:  "chained with &&",
			input: "cd path && git add .",
			want:  "cd",
		},
		{
			name:  "chained with ||",
			input: "false || echo hi",
			want:  "false",
		},
		{
			name:  "chained with pipe",
			input: "ls -la | grep foo",
			want:  "ls",
		},
		{
			name:  "chained with semicolon",
			input: "cd path ; git add .",
			want:  "cd",
		},
		{
			name:  "double-quoted string with operator",
			input: `echo "a && b"`,
			want:  "echo",
		},
		{
			name:  "single-quoted string with operator",
			input: `echo 'a && b'`,
			want:  "echo",
		},
		{
			name:  "command with flags",
			input: "ls -la",
			want:  "ls",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "whitespace only",
			input: "   ",
			want:  "",
		},
		{
			name:  "operator at start",
			input: "&& echo hi",
			want:  "",
		},
		{
			name:  "no leading space before operator",
			input: "cd path&&echo hi",
			want:  "cd",
		},
		{
			name:  "single and double quote nesting",
			input: `echo "'hello' && world"`,
			want:  "echo",
		},
		{
			name:  "single command no args",
			input: "cd",
			want:  "cd",
		},
		{
			name:  "pipe without operator spacing",
			input: "cmd1|cmd2",
			want:  "cmd1",
		},
		{
			name:  "curl with url",
			input: `curl "http://example.com"`,
			want:  "curl",
		},
		{
			name:  "curl with single-quoted url",
			input: "curl 'http://example.com'",
			want:  "curl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BashCommandPrefix(tt.input)
			if got != tt.want {
				t.Errorf("BashCommandPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

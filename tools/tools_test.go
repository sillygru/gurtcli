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

func TestIsSudoCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "sudo with command", input: "sudo apt update", want: true},
		{name: "sudo with flags", input: "sudo -E env", want: true},
		{name: "sudo alone", input: "sudo", want: true},
		{name: "sudo with leading space", input: "  sudo rm -rf /", want: true},
		{name: "not sudo", input: "ls -la", want: false},
		{name: "empty", input: "", want: false},
		{name: "sudosomething", input: "sudosomething", want: false},
		{name: "sudo as substring", input: "notsudo", want: false},
		{name: "sudo in quotes", input: `echo "sudo"`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSudoCommand(tt.input)
			if got != tt.want {
				t.Errorf("IsSudoCommand(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeShellArg(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "simple", input: "hello", want: "'hello'"},
		{name: "with single quote", input: "it's", want: `'it'\''s'`},
		{name: "with spaces", input: "my password", want: "'my password'"},
		{name: "empty", input: "", want: "''"},
		{name: "multiple quotes", input: `a'b'c`, want: `'a'\''b'\''c'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EscapeShellArg(tt.input)
			if got != tt.want {
				t.Errorf("EscapeShellArg(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMatchCommandPattern(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		cmd     string
		want    bool
	}{
		{name: "exact match success", pattern: "git commit", cmd: "git commit", want: true},
		{name: "exact match reject args", pattern: "git commit", cmd: "git commit -m \"foo\"", want: false},
		{name: "wildcard prefix match success", pattern: "git commit *", cmd: "git commit -m \"foo\"", want: true},
		{name: "wildcard prefix match exact cmd", pattern: "git commit *", cmd: "git commit", want: true},
		{name: "wildcard prefix match reject non-matching", pattern: "git commit *", cmd: "git push origin main", want: false},
		{name: "wildcard prefix no space", pattern: "git commit*", cmd: "git commit-tree", want: true},
		{name: "single word wildcard match", pattern: "git *", cmd: "git status", want: true},
		{name: "single word wildcard reject", pattern: "git *", cmd: "npm test", want: false},
		{name: "middle wildcard match", pattern: "npm run * --force", cmd: "npm run build --force", want: true},
		{name: "empty inputs", pattern: "", cmd: "git commit", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchCommandPattern(tt.pattern, tt.cmd)
			if got != tt.want {
				t.Errorf("MatchCommandPattern(%q, %q) = %v, want %v", tt.pattern, tt.cmd, got, tt.want)
			}
		})
	}
}

func TestDefaultCommandPattern(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{name: "git commit with flags", cmd: "git commit -m \"initial\"", want: "git commit *"},
		{name: "npm run dev", cmd: "npm run dev", want: "npm run *"},
		{name: "single word command", cmd: "ls", want: "ls *"},
		{name: "command with flag as 2nd word", cmd: "ls -la", want: "ls *"},
		{name: "empty string", cmd: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultCommandPattern(tt.cmd)
			if got != tt.want {
				t.Errorf("DefaultCommandPattern(%q) = %q, want %q", tt.cmd, got, tt.want)
			}
		})
	}
}


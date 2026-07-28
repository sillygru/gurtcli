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

func TestSplitCdCommand(t *testing.T) {
	tests := []struct {
		name        string
		cmd         string
		wantCdDir   string
		wantRestCmd string
		wantHasCd   bool
	}{
		{
			name:        "cd with && and subcommand",
			cmd:         "cd /path/to/dir && npm test",
			wantCdDir:   "/path/to/dir",
			wantRestCmd: "npm test",
			wantHasCd:   true,
		},
		{
			name:        "cd with trailing slash and semicolon",
			cmd:         "cd /path/to/dir/ ; git status",
			wantCdDir:   "/path/to/dir/",
			wantRestCmd: "git status",
			wantHasCd:   true,
		},
		{
			name:        "cd with single quoted path",
			cmd:         "cd '/path/with space' && go test ./...",
			wantCdDir:   "/path/with space",
			wantRestCmd: "go test ./...",
			wantHasCd:   true,
		},
		{
			name:        "cd with double quoted path",
			cmd:         `cd "/path/with space" && go test ./...`,
			wantCdDir:   "/path/with space",
			wantRestCmd: "go test ./...",
			wantHasCd:   true,
		},
		{
			name:        "cd without subcommand",
			cmd:         "cd /path/to/dir",
			wantCdDir:   "/path/to/dir",
			wantRestCmd: "",
			wantHasCd:   true,
		},
		{
			name:        "cd alone",
			cmd:         "cd",
			wantCdDir:   "",
			wantRestCmd: "",
			wantHasCd:   true,
		},
		{
			name:        "not a cd command",
			cmd:         "ls -la",
			wantCdDir:   "",
			wantRestCmd: "",
			wantHasCd:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCdDir, gotRestCmd, gotHasCd := SplitCdCommand(tt.cmd)
			if gotCdDir != tt.wantCdDir || gotRestCmd != tt.wantRestCmd || gotHasCd != tt.wantHasCd {
				t.Errorf("SplitCdCommand(%q) = (%q, %q, %v), want (%q, %q, %v)",
					tt.cmd, gotCdDir, gotRestCmd, gotHasCd, tt.wantCdDir, tt.wantRestCmd, tt.wantHasCd)
			}
		})
	}
}

func TestEffectiveBashCommand(t *testing.T) {
	ws := t.TempDir()

	eff, cdDir, isOutside, hasCd := EffectiveBashCommand(ws, "cd "+ws+" && npm test")
	if eff != "npm test" || !hasCd || isOutside || cdDir != ws {
		t.Errorf("EffectiveBashCommand(current) = (%q, %q, %v, %v), want (\"npm test\", %q, false, true)", eff, cdDir, isOutside, hasCd, ws)
	}

	extDir := "/tmp/some_outside_dir_12345"
	effExt, cdDirExt, isOutsideExt, hasCdExt := EffectiveBashCommand(ws, "cd "+extDir+" && git status")
	if effExt != "git status" || !hasCdExt || !isOutsideExt || cdDirExt != extDir {
		t.Errorf("EffectiveBashCommand(outside) = (%q, %q, %v, %v), want (\"git status\", %q, true, true)", effExt, cdDirExt, isOutsideExt, hasCdExt, extDir)
	}
}

func TestTokenizeCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []string
	}{
		{name: "simple command", cmd: "ls -la /tmp", want: []string{"ls", "-la", "/tmp"}},
		{name: "single quoted arg", cmd: `grep 'foo bar' /tmp/file`, want: []string{"grep", "'foo bar'", "/tmp/file"}},
		{name: "double quoted arg", cmd: `echo "hello world"`, want: []string{"echo", `"hello world"`}},
		{name: "mixed quotes", cmd: `find /path -name "*.go"`, want: []string{"find", "/path", "-name", `"*.go"`}},
		{name: "empty", cmd: "", want: nil},
		{name: "whitespace", cmd: "   ", want: nil},
		{name: "single token", cmd: "ls", want: []string{"ls"}},
		{name: "trailing spaces", cmd: "  cat /etc/passwd  ", want: []string{"cat", "/etc/passwd"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenizeCommand(tt.cmd)
			if len(got) != len(tt.want) {
				t.Errorf("tokenizeCommand(%q) = %v (len=%d), want %v (len=%d)", tt.cmd, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("tokenizeCommand(%q)[%d] = %q, want %q", tt.cmd, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsPathLike(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{s: "/tmp", want: true},
		{s: "/home/user/file.txt", want: true},
		{s: "~/documents", want: true},
		{s: "./relative", want: true},
		{s: "../parent", want: true},
		{s: "file.txt", want: false},
		{s: "myfile", want: false},
		{s: "-n", want: false},
		{s: "--recursive", want: false},
		{s: "pattern", want: false},
		{s: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := isPathLike(tt.s)
			if got != tt.want {
				t.Errorf("isPathLike(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestExtractPathArgFromCommand(t *testing.T) {
	ws := t.TempDir()
	insidePath := ws + "/inside"

	tests := []struct {
		name          string
		cmd           string
		wantPath      string
		wantIsOutside bool
		wantFound     bool
	}{
		{
			name:          "find with external path",
			cmd:           "find /tmp -name foo",
			wantPath:      "/tmp",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "find with external path no flags",
			cmd:           "find /tmp",
			wantPath:      "/tmp",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "ls with external path",
			cmd:           "ls -la /tmp",
			wantPath:      "/tmp",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "cat with external file",
			cmd:           "cat /etc/passwd",
			wantPath:      "/etc/passwd",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "grep skips pattern and takes path",
			cmd:           "grep -r pattern /tmp",
			wantPath:      "/tmp",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "grep with flags before pattern",
			cmd:           "grep -r -n pattern /tmp/file",
			wantPath:      "/tmp/file",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "chmod skips mode and takes path",
			cmd:           "chmod 755 /some/script.sh",
			wantPath:      "/some/script.sh",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "chown skips user and takes path",
			cmd:           "chown user:group /some/file",
			wantPath:      "/some/file",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "inside workspace path returns found but not outside",
			cmd:           "ls " + insidePath,
			wantPath:      insidePath,
			wantIsOutside: false,
			wantFound:     true,
		},
		{
			name:          "not a path command",
			cmd:           "echo hello",
			wantPath:      "",
			wantIsOutside: false,
			wantFound:     false,
		},
		{
			name:          "no path argument",
			cmd:           "ls",
			wantPath:      "",
			wantIsOutside: false,
			wantFound:     false,
		},
		{
			name:          "not a path-like argument",
			cmd:           "find foo",
			wantPath:      "",
			wantIsOutside: false,
			wantFound:     false,
		},
		{
			name:          "empty command",
			cmd:           "",
			wantPath:      "",
			wantIsOutside: false,
			wantFound:     false,
		},
		{
			name:          "tree with external path",
			cmd:           "tree /tmp",
			wantPath:      "/tmp",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "head with external file",
			cmd:           "head -n 20 /var/log/syslog",
			wantPath:      "/var/log/syslog",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "tail with external file",
			cmd:           "tail -f /var/log/syslog",
			wantPath:      "/var/log/syslog",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "rm on external path",
			cmd:           "rm -rf /tmp/cache",
			wantPath:      "/tmp/cache",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "cp with external source",
			cmd:           "cp /tmp/source /tmp/dest",
			wantPath:      "/tmp/source",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "diff with external left",
			cmd:           "diff /tmp/a /tmp/b",
			wantPath:      "/tmp/a",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "sed with external file",
			cmd:           "sed -i 's/foo/bar/g' /tmp/file.txt",
			wantPath:      "/tmp/file.txt",
			wantIsOutside: true,
			wantFound:     true,
		},
		{
			name:          "relative path ./ is found",
			cmd:           "cat ./somefile",
			wantPath:      "./somefile",
			wantIsOutside: IsPathOutsideWorkspace(ws, "./somefile"),
			wantFound:     true,
		},
		{
			name:          "home dir ~ path is found",
			cmd:           "cat ~/.bashrc",
			wantPath:      "~/.bashrc",
			wantIsOutside: false, // ~ is not expanded, so it's treated as relative to workspace
			wantFound:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotIsOutside, gotFound := ExtractPathArgFromCommand(ws, tt.cmd)
			if gotPath != tt.wantPath || gotIsOutside != tt.wantIsOutside || gotFound != tt.wantFound {
				t.Errorf("ExtractPathArgFromCommand(%q, %q) = (%q, %v, %v), want (%q, %v, %v)",
					ws, tt.cmd, gotPath, gotIsOutside, gotFound, tt.wantPath, tt.wantIsOutside, tt.wantFound)
			}
		})
	}
}

func TestCommandsWithPathArg(t *testing.T) {
	cmds := CommandsWithPathArg()
	if len(cmds) == 0 {
		t.Fatal("CommandsWithPathArg() returned empty map")
	}

	// Verify key commands are present.
	expected := []string{"find", "ls", "cat", "grep", "sed", "rm", "cp", "mv", "chmod", "chown"}
	for _, e := range expected {
		if !cmds[e] {
			t.Errorf("CommandsWithPathArg() missing expected command %q", e)
		}
	}
}



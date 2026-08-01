package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"text/template"
)

func TestRenderSystemPromptSubstitutesVariables(t *testing.T) {
	m := model{workspaceRoot: "/work", modelName: "gpt-test"}

	got, err := renderSystemPrompt(m)
	if err != nil {
		t.Fatalf("renderSystemPrompt: %v", err)
	}

	for _, want := range []string{
		"gpt-test",
		"/work",
		runtime.GOOS,
		runtime.GOARCH,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered prompt missing %q", want)
		}
	}
}

func TestRenderSystemPromptAppendsAGENTS(t *testing.T) {
	tmp := t.TempDir()
	content := "# Project rules\n- Always handle errors\n"
	if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m := model{workspaceRoot: tmp, modelName: "gpt-test"}
	got, err := renderSystemPrompt(m)
	if err != nil {
		t.Fatalf("renderSystemPrompt: %v", err)
	}

	if !strings.Contains(got, "## AGENTS.md") {
		t.Errorf("expected AGENTS.md section, got:\n%s", got)
	}
	if !strings.Contains(got, "- Always handle errors") {
		t.Errorf("expected AGENTS.md body to be appended, got:\n%s", got)
	}
}

func TestRenderSystemPromptSkipsMissingOrEmptyAGENTS(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		write   bool
	}{
		{name: "missing file", write: false},
		{name: "empty file", write: true, content: ""},
		{name: "whitespace only", write: true, content: "   \n\t\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			if tt.write {
				if err := os.WriteFile(filepath.Join(tmp, "AGENTS.md"), []byte(tt.content), 0644); err != nil {
					t.Fatal(err)
				}
			}

			m := model{workspaceRoot: tmp, modelName: "gpt-test"}
			got, err := renderSystemPrompt(m)
			if err != nil {
				t.Fatalf("renderSystemPrompt: %v", err)
			}

			if strings.Contains(got, "AGENTS.md") {
				t.Errorf("AGENTS.md section should be omitted, got:\n%s", got)
			}
		})
	}
}

func TestEmbeddedPromptsRender(t *testing.T) {
	if strings.TrimSpace(systemPromptTemplate) == "" {
		t.Fatal("systemPromptTemplate is empty")
	}
	if strings.TrimSpace(sessionTitlePrompt) == "" {
		t.Fatal("sessionTitlePrompt is empty")
	}

	// Every {{.Var}} in system.md must be provided by renderSystemPrompt, so
	// executing the template with the full variable set must succeed and leave
	// no unresolved references behind.
	tmpl, err := template.New("system").Parse(systemPromptTemplate)
	if err != nil {
		t.Fatalf("parsing systemPromptTemplate: %v", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, map[string]string{
		"OS":        runtime.GOOS,
		"Arch":      runtime.GOARCH,
		"Workspace": "/work",
		"CWD":       "/work",
		"Model":     "gpt-test",
	}); err != nil {
		t.Fatalf("executing systemPromptTemplate: %v", err)
	}
	if strings.Contains(buf.String(), "{{") {
		t.Errorf("systemPromptTemplate left unresolved template vars:\n%s", buf.String())
	}

	// session-title.md must be a plain, single-line-contract prompt with no
	// template placeholders.
	if strings.Contains(sessionTitlePrompt, "{{") {
		t.Errorf("sessionTitlePrompt must not contain template vars:\n%s", sessionTitlePrompt)
	}
}

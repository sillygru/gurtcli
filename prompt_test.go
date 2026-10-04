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

// Every tool result the harness produces must start with the status line the
// system prompt tells the model to look for. Without it the model cannot tell a
// failed or truncated call from a clean one, which is the mechanism behind
// post-tool-failure fabrication.
func TestToolResultHeaderContract(t *testing.T) {
	ok := toolResultHeader(true)
	failed := toolResultHeader(false)

	if ok == failed {
		t.Fatal("OK and FAILED headers must differ, otherwise the status line carries no signal")
	}
	for _, h := range []string{ok, failed} {
		if strings.Contains(h, "{{") {
			t.Errorf("tool result header must be static, got %q", h)
		}
		if strings.TrimSpace(h) != h {
			t.Errorf("tool result header must not carry surrounding whitespace, got %q", h)
		}
		if strings.Contains(h, "\n") {
			t.Errorf("tool result header must be a single line, got %q", h)
		}
	}
}

// The system prompt documents a specific status vocabulary. If the harness
// emits anything else, the contract in the prompt is a lie.
func TestSystemPromptDocumentsStatusVocabulary(t *testing.T) {
	for _, token := range []string{toolResultHeader(true), toolResultHeader(false)} {
		if !strings.Contains(systemPromptTemplate, token) {
			t.Errorf("system prompt does not document %q, so the model is never told the status line exists", token)
		}
	}
}

// A prompt that tells the model not to verify is a prompt that produces
// confident, wrong reports. These instructions are the harness's only defense.
func TestSystemPromptKeepsVerificationDiscipline(t *testing.T) {
	required := []string{
		"ran the verification",
		"did not verify",
		"status: FAILED",
		"Never invent",
	}
	for _, phrase := range required {
		if !strings.Contains(systemPromptTemplate, phrase) {
			t.Errorf("system prompt lost required verification guidance: %q", phrase)
		}
	}
}

// The plan-then-deliberate instruction pattern measurably burned reasoning
// budget without improving outcomes, so it should not come back.
func TestSystemPromptDoesNotEncourageUpfrontPlanning(t *testing.T) {
	banned := []string{
		"Plan, then execute",
		"Before multi-step work, state a short plan",
		"state a short plan",
	}
	for _, phrase := range banned {
		if strings.Contains(systemPromptTemplate, phrase) {
			t.Errorf("system prompt reintroduced deliberation-heavy planning: %q", phrase)
		}
	}
}

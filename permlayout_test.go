package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/viewport"
	"github.com/sillygru/gurtcli/llm"
	"github.com/sillygru/gurtcli/ui"
)

func permModel(w, h int, name string, args string, ext string, sudo bool) model {
	m := model{
		theme:        ui.ThemeRegistry[0].NewFunc(),
		width:        w,
		height:       h,
		chatViewport: viewport.New(),
		toolExec:     &toolExecState{},
		pendingPerm: &pendingPerm{
			toolCall:     llm.ToolCall{Function: llm.ToolCallFunction{Name: name, Arguments: args}},
			externalPath: ext,
			sudo:         sudo,
		},
	}
	m.chatViewport.FillHeight = true
	m.chatViewport.SetWidth(w)
	return m.adjustViewportHeight()
}

func TestPermPromptFitsScreen(t *testing.T) {
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("line %d some fairly long content that might soft wrap in a narrow terminal window", i))
	}
	body := strings.Join(lines, "\n")
	bash, _ := json.Marshal(map[string]string{"title": "run a big thing", "command": body})
	write, _ := json.Marshal(map[string]string{"filePath": "/tmp/x.go", "content": body})
	edit, _ := json.Marshal(map[string]string{"filePath": "/tmp/x.go", "oldString": body, "newString": body + "\nmore"})

	cases := []struct {
		name, args, ext string
		sudo            bool
	}{
		{"run_bash", string(bash), "", false},
		{"run_bash", string(bash), "", true},
		{"write_file", string(write), "", false},
		{"write_file", string(write), "/etc/hosts", false},
		{"edit_file", string(edit), "", false},
		{"read_file", `{"filePath":"/tmp/x.go"}`, "", false},
	}

	for _, c := range cases {
		for _, h := range []int{12, 18, 20, 24, 30, 40, 60} {
			for _, w := range []int{50, 60, 100, 160} {
				for _, scroll := range []int{0, 40, 5000} {
					m := permModel(w, h, c.name, c.args, c.ext, c.sudo)
					m.permScroll = scroll
					view := m.chatView()
					rows := strings.Split(view, "\n")
					if len(rows) > h {
						t.Errorf("%s ext=%q sudo=%v w=%d h=%d scroll=%d: view is %d rows, screen is %d",
							c.name, c.ext, c.sudo, w, h, scroll, len(rows), h)
					}
					// On a reasonable terminal the view fills the screen exactly
					// and the prompt sits at the very bottom of it.
					if h >= 24 {
						if len(rows) != h {
							t.Errorf("%s w=%d h=%d scroll=%d: view is %d rows, want exactly %d",
								c.name, w, h, scroll, len(rows), h)
						}
						if last := rows[len(rows)-1]; !strings.Contains(last, "╰") {
							t.Errorf("%s w=%d h=%d: prompt not at bottom; last row = %q", c.name, w, h, last)
						}
						if !c.sudo && !strings.Contains(view, "Allow this action?") {
							t.Errorf("%s w=%d h=%d: prompt question missing", c.name, w, h)
						}
					}
				}
			}
		}
	}
}

func TestPermScrollClamps(t *testing.T) {
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	bash, _ := json.Marshal(map[string]string{"title": "big", "command": strings.Join(lines, "\n")})

	m := permModel(100, 40, "run_bash", string(bash), "", false)
	m = m.scrollPerm(-5)
	if m.permScroll != 0 {
		t.Errorf("scroll up from top = %d, want 0", m.permScroll)
	}
	for i := 0; i < 200; i++ {
		m = m.scrollPerm(5)
	}
	_, _, total, visible := m.renderPermOverlay()
	if m.permScroll+visible < total {
		t.Errorf("cannot scroll to the end: offset %d + %d visible < %d total", m.permScroll, visible, total)
	}
	if m.permScroll >= total {
		t.Errorf("scrolled past the end: offset %d, total %d", m.permScroll, total)
	}
	before := m.permScroll
	m = m.scrollPerm(-5)
	if m.permScroll != before-5 {
		t.Errorf("scroll up from bottom = %d, want %d", m.permScroll, before-5)
	}
}

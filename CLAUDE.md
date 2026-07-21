# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`AGENTS.md` in this repo holds the coding rules for this codebase (error handling, tool-execution invariants, keychain rules, testing conventions). Read it — this file covers build/architecture, not style.

## Commands

```bash
go build ./...                     # build all packages
go build -o gurtcli . && ./gurtcli # build and run the TUI
go test ./...                      # all tests
go test ./llm -run TestFetchModels # single test
go vet ./...                       # the only linter configured
gofmt -w <files>                   # formatting
```

There is no Makefile and no golangci config. CI (`.github/workflows/build.yml`) runs only `go build ./...`; tags matching `v*` trigger a goreleaser release.

Useful flags while developing: `--debug` (debug log + resource monitor), `--force-local` (skip the remote `llmdetails.json` fetch), `--reconfigure` (redo provider/model setup), `--yolo` (skip permission prompts).

## Release process

`bump-version-prompt.md` is the authoritative checklist. In short: bump the version in **both** `version.go` and `npm/package.json`, commit (title <40 chars, description summarizing every change by feature, not by file path), `git push origin main`, then `git push origin <tag>` separately — the tag push is what triggers the release workflow. `Version`, `CommitCount`, and `TelemetrySecret` in `version.go` are overwritten at release time via ldflags.

## Architecture

A single-binary Bubble Tea (v2, `charm.land/bubbletea/v2`) TUI. The whole app is one Elm-architecture state machine; there is no server, no plugin system, and deliberately only five tools.

**Root package (`package main`) is the TUI.** It is split by MVU role, not by feature:

- `model.go` — the `model` struct (one big struct holding all app state), the `state` enum (`stateWelcome` → `stateProviderPick` → … → `stateChat`), `initialModel`, and session ↔ model conversion (`toSession`/`applySession`).
- `update.go` (~3.7k lines) — every `Update` handler. `Update` dispatches on `m.state` to a `updateX(msg)` method per state. Also owns the LLM streaming command (`startChatStreamCmd`), tool-call execution loop (`processToolCalls` → `executeNextTool`), slash commands (`handleSlashCommand`), and the embedded prompts.
- `views.go` — a `xxxView()` per state, composed in `View()`.
- `selection.go` / `copyzones.go` — mouse text selection over the rendered transcript and click-to-copy hit regions. All X coordinates are terminal **cells**, not runes/bytes (wide glyphs occupy two columns); see the `textSelection` doc comment.
- `autoupdate.go` — background version check and self-replacing `/update`.

**Packages:**

- `llm/` — provider abstraction. `chat.go` builds OpenAI-shaped or Anthropic-shaped request bodies and parses both SSE dialects into a single `StreamEvent` channel; Gemini goes through the OpenAI-compatible path. `models.go` fetches the live model list per provider. `llmdetails.go` supplies capability metadata (context window, reasoning/thinking support, pricing) from `llm/llmdetails.json`, fetched from GitHub `main` at runtime with the embedded copy as fallback.
- `tools/` — the five tools (`read_file`, `write_file`, `edit_file`, `delete_file`, `run_bash`), their JSON schemas (`Definitions`), workspace-escape checks (`safePath`, `IsPathOutsideWorkspace`), and the permission classification helpers (`IsDestructive`, `BashCommandPrefix`, `DefaultSafeBashPrefixes`).
- `sessions/` — SQLite (`modernc.org/sqlite`, pure Go, CGO_ENABLED=0) session persistence at `~/.config/gurtcli/sessions.db`, with a `migrate` function that must stay append-only. Sessions are keyed by workspace.
- `config/` — `config.json` (non-secret settings, saved endpoints, theme, telemetry flag), `keychain.go` (API keys, service name `gurtcli`, account = provider), `dotenv.go` (offer to read/write a key in the project `.env`).
- `ui/` — pure rendering: theme, markdown, diffs, tables, tool cards. Takes data, returns styled strings; holds no state.
- `files/` — workspace walking with `.gitignore` honoring, for `@file` autocomplete.
- `stats/`, `telemetry/`, `history/`, `debug/` — `gurtcli stats` subcommand, anonymous install counting, input history, debug logging.

### Things that bite

- **Transcript render cache.** Rendering all messages every frame is too slow, so finalized messages are cached (`transcriptCacheKeyStr`, `transcriptBoundary`, `extendTranscriptCache`) and only the in-flight assistant message is re-rendered (`renderStreamingPart`). Any change to how a *finalized* message renders must invalidate the cache (`invalidateTranscriptCache`); anything that changes the render key (theme, width) is already handled.
- **The system prompt** is `prompts/system.md`, embedded at compile time and rendered as a Go template (`renderSystemPrompt`) with OS/arch/workspace/model. A user's project `AGENTS.md` is appended to it — that is how this repo's own rules reach the agent.
- **Prompt caching** is provider-wide and depends on stable message-prefix ordering; reordering or mutating earlier messages silently destroys cache hits and inflates cost.
- **Permission flow**: destructive tool calls suspend the tool loop into a `pendingPerm` overlay, and the loop resumes from `executeNextTool` after the answer. Session-level and permanent allowlists live in the model and config respectively.

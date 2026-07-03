# For LLM agents editing this codebase

## Before writing code

Think through:

1. **Failure modes** — what if the network drops? The LLM returns garbage? A file is locked? The user hits Ctrl+C mid-write?
2. **Side effects** — what external systems does this code touch? Filesystem, network, env vars?
3. **Security** — shell injection from user prompts? Path traversal in file tools? API keys leaked in output?
4. **Reversibility** — if a file write fails halfway, is the system in a clean state? Can the user recover?

## Go rules

- **Handle all errors.** No `_ =` swallows. No discarding return values from anything that can fail.
- **No `any` where a concrete type works.** Define interfaces and structs explicitly.
- **No `time.Sleep` for synchronization.** Use `sync.WaitGroup`, `errgroup`, or channels.
- **No magic numbers.** Config values belong in constants or env vars.
- **`main.go` stays under 300 lines.** Extract packages when it grows: `tools/`, `llm/`, `config/`.

## Error handling

- Catch specific errors. Don't blanket `if err != nil { return err }` at every level — add context with `fmt.Errorf("reading config: %w", err)`.
- Network calls to LLMs need retry with backoff. Use a small retry loop (3 attempts, exponential backoff) — don't let a transient network error kill the session.
- File operations should verify the path is within the workspace. Reject `../` escapes.

## Concurrency

- The TUI is single-threaded per session. No concurrent LLM calls.
- If you add streaming (LLM response streaming to stdout), use a single goroutine with proper cancellation via `context.Context`.
- Always pass `context.Context` as the first argument to any function that does I/O.

## Tool execution

- All tools must refuse operations outside the workspace root unless the user explicitly permits it.
- `read_file` — reject paths outside the workspace root. Support optional line offset and limit.
- `write_file` — create parent directories if they don't exist.
- `edit_file` — fail cleanly if the old string has zero matches or multiple matches (no ambiguous edits).
- `delete_file` — reject paths outside the workspace root.
- `run_bash` — must have a timeout (default 30s). Kill the process if it exceeds the timeout. Capture both stdout and stderr.

## API keys

- Use the OS keychain (`github.com/zalando/go-keyring`) to store and retrieve API keys.
- The keyring service name is `gurtcli`. The account name is the provider (e.g. `openai`, `anthropic`, or `custom:http://...`).
- If the keychain is unavailable (headless server, CI), fall back to the `GURT_API_KEY` env var. If that's also unset, prompt the user each session.
- Never write API keys to disk outside the keychain. Config files must not contain secrets.

## LLM calls

- Support OpenAI, Claude, and any OpenAI-compatible endpoint.
- The provider is selected based on the model name prefix or `GURT_BASE_URL` env var.
- API errors must be surfaced to the user, not swallowed.
- Token usage is tracked and printed after each response.
- The system prompt is embedded in the binary at compile time.

## Testing

- Every tool function gets a test.
- Tests use `t.TempDir()` for file operations — never write to the real filesystem.
- `run_bash` tests use a timeout and verify stderr capture.

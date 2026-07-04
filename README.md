# gurtcli

A coding agent in your terminal. Type what you want. It does the rest.

```bash
npm install -g gurtcli
gurtcli
```

Done. That's the install and setup.

## Why

Most coding AI tools are bloated. Slow to start. Drowning in config files. Too many buttons.

Gurt is the opposite:

- **One install command.** `npm install -g gurtcli`. That's it.
- **One entry point.** `gurtcli` opens a chat. No subcommands. No menus.
- **Zero config.** Get an API key. First run saves it to your OS keychain.
- **Fast startup.** Written in Go. Compiles to a single binary. No runtime to load.
- **Small surface.** Five tools. Read, write, edit, delete, run. Anything more is noise.

## How it works

1. You type what you want.
2. Gurt sends it to an LLM.
3. The LLM decides what to do — read a file, edit code, run a command.
4. Gurt does it. Shows you the result.
5. Repeat until you're done.

No multi-step approval chains. No subagent trees. No plugin marketplace. Just a chat loop that can touch your filesystem.

## Quick start

```bash
npm install -g gurtcli
gurtcli
```

First run walks you through provider selection, API key entry, and model picker. After that you're in the chat.

Type `/exit` or press `ctrl+c` to quit.

### Flags (all optional)

```
--model <name>                  skip model picker
--provider <name>               skip provider picker (openai, anthropic)
--yolo                          skip all permission prompts
--dangerously-skip-permissions  skip all permission prompts
--reconfigure                   force provider and model setup
--version                       print version and exit
```

## Slash commands

| Command | What it does |
|---|---|
| `/help` | Show available commands |
| `/model` | Change model for current provider |
| `/provider` | Change provider |
| `/auth` | Change API key for current provider |
| `/session` | Switch to a saved session |
| `/new` | Start a fresh session |
| `/reasoning` | Toggle reasoning visibility |
| `/thinking` | Set thinking type (adaptive/enabled/disabled) |
| `/effort` | Set effort level (low/medium/high/xhigh/max) |
| `/exit` | Quit the application |

Type `/` in chat to see autocomplete suggestions for all commands.

## Sessions

Every chat session is automatically saved to `.gurtcli/sessions/` in your workspace. Sessions persist your message history, provider, model, and reasoning configuration across restarts.

- **Resume a session** — launch `gurtcli` in the same directory and it picks up where you left off.
- **Switch sessions** — `/session` shows a list of saved sessions.
- **New session** — `/new` saves the current session and starts fresh.

Session data is stored as JSON files and is portable across machines.

## Provider & model setup

Supports **OpenAI**, **Anthropic**, and any **OpenAI-compatible endpoint**.

First run shows a provider picker. Choose one and enter your API key. The key is saved to your OS keychain — no env vars or config file secrets.

After picking a provider, models are fetched from the API and displayed in a filterable list. Type to narrow down, press enter to select.

Use `--provider` and `--model` flags to skip setup entirely:

```bash
gurtcli --provider anthropic --model claude-sonnet-5-20260630
```

### Custom endpoints

Select "Custom" in the provider picker to use any OpenAI-compatible endpoint (Groq, OpenRouter, Ollama, etc.). You can use it one-time or save it as a named endpoint for reuse.

Saved endpoints appear in the provider list. Press `d` to delete a saved endpoint.

## Reasoning configuration

When your model supports it, gurt lets you configure reasoning behavior after model selection.

**Anthropic models** — two settings:
- **Thinking type** — `adaptive`, `enabled`, or `disabled`
- **Effort level** — `low`, `medium`, `high`, `xhigh`, or `max`

**OpenAI models** — one setting:
- **Reasoning effort** — `none`, `low`, `medium`, `high`, `xhigh`, or `max`

Navigate with `↑`/`↓`, change values with `←`/`→`, confirm with `enter`.

Change these mid-session with `/thinking <type>` and `/effort <level>`.

Toggle reasoning visibility inline with `/reasoning` or click the `[▼]` / `[▶]` toggle with your mouse.

## Permissions

Destructive operations (write, edit, delete, run) prompt for confirmation:

```
❯ y
(y)es / (n)o / allow for (a)ll
```

- `y` — allow once
- `n` — deny once
- `a` — allow for the rest of this session

Use `--yolo` or `--dangerously-skip-permissions` to skip all prompts.

## AGENTS.md

If your project has an `AGENTS.md` file in its root directory, its contents are appended to the system prompt on every LLM call. Use this to provide project-specific instructions, conventions, and context that the LLM should follow.

## API keys

Gurt stores your API key in the OS keychain. No env vars needed. You're prompted once on first run and never again.

macOS uses Keychain. Linux uses Secret Service (libsecret). Windows uses Credential Manager.

**No keychain available?** (headless server, CI) — Gurt asks for the key each session. For CI, set `GURT_API_KEY` as an env var.

Your model choice is saved to `~/.config/gurtcli/config.json` after first run.

## Tools

| Tool | Does |
|------|------|
| `read_file` | Read a file with optional offset and limit |
| `write_file` | Create or overwrite a file (creates parent dirs) |
| `edit_file` | Replace exact text match in a file |
| `delete_file` | Remove a file |
| `run_bash` | Execute a shell command with timeout |

## Why Go

- Compiles to a single binary. No Python/Node runtime needed.
- Startup in milliseconds, not seconds.
- Cross-platform with one build flag.
- No dependency hell. `go build` produces one file you can scp anywhere.

## Why npm as the install path

Because `npm install -g` is the least friction for developers. The npm package is a thin wrapper that downloads the right Go binary for your OS. You never touch Go.

## Philosophy

This tool does one thing: takes a natural language request and turns it into filesystem operations. It doesn't need a plugin system. It doesn't need MCP. It doesn't need skills. It reads your prompt, calls an LLM, and runs tools. That's the whole loop.

Adding features is easy. The hard part is stopping.

## License

MIT

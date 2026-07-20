# gurtcli — The oversimplified agent

Type what you want. It does the rest.

```bash
npm install -g gurtcli
gurtcli
```

Done. That's the entire install and setup.

> **Windows users — [download the binary](https://github.com/sillygru/gurtcli/releases) instead. npm has issues on Windows.**

## What it is

A chat loop that can touch your filesystem. That's it.

1. You type what you want.
2. Gurt sends it to an LLM.
3. The LLM decides what to do — read a file, edit code, run a command.
4. Gurt does it. Shows you the result.s
5. Repeat until you're done.

**Five tools. One loop. No plugins. No MCP. No agents. No skills. No vector stores. No RAG. No config files. No dashboards. No subagent trees.**

Adding features is easy. The hard part is stopping.

## Quick start

```bash
npm install -g gurtcli
gurtcli
```

First run walks you through picking a provider and entering an API key (saved to your OS keychain). After that you're in the chat.

`/exit` or `ctrl+c` to quit.

### Flags (all optional)

```
--model <name>                  skip model picker
--provider <name>               skip provider picker (openai, anthropic, gemini, custom)
--yolo                          skip all permission prompts
--dangerously-skip-permissions  skip all permission prompts
--reconfigure                   force provider and model setup
--force-local                   use embedded model details instead of fetching from GitHub
--debug                         enable debug logging and resource monitor
--version                       print version and exit
```

## Slash commands

| Command | What it does |
|---|---|
| `/help` | Show available commands |
| `/model` | Change model |
| `/provider` | Change provider |
| `/auth` | Change API key |
| `/session` | Switch sessions |
| `/new` | Fresh session |
| `/show-reasoning` | Toggle reasoning visibility |
| `/reasoning` | Set thinking type / reasoning effort |
| `/effort` | Set effort level |
| `/allow` | Manage always-allowed tools and commands |
| `/update` | Update to latest version |
| `/theme` | Change color theme |
| `/telemetry` | Toggle anonymous usage telemetry |
| `/version` | Show version and check for updates |
| `/exit` | Quit |

Type `/` in chat to see autocomplete.

## Sessions

Every chat is auto-saved to `~/.config/gurtcli/sessions.db` (SQLite). Sessions persist your history, provider, model, and reasoning config across restarts.

- **Switch** — `/session` shows a list of saved sessions.
- **New** — `/new` saves the current session and starts fresh.

## Providers & models

Supports **OpenAI**, **Anthropic**, **Google Gemini**, and any **OpenAI-compatible endpoint** (Groq, OpenRouter, Ollama, etc.).

First run shows a provider picker. Choose one, enter your API key. The key goes to your OS keychain — never to a file.

Models are fetched from the API and displayed in a filterable list. Type to filter, enter to select.

```bash
gurtcli --provider anthropic --model claude-sonnet-5-20260630
```

Use **Custom** to hit any OpenAI-compatible endpoint. Save it as a named endpoint for reuse. Press `d` to delete a saved endpoint.

## Reasoning

When your model supports it:

- **Anthropic** — thinking type (`adaptive`, `enabled`, `disabled`) + effort (`low`, `medium`, `high`, `xhigh`, `max`)
- **OpenAI** — reasoning effort (`none`, `low`, `medium`, `high`, `xhigh`, `max`)

Navigate with `↑`/`↓`, change values with `←`/`→`, confirm with `enter`.
Change mid-session with `/reasoning <type>` and `/effort <level>`.
Toggle visibility inline with `/show-reasoning` or click `[▼]` / `[▶]`.

## Permissions

Destructive operations (write, edit, delete, run) prompt for confirmation. Navigate with `↑`/`↓`, confirm with `enter`:

- **Yes** — allow once
- **Allow every edit for this session** / **Allow deletion of files for this session** — session-level permission
- **Allow everything for this session** — allow all destructive tools for the session
- **No** — deny once

For `run_bash`: **Allow `<prefix>` for this session** or **Always allow `<prefix>`** (permanent).

For paths outside the workspace: allow the directory for the session or permanently.

`--yolo` / `--dangerously-skip-permissions` skips all prompts.

### Always-allowed

By default, only `read_file` is always allowed. Safe command prefixes like `cat`, `ls`, `grep`, `find`, `head`, `tail`, `echo`, `pwd` are always allowed.

Manage with `/allow`.

## AGENTS.md

Place an `AGENTS.md` in your project root. Its contents are appended to every LLM call as system context. Use it to give the agent project-specific conventions and instructions.

## API keys

Stored in your OS keychain. macOS uses Keychain. Linux uses Secret Service. Windows uses Credential Manager.

**No keychain?** (headless server, CI) — Gurt asks each session, or set `GURT_API_KEY` as an env var.

Never written to disk outside the keychain.

## The five tools

| Tool | What it does |
|---|---|
| `read_file` | Read a file with optional offset and limit |
| `write_file` | Create or overwrite a file (creates parent dirs) |
| `edit_file` | Replace exact text in a file |
| `delete_file` | Remove a file |
| `run_bash` | Run a shell command with timeout |

Anything more is noise.

## Updating

Gurt checks for updates in the background on startup. Manual update: `/update`.

## Telemetry

Anonymous usage data on startup to count active installs. No personal data — no names, emails, IPs, file paths. Scoped to a random UUID at `~/.config/gurtcli/telemetry-id`.

**Enabled by default.** Toggle with `/telemetry`.

## License

MIT

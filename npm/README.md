# gurtcli

A coding agent in your terminal. Type what you want. It does the rest.

```bash
npm install -g gurtcli
gurtcli
```

Written in Go. Distributed as a single binary. The npm package is a thin installer — `npm install -g gurtcli` downloads the right binary for your OS.

**Zero config.** First run prompts for provider (OpenAI, Anthropic, or any OpenAI-compatible endpoint) and API key. Key is saved to your OS keychain. Model choice is persisted.

## Quick start

```bash
npm install -g gurtcli
gurtcli
```

Pick a provider, enter your API key, select a model. You're in the chat.

## CLI flags

| Flag | Purpose |
|---|---|
| `--model <name>` | Skip model picker |
| `--provider <provider>` | Skip provider picker (openai, anthropic) |
| `--yolo` | Skip all permission prompts |
| `--dangerously-skip-permissions` | Same as --yolo |
| `--reconfigure` | Force provider/model setup |
| `--version` | Print version and exit |

## Slash commands

| Command | What it does |
|---|---|
| `/help` | Show available commands |
| `/model` | Change model |
| `/provider` | Change provider |
| `/auth` | Change API key |
| `/session` | Switch to a saved session |
| `/new` | Start a fresh session |
| `/reasoning` | Toggle reasoning visibility |
| `/thinking` | Set thinking type (adaptive/enabled/disabled) |
| `/effort` | Set effort level (low/medium/high/xhigh/max) |
| `/exit` | Quit |

## How it works

1. You describe what you want in natural language.
2. Gurt sends it to an LLM.
3. The LLM decides which tool to use — read, write, edit, delete, or run a shell command.
4. Gurt executes it and shows the result.

That's the whole loop. No plugins. No MCP. No subagents.

---

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).  
[GitHub](https://github.com/sillygru/gurtcli) · [Issues](https://github.com/sillygru/gurtcli/issues)

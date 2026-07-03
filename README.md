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

## Usage

```bash
gurtcli
```

First run asks which model to use. After that you're in the chat.

Type `/exit` or press `ctrl+c` to quit.

### Flags (all optional)

```
--model gpt-5.5              pick a model
--yolo                      skip confirmations
--dangerously-skip-permissions  skip confirmations
```

## API keys

Gurt stores your API key in the OS keychain. No env vars needed. You're prompted once on first run and never again.

macOS uses Keychain. Linux uses Secret Service (libsecret). Windows uses Credential Manager.

**No keychain available?** (headless server, CI) — Gurt asks for the key each session. For CI, set `GURT_API_KEY` as an env var.

Your model choice is saved to `~/.config/gurtcli/config.json` after first run.

## Tools

| Tool | Does |
|------|------|
| `read_file` | Read a file |
| `write_file` | Create or overwrite |
| `edit_file` | Replace text in a file |
| `delete_file` | Remove a file |
| `run_bash` | Execute a shell command |

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

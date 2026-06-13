# clk

Auto-capture dev activity and push it to Clockify.

`clk` is a single Go binary (CLI + background daemon) that captures your development activity automatically and lets you review and push it to Clockify as time entries — without changing what you already do.

📖 **Full documentation:** <https://vdpeijl.github.io/clk/> — install, auth, init, review, push guides, a complete command reference, and configuration. Sources live in [`docs/`](docs/).

## Install

```sh
# macOS (Homebrew)
brew install vdpeijl/tap/clk

# Linux
curl -sSfL https://raw.githubusercontent.com/vdpeijl/clk/main/install.sh | sh
```

## Quick start

```sh
clk auth login        # store your Clockify API key
clk init              # install hooks in the current repo
clk list today        # see today's captured sessions
clk review            # review and edit before pushing
clk push              # push to Clockify
```

## Commands

| Command | Description |
|---|---|
| `clk version` | Print the version |
| `clk auth login` | Store your Clockify API key |
| `clk init` | Install hooks and register the current project |
| `clk up / down / status` | Control and inspect the background daemon |
| `clk logs [-f]` | Show (and optionally follow) the daemon log |
| `clk list [today\|yesterday\|week\|month]` | List captured sessions |
| `clk review` | Interactive TUI to review sessions before pushing |
| `clk push [--merge]` | Push sessions to Clockify |
| `clk link` | Map the current repo to a Clockify project |
| `clk log <duration> <description>` | Log a manual entry |
| `clk unpush` | Remove a previously pushed entry |
| `clk upgrade` | Upgrade to the latest version |

## Configuration

- `.clk.toml` — committed per-repo: Clockify project/task mapping and description template.
- `~/.clk/config.toml` — personal, never committed: API key, workspace, local overrides.
- `CLOCKIFY_API_KEY` env variable overrides the stored key.

## Development

```sh
go build ./cmd/clk/   # build
go test ./...          # run tests
```

Requires Go 1.21+. No CGO — the binary is fully static.

## License

MIT

---
title: Set up a repo
layout: default
nav_order: 4
---

# Set up a repo — `clk init`

Run `clk init` from inside a repository to start capturing activity there.

```sh
clk init
```

The command, keyed off the repository root:

1. **Detects the dev tools in use** — Claude Code, Cursor, Copilot, and git —
   and installs each one's capture hook.
2. **Registers the repository** so the background daemon watches it for file
   activity.
3. **Scaffolds a committed `.clk.toml`** carrying the project mapping and
   description template, so teammates inherit the conventions when they clone.

`clk init` is safe to re-run: hooks that are already installed are left alone,
and an existing `.clk.toml` is never overwritten.

## What gets written

| Path | Purpose |
|---|---|
| `.claude/settings.json` | Claude Code PostToolUse hook (if Claude Code is detected) |
| `.cursor/hooks.json` | Cursor `afterFileEdit` / `beforeShellExecution` hooks |
| `.copilot/hooks.json` | Copilot `postToolUse` hook |
| `.git/hooks/post-commit` | git commit capture (existing scripts are preserved) |
| `.clk.toml` | Committed per-repo Clockify mapping and description template |

If no supported tool is detected, only the daemon's file-watch capture runs.

## Start the daemon

Capture happens in a background daemon. Start it once:

```sh
clk up        # start the daemon
clk status    # check it's running and see buffered events
```

See the [daemon commands](commands#daemon) for `down`, `status`, and `logs`.

## Map the repo to a Clockify project

The first time an unmapped project would be pushed, `clk` prompts you to pick a
Clockify project. You can also do it up front:

```sh
clk link              # fuzzy-pick a Clockify project for this repo
```

See [`clk link`](commands#clk-link) and [Configuration](configuration) for
details, including personal overrides and tasks.

## Next steps

Once activity is being captured, [review your sessions](review).

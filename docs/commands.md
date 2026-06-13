---
title: Command reference
layout: default
nav_order: 7
---

# Command reference

Every `clk` command. Run `clk <command> --help` for the canonical, built-in
help.

## Overview

| Command | Description |
|---|---|
| [`clk version`](#clk-version) | Print the clk version |
| [`clk auth login`](#clk-auth-login) | Store your Clockify API key and select a workspace |
| [`clk init`](#clk-init) | Install tool hooks and register the current project |
| [`clk link`](#clk-link) | Map a local project to a Clockify project (and optional task) |
| [`clk up`](#daemon) | Start the background capture daemon |
| [`clk down`](#daemon) | Stop the background capture daemon |
| [`clk status`](#daemon) | Show the daemon state and buffered events |
| [`clk logs`](#clk-logs) | Show the daemon log, optionally following new output |
| [`clk list`](#clk-list) | List reconstructed work sessions for a period |
| [`clk review`](#clk-review) | Interactively review sessions before pushing |
| [`clk push`](#clk-push) | Push reviewed sessions to Clockify |
| [`clk unpush`](#clk-unpush) | Delete previously pushed entries from Clockify |
| [`clk log`](#clk-log) | Create a manual Clockify time entry |
| [`clk hook`](#clk-hook) | Ingest activity from an editor or tool hook (internal) |

A `clk completion` command is also available to generate shell autocompletion
scripts (`clk completion --help`).

---

## `clk version`

```sh
clk version
```

Prints the installed version, e.g. `clk 1.0.0`.

---

## `clk auth login`

```sh
clk auth login
```

Prompts for your Clockify API key, verifies it by listing your workspaces, lets
you pick one when you belong to more than one, and stores both in
`~/.clk/config.toml` at mode `0600`. The `CLOCKIFY_API_KEY` environment
variable, when set, overrides the stored key at runtime.

See the [authentication guide](auth).

---

## `clk init`

```sh
clk init
```

Detects the dev tools in use (Claude Code, Cursor, Copilot, git), installs their
capture hooks, registers the repository so the daemon watches it for file
activity, and scaffolds a committed `.clk.toml` carrying the project mapping and
description template so teammates inherit the conventions on clone.

See the [repo setup guide](init).

---

## `clk link`

```sh
clk link                       # pick a project for the current repo
clk link <project>             # map the current repo to <project>
clk link <token> <project>     # map an explicit project token
```

Maps a local project token to a Clockify project so pushed sessions land in the
right place. With no project argument it shows a fuzzy-pick list of your Clockify
projects and remembers the choice — the same prompt-once flow used the first time
an unmapped project would be pushed.

The mapping is written to the committed `.clk.toml` so teammates inherit it on
pull.

**Flags**

| Flag | Description |
|---|---|
| `--task <name\|id>` | Clockify task name or id to pin alongside the project |
| `--personal` | Store the mapping as a personal override in `~/.clk` instead of the committed `.clk.toml` |

A project/task argument is resolved by exact id, then exact (case-insensitive)
name, then an unambiguous fuzzy match.

---

## Daemon — `clk up` / `clk down` / `clk status`

```sh
clk up        # start the background capture daemon
clk down      # stop it (waits up to 5s for a graceful exit, then SIGKILL)
clk status    # show daemon state and buffered events
```

`clk up` is a no-op if the daemon is already running. `clk status` reports the
pid, uptime, currently buffered events, and total events captured since start;
it prints `daemon: stopped` when nothing is running.

---

## `clk logs`

```sh
clk logs        # print the daemon log and exit
clk logs -f     # print, then follow new output until Ctrl-C
```

**Flags**

| Flag | Description |
|---|---|
| `-f`, `--follow` | Follow the log output |

Prints `no daemon logs yet` when the log file does not exist.

---

## `clk list`

```sh
clk list today
clk list yesterday
clk list week
clk list month
```

Reads captured events for the given period from `~/.clk/state.db`, reconstructs
them into work sessions, and prints them with date, start/end, duration,
project, branch, issue, and source. The period argument is **required**. Weeks
start on Monday; months start on the 1st.

---

## `clk review`

```sh
clk review [today|yesterday|week|month]
```

Opens an interactive TUI to merge, split, edit, re-assign, or drop the period's
sessions and push them from within the UI. Defaults to today. See the
[review guide](review).

---

## `clk push`

```sh
clk push [today|yesterday|week|month]
clk push --merge
```

Reconstructs sessions for the given period (default today) and registers them in
Clockify as idempotent time entries. See the [push guide](push).

**Flags**

| Flag | Description |
|---|---|
| `--merge` | Collapse the day's sessions per project into a single entry |

---

## `clk unpush`

```sh
clk unpush [today|yesterday|week|month]
```

Explicitly removes the Clockify entries `clk` created for sessions in the given
period (default today) and forgets their push links, so a later push treats
those sessions as new. This is the only command that deletes from Clockify;
`push` never does.

---

## `clk log`

```sh
clk log <duration> <description>
```

Creates a one-off Clockify time entry for the current project ending now and
starting `<duration>` ago. Duration accepts Go syntax such as `45m` or `1h30m`.
The description is used verbatim (no template expansion). Requires the current
repo to be mapped (see [`clk link`](#clk-link)).

---

## `clk hook`

```sh
clk hook claude-code      # ingest a Claude Code PostToolUse payload from stdin
```

Internal: invoked by the capture hooks `clk init` installs, not run by hand. It
reads a tool's payload from stdin, attaches the current git branch and
`PROJ-123`-style issue id, and stores the resulting event in `~/.clk/state.db`.

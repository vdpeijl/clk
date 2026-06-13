---
title: Configuration
layout: default
nav_order: 8
---

# Configuration

`clk` reads configuration from two files plus the environment, merged with a
defined precedence. Missing files are treated as empty, never as errors.

## Files

| Source | Scope | Committed? | Holds |
|---|---|---|---|
| `.clk.toml` | per-repo | **yes** — commit it | Clockify project/task mapping, billable flag, description template |
| `~/.clk/config.toml` | personal | **no** | API key, workspace, push rounding, personal mapping overrides |
| `CLOCKIFY_API_KEY` | environment | n/a | Overrides the stored API key at runtime |

Precedence for the API key: `CLOCKIFY_API_KEY` overrides `~/.clk/config.toml`.
Personal mappings in `~/.clk/config.toml` win over the committed `.clk.toml`
mapping locally without changing the shared file.

## `.clk.toml` (committed)

Scaffolded by [`clk init`](init) and edited by [`clk link`](commands#clk-link).

```toml
[clockify]
project  = ""                       # default Clockify project id
task     = ""                       # optional default task id
billable = false                    # mark pushed entries billable
template = "{issue} {branch}: {summary}"

# Token-keyed mappings written by the prompt-once pick and `clk link`.
[mappings.my-repo]
project = "64b...e21"
task    = ""
```

## `~/.clk/config.toml` (personal, never commit)

Written by [`clk auth login`](auth) and `clk link --personal`. Created at mode
`0600` because it holds your API key.

```toml
[clockify]
api_key   = "xxxxxxxx"
workspace = "64a...c10"

[push]
round = "15m"                       # off | 5m | 6m | 15m

# Personal overrides that win over the committed mapping.
[mappings.my-repo]
project = "64b...e21"
```

## Description template

The `template` field controls how each session's description is rendered on
push. Placeholders are replaced from the session; a placeholder whose source is
empty expands to an empty string.

| Placeholder | Expands to |
|---|---|
| `{issue}` | The detected issue id (e.g. `PROJ-123`) |
| `{branch}` | The git branch |
| `{summary}` | The session description |
| `{files}` | The touched files, comma-separated |

Default: `{issue} {branch}: {summary}`.

## Push rounding

`push.round` rounds each entry's duration before it is sent to Clockify.
Accepted values are `off`, `5m`, `6m`, and `15m`. An empty or unrecognized value
falls back to the default, **nearest 15m**.

## Clean start — no migration

{: .important }
> `clk` does not migrate from any prior local database. There is no import or
> upgrade path from an earlier tool or schema. The first run creates a fresh
> `~/.clk/state.db` and begins capturing from that point forward; nothing
> recorded beforehand is imported.

---
title: Push to Clockify
layout: default
nav_order: 6
---

# Push to Clockify — `clk push`

`clk push` reconstructs sessions for a period and registers them in Clockify as
time entries.

```sh
clk push                 # today (default)
clk push yesterday
clk push week
clk push month
```

## Idempotent by design

Pushing the same period twice does not create duplicates. For each session:

- a **new** session is **created**;
- a previously pushed session whose payload **changed** is **updated**;
- an **unchanged** one is **skipped**;
- a session that was pushed but **no longer exists locally** is **warned**
  about — never deleted.

This means you can review, push, capture more, and push again freely.

## Merging a day into one entry

By default each session becomes its own Clockify entry, preserving its real
start time. With `--merge`, the day's sessions for a project are collapsed into a
single entry whose duration is the rounded total:

```sh
clk push --merge
```

## Mapping required

A session is only pushed if its project token is mapped to a Clockify project.
Unmapped sessions are skipped with a hint to run [`clk link`](commands#clk-link).
See [Configuration](configuration) for mappings, rounding, and the description
template.

## Removing entries — `clk unpush`

`push` never deletes from Clockify. To remove the entries `clk` created for a
period, use the explicit `unpush`:

```sh
clk unpush               # today (default)
clk unpush week
```

This deletes those Clockify entries and forgets their push links, so a later
push treats the sessions as new.

## Manual entries — `clk log`

For one-off time that wasn't captured, log it directly:

```sh
clk log 45m "pairing on the deploy script"
clk log 1h30m "incident call"
```

The entry ends now and starts `<duration>` ago, against the current repo's
mapped project. The description is used verbatim (no template expansion).

## Next steps

See the [full command reference](commands) for every command and flag.

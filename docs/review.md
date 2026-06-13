---
title: Review sessions
layout: default
nav_order: 5
---

# Review sessions — `clk review`

`clk review` opens an interactive terminal UI listing the period's reconstructed
sessions and lets you correct them before anything reaches Clockify.

```sh
clk review               # today (default)
clk review yesterday
clk review week
clk review month
```

## What you can do

From the review UI you can:

- **Merge** sessions the gap heuristic split too eagerly.
- **Split** a session that actually covers two tasks.
- **Edit** a session's description.
- **Re-assign** a session's Clockify project and task.
- **Drop** noise you don't want to bill.
- **Push** the reviewed sessions to Clockify from within the UI.

Review orchestrates the same reconstruction, templating, planning, and Clockify
machinery as [`clk push`](push) — it just adds the human-in-the-loop editing
step in front of it. Anything you push from review is recorded identically to a
command-line push, so the two are interchangeable and idempotent together.

## Inspect first with `clk list`

If you only want to see what was captured without editing, use
[`clk list`](commands#clk-list):

```sh
clk list today
```

It prints each session's date, start/end, duration, project, branch, issue, and
source.

## Next steps

When the sessions look right, [push them to Clockify](push).

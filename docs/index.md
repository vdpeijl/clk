---
title: Home
layout: default
nav_order: 1
---

# clk

Auto-capture dev activity and push it to Clockify.
{: .fs-6 .fw-300 }

`clk` is a single Go binary (CLI + background daemon) that captures your
development activity automatically and lets you review and push it to Clockify
as time entries — without changing what you already do.

[Get started](install){: .btn .btn-primary .mr-2 }
[View on GitHub](https://github.com/vdpeijl/clk){: .btn }

---

## How it works

1. **Capture.** A background daemon watches your repositories and ingests hooks
   from the editors and tools you already use (Claude Code, Cursor, Copilot,
   git). Raw activity events land in a local database at `~/.clk/state.db`.
2. **Reconstruct.** Events are stitched into work *sessions* with a start, end,
   project, branch, and issue id.
3. **Review.** You correct the reconstructed sessions in an interactive TUI —
   merge, split, edit, re-assign, or drop — before anything leaves your machine.
4. **Push.** Reviewed sessions become Clockify time entries. Pushing is
   idempotent, so you can re-run it safely.

## Quick start

```sh
clk auth login        # store your Clockify API key
clk init              # install hooks in the current repo
clk list today        # see today's captured sessions
clk review            # review and edit before pushing
clk push              # push to Clockify
```

Each step has its own guide:

- [Install](install)
- [Authenticate (`clk auth login`)](auth)
- [Set up a repo (`clk init`)](init)
- [Review sessions (`clk review`)](review)
- [Push to Clockify (`clk push`)](push)
- [Full command reference](commands)
- [Configuration](configuration)

## Clean start — no migration

{: .important }
> `clk` does **not** migrate from any prior local database. There is no import
> step and no upgrade path from an earlier tool or schema. The first run creates
> a fresh `~/.clk/state.db` and begins capturing from that point forward.
> Anything recorded before you installed `clk` is not imported.

## License

`clk` is released under the [MIT License](license). See the
[`LICENSE`](https://github.com/vdpeijl/clk/blob/main/LICENSE) file in the
repository for the full text.

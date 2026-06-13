---
title: Authenticate
layout: default
nav_order: 3
---

# Authenticate — `clk auth login`

Before `clk` can talk to Clockify it needs your API key and a workspace.

```sh
clk auth login
```

The command:

1. Prompts for your **Clockify API key**.
2. Verifies the key by listing your workspaces.
3. Lets you **pick a workspace** when you belong to more than one (it
   auto-selects when there is only one).
4. Stores both the key and the selected workspace in `~/.clk/config.toml`, which
   is created at mode `0600` so the secret is never world-readable.

## Where to find your API key

In Clockify, open **Profile Settings** and scroll to **API**. Generate (or copy)
a key and paste it at the prompt.

## Overriding the key

The `CLOCKIFY_API_KEY` environment variable, when set, overrides the stored key
at runtime — handy for CI or for keeping the key out of a file entirely:

```sh
CLOCKIFY_API_KEY=xxxxxxxx clk push
```

## Re-running

Running `clk auth login` again updates only the credentials and workspace. Any
personal settings you have configured — push rounding, mapping overrides — are
preserved.

## Next steps

With credentials in place, [set up a repository with `clk init`](init).

---
title: Install
layout: default
nav_order: 2
---

# Install

`clk` ships as a single static Go binary — no runtime, no CGO, no external
dependencies.

## macOS (Homebrew)

```sh
brew install vdpeijl/tap/clk
```

## Linux

```sh
curl -sSfL https://raw.githubusercontent.com/vdpeijl/clk/main/install.sh | sh
```

## From source

Requires Go 1.21+.

```sh
git clone https://github.com/vdpeijl/clk
cd clk
go build ./cmd/clk/
```

This produces a `clk` binary in the current directory. Move it onto your `PATH`
(for example `sudo install clk /usr/local/bin/`).

## Verify the install

```sh
clk version
```

## Next steps

Once `clk` is on your `PATH`, continue with
[authenticating against Clockify](auth).

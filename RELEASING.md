# Releasing clk

`clk` is distributed as static binaries built by [GoReleaser](https://goreleaser.com)
and published to GitHub Releases. macOS users install via a Homebrew tap, Linux
users via `install.sh`, and everyone can self-update with `clk upgrade`.

## Distribution channels

| Channel | How it works |
|---|---|
| GitHub Releases | GoReleaser cross-compiles `darwin`/`linux` × `arm64`/`amd64`, archives each as `clk_<os>_<arch>.tar.gz`, and attaches them plus `checksums.txt`. |
| Homebrew | `brew install vdpeijl/tap/clk` — GoReleaser commits a formula to [`vdpeijl/homebrew-tap`](https://github.com/vdpeijl/homebrew-tap) on each release. |
| `install.sh` | `curl -sSfL .../install.sh \| sh` detects OS/arch and pulls the matching archive from the latest release. |
| `clk upgrade` | Self-update — resolves the latest release via the GitHub API and atomically replaces the running binary. |

The version is stamped into the binary at build time via
`-ldflags -X github.com/vdpeijl/clk/cmd.Version=<tag>`, so `clk version` and
`clk upgrade` know what they are running. Local `go build` produces `dev`.

## One-time setup

1. Create the tap repository `vdpeijl/homebrew-tap` (empty is fine).
2. Create a GitHub Personal Access Token with `contents:write` on that tap repo
   and add it to **this** repo's Actions secrets as `HOMEBREW_TAP_GITHUB_TOKEN`.
   (The release on `vdpeijl/clk` itself uses the automatic `GITHUB_TOKEN`.)

## Cutting a release

Releases are triggered by pushing a semver tag. From an up-to-date `main`:

```sh
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

The [`Release` workflow](.github/workflows/release.yml) runs GoReleaser, which:

- builds the four binaries and uploads them to a new GitHub Release for the tag,
- generates `checksums.txt` and a changelog from commit messages,
- updates the Homebrew formula in the tap.

Use a `-rc`/`-beta` pre-release suffix (e.g. `v0.2.0-rc.1`) to publish a
pre-release without affecting `latest`.

## Validating before tagging

GoReleaser can dry-run locally without publishing:

```sh
goreleaser check                       # validate .goreleaser.yaml
goreleaser release --snapshot --clean  # build everything into ./dist, no upload
```

## Verifying a release

```sh
# Linux
curl -sSfL https://raw.githubusercontent.com/vdpeijl/clk/main/install.sh | sh
clk version

# macOS
brew install vdpeijl/tap/clk
clk version

# Self-update from an older build
clk upgrade
```

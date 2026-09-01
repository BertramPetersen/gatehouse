---
title: Installation
description: All install options, prerequisites, update, and uninstall.
---

## macOS / Linux

```sh
curl -fsSL https://raw.githubusercontent.com/BertramPetersen/gatehouse/main/docs/install.sh | sh
```

The installer keeps the real binary in `~/.gatehouse/bin` and exposes `gatehouse` through a symlink in `~/.local/bin` or `/usr/local/bin`. That keeps future `gatehouse update` runs in a user-owned location instead of rewriting a system binary in place.

It also installs or refreshes the background daemon for you by running `gatehouse daemon restart`, preferring a managed service (launchd on macOS, systemd user service on Linux) and falling back to a detached daemon if that path is unavailable. If the restart fails, the install command fails.

Official release binaries installed this way include the default self-hosted telemetry host and website ID. Disable telemetry with `GATEHOUSE_TELEMETRY=0`, or override the host and website ID with `GATEHOUSE_UMAMI_HOST` and `GATEHOUSE_UMAMI_WEBSITE_ID`.

## Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/BertramPetersen/gatehouse/main/docs/install.ps1 | iex
```

Installs the binary and restarts the background daemon automatically with `gatehouse.exe daemon restart`, preferring a managed Task Scheduler task and falling back to a detached daemon if needed. If the restart fails, the install command fails.

Official release binaries installed this way include the default self-hosted telemetry host and website ID. Disable telemetry with `GATEHOUSE_TELEMETRY=0`, or override the host and website ID with `GATEHOUSE_UMAMI_HOST` and `GATEHOUSE_UMAMI_WEBSITE_ID`.

## Go install

```sh
go install github.com/BertramPetersen/gatehouse/cmd/gatehouse@latest
```

`go install` builds the CLI without an embedded telemetry website ID, so telemetry stays off by default unless you later set `GATEHOUSE_UMAMI_WEBSITE_ID` at runtime.

## From source

```sh
git clone git@github.com:BertramPetersen/gatehouse.git
cd gatehouse
make build
make install
```

`make build` embeds the telemetry host from `GATEHOUSE_UMAMI_HOST` in a repo-local `.env` first, then `UMAMI_HOST` from the shell, then the default self-hosted host. It embeds the telemetry website ID from `GATEHOUSE_UMAMI_WEBSITE_ID` in `.env` first, then `UMAMI_WEBSITE_ID` from the shell, then the default website ID.

## Prerequisites

- **git** - required
- **One supported agent runner** - `claude`, `codex`, `grok`, `acli` (Rovo Dev), `opencode`, `pi`, `copilot`, or `agy` (Antigravity), or a configured Cursor/ACP runner such as `agent: cursor`; see [Global Config](/gatehouse/reference/global-config/) for ACP requirements
- **Optional, for PRs and CI:**
  - `gh` CLI (GitHub)
  - `glab` CLI (GitLab)
  - `GATEHOUSE_BITBUCKET_EMAIL` and `GATEHOUSE_BITBUCKET_API_TOKEN` (Bitbucket Cloud)
  - `az` CLI with the `azure-devops` extension (Azure DevOps)
  - `tea` CLI (Gitea)

Run `gatehouse doctor` to check native agents, ACP aliases such as `cursor`, provider tools, and whether the configured global runner can start a validation gate.
Every validation gate requires a runnable pipeline agent and otherwise fails before its first pipeline step.

See [Provider Integration](/gatehouse/guides/provider-integration/) for PR and CI setup per host.

## Update

```sh
gatehouse update
gatehouse update --beta
gatehouse update -y
```

This downloads the latest release from GitHub, verifies the SHA-256 checksum, atomically replaces the binary, and resets the daemon so it picks up the new executable. It prefers the managed service path and falls back to a detached daemon if service startup is unavailable or fails.

`gatehouse update` installs the latest stable release.
Use `gatehouse update --beta` to opt into prereleases and install the latest beta when one is newer than the current stable release.
Use `gatehouse update -y` to answer yes to the daemon-executable-mismatch prompt described below.

Because `update` installs the latest official release binary, it installs a binary with the default self-hosted telemetry host and website ID. Disable telemetry with `GATEHOUSE_TELEMETRY=0`, or override the host and website ID with `GATEHOUSE_UMAMI_HOST` and `GATEHOUSE_UMAMI_WEBSITE_ID`.

If pending or running pipeline runs exist, the update refuses to restart the daemon and prints each active run's ID, status, branch, and short head SHA. Pass `--force` to restart the daemon anyway and accept that those runs may fail; `-y`/`--yes` does **not** bypass this guard.
If the running daemon was started from a different binary, the update still prompts before replacing it; `-y`/`--yes` answers that prompt non-interactively.
If the daemon executable path cannot be determined, the update aborts before replacing the binary.
If the daemon does not come back cleanly after a successful replacement, the new binary stays installed but the command reports the daemon reset failure.

Background update checks run automatically on each CLI invocation (except `update` itself and version queries `--version` / `-v`, which stay side-effect-free). Suppress with `GATEHOUSE_NO_UPDATE_CHECK=1`.

## Remove from a repo

```sh
gatehouse eject
```

Removes the `gatehouse` remote, deletes the bare repo, cleans up worktrees, and removes the database record.
It does not remove repo-local agent skill files created by `gatehouse init`.

## Uninstall

Stop the daemon, delete the binary, and clear state:

```sh
gatehouse daemon stop
rm -f ~/.local/bin/gatehouse /usr/local/bin/gatehouse
rm -rf ~/.gatehouse
```

If you configured [`worktree_roots`](/gatehouse/reference/global-config/#worktree_roots), also delete the run worktree directories it placed outside `~/.gatehouse`.

On macOS, also remove `~/Library/LaunchAgents/com.bertrampetersen.gatehouse.daemon.*.plist`. On Linux, also remove `~/.config/systemd/user/gatehouse-daemon-*.service`. On Windows, remove the `gatehouse-daemon-*` Task Scheduler task.

<h1 align="center"><code>git push gatehouse</code></h1>
<p align="center">
  <a href="https://github.com/BertramPetersen/gatehouse/actions/workflows/release.yml"
    ><img
      alt="Release"
      src="https://img.shields.io/github/actions/workflow/status/BertramPetersen/gatehouse/release.yml?style=flat-square&label=release"
  /></a>
  <a href="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-blue?style=flat-square"
    ><img
      alt="Platform"
      src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux%20%7C%20Windows-blue?style=flat-square"
  /></a>
</p>

<h3 align="center">Define your own gates. Nothing ships until every one is green.</h3>

<p align="center"><strong>English</strong> · <a href="README.zh-CN.md">简体中文</a></p>

<p align="center">
  <img src="https://raw.githubusercontent.com/BertramPetersen/gatehouse/main/demo.gif" alt="gatehouse demo" width="800" />
</p>

`gatehouse` puts a local git proxy in front of your real remote.
Push to `gatehouse` instead of `origin`, and it spins up a disposable worktree, runs an AI-driven validation pipeline, forwards the branch to the configured push target only after every check passes, and opens a clean PR automatically.

- **Your own gates** - declare extra checks in `.gatehouse.yaml`; each is anchored after the core step it extends and can only ADD a verdict, so configuring gates never skips, reorders, or replaces a built-in one.
- **Non-blocking** - the pipeline runs in an isolated worktree without disrupting your work.
- **Agent-agnostic** - `claude`, `codex`, `grok`, `rovodev`, `opencode`, `pi`, `copilot`, `antigravity`, or `cursor` / `acp:<target>` via `acpx`, with ordered fallbacks; every gate requires a runnable configured pipeline agent.
- **Agent-native** - `/gatehouse` lets your coding agent do a task and gate it, or gate existing committed work: it runs the pipeline, has the pipeline apply safe fixes, and escalates the rest to you.
- **Human stays in charge** - auto-fix or review findings, your call.
- **Clean PRs by default** - push, open PR, watch CI, and auto-fix failures in one shot.

Full documentation: <https://bertrampetersen.github.io/gatehouse/>

## How it works

```
        your branch
            │  git push gatehouse
            ▼
   ┌────────────────────────────────────────────────┐
   │  disposable worktree — your work stays put     │
   │  review → test → docs → lint → push → PR → CI  │
   └────────────────────────────────────────────────┘
            │  every check green
            ▼
        clean PR, opened for you
```

Each step either passes on its own or stops with a **finding** for you to act on.
Safe, mechanical fixes are applied automatically; anything that touches your intent is escalated for you to **approve**, **fix**, or **skip**.
Nothing reaches the configured push target until every check is green.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/BertramPetersen/gatehouse/main/docs/install.sh | sh
```

Windows, Go install, and build-from-source instructions are in the [installation guide](https://bertrampetersen.github.io/gatehouse/start-here/installation/).

## Quick Start

```sh
$ gatehouse init
  ✓ Gate initialized

    repo  /Users/you/src/my-repo
    gate  gatehouse → /Users/you/.gatehouse/repos/abc123def456.git
  remote  git@github.com:you/my-repo.git
   skill  /gatehouse installed for agents at user level

  Push through the gate with:
  git push gatehouse <branch>

$ git checkout my-branch

# do some work in the branch...

$ git push gatehouse
  * Pipeline started

  Run gatehouse to review.

$ gatehouse
# opens the TUI for the active run
```

For GitHub fork contributions, keep `origin` pointed at the parent repository and initialize with `gatehouse init --fork-url <your-fork-url>`.

From the TUI you act on each **finding**: **auto-fix** ones are applied for you (or approve to let them), **ask-user** ones are a judgement call you approve, fix, or skip.
Once every check is green, the gate forwards your branch to the configured push target and opens the PR for you, so there is no manual `git push origin` and no hand-written PR body.
Prefer to let your coding agent drive the same flow headlessly?
Use `/gatehouse` (see below).

## Three ways to trigger the gate

Every change runs through the same pipeline. Pick the entry point that fits how you're working when the change is ready:

- **`git push gatehouse`** - the explicit Git path. Push a committed branch to the gate remote instead of `origin`.
- **`gatehouse`** - the TUI. Run it after making changes (no commit needed) and a wizard walks you through creating a branch, committing, and pushing through the gate, then attaches to the run. `gatehouse -y` does all of that automatically.
- **`/gatehouse`** - the agent skill. Tell the coding agent to do a task and gate it with `/gatehouse <task>`, or use bare `/gatehouse` to gate existing committed work. It runs the pipeline, has the pipeline apply safe fixes, and stops to ask you about anything that needs a human call.

`gatehouse init` installs the `/gatehouse` skill for Claude Code and other agents. Under the hood the skill drives `gatehouse axi`, a non-interactive TOON interface to the same approval flow.

See the [quick start](https://bertrampetersen.github.io/gatehouse/start-here/quick-start/) for the full first-run walkthrough.

## Development

```sh
make build   # Build bin/gatehouse with version info
make test    # Run go test -race ./... (excludes the e2e suite)
make e2e     # Run the tagged end-to-end agent journey suite
make e2e-record # Re-record e2e fixtures when agent wire formats change
make lint    # Check generated skill drift and run go vet ./...
make skill   # Regenerate committed gatehouse skill files
make fmt     # Run gofmt -w .
make demo    # Regenerate demo.gif and demo.mp4 (needs vhs and ffmpeg)
make docs    # Build the Astro docs site in docs/dist
```

See `Makefile` for the full target list.

`make e2e-record` overwrites `internal/e2e/fixtures/` from the real `claude`, `codex`, `opencode`, and `antigravity` CLIs, spends real API quota, and should be reviewed before committing.

## Credits

`gatehouse` is a fork of [kunchenguid/no-mistakes](https://github.com/kunchenguid/no-mistakes)
by Kun Chen, released under the MIT License, which originated the gated-push
pipeline architecture this project is built on. `gatehouse` adds
repository-declared custom gates and is maintained independently.

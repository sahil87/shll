# Project Context

## Overview

`shll` is a meta-CLI for the sahil87 open-source toolkit. It composes operations that span all the per-tool CLIs (`hop`, `wt`, `fab-kit`, `rk`, `tu`, `idea`) so users have one entry point for cross-toolkit concerns.

The name comes from the project's landing domain: [`ai.shll.in`](https://ai.shll.in).

## Tech stack

- **Language**: Go (≥1.22), single-binary CLI
- **CLI framework**: [cobra](https://github.com/spf13/cobra) (subcommand routing, completion generation)
- **Process execution**: `os/exec` wrapped in `internal/proc` (per Constitution Principle I)
- **Build**: `justfile` → `scripts/` (per Constitution Principle VI), mirrors hop's layout
- **Distribution**: Homebrew tap at [`sahil87/homebrew-tap`](https://github.com/sahil87/homebrew-tap), formula `shll`
- **CI**: GitHub Actions cross-compile + release on `v*` tags, auto-PRs the formula bump to the tap

## Repo shape

Mirrors `hop`:

```
shll/
├── src/
│   ├── go.mod
│   ├── cmd/shll/         # cobra entry + subcommand files (main.go, update.go, shell_init.go, version.go)
│   └── internal/
│       └── proc/         # subprocess wrapper (Run, RunForeground, ErrNotFound)
├── scripts/              # build.sh, install.sh, release.sh
├── justfile              # one-line recipes delegating to scripts/
├── README.md
├── LICENSE
├── fab/                  # fab-kit project state
└── docs/                 # memory + specs
```

## Subcommands (current scope)

| Command | Purpose |
|---------|---------|
| `shll update` | Run `brew update` once, self-upgrade shll, then delegate to each installed tool's own `update` (with `--skip-brew-update` when supported) — falling back to `brew upgrade sahil87/tap/<formula>` only for a tool with no `update` subcommand |
| `shll shell-init <shell>` | Concatenate the shell-init output of all sahil87 tools that expose one (today: `tu shell-init`, `hop shell-init`, `wt shell-init`) |
| `shll version` | Print versions of `shll` itself and every installed sahil87 tool |

## Tool roster (hardcoded, see Constitution III)

| Tool | Brew formula | Has `update`? | Has `shell-init`/`shell-setup`? |
|------|--------------|---------------|---------------------------------|
| `fab-kit` | `sahil87/tap/fab-kit` | yes | no |
| `rk` | `sahil87/tap/rk` | yes | no |
| `tu` | `sahil87/tap/tu` | yes | yes (`shell-init`) |
| `hop` | `sahil87/tap/hop` | yes | yes (`shell-init`) |
| `wt` | `sahil87/tap/wt` | yes | yes (`shell-init`) |
| `idea` | `sahil87/tap/idea` | yes | no |

All six tools expose an `update` subcommand. `shll update` upgrades each installed tool by **delegating to that tool's own `update`** (appending `--skip-brew-update` when the tool advertises it) rather than calling `brew upgrade <formula>` directly — this preserves each tool's post-upgrade side effects, e.g. rk's daemon restart (Constitution Principle IV). A tool with no `update` subcommand would fall back to `brew upgrade`. Per-tool `update` commands continue to work standalone (Constitution Principle IV) — `shll update` does not deprecate them.

## External commands shll invokes

- `brew update --quiet` (once per `shll update`)
- `brew info --json=v2 sahil87/tap/<formula>`
- `brew upgrade sahil87/tap/<formula>` (shll self-upgrade, and the fallback for a tool with no `update` subcommand)
- `brew list --formula --versions sahil87/tap/<formula>` (or `command -v <tool>` for presence check)
- `<tool> update --help` (capability probe — checks for the `--skip-brew-update` substring)
- `<tool> update [--skip-brew-update]` (delegated per-tool upgrade in `shll update`)
- `<tool> shell-init <shell>` (for shell-init composition)
- `<tool> --version` (for version reporting)

All routed through `internal/proc`.

## Conventions inherited from hop

- Subprocess wrapper in `internal/proc` with `Run`, `RunForeground`, `ErrNotFound`
- Cobra root in `cmd/shll/root.go`; one file per subcommand
- `version` variable injected via `-ldflags` at build time
- `errSilent` sentinel for cases where cobra should not double-print an error message
- Test files alongside source (`update_test.go`, `shell_init_test.go`)

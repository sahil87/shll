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
| `shll update` | Run `brew update && brew upgrade sahil87/tap/<each>` for every installed sahil87 tool |
| `shll shell-init <shell>` | Concatenate the shell-init output of all sahil87 tools that expose one (today: `hop shell-init`, `wt shell-setup`) |
| `shll version` | Print versions of `shll` itself and every installed sahil87 tool |

## Tool roster (hardcoded, see Constitution III)

| Tool | Brew formula | Has `update`? | Has `shell-init`/`shell-setup`? |
|------|--------------|---------------|---------------------------------|
| `fab-kit` | `sahil87/tap/fab-kit` | yes (existing) | no |
| `rk` | `sahil87/tap/rk` | yes (existing) | no |
| `tu` | `sahil87/tap/tu` | yes (existing) | no |
| `hop` | `sahil87/tap/hop` | yes (existing) | yes (`shell-init`) |
| `wt` | `sahil87/tap/wt` | no | yes (`shell-setup`) |
| `idea` | `sahil87/tap/idea` | no | no |

Per-tool `update` commands continue to work standalone (Constitution Principle IV) — `shll update` does not deprecate them.

## External commands shll invokes

- `brew update --quiet`
- `brew info --json=v2 sahil87/tap/<formula>`
- `brew upgrade sahil87/tap/<formula>`
- `brew list --formula --versions sahil87/tap/<formula>` (or `command -v <tool>` for presence check)
- `<tool> shell-init <shell>` and `<tool> shell-setup` (for shell-init composition)
- `<tool> --version` (for version reporting)

All routed through `internal/proc`.

## Conventions inherited from hop

- Subprocess wrapper in `internal/proc` with `Run`, `RunForeground`, `ErrNotFound`
- Cobra root in `cmd/shll/root.go`; one file per subcommand
- `version` variable injected via `-ldflags` at build time
- `errSilent` sentinel for cases where cobra should not double-print an error message
- Test files alongside source (`update_test.go`, `shell_init_test.go`)

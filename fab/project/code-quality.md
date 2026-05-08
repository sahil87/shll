# Code Quality

## Principles

- Readability and maintainability over cleverness
- Follow existing project patterns — when in doubt, look at how `hop` does it
- Prefer composition over inheritance (Go idioms — small interfaces, embedded behavior)
- Subprocess invocation goes through `internal/proc`, never raw `os/exec` from command code
- Sub-tool integration goes through the sub-tool's own CLI (Constitution III) — never reimplement what a sub-tool already provides

## Anti-Patterns

- God functions (>50 lines without clear reason)
- Duplicating logic that exists in a sub-tool's CLI
- Magic strings or numbers without named constants — formula names, timeout durations, tool roster entries all belong in named constants
- Hardcoding `/opt/homebrew` or `/usr/local` paths — derive from `os.Executable()` and symlink resolution (see hop's `isBrewInstalled`)
- Parsing `brew` output with regex when `--json=v2` is available

## Test Strategy

test-alongside

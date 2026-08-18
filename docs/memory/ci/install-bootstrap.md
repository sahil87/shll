---
type: memory
description: "`scripts/install.sh` — the `curl …` pipe-to-`sh` toolkit bootstrap served at shll.ai/install: POSIX-sh, main()-truncation-guarded, owns the whole pre-brew phase (platform-correct git/curl/tmux preflight, headless `NONINTERACTIVE=1` Homebrew bootstrap, absolute-`$BREW` + shellenv threading), capability-probed tap-trust, then `exec shll install \"$@\"`. The `scripts/install.sh` path is load-bearing — shll.ai raw-fetches it from `main`."
---
# ci/install-bootstrap

The copy-paste install one-liner. Source: `scripts/install.sh`.

```sh
curl -fsSL https://shll.ai/install | sh                # install everything
curl -fsSL https://shll.ai/install | sh -s -- hop wt   # install a subset
```

## Overview

`scripts/install.sh` is a POSIX-sh bootstrap that owns the whole **pre-brew phase**: it preflights the dependencies the install needs (git/CLT, curl, tmux), bootstraps Homebrew headlessly when absent, then solves the circularity that `shll` cannot trust/install its own Homebrew formula before that binary exists on `PATH`. Once `shll` is present it `exec`s into `shll install "$@"`, which owns all the post-brew intelligence — roster knowledge, subset filtering, per-formula trust for the other six tools, graceful skips (Constitution III — wrap, don't reinvent). The script carries none of that logic.

## Behavior contract

The body is wrapped `main() { … }; main "$@"` and runs `set -eu`. In `main`'s evaluation order:

1. **Preflight — platform-correct probes, one consolidated report.** Before any Homebrew check, `preflight()` probes:
   - **git** — Darwin: `xcode-select -p >/dev/null 2>&1` (the real CLT presence check; `command -v git` is never used on macOS because the CLT shim at `/usr/bin/git` false-positives when the Command Line Tools are not installed). Linux: `command -v git`.
   - **curl** — `command -v curl` (brew.sh and brew both need it).
   - **tmux** — `command -v tmux`.

   Every miss is collected and printed **all at once** to stderr as a single consolidated block, each line carrying a per-platform fix command (macOS git/CLT → `xcode-select --install`; Linux → `sudo apt-get install -y <pkg>` with a generic package-manager fallback; tmux → `brew install tmux` once Homebrew is in place) — never fail-on-first. Then the fatality matrix applies:
   - **curl missing → fatal** (exit 1 after the report; brew.sh and brew both require it).
   - **git missing → fatal on Linux when brew is absent** (brew.sh's Linux prerequisite); **fatal on macOS when brew is already present** (brew's git/tap operations need the real CLT); **informational-only on macOS when brew is absent** — the `NONINTERACTIVE=1` bootstrap installs the CLT itself via `softwareupdate`, so the script prints a note saying so instead of failing.
   - **tmux missing → warn-only, never fatal** — tmux is run-kit's *runtime* dependency, not an install prerequisite (Constitution V); the warning carries the fix command so a fresh-VM user learns about it at install time, not at first `rk` use.
2. **Homebrew bootstrap when absent.** If `command -v brew` fails, print a progress line, download the official installer with `curl -fsSL` into a variable — a failed or empty download aborts with a clear error, never an empty `/bin/bash -c ""` no-op — then run it headlessly: `NONINTERACTIVE=1 /bin/bash -c "$brew_installer"`. Unconditional — no opt-in flag (stdin is the pipe, so nothing can prompt; positional args are the `shll install` subset, so a flag would collide with tool names). brew.sh is bash-only, so the POSIX-sh script invokes it via `/bin/bash -c` — bash is present on fresh macOS and Ubuntu images. Under `set -eu` a failing installer aborts the script with the installer's own error output — the same surface-the-error tolerance as the trust/install steps.
3. **Absolute `$BREW` for the rest of the run.** A fresh brew install is not on `PATH` in the running process, so the script resolves `BREW` once — `command -v brew` when brew was already present; post-bootstrap, the first executable among `/opt/homebrew/bin/brew` (Apple Silicon), `/usr/local/bin/brew` (Intel macOS), `/home/linuxbrew/.linuxbrew/bin/brew` (Linux); if no candidate is executable, a clear error and exit 1 (never a silent bare-`brew` failure). Every subsequent brew call (trust probe, `brew trust`, `brew install`) goes through `"$BREW"`.
4. **shellenv — post-bootstrap only.** After a bootstrap, `eval "$("$BREW" shellenv)"` runs in-process before the exec hand-off (the freshly installed `shll` lives in the brew prefix and is otherwise command-not-found), and the script prints the exact rc line to persist (e.g. `eval "$(/opt/homebrew/bin/brew shellenv)"`) — brew's shellenv is the user's rc line to keep; `shll shell-setup` wires shll's own init, not brew's. When brew was already on `PATH`, no bootstrap runs and no shellenv line is printed.
5. **Idempotent shll short-circuit.** If `command -v shll` succeeds, skip the whole trust/install block and fall straight through to the exec.
6. **Trust-then-install shll (only when missing).** Otherwise print one progress line (`shll not found — installing sahil87/tap/shll via Homebrew...`) to stdout, then:
   - **Capability-probed trust.** `"$BREW" trust --help >/dev/null 2>&1` gates the trust step. On success run `"$BREW" trust --formula sahil87/tap/shll`; on non-zero (pre-6.0 Homebrew has no `brew trust` and no trust requirement) skip it silently. This mirrors the Go probe `brewTrustAvailable` (`src/cmd/shll/brew.go:67`) — **the probe is the contract, never a version-floor check**. See [cli/install §Per-formula trust before install](/cli/install.md#per-formula-trust-before-install).
   - `"$BREW" install sahil87/tap/shll`.
   - Under `set -e`, a failing `brew trust` or `brew install` on Homebrew 6.0+ aborts the script with brew's own error output — no swallowing. That surface-the-error tolerance is intentional.
7. **Exec hand-off.** `exec shll install "$@"` — every arg forwarded verbatim as the install subset (tool names, e.g. `hop wt`). `shll install` validates the names itself (`resolveTargets`, `allowShll=false`; the alias `rk` resolves to `run-kit`). The script contains zero roster/subset/per-tool-trust logic.

## Requirements

### Requirement: Truncation-safe, non-interactive POSIX bootstrap
The script SHALL be POSIX `sh` (it is piped to `sh`, not bash), wrap its entire body in `main() { … }` invoked only by a final `main "$@"`, and be fully non-interactive (stdin is the pipe; nothing prompts). It SHALL pass `sh -n` (and shellcheck when available). The Homebrew installer it invokes is bash-only and runs via `/bin/bash -c` — the outer script itself stays sh.

#### Scenario: Truncated download executes nothing
- **GIVEN** a partial pipe delivers only the leading portion of the script
- **WHEN** `sh` reads it
- **THEN** because `main "$@"` is the sole invocation and the last line, a partial body defines a function but never runs it — nothing half-formed executes

### Requirement: Preflight probes git, curl, and tmux before any Homebrew check
The script SHALL run a preflight first: git probed via `xcode-select -p` on Darwin (never `command -v git` — the `/usr/bin/git` CLT shim false-positives) and `command -v git` on Linux; curl and tmux via `command -v`. Every miss SHALL be reported in one consolidated stderr block with a per-platform fix command — never fail-on-first. Exit semantics: curl missing is fatal; git missing is fatal on Linux-without-brew and on macOS-with-brew, informational-only on macOS-without-brew (the bootstrap installs the CLT itself); tmux missing is warn-only.

#### Scenario: fresh macOS with neither CLT nor Homebrew
- **GIVEN** `xcode-select -p` fails and `brew` is not on `PATH`
- **WHEN** the preflight runs
- **THEN** the consolidated report lists git/CLT with `xcode-select --install`, an informational note says the Homebrew bootstrap installs the CLT itself, and the script proceeds to the bootstrap rather than exiting

#### Scenario: only tmux missing
- **GIVEN** git and curl are present and tmux is not
- **WHEN** the preflight completes
- **THEN** a warning with the tmux fix command is printed and the script proceeds (exit path unaffected)

### Requirement: Bootstrap Homebrew headlessly when absent
When `command -v brew` fails, the script SHALL print a progress line, download the official installer (`curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh`) into a variable — aborting with a clear error when the download fails or returns empty — and run it via `NONINTERACTIVE=1 /bin/bash -c "$brew_installer"` — unconditionally, no opt-in flag. Afterward it SHALL resolve `BREW` to an absolute path (`command -v brew`, else the first executable of `/opt/homebrew/bin/brew`, `/usr/local/bin/brew`, `/home/linuxbrew/.linuxbrew/bin/brew`; none found → clear error and exit 1), use `"$BREW"` for every brew call, `eval "$("$BREW" shellenv)"` in-process before the exec, and print the persistent rc line for the user's shell. When brew is already present, no bootstrap runs and no shellenv line is printed.

#### Scenario: brew absent on a fresh machine
- **GIVEN** preflight passed and `command -v brew` fails
- **WHEN** `main` reaches the brew step
- **THEN** the official installer runs headlessly, `$BREW` resolves to the installed prefix's `brew`, shellenv is eval'd in-process, and the exact rc line is printed for future shells

### Requirement: Delegate all intelligence to `shll install`
The final action SHALL be `exec shll install "$@"`. The script SHALL NOT duplicate roster knowledge, subset filtering, per-formula trust for the other tools, or graceful skips.

#### Scenario: subset pass-through
- **GIVEN** the script is invoked as `sh -s -- hop wt`
- **WHEN** it reaches the hand-off
- **THEN** it runs `exec shll install hop wt` (args forwarded verbatim), and the script itself carries no roster/subset logic

## The shll.ai raw-fetch URL contract

`scripts/install.sh` on `main` is what shll.ai serves at `shll.ai/install`. The site repo's build fetches `https://raw.githubusercontent.com/sahil87/shll/main/scripts/install.sh` into its `public/install` with a fail-hard `curl -f` (sahil87/shll.ai#84).

**The path is load-bearing.** Renaming or moving `scripts/install.sh` breaks the site deploy (the raw-fetch 404s). This is why the local dev script lives at a different path — see [The local dev script](#the-local-dev-script).

## The local dev script

`scripts/install-local.sh` is the **local dev install script** (bash: `./scripts/build.sh`, then copy `./bin/shll` to `~/.local/bin/shll`), delegated to by the `justfile` `install` recipe — `just install` builds and installs the binary locally. The bootstrap owns the pinned `scripts/install.sh` path (see the URL contract above). (m1zt)

## Design Decisions

### Script owns the pre-brew phase; `shll install` owns everything post-brew
**Decision**: All logic that must run before brew and shll exist — preflight probes, the headless Homebrew bootstrap, `$BREW`/shellenv handling, and the shll self-trust/install — lives in `scripts/install.sh`; the script then `exec shll install "$@"`. Roster knowledge, subset filtering, per-formula trust for the other tools, and graceful skips stay in Go.
**Why**: Keeps all post-brew intelligence versioned and tested in Go (Constitution III). The pre-brew steps are exactly the circularity carve-out the thin-bootstrap design grants the script — `shll install` cannot probe-or-bootstrap Homebrew because the user cannot have `shll` without Homebrew having worked.
**Rejected**: (a) Teaching `shll install` to bootstrap brew — unreachable code (you cannot have shll without brew having worked). (b) A fat script re-implementing roster logic — violates Constitution III and would drift from `shll install`.
*Introduced by*: `m1zt`; extended from the shll-self bootstrap to the full pre-brew phase by 260817-nava-install-bootstrap-gaps

### Capability probe for trust, not a version check
**Decision**: `brew trust --help` exit 0 gates the trust step.
**Why**: Mirrors the codebase contract `brewTrustAvailable` ("the probe is the contract"); pre-6.0 brews have no `brew trust` and need no trust.
**Rejected**: Parsing `brew --version` for a 6.0 floor — brittle and off-contract.
*Introduced by*: `m1zt`

### Bootstrap Homebrew headlessly when absent
**Decision**: The script installs Homebrew headlessly (`NONINTERACTIVE=1` brew.sh, invoked via `/bin/bash -c`, from the official raw URL) when absent — reversing the earlier require-Homebrew-and-exit stance (m1zt).
**Why**: Fresh-VM testing (2026-08-17) proved the official installer runs fully headless on macOS (including the CLT install via `softwareupdate`) and on Linux (given git) — removing the original "large, surprising side effect" objection. The hard stop also contradicted the "clean machine to wired toolkit" promise, and the brew-not-on-PATH trap made even the manual path fail on re-run.
**Rejected**: Keeping the https://brew.sh pointer + exit 1 — proven to be the single biggest fresh-VM friction point.
*Introduced by*: 260817-nava-install-bootstrap-gaps

## Test gate

Behavior is verified by inspection and a syntax/lint gate — no Go changes, no CI wiring:

- `sh -n scripts/install.sh` (and `dash -n`) — the enforceable gate.
- shellcheck when available (an apply/review gate, deliberately **not** wired into CI — adding a workflow would be scope expansion with no requirement behind it).
- The runtime paths (preflight matrix, brew-absent bootstrap, shll-present short-circuit, arg pass-through) are verified by reading the script, not by executing brew. The heavy runtime claim — `NONINTERACTIVE=1` brew.sh fully headless, including the CLT install via `softwareupdate` — is validated by fresh-VM testing (2026-08-17), which the script encodes rather than re-derives.

## Cross-references

- The delegation target and the trust contract it mirrors: [cli/install](/cli/install.md) (esp. [§the curl \| sh upstream entry point](/cli/install.md#the-curl--sh-upstream-entry-point) and [§Per-formula trust before install](/cli/install.md#per-formula-trust-before-install)).
- The Go trust capability probe this script mirrors: `brewTrustAvailable` in `src/cmd/shll/brew.go`.
- User-facing docs leading with the one-liner: `README.md` (`## Install`), `docs/site/install.md` (Bootstrap via Homebrew), `docs/site/workflows.md` (fresh-machine walkthrough).
- Constitution III (Wrap, Don't Reinvent — all post-brew intelligence stays in `shll install`), V (Graceful Degradation — trust degrades to skip on pre-6.0; tmux-missing warns rather than blocks), VI (Thin Justfile — the renamed dev script keeps logic in `scripts/`).

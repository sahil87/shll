---
type: memory
description: "`scripts/install.sh` — the `curl …` pipe-to-`sh` toolkit bootstrap served at shll.ai/install: POSIX-sh, main()-truncation-guarded, brew-required (never auto-installed), capability-probed tap-trust, and `exec shll install \"$@\"` delegation. Owns the load-bearing `scripts/install.sh` path shll.ai raw-fetches from `main`."
---
# ci/install-bootstrap

The copy-paste install one-liner. Source: `scripts/install.sh`.

```sh
curl -fsSL https://shll.ai/install | sh                # install everything
curl -fsSL https://shll.ai/install | sh -s -- hop wt   # install a subset
```

## Overview

`scripts/install.sh` is a **thin** POSIX-sh bootstrap. Its only job is to solve the circularity that `shll` cannot trust/install its own Homebrew formula before that binary exists on `PATH`. Once `shll` is present it `exec`s into `shll install "$@"`, which owns all the intelligence — roster knowledge, subset filtering, per-formula trust for the other six tools, graceful skips (Constitution III — wrap, don't reinvent). The script is a few dozen auditable lines and carries none of that logic.

## Behavior contract

The body is wrapped `main() { … }; main "$@"` and runs `set -eu`. In `main`'s evaluation order:

1. **Homebrew required, never auto-installed.** If `command -v brew` fails, write `Homebrew is required but was not found.` and an `Install it from https://brew.sh, then re-run this script.` pointer to **stderr** and `exit 1`. The script never installs Homebrew (explicitly ruled out in design — see [Design Decisions](#design-decisions)).
2. **Idempotent shll short-circuit.** If `command -v shll` succeeds, skip the whole bootstrap block (no `brew trust`, no `brew install`) and fall straight through to the exec.
3. **Trust-then-install shll (only when missing).** Otherwise print one progress line (`shll not found — installing sahil87/tap/shll via Homebrew...`) to stdout, then:
   - **Capability-probed trust.** `brew trust --help >/dev/null 2>&1` gates the trust step. On success run `brew trust --formula sahil87/tap/shll`; on non-zero (pre-6.0 Homebrew has no `brew trust` and no trust requirement) skip it silently. This mirrors the Go probe `brewTrustAvailable` (`src/cmd/shll/brew.go:67`) — **the probe is the contract, never a version-floor check**. See [cli/install §Per-formula trust before install](/cli/install.md#per-formula-trust-before-install).
   - `brew install sahil87/tap/shll`.
   - Under `set -e`, a failing `brew trust` or `brew install` on Homebrew 6.0+ aborts the script with brew's own error output — no swallowing. That surface-the-error tolerance is intentional.
4. **Exec hand-off.** `exec shll install "$@"` — every arg forwarded verbatim as the install subset (tool names, e.g. `hop wt`). `shll install` validates the names itself (`resolveTargets`, `allowShll=false`; the alias `rk` resolves to `run-kit`). The script contains zero roster/subset/per-tool-trust logic.

## Requirements

### Requirement: Truncation-safe, non-interactive POSIX bootstrap
The script SHALL be POSIX `sh` (it is piped to `sh`, not bash), wrap its entire body in `main() { … }` invoked only by a final `main "$@"`, and be fully non-interactive (stdin is the pipe; nothing prompts). It SHALL pass `sh -n` (and shellcheck when available).

#### Scenario: Truncated download executes nothing
- **GIVEN** a partial pipe delivers only the leading portion of the script
- **WHEN** `sh` reads it
- **THEN** because `main "$@"` is the sole invocation and the last line, a partial body defines a function but never runs it — nothing half-formed executes

### Requirement: Homebrew is a hard prerequisite
When `brew` is absent from `PATH`, the script SHALL print the https://brew.sh pointer to stderr and exit 1 without installing anything.

#### Scenario: brew missing
- **GIVEN** `brew` is not on `PATH`
- **WHEN** the script runs
- **THEN** it writes the "Homebrew is required" message + https://brew.sh pointer to stderr and exits 1, with no Homebrew auto-install

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

### Thin bootstrap, not a fat installer
**Decision**: The script solves only the bootstrap circularity, then `exec shll install "$@"`.
**Why**: Keeps all roster/subset/trust intelligence versioned and tested in Go (Constitution III). The script is a few dozen auditable lines the site can describe as short.
**Rejected**: (a) Pointing users at the `all` meta-formula (`brew trust --formula sahil87/tap/all && brew install sahil87/tap/all`) — still demands the trust ceremony and offers no subset form. (b) A fat script re-implementing roster logic — violates Constitution III and would drift from `shll install`.
*Introduced by*: `m1zt`

### Capability probe for trust, not a version check
**Decision**: `brew trust --help` exit 0 gates the trust step.
**Why**: Mirrors the codebase contract `brewTrustAvailable` ("the probe is the contract"); pre-6.0 brews have no `brew trust` and need no trust.
**Rejected**: Parsing `brew --version` for a 6.0 floor — brittle and off-contract.
*Introduced by*: `m1zt`

### Require Homebrew; never auto-install it
**Decision**: Missing `brew` prints a pointer to https://brew.sh and exits 1.
**Why**: Auto-installing Homebrew from a piped-to-`sh` script is a large, surprising side effect; a pointer keeps the bootstrap thin and honest.
**Rejected**: Bundling a Homebrew install — explicitly ruled out in the originating conversation.
*Introduced by*: `m1zt`

## Test gate

Behavior is verified by inspection and a syntax/lint gate — no Go changes, no CI wiring for this change:

- `sh -n scripts/install.sh` (POSIX syntax) — the enforceable gate; shellcheck was unavailable in the apply environment.
- shellcheck when available (an apply/review gate, deliberately **not** wired into CI — adding a workflow would be scope expansion with no requirement behind it).
- The runtime paths (brew-absent, shll-present short-circuit, arg pass-through) are verified by reading the script, not by executing brew.

## Cross-references

- The delegation target and the trust contract it mirrors: [cli/install](/cli/install.md) (esp. [§the curl \| sh upstream entry point](/cli/install.md#the-curl--sh-upstream-entry-point) and [§Per-formula trust before install](/cli/install.md#per-formula-trust-before-install)).
- The Go trust capability probe this script mirrors: `brewTrustAvailable` in `src/cmd/shll/brew.go`.
- User-facing docs leading with the one-liner: `README.md` (`## Install`), `docs/site/install.md` (Bootstrap via Homebrew), `docs/site/workflows.md` (fresh-machine walkthrough).
- Constitution III (Wrap, Don't Reinvent — all intelligence stays in `shll install`), V (Graceful Degradation — trust degrades to skip on pre-6.0), VI (Thin Justfile — the renamed dev script keeps logic in `scripts/`).

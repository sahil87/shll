# Intake: Copy-paste installer one-liner — `scripts/install.sh` served at shll.ai/install

**Change**: 260715-m1zt-install-bootstrap-one-liner
**Created**: 2026-07-15

## Origin

Promptless dispatch (Create-Intake Procedure, `{questioning-mode} = promptless-defer`) from a synthesized description whose design decisions were made in conversation with the user — captured here, not re-litigated.

> **Change: copy-paste installer one-liner — `scripts/install.sh` served at shll.ai/install**
>
> Goal: users install the whole sahil87 toolkit with `curl -fsSL https://shll.ai/install | sh`, or a subset with `curl -fsSL https://shll.ai/install | sh -s -- hop wt`.
>
> This repo adds: (1) the canonical bootstrap script `scripts/install.sh` — POSIX sh, `main()`-wrapped, thin (require Homebrew, trust+install shll if missing, `exec shll install "$@"`), shellcheck-clean, idempotent, fully non-interactive, a few dozen auditable lines; (2) a README `## Install` section leading with the one-liner. Context in flight: sahil87/shll.ai#84 (draft) fetches `https://raw.githubusercontent.com/sahil87/shll/main/scripts/install.sh` into the site's `public/install` at build time (fail-hard `curl -f`) — THIS change must merge to main first or that deploy fetch 404s.

**Intake-time discovery (not in the description)**: `scripts/install.sh` **already exists** in this repo — it is the local dev install script (`./scripts/build.sh` + copy to `~/.local/bin/shll`) that the justfile `install` recipe delegates to (justfile line 11, its only reference). The description assumed the file was new. The bootstrap's path is pinned by shll.ai#84's raw-fetch URL, so the existing dev script is renamed to make room — see What Changes and Assumptions row 5.

## Why

1. **The pain point**: installing the toolkit today takes a four-step ceremony that requires knowing about Homebrew 6.0's tap-trust requirement: `brew trust --formula sahil87/tap/shll`, `brew install sahil87/tap/shll`, `shll install`, `shll shell-setup`. The trust step is non-obvious (skipping it surfaces as an opaque sandbox failure), and there is no single copy-paste entry point of the kind users expect from modern toolkits (`curl … | sh` is the de-facto industry install UX — Homebrew itself, rustup, uv, etc.).
2. **The consequence of not doing it**: the shll.ai site PR (sahil87/shll.ai#84, draft) is blocked — its deploy step fetches `https://raw.githubusercontent.com/sahil87/shll/main/scripts/install.sh` with fail-hard `curl -f` at build time, which 404s until this file exists on main. The site docs in that PR already promise the one-liner ("Requires Homebrew — the script exits with a pointer if it's absent"). Onboarding friction persists and the published promise would be false.
3. **Why this approach**: a *thin* bootstrap keeps all intelligence in `shll install` (Constitution III — wrap, don't reinvent): roster knowledge, subset filtering, per-formula trust for the other tools, and graceful skips already live there, versioned and tested in Go. The script only solves the strict bootstrap circularity — shll cannot trust/install its own formula before it exists. Rejected alternatives: (a) pointing users at the `all` meta-formula (`brew trust --formula sahil87/tap/all && brew install sahil87/tap/all`) — still demands the trust ceremony and offers no subset form; (b) auto-installing Homebrew from the script — explicitly ruled out in conversation (print a pointer to https://brew.sh and exit 1); (c) a fat script that re-implements roster logic — violates Constitution III and would drift from `shll install`.

## What Changes

### 1. `scripts/install.sh` — the canonical bootstrap (replaces the dev script at this path; see §2)

New content at `scripts/install.sh`, versioned next to the CLI it bootstraps. Behavioral contract (all points decided in conversation):

- **POSIX sh** — it is piped to `sh`, not bash. Must pass shellcheck cleanly if shellcheck is available.
- **Truncation-safe**: the entire body is wrapped in `main() { ... }; main "$@"` so a partially downloaded script cannot execute a half-script.
- **Thin bootstrap ONLY** (Constitution III):
  - If `brew` is not on PATH → print a pointer to https://brew.sh to stderr and `exit 1`. Do NOT auto-install Homebrew.
  - If `shll` is already on PATH → skip the bootstrap entirely (idempotent) and go straight to the exec.
  - Else: `brew trust --formula sahil87/tap/shll` (Homebrew 6.0 tap-trust ceremony — 6.0 defaults `HOMEBREW_REQUIRE_TAP_TRUST=1` and the formula's sandboxed `def install` re-checks a persisted trust record), then `brew install sahil87/tap/shll`.
  - Finally `exec shll install "$@"` — all args pass through as the subset (tool names, e.g. `hop wt tu run-kit fab-kit idea`; `shll install` already accepts `cobra.ArbitraryArgs` and validates names itself).
- **`brew trust` tolerance**: on pre-6.0 Homebrew the `trust` subcommand does not exist and no trust is needed — detect and skip silently; on 6.0+ let a trust/install failure surface with brew's own error output (no swallowing). Detection mirrors the Go capability probe `brewTrustAvailable` (src/cmd/shll/brew.go:67): `brew trust --help` exit 0 ⇒ available — a capability probe, never a version-floor check ("the probe is the contract").
- **No duplicated intelligence**: roster knowledge, subset filtering, per-formula trust for the other six tools, graceful skips — all stay in `shll install`. The script must NOT duplicate any of it.
- **Fully non-interactive**: stdin is the pipe; nothing may prompt.
- **Target length**: a few dozen auditable lines (the shll.ai#84 site docs describe it that way).

Reference sketch (shape, not final code — apply refines wording/comments):

```sh
#!/bin/sh
# shll toolkit bootstrap — served at https://shll.ai/install
#   curl -fsSL https://shll.ai/install | sh                # everything
#   curl -fsSL https://shll.ai/install | sh -s -- hop wt   # subset
set -eu

main() {
    if ! command -v brew >/dev/null 2>&1; then
        echo "Homebrew is required but was not found." >&2
        echo "Install it from https://brew.sh, then re-run this script." >&2
        exit 1
    fi

    if ! command -v shll >/dev/null 2>&1; then
        echo "shll not found — installing sahil87/tap/shll via Homebrew..."
        # Homebrew 6.0+ requires a persisted trust record before a tap
        # formula's sandboxed install may run. Pre-6.0 brews have no
        # `brew trust` (and no trust requirement) — probe and skip silently.
        if brew trust --help >/dev/null 2>&1; then
            brew trust --formula sahil87/tap/shll
        fi
        brew install sahil87/tap/shll
    fi

    exec shll install "$@"
}

main "$@"
```

Under `set -e`, a failing `brew trust` or `brew install` on 6.0+ aborts the script with brew's own error output — exactly the decided tolerance behavior.

### 2. Rename the existing dev script: `scripts/install.sh` → `scripts/install-local.sh`

The current `scripts/install.sh` (bash; `./scripts/build.sh` then copy `./bin/shll` to `~/.local/bin/shll`) is the `just install` target — a build recipe, not a user-facing bootstrap. The bootstrap MUST own the `scripts/install.sh` path (pinned by shll.ai#84's raw-fetch URL), so:

- `git mv scripts/install.sh scripts/install-local.sh` (content unchanged).
- justfile line 11: `./scripts/install.sh` → `./scripts/install-local.sh` (the recipe name `install` and its comment stay; the user-facing `just install` UX is unchanged). This is the only reference in the repo (verified by grep across justfile, scripts/, .github/, README.md, docs/site/).

### 3. `README.md` — Install section leads with the one-liner

- Add a new `## Install` section **immediately after the repo title / toolkit blockquote / one-line description** (i.e., after current line 5, before `## Why shll?`), leading with the install-everything form first, subset form second:

  ```sh
  curl -fsSL https://shll.ai/install | sh
  ```

  ```sh
  curl -fsSL https://shll.ai/install | sh -s -- hop wt
  ```

  followed by a short description: requires Homebrew (the script exits with a pointer to https://brew.sh if it's absent); bootstraps `shll` itself (recording Homebrew 6.0 tap trust), then hands off to `shll install` for the rest of the roster; idempotent.
- **Absorb the existing `## Install` section** (currently after Quick start): the manual brew bootstrap one-liner (`brew trust --formula sahil87/tap/shll && brew install sahil87/tap/shll`), the `all` meta-formula note, and the `### From source` subsection move into the new section as the manual/alternative paths; the old section is removed.
- **Quick start** updates its first two bootstrap lines to the curl one-liner (`curl -fsSL https://shll.ai/install | sh`, then `shll shell-setup`, then `exec $SHELL`) and trims the bootstrap-explanation paragraph accordingly — the tap-trust deep-dive stays in Troubleshooting and docs/site/install.md.

### 4. `docs/site/install.md` — light touch (lead with the one-liner)

This page is pulled by shll.ai (committed into `content/shll/site/` and rendered at shll.ai/tools/shll/install); shll.ai#84 does NOT touch it (verified: #84 changes only site-local `getting-started/install.md`, `index.mdx`, `deploy.yml`, `.gitignore`). Add the one-liner (both forms) at the top of the "Bootstrap via Homebrew" section as the recommended path, keeping the manual trust-then-install flow as the explicit/manual alternative. Light touch only — no restructure.

### Explicitly NOT in scope

- No new `shll` subcommand (Constitution VII untouched) and no Go code changes.
- No CI wiring for shellcheck (the shellcheck-clean requirement is a local apply/review gate: run shellcheck if available, plus `sh -n` syntax check).
- No changes to the shll.ai repo (PR #84 is in flight there and is not to be redone).
- No auto-install of Homebrew.

### Merge-order constraint (for ship)

THIS change must merge to main **before** sahil87/shll.ai#84 — the site's deploy fetch (`curl -f` of raw main `scripts/install.sh`) 404s until the file exists on main.

## Affected Memory

- `ci/install-bootstrap`: (new) the curl|sh bootstrap script contract — POSIX/`main()`-wrapper/brew-required/trust-probe/exec-delegation, the shll.ai raw-fetch URL contract (`scripts/install.sh` on main → shll.ai/install) with its merge-order constraint, and the dev-script rename to `scripts/install-local.sh`.
- `cli/install`: (modify) note the new upstream entry point — `curl -fsSL https://shll.ai/install | sh [-s -- <tools…>]` execs into `shll install "$@"`, making the script's arg pass-through part of `shll install`'s public surface.

## Impact

- **Files**: `scripts/install.sh` (new content, ~30–40 lines), `scripts/install-local.sh` (renamed dev script, content unchanged), `justfile` (1 line), `README.md` (Install/Quick-start restructure), `docs/site/install.md` (light addition).
- **Code**: no Go changes; `src/` untouched. No new subprocess invocations inside shll itself (Constitution I unaffected — the script is the thing being shipped, and it shells to brew/shll by design).
- **Cross-repo**: unblocks sahil87/shll.ai#84 (its deploy fetch depends on this file existing on main). The `scripts/install.sh` path becomes a published URL contract — renaming/moving it later breaks the site deploy.
- **Users**: `just install` (dev flow) behavior unchanged; new users get a single copy-paste install path; existing manual brew path keeps working.
- **Testing**: shellcheck (if available) + `sh -n` on the script; behavior verification is manual/by-inspection (brew-absent path, shll-present short-circuit, arg pass-through).

## Open Questions

None — all material decisions were made in the originating conversation (captured above); the intake-time discoveries (path collision, docs/site page) resolved to Confident assumptions below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Bootstrap lives at `scripts/install.sh`, served as shll.ai/install via shll.ai#84's build-time raw fetch of main | Discussed — the URL contract is pinned by the in-flight site PR; path is user-given | S:95 R:50 A:95 D:95 |
| 2 | Certain | Thin-bootstrap contract: require brew (pointer to https://brew.sh + exit 1, never auto-install); trust+install shll only when missing; `exec shll install "$@"`; zero roster/subset logic in the script | Discussed — Constitution III; all intelligence stays in `shll install` | S:95 R:65 A:95 D:95 |
| 3 | Certain | POSIX sh, `main(){…}; main "$@"` truncation guard, fully non-interactive, shellcheck-clean, a few dozen lines | Discussed — script is piped to `sh`; site docs describe it as short and auditable | S:95 R:85 A:95 D:95 |
| 4 | Certain | `brew trust` tolerance: pre-6.0 detect-and-skip silently; on 6.0+ let trust/install failures surface with brew's own error output (`set -e`, no swallowing) | Discussed — pre-6.0 has no trust requirement; 6.0+ errors are brew's to explain | S:90 R:85 A:90 D:90 |
| 5 | Confident | Path collision resolution: existing dev install script renamed `scripts/install.sh` → `scripts/install-local.sh`; justfile `install` recipe updated (its sole reference); bootstrap takes the pinned path | NOT discussed — description assumed the file was new; the bootstrap path is forced by the raw-fetch URL contract, so only the rename target was open (low-stakes) | S:35 R:75 A:60 D:55 |
| 6 | Certain | Trust-availability probe is `brew trust --help` exit-0 (capability probe, never a version-floor check) | Codebase contract — mirrors `brewTrustAvailable` in src/cmd/shll/brew.go ("the probe is the contract") | S:60 R:85 A:95 D:85 |
| 7 | Confident | README restructure: new `## Install` after the intro line (everything form first, subset second); old `## Install` body (manual brew bootstrap, `all` meta-formula, from-source) absorbed into it; Quick start's bootstrap lines replaced by the one-liner | Discussed placement + "replacing/absorbing any existing brew install instructions"; exact absorption shape is agent judgment | S:70 R:90 A:70 D:60 |
| 8 | Confident | `docs/site/install.md` gets a light update (one-liner leads the Bootstrap section) | NOT in the described 2-item scope, but the page is pulled by shll.ai and rendered at /tools/shll/install, and shll.ai#84 verifiably does not touch it — omitting the canonical one-liner there would leave the deep install guide leading with the ceremony the one-liner replaces | S:35 R:85 A:70 D:55 |
| 9 | Confident | No CI shellcheck wiring — "must pass shellcheck cleanly if shellcheck is available" is an apply/review gate (shellcheck + `sh -n`), not a workflow change | Description phrases it conditionally; adding CI is scope expansion with no requirement behind it | S:50 R:90 A:60 D:55 |
| 10 | Confident | Script output stays minimal: one progress line before the bootstrap install (e.g. "shll not found — installing via Homebrew…"); brew and `shll install` stream their own output | Low-stakes presentation; consistent with the thin-wrapper posture | S:45 R:90 A:70 D:60 |
| 11 | Certain | No new shll subcommand, no Go changes (Constitution VII untouched) | Discussed — stated explicitly in the description | S:90 R:80 A:95 D:95 |
| 12 | Certain | Merge-order: this change lands on main before shll.ai#84 merges (deploy fetch 404s otherwise) — a ship-stage note, no mechanism needed in this repo | Discussed — stated explicitly as the merge-order constraint | S:90 R:70 A:90 D:90 |

12 assumptions (7 certain, 5 confident, 0 tentative, 0 unresolved).

# Intake: Tap-trust DX for Homebrew 6.0

**Change**: 260626-0854-tap-trust-dx
**Created**: 2026-06-26

## Origin

Initiated from a `/fab-discuss` session investigating a real-world install failure: until someone manually ran `brew trust --formula sahil87/tap/fab-kit`, users could not install `fab-kit`. The discussion traced this to a behavior change in Homebrew 6.0 and worked through the DX implications conversationally, ending with the user explicitly choosing the design via three decisions.

> User's framing: *"Until this was typed: `brew trust --formula sahil87/tap/fab-kit` people weren't able to install fab-kit. Check the latest commands in README, see if they need updating. Look at `shll install` — do we need `shll install --trust`, and is `shll shell-setup --trust-tap` still needed? What's the simplest DX for folks to use shll and its sister utilities?"*

Interaction mode: conversational (extended `/fab-discuss`, then three explicit design decisions via structured questions, then "yes, go ahead, start the change").

**Decisions the user made explicitly** (via structured choice):
1. `shll install` establishes trust **by default**, with a `--no-trust` opt-out.
2. `shll shell-setup --trust-tap` is **removed entirely**.
3. The 38a6 Linux trust-bypass workaround is **removed now** (closes backlog `[tkch]`).

This change supersedes the temporary approach in backlog `[38a6]` and closes its cleanup follow-up `[tkch]`.

## Why

**The problem.** Homebrew 6.0 changed tap-trust from an advisory warning into a hard install requirement. Verified against the installed Homebrew 6.0.4 source on a Linux box:

- `HOMEBREW_REQUIRE_TAP_TRUST` now defaults to **`true`** (`env_config.rb:643`, `default: true`). Trust is mandatory by default — no env var needed to trigger enforcement. The README and shll's memory still describe it as a 2–3× env-*hint* (the 5.1.x transition framing), which is now obsolete.
- The trust check fires in **two places**: at formula **load** time (`formulary.rb:130 require_trusted_formula!`, outside the sandbox — here, naming the fully-qualified formula on the CLI is *explicitly allowed* via `trust.rb` `explicitly_allowed?`/ARGV), and again during the **sandboxed `install`** (the in-sandbox re-check has a different ARGV — the formula path, not the qualified name — so CLI-naming does **not** satisfy it; a real trust record is required).
- shll's tap formulae use `url` + `def install` (e.g. `shll.rb:29 def install` → `bin.install "shll"`), **not** a `bottle do` block. So `brew install sahil87/tap/<formula>` runs a sandboxed install → the in-sandbox re-check fires → a persisted trust record (tap- or formula-level) is genuinely required.

**Why the current design fails the user.**
- The README quick-start runs `shll install` **before** `shll shell-setup --trust-tap`. On current brew, trust must exist *before* install, so `shll install` is refused at the build gate — the documented happy path is broken.
- Trust lives in the wrong command: `shell-setup --trust-tap` is shell-wiring, but trust is about *installing/loading formulae*. Its `export HOMEBREW_REQUIRE_TAP_TRUST=1` line is now **redundant** (it just re-sets the brew default) and was never the thing that unblocks installs (the `brew trust` record is). It also runs **whole-tap** trust (`brew trust --tap`), which Homebrew explicitly recommends against for third parties ("prefer trusting the specific formula you need").
- The 38a6 Linux workaround (`brewEnv()` injecting `HOMEBREW_NO_REQUIRE_TAP_TRUST=1`) now **masks** the requirement: `shll install`/`update` silently bypass trust on Linux, leaving the user's brew untrusted, so the next plain `brew install/upgrade` wall-hits with no explanation. The upstream bug it worked around is **fixed in 6.0.4** (verified: `formula_installer.rb:1146` does `sandbox.allow_read_if_exists path: Homebrew::Trust.trust_file`; `sandbox.rb:270` exempts the trust file from `deny_read_home`).

**What happens if we don't fix it.** New users keep hitting opaque per-formula install failures, the README sends them down a broken path, and Linux users stay in a fragile silently-untrusted state.

**Why this approach.** shll is the meta-tool that knows its exact roster, so it can do the Homebrew-recommended **per-formula** trust automatically for precisely the tools it installs — turning "install the toolkit" into one trusted command at the recommended granularity, while reducing surface area (remove `--trust-tap`) and removing an obsolete, now-harmful workaround.

## What Changes

### 1. `shll install` — trust per-formula by default, `--no-trust` opt-out

Before installing each missing roster tool, trust its formula:

```sh
brew trust --formula sahil87/tap/<formula>   # per tool in the install set, before its brew install
```

- **Default behavior**: `shll install` (and subset `shll install hop wt`) trusts each formula in the install set, then installs. `brew trust` is idempotent (`Already trusted formula: …`, exit 0), so re-runs stay clean.
- **`--no-trust` flag**: skips the trust step entirely, for users who manage trust themselves.
- **Granularity**: per-formula (`brew trust --formula sahil87/tap/<formula>`), NOT whole-tap — aligns with Homebrew's recommendation and trusts only what shll actually manages.
- **Graceful degradation (Constitution V)**: if `brew trust` is unavailable (brew too old to ship it) or the ceremony fails, warn and continue to the install attempt rather than hard-aborting. On pre-6.0 brew, trust isn't required, so absence of `brew trust` is safe; on 6.0+, it's present. New helper `brewTrustFormula(ctx, formula)` in `brew.go` routes through `internal/proc` (Constitution I); reuse the existing `brewTrustAvailable` capability probe.
- **Bootstrap note**: shll cannot trust its *own* formula before it exists — `brew trust sahil87/tap/shll && brew install sahil87/tap/shll` remains the one-time bootstrap (see README below). shll owns trust for the other six.

### 2. `shll shell-setup` — remove `--trust-tap`, back to pure rc-wiring

- Delete the `--trust-tap` flag, the `export HOMEBREW_REQUIRE_TAP_TRUST=1` managed line and its merge logic (`exportTrustLine`, the export branch of `wantLines`), the `ensureTrustFunc` seam threaded through `runShellSetup`/`runShellSetupDefault`/`runShellSetupPrint`, and the now-unused whole-tap ceremony in `brew.go` (`ensureTapTrust`, `brewTrustTap`, `trustHatchHint`, `tapName` if unused elsewhere).
- `shell-setup` reverts to maintaining only the `eval "$(shll shell-init <shell>)"` line. The sentinel block becomes single-line again. The `TestNoProcImports` invariant on `shell_setup.go` gets *stronger* (the trust seam is gone — the file goes back to pure file-I/O).
- Migration: existing rc blocks that contain the export line should be cleaned up to drop it on the next `shll shell-setup` run (the export is inert/redundant, but leaving a stale `HOMEBREW_REQUIRE_TAP_TRUST=1` export is harmless if removal is complex — decide in plan).
- Net surface-area reduction (Constitution VII): a flag is removed, no new top-level command added.

### 3. Remove the 38a6 Linux workaround (closes `[tkch]`)

- Drop `brewEnv()`'s Linux-gated `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` injection and its `osGoos` gate (`brew.go`).
- Remove the workaround from the brew call sites: `brew install <formula>` (`install.go`), `brew update --quiet` and `brew upgrade` self/fallback (`update.go`).
- Decide the fate of the `Env`-carrying proc plumbing added by 38a6 (`proc.Request.Env` / `RunForegroundEnv` / `requestEnv`): if nothing else uses it after removal, either strip it or keep it as harmless general-purpose infra — plan decides. Per backlog `[tkch]`, prefer simplest.
- Delete/repurpose the workaround tests: `TestInstall_BrewInstallCarriesWorkaroundEnvOnLinux`, `TestInstall_BrewInstallNoWorkaroundEnvOnDarwin`, and the `update.go` equivalents (flip to assert no override, or remove).
- **Floor**: requires Homebrew ≥ 6.0.4 (the fix). Users on 6.0.0–6.0.3 Linux must `brew update` first — call this out in the README rather than gating in code (the user chose "remove now," not "gate on brew version").

### 4. README + `docs/site/install.md` rewrite

**Quick-start reorder** (bootstrap trust first):

```sh
brew trust sahil87/tap/shll && brew install sahil87/tap/shll   # bootstrap: trust + install shll
shll install                                                   # trusts (per-formula) + installs the other 6
shll shell-setup                                               # pure rc wiring — no trust flag
exec $SHELL
```

Show the 3-line form as primary (legible, `shll install`'s per-tool output stays readable); offer the chained one-liner as an "all at once" variant.

- Rewrite the "Tap sahil87/tap is allowed by default" troubleshooting section: it's a **hard block** now, not a 2–3× nag. Explain the load-gate vs. sandboxed-install-gate distinction briefly (why CLI-naming isn't enough), and why the bootstrap `brew trust` line is required (binary-download formula runs a sandboxed `def install`, not a bottle pour).
- Remove all `--trust-tap` documentation. Document `--no-trust` on `shll install`.
- Note the Homebrew ≥ 6.0.4 requirement and the `brew update` remedy for stragglers.

### 5. `shll doctor` — read-only "formula trusted?" check

`doctor` gains one more per-tool sub-check: for each **installed** roster tool, query whether its formula is trusted and **WARN** if not (an untrusted-but-installed tool still runs, but its next `brew upgrade` — via `shll update` or plain brew — will be refused on Homebrew 6.0+).

- **Read-only, via brew** (Constitution III — never read `~/.homebrew/trust.json` directly): query trust state with `brew trust --json=v1` and check each installed roster formula against the trusted entries (tap- or formula-level trust both count). Reuse `brewTrustAvailable` as the gate.
- **Severity**: untrusted-but-installed → `WARN` (it works now, breaks on upgrade), folded into the existing worst-check-wins OK/WARN/FAIL marker — same tier as "installed but unwired". Actionable suggestion: `formula not trusted — run 'shll install' (or 'brew trust --formula sahil87/tap/<x>'); future upgrades will fail without it`.
- **Scope**: applies to the six roster tools only — the always-OK `shll` self row is unchanged (consistent with how the wiring check skips the self row), and not-installed tools already FAIL on the binary check (trust is moot there).
- **Graceful degradation**: if brew is absent or too old to ship `brew trust` (pre-6.0, where trust isn't required anyway), skip the trust sub-check silently — never WARN on a state we can't determine. `doctor` stays strictly read-only and keeps its any-FAIL→exit-1 / `--json` contract.

This is the chosen closure for the "installed outside `shll install`" gap (e.g. a tool installed manually or before this feature) — surfacing it where the user can act, without adding a second trust-*mutating* path.

### Non-goals (explicitly out of scope)

- **Shipping real `bottle do` bottles** from CI. A true bottle *pour* runs no sandboxed install, so `brew install sahil87/tap/<x>` would need no pre-trust at all — eliminating the bootstrap step entirely. This is a release-infra change across all tap repos and warrants its own backlog item, not this change.
- **`shll update` mutating trust.** `update` will NOT establish trust (rejected: silently changing trust state on an upgrade command violates least-surprise). Trust mutation stays confined to `install`; `update` relies on `install` having trusted the tools, and `doctor` (section 5) surfaces any untrusted-but-installed tool so the user can re-run `shll install`.

## Affected Memory

- `cli/install`: (modify) default per-formula trust before install, `--no-trust` flag, removal of the 38a6 `brewEnv` workaround on `brew install`
- `cli/shell-setup`: (modify) remove `--trust-tap`, the export line + merge logic, and the `ensureTrustFunc` ceremony seam; pure rc-wiring; stronger `TestNoProcImports`
- `cli/update`: (modify) removal of the 38a6 `brewEnv` workaround on `brew update`/`brew upgrade`
- `cli/commands`: (modify) `brew.go` helper changes — add `brewTrustFormula`, keep `brewTrustAvailable`, remove `ensureTapTrust`/`brewTrustTap`/`trustHatchHint`/`brewEnv`
- `cli/doctor`: (modify) new read-only per-tool "formula trusted?" sub-check (WARN on installed-but-untrusted), via `brew trust --json=v1`
- `internal/proc`: (modify) fate of the 38a6 `Env`/`RunForegroundEnv` plumbing (strip or keep)

## Impact

- **Code**: `src/cmd/shll/install.go`, `src/cmd/shll/shell_setup.go`, `src/cmd/shll/update.go`, `src/cmd/shll/doctor.go`, `src/cmd/shll/brew.go`, `src/cmd/shll/tools.go` (`tapName`/`formulaPrefix` constants), possibly `src/internal/proc/proc.go`. Tests: `install_test.go`, `shell_setup_test.go`, `update_test.go`, `doctor_test.go`, `brew_test.go`, possibly `proc_test.go`.
- **Docs**: `README.md`, `docs/site/install.md`, `docs/site/workflows.md` (if it references `--trust-tap`).
- **External behavior**: removes a flag (`--trust-tap`) — breaking for anyone who scripted it; changes `shll install` to mutate trust state by default (mitigated by `--no-trust`).
- **Backlog**: closes `[tkch]`; supersedes `[38a6]`. A new backlog item should be filed for the bottle-shipping non-goal.
- **Constitution**: I (proc routing for the new trust ceremony), III/IV (still wrapping brew), V (graceful degradation when `brew trust` absent), VII (net surface reduction — removes a flag; `--no-trust` is a flag on existing `install`, no new top-level command).
- **Personal memory** already updated: `homebrew-linux-sandbox-trust-bug.md` records the verified 6.0.4 fix and that `[tkch]` is actionable.

## Open Questions

- Should the `shell-setup` migration actively strip a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line from existing rc blocks, or leave it (inert)? (Plan decides; leaning: strip it for cleanliness since the block is already rewritten.)
- Keep or strip the `proc.Request.Env` plumbing once the 38a6 workaround is gone? (Plan decides; backlog `[tkch]` prefers simplest.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `shll install` trusts per-formula by default, `--no-trust` opts out | User explicitly chose "default-on, --no-trust opt-out" via structured question | S:90 R:70 A:90 D:90 |
| 2 | Confident | Remove `shell-setup --trust-tap` entirely | User explicitly chose "remove entirely"; breaking removal so R is moderate | S:90 R:60 A:85 D:90 |
| 3 | Confident | Remove the 38a6 workaround now (closes tkch) | User chose "remove now"; brew 6.0.4 fix verified; regresses 6.0.0–6.0.3 stragglers (README mitigates) | S:90 R:55 A:90 D:85 |
| 4 | Certain | Per-formula trust granularity (not whole-tap) | Homebrew docs recommend per-formula for third parties; shll knows its roster | S:80 R:70 A:90 D:80 |
| 5 | Certain | README/docs rewrite with bootstrap-first ordering | Derived from verified brew 6.0.4 behavior; docs trivially reversible | S:85 R:90 A:90 D:85 |
| 6 | Confident | On `brew trust` failure/absence, warn and continue (degrade, not abort) | Constitution V; pre-6.0 brew doesn't require trust so absence is safe, 6.0+ ships it | S:60 R:70 A:80 D:75 |
| 7 | Certain | `doctor` gains a read-only trusted? check (in scope); `update` does NOT mutate trust | User explicitly chose option 2 (doctor check) and rejected option 3 (update trust mutation) | S:90 R:80 A:85 D:90 |
| 8 | Confident | Shipping real `bottle do` bottles is out of scope (separate backlog item) | Big release-infra change across tap repos; clearly beyond this change | S:70 R:80 A:75 D:75 |
| 9 | Certain | Bootstrap line `brew trust sahil87/tap/shll && brew install …` | Verified: idempotent trust, `&&` safe; shorthand infers --formula | S:85 R:95 A:95 D:80 |
| 10 | Confident | Create as fresh NL change ID, reference tkch for archive marking | Scope is a superset of tkch; fresh ID avoids implying change == tkch | S:70 R:75 A:70 D:65 |

10 assumptions (5 certain, 5 confident, 0 tentative, 0 unresolved). Run /fab-clarify to review.

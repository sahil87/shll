# Standards conformance report — 260820-t26g-roster-desktop-entry

Audited against `docs/site/standards/` (canonical; byte-matched to the embedded copies by `TestStandardsEmbedMatchesCanonical`) on a dev build from repo HEAD, after the roster reorder + rk-desktop roster entry landed. Audited shll version: dev build of this branch (standards are versioned with the shll release; this change is unreleased).

| Standard | Result |
|----------|--------|
| `principles` | PASS |
| `install-composition` | PASS |
| `update` | PASS |
| `help-dump` | PASS |
| `skill` | PASS (canonical bundle updated + re-embedded) |
| `readme-extraction` | PASS (fixed here — stale roster enumerations) |
| `version` | PASS (not re-audited — untouched surface) |
| `shell-init` | PASS (not re-audited — rk-desktop ships no shell-init; eval-safety untouched) |

## principles

PASS — per-principle notes: №1 (non-interactive) the delegated paths inherit the existing prompt-free shapes (no new prompts; `rk desktop install/update` run foregrounded prompt-free per the update standard); №2 (machine-readable) `list --json` / `doctor --json` / `check-updates --json` all carry rk-desktop rows through the existing schemas (no new field needed); №5 (`--dry-run` honesty) both install and update previews render the exact delegated argv (`rk desktop install` / `rk desktop update`) from the same builders the live path uses; №8 (graceful degradation) missing `rk`, unsupported platform, and absent rk-desktop all degrade to skip-with-note / `not installed`, never a crash; №10 (`skill` bundle) updated for the new roster (see below).

## install-composition

PASS — Policy A: no new formula and no `depends_on` introduced; rk-desktop's run-kit dependency is expressed as a runtime probe (`rk desktop status`) + roster adjacency only, exactly the standard's model. The delegated seam probes before invoking (`rk desktop …` never runs when the probe reports the platform refusing or the prerequisite missing) and degrades with an actionable note. Policy B: install docs stay centralized — the rk-desktop delegation is documented in `docs/site/install.md` (the curated install destination), not in a per-repo README snippet.

## update

PASS (shll as consumer) — rk-desktop delegates to `rk desktop update` prompt-free with inherited stdio; no `--skip-brew-update` probe (the flag is a brew concern; rk-desktop never runs a brew refresh), no brew-upgrade fallback and no relink heal (no formula, no keg). Its producer-side conformance is run-kit's own rollout surface, not shll's.

## help-dump

PASS — no command-tree change (no new/renamed/removed subcommand); only Long help TEXT changed (roster enumerations + the rk-desktop delegation notes in `install`/`update`/`uninstall`/`doctor`). The dump walks the live tree programmatically, so the new text flows through verbatim. `TestHelpDump*` green.

## skill

PASS — `docs/site/skill.md` (the canonical shll bundle) updated: roster enumeration in the new order including rk-desktop, and the install/update/uninstall capability lines describe the delegated paths. Re-embedded via `scripts/sync-standards.sh`; `TestSkillEmbedMatchesCanonical` green. The `shll setup agent` bootstrap description picks up rk-desktop's `SkillHint` automatically from the roster (pinned by `TestRosterSkillHints`).

## readme-extraction

PASS — fixed here: the README's intro roster enumeration and the `shll install` section's stale leaves-first roster listing were updated to the new order with the rk-desktop delegation note. No structural change (H1 → blockquote → badge run → prose intact); the README stays slimmed to bootstrap+pointer per the Policy B carve-out.

## version / shell-init

PASS (not re-audited) — the root `--version` producer surface is untouched (its conformance test, `TestRootVersionFlag_VersionStandardConformance`, stays green); the shell-init composition gains no new integrator (rk-desktop ships no shell-init), so the eval-safety invariants are untouched and `shell_init_test.go` stays green.

## Deferred

None. The run-kit companion (freezing `errDesktopMacOnly` with a test in run-kit, or a stable token/exit code if message matching proves unstable) is explicitly out of this change's queue per the intake — not a standard violation, a robustness follow-up owned by the run-kit repo.

# Intake: install.sh upgrades an already-installed shll before handing off

**Change**: 260929-6ywy-install-sh-upgrade-stale-shll
**Created**: 2026-09-29

## Origin

> HexoKit rebrand plan (run-kit repo, `fab/plans/sahil/26-09-12-hexokit-rebrand-remaining.md`), row **T2 step (a)**: "`scripts/install.sh` runs `brew upgrade sahil87/tap/shll` when shll is already installed, before `shll install`, so a stale shll never drives an install; release shll." Dispatched by the operator as a one-shot task (Sahil gave explicit go-ahead for Phase 4 T1 + T2). Step (b) — dropping hexokit-site's stale-shll guard — is a separate change in hexokit-site, done only after this ships and is released. Out of scope here.

## Why

`scripts/install.sh` (served at `hexokit.com/install`) short-circuits when `shll` is already on PATH: `command -v shll` succeeds → it skips the whole trust/install block and hands straight off to `shll install "$@"` then `exec shll update <tools>`. Whatever shll version the user happens to have drives the install. That is a real bug: shll ≤ v0.1.33 rejects the `hexokit` target (the roster only learned `hexokit` in v0.1.34), so `curl -fsSL https://hexokit.com/install | sh` on a box with an old shll fails, even though the current shll would accept it.

hexokit-site worked around this with a stale-shll guard in its composed epilogue (it probes `shll install --dry-run hexokit` and passes `run-kit` instead when that fails). That guard is a workaround for this script's behavior and should go once the script stops letting a stale shll drive the install. The same staleness hits any future roster/flag change, not just this rename.

Upgrading shll inside the bootstrap (not inside `shll install`) is the right layer: the running binary cannot upgrade itself before it parses its own args, and the script already owns everything about installing shll itself (the self-install circularity carve-out). `shll update` upgrades shll too, but it runs *after* `shll install`, too late.

## What Changes

### `scripts/install.sh` — shll handoff phase

Today (inside `main`, after `phase_start "shll handoff"`):

```sh
if ! command -v shll >/dev/null 2>&1; then
    echo "shll not found — installing sahil87/tap/shll via Homebrew..."
    if "$BREW" trust --help >/dev/null 2>&1; then
        "$BREW" trust --formula sahil87/tap/shll
    fi
    "$BREW" install sahil87/tap/shll
fi
shll install "$@"
```

New behavior: add an `elif` branch for "shll present AND brew-managed" that upgrades it before the hand-off:

```sh
if ! command -v shll >/dev/null 2>&1; then
    echo "shll not found — installing sahil87/tap/shll via Homebrew..."
    <trust probe + trust>
    "$BREW" install sahil87/tap/shll
elif "$BREW" list --versions sahil87/tap/shll >/dev/null 2>&1; then
    echo "shll found — upgrading sahil87/tap/shll via Homebrew so a stale shll never drives the install..."
    <trust probe + trust>
    "$BREW" upgrade sahil87/tap/shll
fi
shll install "$@"
```

Details:
- **Brew-managed gate**: `"$BREW" list --versions sahil87/tap/shll` — exit 0 when the formula is installed (verified: prints `shll 0.1.35`, rc 0), non-zero when not. A shll on PATH that brew does not manage (e.g. a `just install` dev build at `~/.local/bin/shll`) skips the upgrade silently — no error, no line.
- **Trust before upgrade**: reuse the same capability-probed trust step (`"$BREW" trust --help` gate → `"$BREW" trust --formula sahil87/tap/shll`). Covers a formula installed on a pre-6.0 brew that has since become 6.0+ (upgrade is a sandboxed install and needs the trust record). Idempotent — verified on Homebrew 7.0.7: `Already trusted formula: sahil87/tap/shll`, rc 0. Factor the trust step into a small helper function (e.g. `trust_shll`) so both branches share it rather than duplicating the probe.
- **Already current**: `brew upgrade` on a current formula exits 0 with a warning (`already installed`) — no special-casing.
- **Failure**: under `set -eu` a failing upgrade aborts the script with brew's own error output — the same surface-the-error tolerance as the existing `brew trust` / `brew install` steps. No swallowing.
- **Fresh install branch unchanged**: shll missing → trust + `brew install` (already current; no upgrade).
- Brew's own auto-update (`brew upgrade` runs `brew update` unless `HOMEBREW_NO_AUTO_UPDATE`) refreshes the tap, so the upgrade sees the new version. No explicit `brew update`.
- Update the handoff comment and the header comment block to state the new upgrade-before-handoff step.

### Memory

- `docs/memory/ci/install-bootstrap.md`: frontmatter description, Overview, Behavior contract step 5 (the "idempotent shll short-circuit" becomes "already-installed shll is upgraded first when brew-managed"), step 3's list of brew calls (add `brew list` / `brew upgrade`), a new/extended Requirement + scenarios (stale brew-managed shll → upgraded before `shll install`; non-brew shll → no upgrade; fresh → install only), a Design Decision (upgrade in the script, gated on brew-managed; rejected: upgrading inside `shll install`, relying on `shll update` which runs after install, unconditional `brew upgrade` that errors on a non-brew shll), and the Test gate runtime-path list.
- `docs/memory/cli/install.md` § The `curl | sh` upstream entry point: its sentence "trust-then-installs `shll` itself only if it is missing" → also upgrades a brew-managed shll that is already present.
- While there: the raw-fetch contract paragraph in `ci/install-bootstrap.md` says the hexokit.com composition block runs `main run-kit`; since hexokit-site#11 its no-arg default is `hexokit` (with the stale-shll fallback to `run-kit`). Correct that present-truth line; do not describe T2(b) as done.

## Affected Memory

- `ci/install-bootstrap`: (modify) upgrade-before-handoff for an already-installed brew-managed shll; stale composition-default line
- `cli/install`: (modify) curl|sh entry-point sentence

## Impact

- `scripts/install.sh` only (POSIX sh, load-bearing path — hexokit-site raw-fetches it from `main`; path unchanged). No Go changes.
- Test gate (per memory): `sh -n scripts/install.sh`, `dash -n`, shellcheck if available; exercise the three handoff branches in isolation with a stub `$BREW` + stub `shll` on a scratch PATH (fresh / brew-managed present / non-brew present), asserting the brew argv sequence.
- Ships to users on merge (hexokit-site's next deploy fetches `main`); a shll release follows for the plan row.

## Open Questions

(none)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Upgrade runs in `scripts/install.sh` before `shll install`, not in Go | Plan row T2(a) states it verbatim; operator task restates it | S:95 R:85 A:95 D:95 |
| 2 | Confident | Gate the upgrade on `"$BREW" list --versions sahil87/tap/shll` (brew-managed) | Unconditional `brew upgrade` errors under `set -e` on a non-brew shll (dev build); verified list rc semantics locally | S:80 R:85 A:80 D:75 |
| 3 | Confident | Run the capability-probed trust step before the upgrade too, via a shared helper | Covers pre-6.0-installed formulas on a 6.0+ brew; verified idempotent (rc 0) on Homebrew 7.0.7 | S:75 R:90 A:75 D:70 |
| 4 | Certain | A failing upgrade aborts (set -e), no swallow | Matches the script's documented surface-the-error tolerance for trust/install | S:85 R:85 A:90 D:85 |
| 5 | Confident | No explicit `brew update`; rely on brew's auto-update in `brew upgrade` | Keeps the script dumb; auto-update is brew's default | S:70 R:90 A:75 D:70 |
| 6 | Confident | Correct the stale `main run-kit` composition-default line in ci/install-bootstrap during hydrate | Present-truth drift in the same file this change edits; small and factual | S:70 R:95 A:80 D:75 |

6 assumptions (2 certain, 4 confident, 0 tentative, 0 unresolved).

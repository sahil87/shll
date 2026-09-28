# Intake: HexoKit Roster Rename (run-kit → hexokit)

**Change**: 260928-guq7-hexokit-roster-rename
**Created**: 2026-09-28

## Origin

> HexoKit rebrand Phase 3, plan row **R1 step (c)** (run-kit repo, `fab/plans/sahil/26-09-12-hexokit-rebrand-remaining.md`): shll roster `Name`+`Formula` → `hexokit`, `LegacyName` gains `run-kit`, `versions.json` row → `hexokit` (retire the S3 `envelope` carry-over), release shll.

One-shot dispatch from the operator, carrying Sahil's explicit go-ahead after he verified the R1(c) gate on his own machine: old formula `run-kit` 3.20.21 → `brew update && brew upgrade` migrated cleanly to `hexokit` 3.20.22 (`Cellar/run-kit` is Homebrew's compat symlink; `brew info sahil87/tap/run-kit` resolves to hexokit, "Old Names: run-kit"); `hexokit`, `rk`, `xk`, and `run-kit` on PATH all resolve to `Cellar/hexokit/3.20.22/bin/hexokit`. R1(a) (homebrew-tap `Formula/hexokit.rb` + `formula_renames.json` `run-kit → hexokit`) and R1(b) (run-kit release v3.20.22) are merged and released.

Pre-intake investigation in this conversation established:

- The roster lives in `src/cmd/shll/tools.go` (`var Roster []Tool`). The prior-rename precedent (`rk → run-kit`) is recorded as `LegacyName: "rk"` (a single string) plus `legacyAliases = map[string]string{"rk": "run-kit"}`. The rk→run-kit `LegacyFormula` field and its migration guard were **retired** in change `260720-h3f6` (brew's `formula_renames.json` handles keg migration; a never-migrated box is simply "not installed").
- **`versions.json` is not in this repo.** It is built at deploy time by the **hexokit-site** repo (`sites/astro-starlight-terminal1/src/lib/versions-manifest.ts`) from `help/<slug>.json` help-dump envelopes + `versions-policy.json`, keyed by slug (`run-kit` today). The "S3 envelope carry-over" is `help/run-kit.json` / the `run-kit` policy key / the `run-kit:run-kit:run-kit` triple in hexokit-site's `refresh-help.yml` — hexokit-site work, not a shll edit. shll only *consumes* the manifest (`internal/versions`, `check_updates.go` looks up `manifest.Tools[tgt.name]`).
- **Cross-repo consumer**: run-kit's `app/backend/internal/updatecheck/updatecheck.go` special-cases the `shll check-updates --json` row whose `name == "run-kit"` (`runKitTool`). Out of scope here; reported as a follow-up.

## Why

1. **Problem**: run-kit is now published as the `hexokit` formula and binary. shll's roster still names it `run-kit`, so `shll install` installs via the old name (`sahil87/tap/run-kit`, resolved only through the rename map), `shll update` delegates to `run-kit update` (a legacy alias binary), `shll list`/`version`/`doctor` label it `run-kit`, and the generated `shll-toolkit` agent skill tells agents to run `shll skill run-kit`.
2. **Consequence if not done**: the toolkit keeps advertising a retired name on every present-tense surface; the plan's roster coupling rule ("each field flips **with** its rename, in the same sitting") is violated now that R1(a)/(b) have shipped; and R1(d)/R2/X3 are blocked behind it.
3. **Approach**: follow the rk→run-kit precedent exactly — flip `Name`/`Formula`/`Update`, keep old names as recognized legacy names/aliases (never remove backward-compat recognition), keep `Repo` on `run-kit` until R2. Because `LegacyName` is a single string and must now hold two prior names (`rk`, `run-kit`), convert it to a slice. Rejected: a second parallel field (`LegacyName2`) — ugly; dropping `rk` from legacy — `rk` still ships as the canonical short command (D2) and must stay trigger vocabulary and a probe fallback.

## What Changes

All paths relative to repo root.

### 1. Roster entry (`src/cmd/shll/tools.go`)

```go
{Name: "hexokit", Formula: formulaPrefix + "hexokit", Update: []string{"hexokit", "update"}, Repo: "run-kit", LegacyNames: []string{"rk", "run-kit"}, Description: "HexoKit — tmux session manager with a web UI; … via `rk code exec` (rk stays as an alias)", …}
```

- `Name` `run-kit` → `hexokit`; `Formula` → `formulaPrefix + "hexokit"`; `Update` → `{"hexokit", "update"}`.
- `Repo` **stays `"run-kit"`** (the GitHub repo rename is R2). Rewrite the `Repo` field doc comment: Name and Repo now diverge for hexokit (repo `run-kit` until the repo rename) — exactly the case the explicit field exists for.
- `Description`: `Run-kit — …` → `HexoKit — …` (keep the rest, incl. `rk code exec` and "(rk stays as an alias)").
- `ProactiveHint`: `shll skill run-kit` → `shll skill hexokit`; `(run-kit)` → `(hexokit)`; `run-kit's web dashboard` → `HexoKit's web dashboard`; `onboarding of run-kit` → `onboarding of HexoKit`; `shll skill run-kit tutorial` → `shll skill hexokit tutorial`. Keep `rk code exec` (binary stays `rk`, D2).
- `rk-desktop` entry: `Name` stays `rk-desktop`, `Repo` stays `run-kit`, argv stays `rk desktop …` (D2 substrate). Description `Run-kit desktop viewer shell` → `HexoKit desktop viewer shell`.
- Roster order comments that name `run-kit` (e.g. "rk-desktop directly after run-kit") → `hexokit`.

### 2. `LegacyName string` → `LegacyNames []string`

- Field doc: the tool's PRIOR binary names in rename order, retained as binary-alias/display surfaces — the hexokit formula still installs `rk` (and Homebrew keeps a `run-kit` compat link after the migration). Empty for every tool except hexokit (`{"rk", "run-kit"}`).
- `version.go` `probeToolVersion`: on `proc.ErrNotFound` ONLY for `tool.Name`, retry each legacy name in order, stopping at the first probe that is not `ErrNotFound` (its output/error is returned). A present-but-broken binary (non-zero exit / timeout) still never defers. If every name is `ErrNotFound`, return `ErrNotFound`. Update the LEGACY-NAME FALLBACK doc comment (rk→run-kit→hexokit renames).
- `agent_setup.go` `agentSkillDescription`: render `Name` + `/`-joined legacy names → `tmux sessions (hexokit/rk/run-kit)`.
- `doctor.go` `probeVersion` comment: update names.

### 3. `legacyAliases` (`tools.go`)

`map[string]string{"rk": "hexokit", "run-kit": "hexokit"}` — `shll update|install|uninstall|skill|changelog rk|run-kit` keep resolving; the notice reads `note: run-kit is now hexokit` / `note: rk is now hexokit`. Keep the explicit map (existing pattern; `rk` is not strictly "retired" but is kept as a target alias as today). Update the doc comment ("rk→run-kit→hexokit renames"). `shll skill`/`shll changelog`'s alias resolution already goes through this map — no bespoke logic.

### 4. Delegation target (`agent_setup.go`, `uninstall.go`)

- `runKitToolName = "run-kit"` → rename to `hexokitToolName = "hexokit"`; `shll setup agent` delegates to `hexokit agent setup [--uninstall] [--yes]`. Rename `runKitAgentSetupArgs`/`delegateRunKitAgentSetup` identifiers only if cheap and consistent; at minimum doc comments/user-visible strings say `hexokit agent setup`.
- `uninstall.go` daemon-stop hint keys on the renamed const (matches Roster `Name == "hexokit"`); any user-visible hint text naming `run-kit` → `hexokit` (keep `rk daemon stop` style command if that's what the hint prints — binary stays `rk`).
- `rkBinary = "rk"`, `rkDesktopRefusalToken`, `isRkDesktopRefusal` are substrate — **unchanged**.

### 5. check-updates manifest lookup (`src/cmd/shll/check_updates.go`)

`resolveOneTarget`'s released-backend lookup `manifest.Tools[tgt.name]` MUST fall back to the tool's `LegacyNames` (in order) when `tgt.name` is absent — the hexokit.com manifest is keyed `run-kit` until hexokit-site renames its row (R1(d)/hexokit-site). Prefer `hexokit` when both keys are present. `inManifest` true if any key matched. JSON output `name` = `hexokit`, `formula` = `hexokit` (from the roster). Extract a small helper (e.g. `manifestEntry(m, tool) (ManifestTool, bool)`) rather than inlining the loop.

### 6. Not changed

- Substrate per D2: `rk`, `rk-desktop`, `RK_*`, `rkBinary`, `rk desktop …` argvs.
- `Repo` fields (R2).
- `internal/versions` schema/URLs; `versions.json` (lives in hexokit-site — no hand edit here).
- History per D11: `fab/changes/archive/**`, memory changelog narrative, `docs/site/standards/skill.md`'s "Precedent: `run-kit context`" historical section.
- github.com/sahil87/run-kit links (R2).

### 7. Tests

- Update every roster-name assertion to `hexokit` (tools_test, agent_setup_test, skill_test, check_updates_test, version_test, list/doctor/install/update/uninstall tests, changelog tests using roster names). Keep `internal/changelog` test's `ReleasesURL("run-kit")` (repo slug — unchanged).
- New/updated coverage: (a) version probe falls back `hexokit` → `rk` → `run-kit` on ErrNotFound only, and a non-ErrNotFound failure on an intermediate name stops the chain; (b) `run-kit` and `rk` aliases resolve to `hexokit` with the notice (update/install/skill); (c) manifest lookup: only `run-kit` key → resolves hexokit's latest; both keys → `hexokit` wins; neither → not in manifest; (d) agent skill description contains `hexokit/rk/run-kit` and the new ProactiveHint verbatim; (e) agent-setup delegation argv is `hexokit agent setup …`.
- `go test ./...`, `go vet ./...`, `gofmt -l .` clean (CI: `.github/workflows/ci.yml`).

### 8. Docs (present-tense surfaces only)

- `README.md`, `docs/site/install.md`, `docs/site/workflows.md`, `docs/site/skill.md`, `docs/site/standards/{update,version,principles,install-composition,help-dump,config-home}.md`: roster lists `run-kit` → `hexokit`; `run-kit agent setup` → `hexokit agent setup`; `shll skill run-kit display` → `shll skill hexokit display`; valid-target lists; the `run-kit` shell-init/list/version/doctor sample rows → `hexokit`; aliases note "legacy aliases `rk` and `run-kit` resolve to `hexokit`". The homebrew-core collision note (install.md:41) stays accurate (homebrew/core `run-kit` is still someone else's) — reword so the toolkit formula is `sahil87/tap/hexokit`. `config-home.md:54` "`run-kit` (becoming `hexokit`) is adopting" → present-tense hexokit. `version.md:43` rename-in-flight exception: mention legacy names (`rk`, `run-kit`).
- README.md:80 "Legacy `rk` keg exception" paragraph describes the migration guard retired in `260720-h3f6` — stale; remove/rewrite (a never-migrated box: `brew upgrade` follows `formula_renames.json`).
- Embedded copies: `src/cmd/shll/standards/*.md` and `src/cmd/shll/skill/skill.md` MUST be refreshed via `just sync-standards` (`scripts/sync-standards.sh`) — the drift-guard test (`TestStandardsEmbedMatchesCanonical`) enforces byte equality. Hand-edit only the canonical `docs/site/` files.
- `docs/memory/**` updates at hydrate, not apply.

## Affected Memory

- `cli/commands`: (modify) Roster entry now `hexokit` (Formula `sahil87/tap/hexokit`, Repo `run-kit`), `LegacyNames` slice, legacyAliases `rk`/`run-kit` → `hexokit`
- `cli/version`: (modify) legacy-name fallback chain `hexokit` → `rk` → `run-kit`
- `cli/check-updates`: (modify) manifest lookup falls back to legacy-name keys; row name/formula `hexokit`
- `cli/setup`: (modify) `hexokit agent setup` delegation
- `cli/uninstall`: (modify) daemon-stop hint keyed on `hexokit`
- `cli/skill`: (modify) `rk`/`run-kit` → `hexokit` alias; `shll skill hexokit`
- `cli/update`: (modify) `hexokit update` delegation; aliases
- `cli/list`: (modify) hexokit row, Repo `run-kit` diverges from Name
- `cli/doctor`: (modify) legacy-name fallback wording
- `cli/changelog`: (modify) alias wording
- `cli/standards-content`: (modify) standards docs relabeled to hexokit

## Impact

- Code: `src/cmd/shll/{tools,version,doctor,agent_setup,uninstall,check_updates}.go` (+ small comment touches elsewhere); tests across `src/cmd/shll/*_test.go`.
- Docs: README + `docs/site/**` + synced embeds.
- Runtime behavior: `shll install` installs `sahil87/tap/hexokit`; `shll update` runs `hexokit update`; `shll check-updates --json` emits `name: "hexokit"` — run-kit's daemon (`updatecheck.go` `runKitTool = "run-kit"`) will then treat that row as a sibling tool (brew-visible version, no selfBrew gate, empty `runKitFields`) until run-kit accepts `hexokit` — **cross-repo follow-up, not in this change**.
- Ships in the next shll release (patch via `just release`).

## Open Questions

- None blocking. Cross-repo follow-ups (reported, not done here): hexokit-site manifest/help slug rename (R1(d)); run-kit `updatecheck.runKitTool` accepting `hexokit`.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Name/Formula/Update → `hexokit`; Repo stays `run-kit` | Plan R1(c) row + rule D15 (repo rename is R2); Sahil's explicit go-ahead | S:95 R:80 A:95 D:95 |
| 2 | Certain | Keep `rk`/`run-kit` as recognized legacy names + target aliases; never drop back-compat | Operator instruction; rk→run-kit precedent (`260720-h3f6` kept LegacyName surfaces) | S:95 R:85 A:90 D:95 |
| 3 | Confident | Convert `LegacyName string` → `LegacyNames []string` {"rk","run-kit"} | Plan says "LegacyName gains run-kit"; single string can't hold two; a parallel field is worse | S:80 R:80 A:85 D:75 |
| 4 | Confident | check-updates falls back to legacy-name manifest keys, preferring `hexokit` | versions.json is generated in hexokit-site keyed `run-kit`; rename order across repos is independent, so shll must tolerate both | S:75 R:85 A:85 D:80 |
| 5 | Certain | No `versions.json` edit in shll; envelope carry-over flagged as hexokit-site work | Investigated: manifest built by hexokit-site from `help/<slug>.json` + `versions-policy.json`; not in this repo | S:90 R:90 A:90 D:90 |
| 6 | Confident | `shll setup agent` delegates to `hexokit agent setup` (rename const to `hexokitToolName`) | Present-truth binary name; `hexokit` is installed by the formula; ErrNotFound already a silent skip | S:75 R:85 A:80 D:75 |
| 7 | Confident | Do not add `xk` as a legacy alias | Not a prior name; D2 says xk is mentioned once as an alias; plan doesn't ask for it | S:70 R:90 A:80 D:70 |
| 8 | Confident | No LegacyFormula / migration guard reintroduced; a never-migrated box relies on brew's `formula_renames.json` | Precedent `260720-h3f6` retired it; Sahil verified the brew rename path | S:80 R:80 A:85 D:85 |
| 9 | Confident | Remove the stale README "Legacy `rk` keg exception" paragraph while relabeling | It documents machinery retired in `260720-h3f6`; touching the same section | S:70 R:90 A:85 D:80 |
| 10 | Certain | run-kit `updatecheck` `runKitTool` follow-up is out of scope, reported only | Operator scoped this to shll R1(c); cross-repo change | S:90 R:90 A:90 D:90 |

10 assumptions (4 certain, 6 confident, 0 tentative, 0 unresolved).

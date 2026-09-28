# Intake: HexoKit Repo Slug (run-kit → hexokit)

**Change**: 260928-ad37-hexokit-repo-slug
**Created**: 2026-09-28

## Origin

> HexoKit rebrand Phase 3, plan row **R2 step (b)** (run-kit repo, `fab/plans/sahil/26-09-12-hexokit-rebrand-remaining.md`): "shll roster `Repo` → `hexokit`, release".

One-shot dispatch from the operator. R2(a) is done: the GitHub repo `sahil87/run-kit` was renamed to `sahil87/hexokit` (GitHub redirects web, clone, releases, raw, and API URLs from the old name — verified working). R1(c) ([shll#103](https://github.com/sahil87/shll/pull/103), shll v0.1.34) already flipped the roster `Name`/`Formula`/`Update` to `hexokit` and deliberately left `Repo: "run-kit"` until this step ("Repo stays run-kit until R2").

Operator's scope instructions (verbatim intent): update the roster `Repo` field; grep for other `github.com/sahil87/run-kit` / `sahil87/run-kit` repo-path references (changelog fetch URLs, README links, docs) and update them to `sahil87/hexokit`. **Do not touch anything about the command name** (`rk` / `run-kit` / `hexokit` binaries, aliases, manifest keys) — that is already correct from R1. Only repo/URL path references.

Pre-intake investigation (this conversation) found every hit.

## Why

1. **Problem**: `shll list` renders `https://github.com/sahil87/run-kit` for the hexokit and rk-desktop rows, and `shll changelog` / `shll update`'s bump digest build `github.com/sahil87/run-kit/releases` / `.../compare/…` URLs and fetch releases from the old slug. All of these only work through GitHub's rename redirect.
2. **Consequence if not done**: every link and fetch depends on a redirect that dies if anyone ever recreates `sahil87/run-kit` (the plan's R2 rule: "never recreate `run-kit` or the redirect dies"); present-tense docs keep naming a repo that no longer exists by that name; R2(d) and X3 expect shll to be on the new slug.
3. **Approach**: flip the explicit `Repo` field on both entries that live in that repo; tests and docs follow. Rejected: deriving `Repo` from `Name` now that hexokit's match — rk-desktop still diverges (it has no repo of its own), so the explicit field and its dead-link guard stay.

## What Changes

All paths relative to repo root.

### 1. Roster (`src/cmd/shll/tools.go`)

- hexokit entry (line ~231): `Repo: "run-kit"` → `Repo: "hexokit"`. hexokit now has `Name == Repo`.
- rk-desktop entry (line ~232): `Repo: "run-kit"` → `Repo: "hexokit"` (rk-desktop ships in the hexokit repo; it has no repo of its own). rk-desktop is now the **only** `Name != Repo` entry.
- `Repo` field doc comment (lines ~80–85) currently says "hexokit's repo is still `run-kit` (the GitHub repo rename lands separately…), and rk-desktop lives in that same repo". Rewrite to present truth: it defaults to Name for most tools but is NOT always equal to Name — rk-desktop ships in the `hexokit` repo and has no repo of its own; the field stays explicit so `shll list` never emits a dead link when a tool's binary name and repo slug diverge.
- Do NOT change `LegacyNames`, `legacyAliases`, `rkBinary`, or any argv.

### 2. Tests (`src/...`)

- `src/cmd/shll/tools_test.go` ~79–80: rk-desktop Repo assertion `run-kit` → `hexokit` (message: "it ships in the hexokit repo"); ~326–327 hexokit Repo assertion → `hexokit`.
- `src/cmd/shll/list_test.go` ~139–140 (`TestList_RepoLinks`): hexokit row must resolve to `githubOrgBase+"hexokit"`. Currently the test also asserts the `.../hexokit` link is ABSENT (dead-link guard) — that assertion is now wrong and must be dropped/inverted; keep the dead `.../rk` and `.../rk-desktop` absence guards. Check the test body for the rk-desktop row expectation too.
- `src/cmd/shll/update_test.go` ~1601: `changelog.CompareURL("run-kit", …)` → `CompareURL("hexokit", …)`.
- `src/cmd/shll/changelog_test.go` ~436 (and any other `changelogServer` fixtures keyed by repo slug `"run-kit"`): key → `"hexokit"` (the fake server is keyed by repo slug, not tool name). Keep test args like `rk@0.1.0..0.2.0` (legacy alias input — command name).
- `src/internal/changelog/changelog_test.go` ~242: `ReleasesURL("run-kit")` → `ReleasesURL("hexokit")` with expected `https://github.com/sahil87/hexokit/releases`.
- Leave untouched: `versions_test.go` / `check_updates_test.go` manifest keys `"run-kit"` (manifest key = legacy tool name, not repo); `skill_test.go` / `version_test.go` `req.Name == "run-kit"` (binary name).
- Run `cd src && gofmt -l . && go vet ./... && go test ./...`.

### 3. Docs

- `README.md:190` — sample `shll list` row URL → `https://github.com/sahil87/hexokit`.
- `README.md:300` — `[hexokit](https://github.com/sahil87/run-kit)` → `.../hexokit`.
- `docs/site/standards/skill.md:19` — link target `https://github.com/sahil87/run-kit` → `.../hexokit`. Leave the `run-kit context` command text as-is (command-name reference, out of scope). Then regenerate embedded copies with `scripts/sync-standards.sh` (never hand-edit `src/cmd/shll/standards/*.md`).

### 4. Memory (present-truth rewrite, hydrate)

- `docs/memory/cli/list.md`: sample rows (23–24) and `--json` sample (53, 59) → `https://github.com/sahil87/hexokit`; line ~66 prose; the "## The run-kit repo-slug footgun" section (~109–136) — rewrite to present truth: hexokit's `Repo` now equals its `Name`; the live divergence is rk-desktop (`Repo: "hexokit"`, a `Name`-derived URL would be the dead `.../rk-desktop` link); `.../rk` is also dead. Rename the heading (e.g. "The repo-slug footgun") and update in-file anchors (`#the-run-kit-repo-slug-footgun`) and cross-file anchors in `cli/commands.md` / `cli/changelog.md`. Update the `TestList_RepoLinks` description (~115, ~162).
- `docs/memory/cli/commands.md`: roster code block (85–86) `Repo: "hexokit"`; line ~105 (`Tool.Repo` bullet: hexokit is no longer the exception; rk-desktop is), ~117 (hexokit entry: `Repo: "hexokit"`, Name == Repo).
- `docs/memory/cli/changelog.md`: line ~27 (hexokit's `Repo` slug is `hexokit`, so the fetch/compare URL is `github.com/sahil87/hexokit`), line ~98 (drop "`Repo: "run-kit"` diverges"; point at the renamed anchor).

### Out of scope

- Command-name references: `rk`, `run-kit` legacy aliases/binaries, `LegacyNames`, `legacyAliases`, the `versions.json` manifest `run-kit` key, `run-kit context` command text.
- Prose "the run-kit repo" in other memory files (e.g. `cli/update.md`, `cli/install.md`, `cli/check-updates.md`) — naming prose, not repo-path/URL references; X3's memory identity sweep (run-kit repo) owns identity prose.
- `fab/changes/**` (history, D11), `docs/memory/operator/bulk-op-commands.md` roster (`run-kit` there is a `hop` local repo name).

## Affected Memory

- `cli/list`: (modify) repo column URLs → `sahil87/hexokit`; repo-slug footgun section rewritten (rk-desktop is the live divergence)
- `cli/commands`: (modify) roster `Repo: "hexokit"` for hexokit + rk-desktop; `Tool.Repo` bullet
- `cli/changelog`: (modify) hexokit changelog/compare URLs use `sahil87/hexokit`

## Impact

- `src/cmd/shll/tools.go` (2 roster fields + doc comment), 5 test files, README, one standards doc + its embedded copy, 3 memory files.
- User-visible: `shll list` repo column, `shll changelog hexokit` URLs and the release fetch, `shll update` digest compare URLs now point at `github.com/sahil87/hexokit` directly (no redirect hop). No command surface changes.
- Release: cut a shll patch release after merge (`just release`).

## Open Questions

(none)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | hexokit `Repo` → `"hexokit"` | Plan row R2(b) and operator instruction, verbatim | S:95 R:90 A:95 D:95 |
| 2 | Certain | rk-desktop `Repo` → `"hexokit"` too | It ships in the same repo (documented in the Repo doc comment); leaving it on `run-kit` would keep the redirect dependency | S:85 R:90 A:90 D:90 |
| 3 | Certain | Command-name refs (`LegacyNames`, aliases, manifest `run-kit` key, binary probes) untouched | Operator instruction; R1 already settled them | S:95 R:90 A:95 D:95 |
| 4 | Confident | Keep `Repo` explicit (not derived from Name) | rk-desktop still diverges; the dead-link guard remains load-bearing | S:80 R:85 A:85 D:85 |
| 5 | Confident | Leave "the run-kit repo" naming prose in other memory files | Operator scoped to repo-path/URL refs; identity prose sweep is X3 | S:70 R:85 A:75 D:70 |
| 6 | Confident | `run-kit context` text in standards/skill.md stays; only the link target changes | Command-name reference, out of scope | S:80 R:90 A:80 D:80 |

6 assumptions (3 certain, 3 confident, 0 tentative, 0 unresolved).

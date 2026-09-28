# Plan: HexoKit Repo Slug (run-kit → hexokit)

**Change**: 260928-ad37-hexokit-repo-slug
**Intake**: `intake.md`

## Requirements

### Roster: Repo slug

#### R1: hexokit and rk-desktop resolve to the hexokit repo
The Roster entries `hexokit` and `rk-desktop` SHALL carry `Repo: "hexokit"`, so every URL built from `Tool.Repo` (`shll list`'s repo column via `repoURL`, `shll changelog`'s release fetch and compare/releases URLs, `shll update`'s digest compare-URL fallback) targets `https://github.com/sahil87/hexokit`. `Repo` SHALL remain an explicit field: rk-desktop is the only `Name != Repo` entry. The `Repo` doc comment SHALL state present truth (no "still `run-kit`" / "until R2" narration). `LegacyNames`, `legacyAliases`, `rkBinary`, argvs, and manifest-key lookups SHALL be unchanged.

- **GIVEN** the roster
- **WHEN** `shll list` renders
- **THEN** the hexokit and rk-desktop rows show `https://github.com/sahil87/hexokit`
- **AND** no row shows `https://github.com/sahil87/run-kit`, `.../rk`, or `.../rk-desktop`

- **GIVEN** `shll changelog rk@0.1.0..0.2.0` (legacy alias input)
- **WHEN** it resolves to hexokit
- **THEN** the release fetch requests `/repos/sahil87/hexokit/releases`

#### R2: Tests pin the new slug
Tests asserting the repo slug SHALL expect `hexokit`: `tools_test.go` (hexokit + rk-desktop `Repo`), `list_test.go` `TestList_RepoLinks` (hexokit row → `.../hexokit`; the dead `.../run-kit`, `.../rk`, `.../rk-desktop` links absent), `update_test.go` (`CompareURL("hexokit", …)`), `changelog_test.go` fixtures keyed by repo slug, `internal/changelog` `ReleasesURL` test. Command-name tests (`req.Name == "run-kit"` probes, manifest `"run-kit"` keys) SHALL be unchanged.

- **GIVEN** the updated roster
- **WHEN** `cd src && go test ./...` runs
- **THEN** all tests pass

### Docs: repo URLs

#### R3: Present-tense repo URLs name sahil87/hexokit
`README.md` and `docs/site/standards/skill.md` SHALL link `https://github.com/sahil87/hexokit` where they linked `https://github.com/sahil87/run-kit`; the embedded `src/cmd/shll/standards/skill.md` SHALL be regenerated via `scripts/sync-standards.sh`. The `run-kit context` command text SHALL be unchanged.

- **GIVEN** the repo after the change
- **WHEN** grepping non-`fab/` files for `sahil87/run-kit`
- **THEN** only memory files hydrated in R4's scope remain before hydrate, and none after hydrate

#### R4: Memory reflects the new slug (hydrate)
`docs/memory/cli/list.md`, `cli/commands.md`, `cli/changelog.md` SHALL state `Repo: "hexokit"` for hexokit and rk-desktop, show `https://github.com/sahil87/hexokit` in sample output, and describe rk-desktop as the live Name/Repo divergence; the "run-kit repo-slug footgun" heading and its anchors SHALL be renamed consistently.

- **GIVEN** hydrate has run
- **WHEN** reading `cli/list.md` § the repo-slug footgun
- **THEN** it says hexokit's Name == Repo and rk-desktop is the divergence guarded by `TestList_RepoLinks`

### Non-Goals

- Command-name references (`rk`, `run-kit` aliases/binaries, `LegacyNames`, manifest `run-kit` key) — settled by R1(c)
- "the run-kit repo" naming prose elsewhere in memory — X3's identity sweep
- `fab/changes/**` history (D11)

## Tasks

### Phase 1: Core Implementation

- [x] T001 `src/cmd/shll/tools.go`: `Repo: "hexokit"` on hexokit and rk-desktop; rewrite the `Repo` field doc comment to present truth <!-- R1 -->
- [x] T002 Tests: `src/cmd/shll/tools_test.go`, `list_test.go` (`TestList_RepoLinks` guard: hexokit → `.../hexokit`; `.../run-kit`, `.../rk`, `.../rk-desktop` absent), `update_test.go`, `changelog_test.go` (repo-slug-keyed fixtures), `src/internal/changelog/changelog_test.go`; run `cd src && gofmt -l . && go vet ./... && go test ./...` <!-- R1 R2 -->
- [x] T003 [P] `README.md` lines ~190, ~300 and `docs/site/standards/skill.md` link → `sahil87/hexokit`; run `scripts/sync-standards.sh` <!-- R3 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: hexokit and rk-desktop Roster entries carry `Repo: "hexokit"`; doc comment has no "still run-kit"/R2 narration
- [x] A-002 R2: repo-slug tests expect `hexokit`; `go test ./...`, `go vet`, `gofmt -l` clean
- [x] A-003 R3: README + standards source and embedded copy link `sahil87/hexokit`; embedded copy byte-matches source
- [x] **N/A**: A-004 R4: memory files reflect `Repo: "hexokit"` with consistent renamed anchors — hydrate-owned; memory updates are intentionally deferred to the hydrate stage and are not a review failure

### Behavioral Correctness

- [x] A-005 R1: `shll changelog` with legacy alias `rk` fetches `/repos/sahil87/hexokit/releases` (fixture keyed `hexokit`)

### Edge Cases & Error Handling

- [x] A-006 R1: `LegacyNames`, `legacyAliases`, manifest-key fallback, and binary probes are unchanged (no command-name edits)

### Code Quality

- [x] A-007 Pattern consistency: edits follow surrounding comment/test style
- [x] A-008 No unnecessary duplication: URL composition stays single-sourced via `githubOrgBase + Repo`

## Notes

- Check items as you review: `- [x]`

## Deletion Candidates

- None — pure repo-slug rename; no existing symbol, branch, or config is made redundant or unused.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `TestList_RepoLinks` inverts its `.../hexokit`-absent guard into a `.../run-kit`-absent guard and adds `.../rk-desktop` absent | The old absent-guard asserted the pre-rename state; the dead-link class now covers the old slug too | S:85 R:90 A:90 D:85 |
| 2 | Confident | Memory edits land at hydrate, not apply | Pipeline convention: hydrate owns `docs/memory/` | S:80 R:90 A:85 D:80 |

2 assumptions (1 certain, 1 confident, 0 tentative).

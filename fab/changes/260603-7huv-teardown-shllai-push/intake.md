# Intake: Teardown shll.ai push wiring (shll.ai now pulls)

**Change**: 260603-7huv-teardown-shllai-push
**Created**: 2026-06-03
**Status**: Draft

## Origin

<!-- How was this change initiated? -->

> /fab-new — "There's an update in the way we integrate with shll.ai. To understand it read
> https://github.com/sahil87/shll.ai/blob/main/docs/specs/help-dump-contract.md#teardown-directive-paste-to-a-tool-repo-agent .
> Implement the change."

**Interaction mode**: One-shot, with one clarifying question asked (the `captured_at` scope decision).

**Source spec** (read verbatim): `sahil87/shll.ai` → `docs/specs/help-dump-contract.md`, §Pull model and the §"Teardown directive (paste to a tool-repo agent)" block. The spec is dated **2026-06-03** (shll.ai change `oa63`).

**The inversion**: shll.ai used to receive each tool's command-reference JSON via a **push** — this repo's CI walked the Cobra tree, wrote `help/shll.json`, and opened an auto-merged PR into `sahil87/shll.ai` using `SHLLAI_TOKEN`. As of 2026-06-03 shll.ai **pulls** instead: its scheduled job (`.github/workflows/scheduled-help-refresh.yml`) `brew install`s the tool, runs `shll help-dump`, and commits the captured JSON itself. This repo no longer pushes anything, so the push wiring is dead code.

**Key decision reached during this conversation**: The teardown directive's `help-dump` invariant explicitly specifies the tool-emitted envelope as `{tool, version, schema_version, root}` and states "**do not emit `captured_at`** (shll.ai stamps it)". Our current `help_dump.go` *does* emit `captured_at`. **User confirmed**: drop `captured_at` from `help-dump` too, so the envelope conforms to the directive and to §3 of the contract ("the `help-dump` output MUST NOT include it"). This is an intentional, contract-mandated exception to the directive's general "do not touch help-dump" wording — the directive's own envelope spec requires it, and `captured_at`'s only purpose (the date-granular value powering the CI no-op guard) dies with the push CI being removed.

## Why

1. **Problem**: The push model is retired upstream. The producer CI + cross-repo PR + auto-merge + `SHLLAI_TOKEN` in `release.yml` now run on every release for no consumer — at best wasted work, at worst a failing release step (the shll.ai-side prerequisites the push relied on — auto-merge enabled, `SHLLAI_TOKEN` carrying `pull-requests:write` — may be revoked once shll.ai stops accepting pushes, and `gh pr merge --auto` is designed to *fail loudly*). Separately, our `help-dump` emits `captured_at`, which the now-clarified contract says it MUST NOT.

2. **Consequence if not fixed**: Every shll release keeps attempting a dead cross-repo push; once shll.ai-side prerequisites are removed the publish step fails, breaking the release pipeline for an integration that no longer exists. The stale `captured_at` field also leaves `help-dump` non-conformant with the pull contract shll.ai now validates against (`schemas.ts` owns `captured_at`).

3. **Why this approach**: We follow the spec's paste-ready teardown directive exactly — remove only the *transport* (the three help-push steps + the publish step in `release.yml`, and `SHLLAI_TOKEN` usage), preserve the `help-dump` *command* that produces the data, and align the envelope with the directive by dropping `captured_at`. The precondition ("puller live and proven") is asserted by the spec itself (dated 2026-06-03, puller is `oa63` and live), so teardown is safe now — it does not create a stale-help gap.

## What Changes

### 1. `.github/workflows/release.yml` — remove the help-push transport (keep the rest)

`release.yml` does four things today: cross-compile, GitHub Release, **help-dump → shll.ai publish (4 steps)**, Homebrew-tap update. Only the help-push transport is removed; cross-compile, GitHub Release, and Homebrew-tap update are untouched. The `release` job remains; the file is **not** deleted (per directive item 5: it still has purpose).

Remove these steps (currently lines ~110–160):

- **"Build native binary for help-dump"** — the dedicated `/tmp/shll-native` build that existed *solely* to run the dump on the runner. (Producer CI — directive item 1.)
- **"Generate help/shll.json"** — `mkdir -p help`, run dump > file, `jq empty`, version==tag assertion. (Producer CI — directive item 1.)
- **"Publish to shll.ai"** — the entire step: clone shll.ai with `SHLLAI_TOKEN`, copy, per-release branch, no-op guard, force-push, `gh pr create`, `gh pr merge --auto --squash`. This is directive items 2 (PR-opening), 3 (auto-merge), and 4 (`SHLLAI_TOKEN` usage) in one step.

`permissions: contents: write` at the workflow level stays (it covers the GitHub Release on `sahil87/shll`, not the cross-repo write). `HOMEBREW_TAP_TOKEN` and the tap step stay.

### 2. `src/cmd/shll/help_dump.go` — drop `captured_at` from the emitted envelope

Align the tool-emitted shape with the directive (`{tool, version, schema_version, root}`) and §3 ("output MUST NOT include `captured_at`"):

- Remove the `CapturedAt string \`json:"captured_at"\`` field from `helpDoc`.
- Remove its assignment `CapturedAt: capturedAt()` in `runHelpDump`.
- Remove the `capturedAt()` function and the `capturedAtLayout` constant (no longer referenced).
- Remove the now-unused `"time"` import.

Resulting envelope:
```go
type helpDoc struct {
	Tool          string   `json:"tool"`
	Version       string   `json:"version"`
	SchemaVersion int      `json:"schema_version"`
	Root          helpNode `json:"root"`
}
```
Everything else in `help_dump.go` (the tree walk, prune-before-render, `nodeText` byte-for-byte, filter rules, `Hidden: true` self-exclusion, version-from-binary) is preserved exactly — those are the contract-faithful core the directive says to keep working.

### 3. `src/cmd/shll/help_dump_test.go` — update tests for the new envelope

- Remove `TestHelpDump_CapturedAtShape` (it asserts `captured_at` matches `^\d{4}-\d{2}-\d{2}T00:00:00Z$`; the field no longer exists).
- Update `TestHelpDump_StructuralDeterminism` if it references `captured_at` ("two same-day dumps differ only in `captured_at`" → now they are simply byte-identical).
- Update `TestHelpDump_ContractShape` if it asserts presence of a `captured_at` key (it must now assert *absence*, or at least not require it).
- Keep all other tests (contract shape, text byte-for-byte, self-exclusion, version passthrough, Execute-path regression). The directive explicitly says keep the `help-dump` test so the contract surface stays protected now that push CI no longer exercises it.

### 4. Documentation / memory updates

The push model is now gone, so docs that describe it as live must be corrected:

- `docs/memory/ci/release-workflow.md` — remove/rewrite the "help-dump → shll.ai publishing" section and the two push-related Design Decision callouts; correct step order and the summary line (no more shll.ai PR; `SHLLAI_TOKEN` gone). State shll.ai now pulls via `help-dump`.
- `docs/memory/cli/help-dump-contract.md` — correct the envelope (drop `captured_at`), the `captured_at` row, the `capturedAt`/`capturedAtLayout` references, the test list (8 → 7 tests; remove `captured_at` shape test), and the cross-reference to the (now-removed) CI publishing flow. Reframe: `help-dump` emits to stdout for shll.ai's puller; `captured_at` is shll.ai-owned.
- `docs/memory/ci/index.md` — update the one-line release-workflow summary (drop the help-dump → shll.ai auto-merge PR / `SHLLAI_TOKEN` mention).
- `fab/backlog.md` — `ep4z` (the original push item) is now superseded by this teardown; note it / leave for `/fab-archive` to reconcile (handled at hydrate, not apply).

### 5. Post-merge manual follow-up (out of code scope — flagged, not done by this change)

- **Delete the `SHLLAI_TOKEN` repo secret** on `sahil87/shll` — only after this PR confirms no other repo reference exists (grep already done: only the push wiring + docs use it). Deleting a GitHub secret is a repo-settings action, not a code change; it is flagged in the PR description for the maintainer to do post-merge.

## Affected Memory

- `ci/release-workflow`: (modify) Remove the help-dump → shll.ai push section + push Design Decisions; correct step order, summary, and `SHLLAI_TOKEN` mention.
- `cli/help-dump-contract`: (modify) Drop `captured_at` from envelope + field table; remove `capturedAt`/`capturedAtLayout`/`captured_at`-test references; reframe `captured_at` as shll.ai-owned and stamped by the puller; correct test count and CI cross-reference.
- `ci/index`: (modify) Update the release-workflow one-line summary.

## Impact

- **Code**: `.github/workflows/release.yml` (remove 3–4 steps), `src/cmd/shll/help_dump.go` (drop `captured_at`), `src/cmd/shll/help_dump_test.go` (drop/adjust `captured_at` tests).
- **Docs**: `docs/memory/ci/release-workflow.md`, `docs/memory/cli/help-dump-contract.md`, `docs/memory/ci/index.md`, `fab/backlog.md`.
- **Secrets / external**: `SHLLAI_TOKEN` repo secret — flag for manual deletion post-merge (not a code change). No change to `HOMEBREW_TAP_TOKEN`.
- **Dependencies**: none added/removed (drops the `time` import in one Go file).
- **Constitution**: VI (Thin Justfile / build pattern) — release still cuts via `v*` tag, cross-compiles, GitHub Release, Homebrew tap; only the help-push transport leaves. No principle is violated. I (Security First) — N/A (the removed shell-out lived in YAML).

## Open Questions

<!-- None blocking. The one material scope decision (captured_at) was asked and resolved. -->

- None. The `captured_at` scope question was asked and resolved (drop it). The teardown precondition (puller live + proven) is asserted by the spec's own dating.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Remove only the help-push transport from `release.yml` (Build native binary for help-dump, Generate help/shll.json, Publish to shll.ai); keep cross-compile, GitHub Release, Homebrew-tap update; do NOT delete the file. | Directive items 1–3 + item 5 ("if the help-push was one job inside a larger workflow, remove just that job and leave the rest intact"). The file retains clear purpose. | S:98 R:80 A:96 D:95 |
| 2 | Certain | Remove `SHLLAI_TOKEN` usage from the workflow; flag the repo-secret deletion as a post-merge manual step, only after confirming no other usage. | Directive item 4. Repo-wide grep confirmed `SHLLAI_TOKEN` appears only in the push wiring and docs about it. Deleting a GitHub secret is a settings action, not code. | S:95 R:75 A:92 D:90 |
| 3 | Certain | Drop `captured_at` from the `help-dump` envelope (field, assignment, `capturedAt()`, `capturedAtLayout`, `time` import) so it emits `{tool, version, schema_version, root}`. | Asked and user-confirmed. Directive's invariant explicitly says "do not emit `captured_at`"; §3 says output MUST NOT include it; its only purpose (no-op-guard date value) dies with the push CI. | S:96 R:70 A:95 D:92 |
| 4 | Certain | Preserve the `help-dump` command's behavior otherwise (Hidden self-exclusion, programmatic tree walk, prune-before-render, byte-for-byte text, version-from-binary) and keep its test (minus the `captured_at` test). | Directive: "do NOT touch the `help-dump` command... keep it working exactly as-is... keep [the] test." Only the transport + the contract-mandated `captured_at` removal change. | S:98 R:85 A:97 D:96 |
| 5 | Certain | Update memory docs (`ci/release-workflow`, `cli/help-dump-contract`, `ci/index`) to reflect pull model + dropped `captured_at`; reconcile backlog `ep4z` at hydrate/archive, not apply. | These docs currently describe the push model as live and `captured_at` as emitted — both now false. fab hydrate stage is the canonical place for memory updates. | S:90 R:85 A:90 D:88 |
| 6 | Confident | Teardown is safe to execute now (precondition "puller live and proven" is satisfied). | The spec is dated 2026-06-03 and states the puller (`oa63`) is live; it provides the directive as actionable now. Can't independently verify shll.ai runtime, but the authoritative spec asserts it. | S:80 R:55 A:75 D:85 |
| 7 | Confident | Verification gate before PR: `shll help-dump` exits 0 with valid JSON to stdout; `go build`/`go test ./...` pass; no active workflow references `SHLLAI_TOKEN`/shll.ai push paths; build/test/release-artifact paths unaffected. | Directive's "Verify before opening the PR" list, plus standard Go build/test. Mechanical to check. | S:92 R:80 A:90 D:88 |

7 assumptions (6 certain, 1 confident, 0 tentative, 0 unresolved).

---
type: memory
description: "shll's audited conformance state against the 4 toolkit standards (principles, help-dump, readme-extraction, skill), audited on HEAD against shll v0.0.23: per-standard audit method, the two fixes (usage-error exit 2, README badge run), the by-design non-gaps (version-without-`--json`, README footer-heading absence), and the conformance-report-in-PR-body convention. The `skill` standard was DEFERRED at audit time ([agst]) and is now ADOPTED by change agst — `shll skill shll` serves the drift-guarded `docs/site/skill.md` bundle."
---
# cli/standards-conformance

shll's own conformance to the sahil87 toolkit's binding, producer-facing standards — the constitution-mandated work that verifies the publisher of the standards itself obeys them. This file records **which standards shll meets on HEAD**, the per-standard audit method, the two gaps this change fixed, the determinations that are conformant by design, and where the one deferred standard is tracked.

> The standards **command** that serves the documents (`shll standards`) is [cli/standards](/cli/standards.md); the standards **documents** themselves (the `docs/site/standards/` restructure + the `skill` contract) are [cli/standards-content](/cli/standards-content.md). This file is the **conformance receipt** — how shll measures up against those documents.

## Why this exists (constitution mandate)

The constitution's Toolkit Standards article (§ Additional Constraints, amended in change `qas0`) binds shll to conform to the toolkit's published standards, canonically authored in this repo's `docs/site/standards/` and served by `shll standards`. shll both **publishes** the standards and **must itself demonstrate** conformance — if the publishing tool does not visibly obey its own standards, the standards read as aspirational rather than binding, undermining the cross-repo rollout (`[std1]`/`[std2]` waves across hop, wt, tu, idea, run-kit, fab-kit). This conformance state is shll's own reference example — the six sibling repos' conformance changes mirror it.

*Introduced by*: change `3sss` (`260717-3sss-toolkit-standards-conformance`), the wave-2 conformance directive (backlog `[std2]`) applied to shll itself.

## Audit method

The audit is **runtime-enumerated, not assumed**: `shll standards` lists the standards, and `shll standards <name>` prints each one; that list is authoritative. As of the audit, the enumeration is **4 standards**, audited against **shll v0.0.23** (`shll version`'s shll row — standards are versioned with the shll release).

- **Standards *text* comes from the installed binary (v0.0.23)**, which is byte-matched to this repo's `docs/site/standards/` by the `TestStandardsEmbedMatchesCanonical` drift guard (see [cli/standards §the drift guard](/cli/standards.md#the-drift-guard-teststandardsembedmatchescanonical)).
- **Behavioral checks run against a dev build from repo HEAD** (`cd src && go build -o /tmp/shll-audit ./cmd/shll`), not the installed v0.0.23 — the repo, not the shipped binary, is what a conformance change can fix.

| Standard | Scope | Audit method | Result |
|----------|-------|--------------|--------|
| `principles` | foundation | Each of the 10 principles assessed against actual subcommand behavior across the whole surface (`update`, `install`, `list`, `doctor`, `version`, `changelog`, `shell-init`, `shell-setup`, `standards`, `uninstall`) | 9 PASS, 1 gap (№4 usage-exit) — **fixed here** |
| `help-dump` | binary | The standard's own "Verifying conformance" checklist, executed verbatim | PASS |
| `readme-extraction` | repo | The standard's own "Verifying conformance" checklist, executed verbatim | 1 gap (badge run) — **fixed here** |
| `skill` | binary+repo | Deferred at audit time (tracked in `[agst]`); **ADOPTED by change agst** — `shll skill shll` serves the drift-guarded `docs/site/skill.md` bundle | deferred → **adopted (agst)** |

## The two fixes

Both gaps were small and additive (a rerouted exit code, a doc edit) — inside the change's "fix what is proportionate here" scope. Neither restructures the tool or breaks an existing output contract.

### 1. Usage errors exit 2 (principle №4)

The toolkit convention is `0` success / `1` operational failure / `2` usage error. Before this change, cobra-level usage errors (unknown command, unknown flag/shorthand, bad arg count, invalid argument) exited **1**, inconsistent with `shll shell-init`/`shll shell-setup`'s own bad-invocation paths which already used exit 2 via `errExitCode{code: 2}`. Fixed so all usage errors exit 2 while operational failures stay 1. The mechanism is fully documented in [cli/commands §exit-code translation](/cli/commands.md#exit-code-translation) — in brief:

- `root.go`'s `SetFlagErrorFunc` wraps flag-parse errors in `errExitCode{code: usageExitCode}` (the clean cobra hook for flag errors; inherited by every subcommand).
- `main.go`'s `translateExit` classifies cobra's *arg/command* usage errors (which carry no typed sentinel in cobra v1.10.2) by their stable message prefixes (`isUsageError` / `cobraUsageErrorPrefixes`) → exit 2, and honors the new `usageExitCode = 2` named constant.
- Operational failures (unknown standard name, a failed brew call, a `doctor` FAIL) still return `errSilent` or a plain error → exit 1, unchanged.

Proven by `src/cmd/shll/main_test.go` (`TestTranslateExit_Contract`, `TestTranslateExit_WrappedErrExitCode`, `TestRootCmd_FlagErrorIsUsageExit`). Classification is **prefix-anchored** (`strings.HasPrefix`), so an operational error whose message merely *contains* a usage word mid-string is not misclassified — pinned by the mid-message test case; a repo grep found no operational message starting with any classified prefix (`tools.go`'s `unknown target …` does not match the `unknown command ` prefix).

### 2. README badge run (readme-extraction head structure)

The readme-extraction standard requires the head order `#` H1 → canonical toolkit blockquote → **badges** → prose. shll's README was missing the contiguous badge run. Fixed by inserting the byte-identical 3-badge run all six sibling repos carry (Latest release / Downloads / Stars, pointed at `sahil87/shll`) immediately after the blockquote in `README.md`.

## Conformant by design (non-gaps)

Two audit items were determined **conformant as-is** — recorded so a future audit does not re-open them as gaps:

- **`shll version` has no `--json`, and that is not a principle №2 gap.** `version` is deliberately frozen as human-paste-for-bug-reports output (version.go's own help states it pastes cleanly into bug reports). The **programmatic** version surface is covered by `shll doctor --json` (per-tool `version` field) and `shll list --json` — see [cli/version](/cli/version.md) and [cli/doctor §`--json`](/cli/doctor.md#--json-output-mode). This is a deliberate design boundary, not a missing feature.
- **The README has no footer heading (`Contributing`/`Development`/`Building`/`License`/`Acknowledgements`), and that is conformant.** Footer headings are pull-*stop* markers in the readme-extraction standard (§2), not required content. The entire README is site-worthy and nothing maintainer-only leaks, so the whole file is the pulled site slice — no footer split is needed.

## The `skill` standard: deferred at audit, ADOPTED by change agst

At audit time shll had no `shll skill` subcommand. The `skill` standard's own Adoption section states rollout is phased per-repo and **no tool shipped `skill` then** — a tool without one is "not yet in violation" (principle №10's bundle obligation is a SHOULD). So the audit reported `skill` as **"deferred, not yet adopted"**, tracked in backlog **`[agst]`**, not an in-scope fix.

**Change `agst` resolved that deferral** — it is `[agst]`'s home, both for the `shll skill` composer and for shll's *own* `skill`-standard adoption:

- shll now ships `shll skill` (the runtime tree lists twelve commands including `skill` and `agent-setup`).
- shll's own bundle is authored at `docs/site/skill.md` (≤150 lines), embedded via the same sync + drift-guard mechanism `shll standards` uses (committed `src/cmd/shll/skill/skill.md` + the extended `scripts/sync-standards.sh` + `TestSkillEmbedMatchesCanonical`), and served by `shll skill shll` in-process byte-identical.
- This satisfies principle №10's bundle obligation at scope `binary+repo` for shll. See [cli/skill](/cli/skill.md) for the composer and the self-bundle embed, and [cli/standards-content §landed design](/cli/standards-content.md#landed-design-shll-agent-setup-skills-placement-not-context-aggregation) for the `shll agent-setup` piece that also landed.

The other six tools' `<tool> skill` bundles remain the per-repo standards waves' work (out of scope for `agst`, which is shll-only).

## Where deferred gaps are tracked

Larger gaps (new subsystems, breaking output-contract changes) defer to `fab/backlog.md` items with fresh 4-char IDs (this repo's deferral convention — precedent `[38a6]`/`[tkch]`/`[agst]`), referenced by ID in the report. This audit created no new deferrals — the only deferred standard (`skill`) was already tracked in `[agst]`, both other gaps were additive and fixed at audit time, and `[agst]` itself has since been resolved (change agst, above).

## The conformance-report-in-PR-body convention

The deliverable is a single fab change whose **PR body carries a per-standard conformance report** (one section per standard: PASS / fixed-here-with-commit / deferred-with-ref), citing the audited shll version. The report is written during apply to `conformance-report.md` in the change folder (the durable hand-off between the separate apply and ship stages) and carried verbatim into the PR body by `/git-pr` at ship — the commit sha for each fix is stamped in at that point. This report-in-PR-body pattern is the template the six sibling repos' `[std2]` conformance changes follow.

## Cross-references

- The command that serves the audited standards (roster, embed, drift guard): [cli/standards](/cli/standards.md).
- The command that ADOPTS the `skill` standard (the composer + shll's own embedded bundle): [cli/skill](/cli/skill.md).
- The standards *documents* and the `skill` contract that defined the (now-resolved) deferred obligation: [cli/standards-content](/cli/standards-content.md).
- The exit-code fix's full mechanism (the `translateExit` classification, `SetFlagErrorFunc`, `usageExitCode`): [cli/commands §exit-code translation](/cli/commands.md#exit-code-translation).
- Constitution → Toolkit Standards (the article this conformance work satisfies) and Principle IV (the standards govern all seven tools, so no single tool owns them — shll is their home).

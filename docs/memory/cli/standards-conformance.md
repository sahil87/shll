---
type: memory
description: "shll's conformance state against the 9 toolkit standards: four audited on HEAD against shll v0.0.23 (principles, help-dump, readme-extraction, skill) with two fixes and by-design non-gaps; version pinned by a conformance test; shell-init/update/install-composition conformant by construction or N/A; config-home N/A (shll has no config file); the rk-desktop roster pass opened no new deferrals. Includes the conformance-report-in-PR-body convention."
---
# cli/standards-conformance

shll's own conformance to the shll toolkit's binding, producer-facing standards — the constitution-mandated work that verifies the publisher of the standards itself obeys them. This file records **which standards shll meets on HEAD**, the per-standard audit method, the two gaps this change fixed, the determinations that are conformant by design, and where the one deferred standard is tracked.

> The standards **command** that serves the documents (`shll standards`) is [cli/standards](/cli/standards.md); the standards **documents** themselves (the `docs/site/standards/` restructure + the `skill` contract) are [cli/standards-content](/cli/standards-content.md). This file is the **conformance receipt** — how shll measures up against those documents.

## Why this exists (constitution mandate)

The constitution's Toolkit Standards article (§ Additional Constraints — qas0) binds shll to conform to the toolkit's published standards, canonically authored in this repo's `docs/site/standards/` and served by `shll standards`. shll both **publishes** the standards and **must itself demonstrate** conformance — if the publishing tool does not visibly obey its own standards, the standards read as aspirational rather than binding, undermining the cross-repo rollout (`[std1]`/`[std2]` waves across hop, wt, tu, idea, run-kit, fab-kit). This conformance state is shll's own reference example — the six sibling repos' conformance changes mirror it.

*Introduced by*: `260717-3sss-toolkit-standards-conformance`, the wave-2 conformance directive (backlog `[std2]`) applied to shll itself.

## Audit method

The audit is **runtime-enumerated, not assumed**: `shll standards` lists the standards, and `shll standards <name>` prints each one; that list is authoritative. `shll standards` enumerates **nine** standards. The **four** documentation/help standards (`principles`, `help-dump`, `readme-extraction`, `skill`) carry a behavioral audit against **shll v0.0.23** (`shll version`'s shll row — standards are versioned with the shll release); their audit table is below. The three producer-surface standards (`update`, `version`, `shell-init`) codify shll's own already-shipped probe behavior, so shll's posture against them is a **by-construction / not-applicable** determination rather than a behavioral audit — see [The three producer-surface standards](#the-three-producer-surface-standards-updateversionshell-init) below. The eighth standard, `install-composition`, is likewise a **by-construction** determination (shll composes the toolkit and probes/degrades by its own constitution) — see [The `install-composition` standard](#the-install-composition-standard-conformant-by-construction) below. The ninth, `config-home`, is **N/A for shll** — shll has no config file — see [The `config-home` standard](#the-config-home-standard-na--shll-has-no-config-file) below.

- **Standards *text* comes from the installed binary (v0.0.23)**, which is byte-matched to this repo's `docs/site/standards/` by the `TestStandardsEmbedMatchesCanonical` drift guard (see [cli/standards §the drift guard](/cli/standards.md#the-drift-guard-teststandardsembedmatchescanonical)).
- **Behavioral checks run against a dev build from repo HEAD** (`cd src && go build -o /tmp/shll-audit ./cmd/shll`), not the installed v0.0.23 — the repo, not the shipped binary, is what a conformance change can fix.

| Standard | Scope | Audit method | Result |
|----------|-------|--------------|--------|
| `principles` | foundation | Each of the 10 principles assessed against actual subcommand behavior across the whole surface (`update`, `install`, `list`, `doctor`, `version`, `changelog`, `shell-init`, `shell-setup`, `standards`, `uninstall`) | 9 PASS, 1 gap (№4 usage-exit) — **fixed here** |
| `help-dump` | binary | The standard's own "Verifying conformance" checklist, executed verbatim | PASS |
| `readme-extraction` | repo | The standard's own "Verifying conformance" checklist, executed verbatim | 1 gap (badge run) — **fixed here** |
| `skill` | binary+repo | Adopted — `shll skill shll` serves the drift-guarded `docs/site/skill.md` bundle (agst; deferred at audit time, tracked in `[agst]`) | **adopted** |

## The two fixes

Both gaps were small and additive (a rerouted exit code, a doc edit) — inside the change's "fix what is proportionate here" scope. Neither restructures the tool or breaks an existing output contract.

### 1. Usage errors exit 2 (principle №4)

The toolkit convention is `0` success / `1` operational failure / `2` usage error. The audit found cobra-level usage errors (unknown command, unknown flag/shorthand, bad arg count, invalid argument) exiting **1**, inconsistent with `shll shell-init`/`shll shell-setup`'s own exit-2 bad-invocation paths; the fix routes all usage errors to exit 2 while operational failures stay 1 (3sss). The mechanism is fully documented in [cli/commands §exit-code translation](/cli/commands.md#exit-code-translation) — in brief:

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

## The `skill` standard: adopted

The audit deferred `skill` (no `shll skill` subcommand existed then; the standard's phased per-repo adoption makes a tool without one "not yet in violation" — principle №10's bundle obligation is a SHOULD; tracked in backlog `[agst]`). The deferral is resolved (agst) — both the `shll skill` composer and shll's *own* `skill`-standard adoption:

- shll ships `shll skill` (the runtime tree's visible surface includes `skill` and the `setup` family).
- shll's own bundle is authored at `docs/site/skill.md` (≤150 lines), embedded via the same sync + drift-guard mechanism `shll standards` uses (committed `src/cmd/shll/skill/skill.md` + the extended `scripts/sync-standards.sh` + `TestSkillEmbedMatchesCanonical`), and served by `shll skill shll` in-process byte-identical.
- This satisfies principle №10's bundle obligation at scope `binary+repo` for shll. See [cli/skill](/cli/skill.md) for the composer and the self-bundle embed, and [cli/standards-content §landed design](/cli/standards-content.md#landed-design-shll-setup-agent-skills-placement-not-context-aggregation) for the `shll setup agent` piece that also landed.

The other six tools' `<tool> skill` bundles remain the per-repo standards waves' work (out of scope for `agst`, which is shll-only).

## The three producer-surface standards (`update`/`version`/`shell-init`)

These three standards (y367) codify the per-tool surfaces `shll` composes. They were authored *from* shll's already-shipped probe behavior. `update` is N/A (shll is the consumer, not a producer) and `shell-init` is conformant by construction as the composer. `version` is the one where shll is itself a producer bound by the standard: it is **behaviorally audited on HEAD and pinned by a conformance test** (`TestRootVersionFlag_VersionStandardConformance`) — the standard's *Verifying conformance* clause is met, closing a test-only gap that no behavior change was needed to fix.

| Standard | Scope | shll's posture | Basis |
|----------|-------|----------------|-------|
| `update` | binary | **N/A — shll is the consumer, not a producer.** `shll update` exists, but only as the composer that probes and delegates to each roster tool's own `update`; shll is bound by no producer-side `update` contract for itself, and inside that delegation loop it self-upgrades via a direct `brew upgrade sahil87/tap/shll`. The standard's producer scope is the six roster tools, explicitly excluding shll. | The standard names shll out of producer scope; `shll update` is the consumer that probes and delegates ([cli/update](/cli/update.md)). |
| `version` | binary | **Conformant — behaviorally audited on HEAD and pinned by test.** shll is a producer here (the standard binds all seven binaries incl. shll). The root `shll --version` flag was audited clause-by-clause against HEAD (source inspection + an empirical stamped-build probe): 8 MUST/SHOULD clauses PASS, and the standard's *Verifying conformance* pinning-test clause is now met by `TestRootVersionFlag_VersionStandardConformance` (`version_test.go`). shll's own `--version` prints through the exact first-line parse the standard requires of everyone else, exits 0, does no network I/O on the version path, and its binary name on PATH == `shll`. | `shll` holds itself to the shape it enforces — the standard codifies shll's own `versionTokenRE`/`versionPrefixRE`/`versionTimeout` behavior verbatim; the producer-side root flag is pinned distinct from the consumer-side subcommand table ([cli/version §the root `--version` flag](/cli/version.md#the-root---version-flag-producer-side--pinned-by-conformance-test)). |
| `shell-init` | binary | **Conformant by construction as the composer.** `shll shell-init` is the consumer/composer, not a producer of per-tool init; it drops any sub-tool that exits non-zero and re-emits the rest, so the composed blob is only ever as safe as each producer's stdout. The standard names shll the composer that conforms by construction. | The eval-safety invariant is shll's own — the standard restates it as the producer obligation ([cli/shell-init](/cli/shell-init.md)). |

Publishing these three created no new deferral. `update` and `shell-init` need no shll conformance fix (consumer / composer). `version` is shll's own `[std2]`-pattern conformance pass, mirroring the four documentation/help standards above: the root `--version` flag is audited on HEAD and its pinning-test gap is closed by a test-only diff (`main.go`/`root.go`/`version.go` untouched — constitution Test Integrity: the test conforms to the spec being pinned), delivered via the [conformance-report-in-PR-body convention](#the-conformance-report-in-pr-body-convention). The **seven roster tools'** conformance to `update`/`version`/`shell-init` is per-repo rollout work (the `[std1]`/`[std2]` wave pattern), out of scope for y367; rk-desktop ships with run-kit, so its producer-side conformance is run-kit's own rollout surface (t26g). fab-kit's `SIGKILL`-on-`brew upgrade` fix (the 2026-07-19 incident) becomes that repo's `update`-conformance work, not shll's.

*Introduced by*: `260719-5ys1-version-standard-conformance` (the `version`-standard behavioral audit + pinning test for shll's own root `--version`).

## The `install-composition` standard: conformant by construction

The eighth standard, `install-composition` (w6ay), binds shll on both policies. shll's posture is **conformant by construction** — the standard codified how shll already composes the toolkit; no shll behavior changed to satisfy it, and no gap was opened.

| Half | scope | shll's posture | Basis |
|------|-------|----------------|-------|
| **Policy A — no sibling `depends_on`** | binary + repo (formula half) | **Conformant.** shll's tap formula declares no `depends_on` on a sibling toolkit formula; composition is expressed through `shll install`, not a package edge — exactly the roster-owns-composition model the standard mandates. shll's formula is bound by Policy A (which covers all seven formulas, shll's included), and it meets it. | The constitution makes the roster the single source of truth (Tool Roster Source of Truth) — `shll install` iterates it; no formula edge duplicates that knowledge ([cli/install](/cli/install.md)). |
| **Policy A — probe siblings, degrade** | binary | **Conformant by construction.** Every roster-tool invocation in shll routes through `internal/proc` and treats `proc.ErrNotFound` (binary absent from PATH — the `exec.LookPath` failure) as a graceful skip: `shll shell-init` drops a missing tool, `shll version`/`shll doctor` report `not installed`, `shll setup agent` skips run-kit delegation silently. This is Constitution V (Graceful Degradation) verbatim, which the standard restates as principle №8's inter-tool seam. | The eval-safety / skip-not-crash invariant is shll's own constitution (Principle V), enforced in review ([cli/shell-init](/cli/shell-init.md), [cli/version](/cli/version.md), [cli/doctor](/cli/doctor.md)). |
| **Policy B — centralized install docs** | repo | **Carve-out — and voluntarily modeling the target shape.** The standard names shll's own README out of Policy B's producer scope (mirroring `update.md`'s shll-out-of-producer-scope carve-out), so nothing here is a violation. But shll's README install surface is slimmed to the clean-machine bootstrap flow (the 4-line curl-bootstrap block plus the subset variant) + a pointer to the [install guide](../../site/install.md) / shll.ai — it carries no manual-bootstrap `brew install` walkthrough or troubleshooting deep-dive. `docs/site/install.md` (rendered at shll.ai/shll/install) is the single curated in-repo install destination, the one legitimate home of the manual `brew trust`+`brew install` bootstrap and full install detail. The retired `all` meta-formula appears nowhere in shll's docs (README, `install.md`, `workflows.md`) — the `install-composition` standard's own Precedent line is the sole surviving mention. This makes the publishing repo demonstrate the bootstrap-plus-pointer shape the six roster repos adopt, rather than keeping a second full copy of the install dance. | The standard's scope text carves shll's README out; the slim is voluntary — shll is the destination, so the detail legitimately lives one click away on the site. |

The **seven roster tools'** conformance to `install-composition` — removing the `fab-kit`/`hop` `depends_on` edges on `wt`/`idea`, retiring the `all` meta-formula from each repo, and de-duplicating per-repo `brew install` README snippets — is per-repo rollout work (the `[std1]`/`[std2]` wave pattern, parallel changes in other repos), out of scope for w6ay, which only authored and wired the standard in shll. shll's own README slim + `docs/site/install.md` curation (d4o6) is the reference example the roster changes mirror — the publisher adopting the bootstrap-plus-pointer shape voluntarily, ahead of the wave, even though its README is carved out of the producer scope.

*Policy B carve-out + voluntary slim introduced by*: `260720-d4o6-install-docs-policy-b`.

## The `config-home` standard: N/A — shll has no config file

The ninth standard, `config-home` (km8t), binds every toolkit tool that has — or grows — a config file. shll's posture is **N/A by construction**: shll is stateless (Constitution II — no database, no state, and no config file; every invocation re-derives at request time), so the fixed-root, cascade, and env-restriction obligations have no subject in shll today. shll is bound the day it grows a config file, like `wt`/`tu`. The toolkit's conforming implementations the standard cites as receipts are `hop` (the reference implementation) and `idea`; `run-kit`'s adoption is its own config-consolidation work, and `fab-kit` is the standard's documented, closed exception. Publishing the standard opened no shll gap and no deferral.

*Introduced by*: `260823-km8t-config-home-standard`.

## The rk-desktop roster-entry conformance pass

The importance-descending roster reorder plus the `rk-desktop` roster entry (the roster's first delegated, non-brew tool) re-ran the constitution-mandated conformance pass (t26g) against `docs/site/standards/` on a dev build from repo HEAD — the same audit method as above, findings recorded in the change folder's `conformance-report.md` per [the conformance-report-in-PR-body convention](#the-conformance-report-in-pr-body-convention). Result: **all eight then-published standards PASS (the pass predates `config-home`), no new deferrals.**

| Standard | Result |
|----------|--------|
| `principles` | PASS — the delegated paths inherit the prompt-free shapes; the `--json` surfaces carry rk-desktop rows through the existing schemas; install/update previews render the exact delegated argv from the same builders the live path uses; a missing `rk`, an unsupported platform, and an absent rk-desktop all degrade to skip-with-note / `not installed` |
| `install-composition` | PASS — no new formula and no `depends_on`; rk-desktop's run-kit dependency is expressed as a runtime probe (`rk desktop status`) + roster adjacency only, exactly Policy A's model; the delegation is documented in the centralized `docs/site/install.md`, not a per-repo README snippet (Policy B) |
| `update` | PASS (shll as consumer) — rk-desktop delegates to `rk desktop update` prompt-free, with no `--skip-brew-update` probe, no brew-upgrade fallback, and no relink heal (no formula, no keg); its producer-side conformance is run-kit's own rollout surface |
| `help-dump` | PASS — no command-tree change; only Long help text changed (roster enumerations + the delegation notes), and the dump walks the live tree |
| `skill` | PASS — the canonical `docs/site/skill.md` bundle carries the roster in the new order including rk-desktop, re-embedded with the drift guard green; the `shll setup agent` description picks up rk-desktop's `SkillHint` from the roster |
| `readme-extraction` | PASS — **fixed here**: the README's stale roster enumerations brought to the current order with the rk-desktop delegation note; head structure intact, the README still slimmed to bootstrap+pointer per the Policy B carve-out |
| `version` | PASS (not re-audited — the root `--version` producer surface is untouched; its conformance test stays green) |
| `shell-init` | PASS (not re-audited — rk-desktop ships no shell-init, so the eval-safety invariants are untouched) |

The run-kit companion (freezing the `rk desktop is macOS-only` refusal message with a test in run-kit, or a stable token/exit code if message matching proves unstable) is a robustness follow-up owned by the run-kit repo — not a standard violation, and not deferred here.

*Introduced by*: `260820-t26g-roster-desktop-entry`.

## Where deferred gaps are tracked

Larger gaps (new subsystems, breaking output-contract changes) defer to `fab/backlog.md` items with fresh 4-char IDs (this repo's deferral convention — precedent `[38a6]`/`[tkch]`/`[agst]`), referenced by ID in the report. This audit created no new deferrals — the only deferred standard (`skill`) was tracked in `[agst]`, both other gaps were additive and fixed at audit time, and `[agst]` is resolved (above).

## The conformance-report-in-PR-body convention

The deliverable is a single fab change whose **PR body carries a per-standard conformance report** (one section per standard: PASS / fixed-here-with-commit / deferred-with-ref), citing the audited shll version. The report is written during apply to `conformance-report.md` in the change folder (the durable hand-off between the separate apply and ship stages) and carried verbatim into the PR body by `/git-pr` at ship — the commit sha for each fix is stamped in at that point. This report-in-PR-body pattern is the template the six sibling repos' `[std2]` conformance changes follow.

## Cross-references

- The command that serves the audited standards (roster, embed, drift guard): [cli/standards](/cli/standards.md).
- The command that ADOPTS the `skill` standard (the composer + shll's own embedded bundle): [cli/skill](/cli/skill.md).
- The standards *documents* (incl. the three producer-surface standards' contracts) and the `skill` contract: [cli/standards-content](/cli/standards-content.md).
- The shll-side consumer machinery for the three producer-surface standards: [cli/update](/cli/update.md), [cli/version](/cli/version.md), [cli/shell-init](/cli/shell-init.md).
- The exit-code fix's full mechanism (the `translateExit` classification, `SetFlagErrorFunc`, `usageExitCode`): [cli/commands §exit-code translation](/cli/commands.md#exit-code-translation).
- Constitution → Toolkit Standards (the article this conformance work satisfies) and Principle IV (the standards govern all seven tools, so no single tool owns them — shll is their home).

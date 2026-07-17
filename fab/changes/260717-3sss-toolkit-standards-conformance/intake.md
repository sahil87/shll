# Intake: Toolkit Standards Conformance

**Change**: 260717-3sss-toolkit-standards-conformance
**Created**: 2026-07-18

## Origin

One-shot `/fab-new` invocation carrying the toolkit-standards rollout **wave-2 conformance directive** (backlog `[std2]`, "Directive 2 — Conformance change"), applied to shll itself. The worktree branch is pre-named `std2`. Raw input:

> Task: Bring this repo and its tool into conformance with the sahil87 toolkit standards.
>
> Precondition: `shll standards` runs on this machine (if the subcommand is missing, run `shll update`; if it still fails, stop and report — do not proceed from memory or the website). This repo's constitution carries the Toolkit Standards article; this task is the conformance work it mandates.
>
> 1. Enumerate at runtime: run `shll standards`, then `shll standards <name>` for every listed entry. The list is authoritative — do not assume which standards exist or what they require.
> 2. Audit this repo against each standard. For mechanical contracts (machine help output, README/docs-site structure), execute the standard's own verification checklist verbatim. For the principles, assess each numbered principle against the tool's actual behavior — prompts and TTY handling, stdout/stderr separation, --json/--dry-run/--yes coverage, exit codes and error wording, idempotency, output volume.
> 3. Fix what is proportionate here: all mechanical-contract violations, and principle gaps that are small and additive (a missing flag, a misrouted stream, an unhelpful error). Larger gaps that would restructure the tool are NOT for this change — record each as a draft change or issue per this repo's convention and reference it.
> 4. Deliverable: one fab change whose PR body contains a conformance report — one section per standard with PASS or the gaps found, each gap dispositioned as fixed here (with the commit) or deferred to <ref>. Include the shll version audited against (`shll version`'s shll row), since standards are versioned with the shll release. Tests green; if the command tree changed, re-verify the machine-help contract afterward.
>
> Note on the "skill" standard specifically: if this repo has not yet implemented a `<tool> skill` subcommand, that is a known, deferred gap (per the toolkit's phased per-repo adoption — no seven-repo flag-day) — report it as "deferred, not yet adopted" rather than treating it as an in-scope fix for this change.

**Precondition verified at intake** (2026-07-18): `shll standards` exits 0 and lists **4 standards** — `principles` (foundation), `help-dump` (binary), `readme-extraction` (repo), `skill` (binary+repo). `shll version` reports **shll v0.0.23**. All four standard documents were read via `shll standards <name>` at intake time.

## Why

1. **The constitution mandates it.** This repo's constitution (§ Additional Constraints → Toolkit Standards, amended 2026-07-18 in change `qas0`) binds shll to the toolkit's published standards. The standards system (changes `i70w`, `qas0`, `vo8c`) is now live; this change is the conformance work that article exists to produce.
2. **shll publishes the standards.** The standards are canonically authored in this repo's `docs/site/standards/` and served by `shll standards`. If the publishing tool itself doesn't demonstrably conform — with an auditable report — the standards read as aspirational rather than binding, undermining the whole cross-repo rollout (`[std1]`/`[std2]` waves across hop, wt, tu, idea, run-kit, fab-kit).
3. **Wave-2 timing.** The `[std2]` precondition is met: the post-`standards/`-move shll release (v0.0.23) is published and installed, `shll standards` runs clean. shll's own conformance change is the reference example the other six repos' conformance changes will mirror.

Not doing this leaves shll's conformance state unknown and unauditable — every standard names shll as an enforcement receipt, and nothing has verified those receipts against the standards' own checklists since the standards were extracted into their current form.

## What Changes

### 1. Runtime-enumerated audit (apply-entry step, re-run — intake findings are advisory)

Apply MUST re-enumerate at runtime — `shll standards`, then `shll standards <name>` for each listed entry — and treat that list as authoritative, even though intake already did so. The 4 entries found at intake and their audit method:

| Standard | Scope | Audit method |
|----------|-------|--------------|
| `principles` | foundation | Assess each of the 10 numbered principles against shll's actual behavior |
| `help-dump` | binary | Execute the standard's "Verifying conformance" checklist verbatim |
| `readme-extraction` | repo | Execute the standard's "Verifying conformance" checklist verbatim |
| `skill` | binary+repo | Report **"deferred, not yet adopted"** — see § 4 |

**Audit target**: the binary built from this repo's HEAD (`cd src && go build ./cmd/shll` — dev build), NOT the installed v0.0.23 — the repo is what this change can fix. The standards *text* comes from the installed `shll standards` (v0.0.23), which matches this repo's `docs/site/standards/` (the `TestStandardsEmbedMatchesCanonical` drift guard pins repo↔embed byte-equality). The report cites **shll v0.0.23** as the version audited against.

**Mechanical checklists to execute verbatim** (from the standards' own "Verifying conformance" sections):

- `help-dump`: exits 0; valid JSON to stdout only; stderr empty; envelope is `{tool, version, schema_version, root}` with no `captured_at`; `completion`/`help`/hidden commands absent from the tree; `version` reflects the built binary (ldflags), not a literal; a minimal test pins exit 0 + valid JSON + expected `tool`/`schema_version`.
- `readme-extraction`: README top is `#` H1 → canonical toolkit blockquote → badges → prose; grep `](./`, `](../`, `](docs/` — every relative target points into `docs/site/` (from README), stays inside `docs/site/` (between tree pages), or is absolute; no relative images anywhere; no `#gh-*-mode-only` fragments; no mermaid fences destined for the site; no `docs/site/` page named `overview`, `readme`, or `commands`; README cross-links its `docs/site/` pages and the absolute command-reference URL `https://shll.ai/shll/commands/`.

**Principles assessment** (each of the 10, against actual behavior): №1 non-interactive by default (TTY-gated prompts, `--yes`, non-TTY refusal); №2 stdout=data/stderr=diagnostics + `--json` for programmatic surfaces; №3 layered help + `help-dump`; №4 fail-fast actionable errors, documented exit codes (`0`/`1`/`2` convention); №5 visible mutation boundaries + `--dry-run` on destructive writes sharing the live code path; №6 stateless/idempotent; №7 compose via subprocess + probed capabilities; №8 graceful degradation (missing tools = skip, TTY-gated color, typed unavailable); №9 bounded output (explicit caps, `--quiet` semantics where present); №10 agent-discoverable docs. shll is the named enforcement receipt for several (№1/№5 `uninstall`, №6 constitution, №7 `update` probe, №8 `shell-init`/`changelog`, №9 `changelog` cap) — the audit verifies those receipts still hold on HEAD and sweeps the rest of the subcommand surface (`update`, `install`, `list`, `doctor`, `version`, `changelog`, `shell-init`, `shell-setup`, `standards`, `uninstall`) for gaps: `--json` coverage on programmatic-read surfaces, exit-code documentation, error wording (what failed / why / next step), stream routing.

**Intake-time smoke results** (advisory only — apply re-runs everything): dev-build `help-dump` passed all envelope checks (valid JSON, `{tool: shll, schema_version: 1}`, no `captured_at`, filtered tree of 10 visible commands, stderr empty, exit 0; `help_dump_test.go` exists). README head structure conforms (H1 → toolkit blockquote → prose); no relative images; no `gh-*-mode-only` fragments; all README relative links point into `docs/site/`. Two items intake could not settle and apply must: (a) shll's README has **no badge lines** — determine whether the badge run is required or optional chrome under the standard; (b) README has **no footer heading** (`Contributing`/`Development`/`Building`/`License`/`Acknowledgements`) so the entire README is the pulled slice — determine whether that is conformant (nothing maintainer-only leaks) or needs a footer split.

### 2. In-scope fixes (proportionate)

- **All mechanical-contract violations** found by the two checklists — fixed in this change.
- **Small, additive principle gaps** — a missing flag, a misrouted stream (e.g., a hint printed to stdout), an unhelpful error message, a missing truncation notice. Threshold: the fix adds surface or rewords output without changing any command's core behavior or breaking its existing output contract.
- Each fix lands as a normal commit in this change and is cited by commit in the report.

### 3. Deferred gaps

Gaps that would restructure the tool (new subsystems, breaking output-contract changes, redesigned commands) are recorded as **`fab/backlog.md` items** (this repo's deferral convention — precedent `[38a6]`/`[tkch]`/`[agst]`) with a fresh 4-char ID each, and the report's disposition line references that ID.

### 4. The `skill` standard — deferred by directive

shll has no `shll skill` subcommand (verified: no such command in `src/cmd/shll/`). Per the directive and the standard's own Adoption section ("No tool ships `skill` today... A tool without a `skill` subcommand is not yet in violation"), the report's skill section reads **"deferred, not yet adopted"** and references backlog `[agst]` (which already carries the `shll skill` + `shll agent-setup` design). Do NOT implement `shll skill` in this change.

### 5. Conformance report (the deliverable)

Written during apply to `fab/changes/260717-3sss-toolkit-standards-conformance/conformance-report.md`, then carried into the PR body at ship (`/git-pr` includes it verbatim). Format:

```markdown
## Conformance report — audited against shll v0.0.23

### principles — {PASS | N gaps}
- №4: {gap} — fixed here ({commit sha}) | deferred to [xxxx]
...
### help-dump — PASS
### readme-extraction — {...}
### skill — deferred, not yet adopted (phased per-repo adoption; tracked in [agst])
```

One section per standard; PASS or per-gap disposition lines; the audited shll version in the heading.

### 6. Verification tail

Tests green (`cd src && go test ./...`). If any fix changes the command tree (new flag/subcommand), re-run the help-dump mechanical checklist afterward and re-confirm `help_dump_test.go` passes.

## Affected Memory

- `cli/standards-conformance`: (new) The audited conformance state — which standards shll meets on HEAD, the per-standard audit method, where deferred gaps are tracked (`[agst]` for skill; any new backlog IDs), and the report-in-PR-body convention.
- `cli/commands`: (modify) Only if audit fixes touch shared command wiring (exit-code sentinels, root flags) — audit-determined.
- Per-command files (`cli/update`, `cli/install`, `cli/version`, ...): (modify) Only those whose command's behavior an in-scope fix actually changes — audit-determined; hydrate scopes to real diffs.

## Impact

- **Code**: `src/cmd/shll/*.go` — audit-determined, expected small (flags, stream routing, error wording, caps/notices) plus matching `*_test.go` updates.
- **Docs**: `README.md`, `docs/site/**` — only if the readme-extraction checklist finds violations.
- **Process artifacts**: `conformance-report.md` in the change folder; possibly new `fab/backlog.md` deferral items.
- **Risk**: low — additive-only by scoping rule; anything larger defers. No release required; the audit runs against a dev build.

## Open Questions

*(none — the directive is fully prescriptive; the two README interpretation items in § 1 are audit-time determinations within agent competence, not user decisions)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | The 4-entry runtime enumeration from installed `shll standards` (v0.0.23) is the authoritative audit scope; apply re-enumerates rather than trusting this intake | Directive states it verbatim; verified running at intake | S:95 R:90 A:95 D:95 |
| 2 | Certain | `skill` standard → report "deferred, not yet adopted" referencing `[agst]`; no `shll skill` implementation in this change | Directive explicit; standard's own Adoption section confirms non-violation | S:95 R:85 A:95 D:95 |
| 3 | Certain | Deliverable is one fab change; PR body carries the per-standard report (PASS / fixed-here+commit / deferred+ref) citing shll v0.0.23 | Directive explicit; version captured at intake | S:90 R:80 A:90 D:90 |
| 4 | Certain | Tests green before ship; help-dump checklist re-run if the command tree changes | Directive explicit; test suite + drift guards exist | S:90 R:90 A:95 D:95 |
| 5 | Confident | Audit target = binary built from repo HEAD (dev build); standards text = installed v0.0.23 (byte-matched to repo by drift guard) | Conformance work must audit what it can fix; installed binary can't take fixes | S:70 R:75 A:85 D:80 |
| 6 | Confident | Deferred gaps recorded as `fab/backlog.md` items with fresh 4-char IDs | "Per this repo's convention" — backlog precedent `[38a6]`/`[tkch]`/`[agst]`; no issue tracker in use here | S:75 R:85 A:80 D:70 |
| 7 | Confident | Report persisted as `conformance-report.md` in the change folder during apply; `/git-pr` carries it into the PR body at ship | Directive names the PR body as the vehicle but apply and ship are separate stages; a change-folder artifact is the only durable hand-off | S:60 R:80 A:80 D:70 |
| 8 | Confident | "Small and additive" threshold: adds a flag / reroutes a stream / rewords an error / adds a cap or notice without breaking existing output contracts; anything needing new subsystems or breaking changes defers | Directive gives examples; the threshold operationalizes them; apply decides-and-records per gap | S:65 R:70 A:75 D:65 |

8 assumptions (4 certain, 4 confident, 0 tentative, 0 unresolved).

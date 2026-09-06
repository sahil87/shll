# Plan: Absorb agentskills.io Guidance into the Skill Standard

**Change**: 260905-q8i5-absorb-agentskills-skill-standard
**Intake**: `intake.md`

## Requirements

### Standard: description-writing rules

#### R1: Bootstrap-description writing rules codified in the standard
`docs/site/standards/skill.md` SHALL gain a subsection under "Landed design: `shll setup agent`" codifying how the placed `shll-toolkit` skill's `description` frontmatter is written: (1) tool names front-loaded as trigger vocabulary (legacy aliases included, e.g. `run-kit/rk`); (2) task-shaped trigger phrases per tool, not just nouns; (3) what + when structure (the agentskills.io activation contract — the description is the only text in context before invocation); (4) ≤1024 characters, strictly; (5) the triggers-vs-operations split: the description carries activation vocabulary, operational/recipe prose belongs in the skill body (read at activation).

- **GIVEN** the amended standard
- **WHEN** a reader consults "Landed design: `shll setup agent`"
- **THEN** the five description-writing rules above are stated normatively (MUST/SHOULD register matching the page's house style)

### Standard: placed-skill spec conformance

#### R2: agentskills.io conformance required for the placed skill
The standard SHALL make agentskills.io conformance an explicit requirement for the placed bootstrap skill (the artifact at `~/.agents/skills/shll-toolkit/SKILL.md` and `~/.claude/skills/shll-toolkit/SKILL.md`): valid YAML frontmatter with portable `name` + `description`; name 1–64 chars matching `^[a-z0-9]+(-[a-z0-9]+)*$` and equal to the skill directory name; description ≤1024 chars. The "Verifying conformance" checklist SHALL gain a placed-skill item naming `skills-ref validate` (github.com/agentskills/agentskills) as the reference method, with equivalent in-repo tests acceptable where the validator is not practically installable. Scope stays explicit: only the placed skill is bound; bundles (`<tool> skill` stdout) are not.

- **GIVEN** the amended standard
- **WHEN** a tool author reads the placed-skill requirements and the conformance checklist
- **THEN** the frontmatter/name/description constraints are normative and the checklist names `skills-ref validate` (or documented equivalent tests)

### Standard: mechanical enforcement

#### R3: Line budget and topics contract enforced by failing tests
The standard ("Rules with teeth" and "Verifying conformance") SHALL require that, in each adopting repo, two currently prose-only checks are enforced by a failing test rather than a review item: the ≤150-line budget (core bundle and each topic page) and the reserved `skill topics` contract (one name per line to stdout, stderr empty, exit 0; empty output for a topic-less tool; no content topic named `topics`). The standard mandates the outcome (a test fails on violation), not the mechanism — extending the drift-guard or a separate conformance test both conform.

- **GIVEN** the amended standard
- **WHEN** an adopting repo ships a bundle over 150 lines or a broken `topics` contract
- **THEN** the standard's text requires that repo's own test suite to fail (outcome mandated, mechanism free)

#### R4: Rejected absorptions recorded as design rationale
The standard SHALL record why four agentskills.io features are deliberately NOT absorbed for bundles: their 500-line budget (ours is tighter — bundles are pulled per-conversation), frontmatter on bundles (stdout payloads, no loader), the experimental `allowed-tools` field, and the `scripts/`/`references/`/`assets/` folder conventions (the binary is the "script"). It SHALL state the complementary-not-converging relationship: agentskills.io governs the placed harness-side format (which the bootstrap skill conforms to per R2); bundles stay binary-embedded and version-locked — version-locking is the answer to placed-file staleness.

- **GIVEN** the amended standard
- **WHEN** a reader asks why bundles don't follow the open spec
- **THEN** the four rejections and the complementary-not-converging statement are present with rationale

### Repo: embedded copy and self-conformance

#### R5: Embedded standard copy refreshed
After amending the canonical `docs/site/standards/skill.md`, the embedded copy at `src/cmd/shll/standards/skill.md` MUST be refreshed via `scripts/sync-standards.sh` and committed, keeping `TestStandardsEmbedMatchesCanonical` green.

- **GIVEN** the amended canonical file
- **WHEN** `scripts/sync-standards.sh` runs and both copies are committed
- **THEN** `go test ./...` passes the drift guard

#### R6: Bootstrap description compressed to ≤1024 via compress + relocate
The roster-generated description (`agentSkillDescription()` over `Roster` in `src/cmd/shll/tools.go`, currently 1365 chars) MUST measure ≤1024 characters. Compression follows the resolved decision: prioritized what+when trigger vocabulary stays in the description; operational/recipe prose (explanatory clauses such as "(HTML, diagrams, reports, a local dev server)", the `open`/`xdg-open`-never-reach-them explanation, "for the proxied-iframe recipe", "(refresh a PR list, open a diff, focus a view)") relocates to the placed skill's body (`agentSkillContent` in `src/cmd/shll/agent_setup.go`), which is read at activation. The compressed description MUST remain single-line and `: `-free, and MUST preserve verbatim the six load-bearing fragments pinned by `TestRosterProactiveHint` (all five ProactiveHint jobs survive): `to proxy a local http port`, `before opening any file or local port in a browser, read`, `publishing an artifact`, `rk code exec`, `tutorial, tour, or onboarding`, `shll skill run-kit tutorial`.

- **GIVEN** the reworked `ProactiveHint` value and body content
- **WHEN** `agentSkillDescription()` renders
- **THEN** the result is ≤1024 chars, single-line, `: `-free, and contains all six pinned fragments
- **AND** the relocated operational prose appears in the skill body's run-kit paragraph

#### R7: Mechanical self-conformance tests
`agent_setup_test.go` MUST gain a test enforcing the ≤1024-char description cap (named constant, no magic number) and assertions covering the placed skill's agentskills.io validity (frontmatter carries exactly portable `name` + `description`; `name` matches `^[a-z0-9]+(-[a-z0-9]+)*$` and equals `skillDirName`) — extending existing tests where they already cover a rule. `TestRosterProactiveHint` and `TestAgentSetup_BodyTeachesTwoStepAndStandards` are updated to the new wording while keeping their five-jobs/fragment pinning semantics. The existing `skill_test.go` coverage of the 150-line budget and `topics` contract is audited against R3's wording and gaps filled (expected: none — `TestSkillEmbedMatchesCanonical` caps at 150 and the reserved-topics tests exist).

- **GIVEN** the amended code and tests
- **WHEN** `go test ./...` runs
- **THEN** all tests pass, and reverting the description to >1024 chars or breaking the name rule would fail a test

### Non-Goals

- Implementations in other toolkit repos (run-kit, fab-kit, …) — bound on their own adoption cadence.
- Converging the bundle format onto agentskills.io (frontmatter, 500-line budget, folder layout) — explicitly rejected.
- Any runtime behavior change to the `shll skill` / `shll standards` serving paths.

### Design Decisions

#### skills-ref validation lands as Go-test equivalents, not a CI step
**Decision**: The standard's checklist names `skills-ref validate` as the reference method with equivalent in-repo tests acceptable; this repo implements the Go-test equivalents (frontmatter validity, name rule, ≤1024 cap) and leaves `.github/workflows/ci.yml` untouched.
**Why**: CI already runs `go test ./...`; the rules are mechanical and testable in-process; adding a third-party validator install to CI buys no additional coverage and adds a supply-chain dependency.
**Rejected**: A `skills-ref validate` CI step (extra dependency for checks Go tests express directly); skipping validation entirely (leaves R2's requirements unenforced in the publishing repo).
*Introduced by*: 260905-q8i5-absorb-agentskills-skill-standard

#### Compression preserves the six pinned fragments verbatim
**Decision**: The ~340-char reduction comes from connective/explanatory prose only; the six `TestRosterProactiveHint` fragments stay verbatim so all five ProactiveHint jobs keep firing.
**Why**: The fragments are the load-bearing trigger/collision vocabulary shipped deliberately by xv71/e09x/e0gt/qn14; the user's decision prioritizes trigger coverage over prose completeness.
**Rejected**: Rewording fragments and updating the test to match (silently weakens the shipped trigger vocabulary the test exists to protect).
*Introduced by*: 260905-q8i5-absorb-agentskills-skill-standard

## Tasks

### Phase 1: Standard amendment

- [x] T001 Amend `docs/site/standards/skill.md`: add the description-writing-rules subsection under "Landed design: `shll setup agent`" and the placed-skill agentskills.io conformance requirements + `skills-ref validate` checklist item under "Verifying conformance" <!-- R1 R2 -->
- [x] T002 Amend `docs/site/standards/skill.md`: mechanical-enforcement wording in "Rules with teeth" + "Verifying conformance" (≤150 budget and `skill topics` contract as failing tests), and the rejected-absorptions/complementary-not-converging passage <!-- R3 R4 -->

### Phase 2: Code + embed

- [x] T003 Compress + relocate: rework run-kit's `ProactiveHint` in `src/cmd/shll/tools.go` to fit the description in ≤1024 chars (six fragments preserved verbatim); extend the run-kit paragraph in `agentSkillContent` (`src/cmd/shll/agent_setup.go`) with the relocated operational prose <!-- R6 -->
- [x] T004 Run `scripts/sync-standards.sh` to refresh `src/cmd/shll/standards/skill.md` <!-- R5 -->

### Phase 3: Tests

- [x] T005 Tests in `src/cmd/shll/agent_setup_test.go`: add the ≤1024 length test (named constant) + placed-skill validity assertions; update `TestRosterProactiveHint` / `TestAgentSetup_BodyTeachesTwoStepAndStandards` to the new wording; audit `skill_test.go` against R3 and fill gaps; run `go test ./...` <!-- R7 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: The standard's "Landed design" section carries the five description-writing rules (front-loaded names, task-shaped phrases, what+when, ≤1024, triggers-vs-body split)
- [x] A-002 R2: The standard requires agentskills.io conformance for the placed skill (frontmatter validity, name rule + dir match, description ≤1024) and the checklist names `skills-ref validate` with the equivalent-tests allowance
- [x] A-003 R3: The standard mandates failing-test enforcement of the ≤150 budget and the `skill topics` contract, outcome-not-mechanism
- [x] A-004 R4: The four rejected absorptions and the complementary-not-converging statement are recorded with rationale

### Behavioral Correctness

- [x] A-005 R6: `agentSkillDescription()` renders ≤1024 chars, single-line, `: `-free, with all six pinned fragments verbatim
- [x] A-006 R6: The relocated operational prose (proxied-iframe recipe pointer context, dashboard explanation, palette-command examples) appears in the placed skill's body

### Scenario Coverage

- [x] A-007 R5: `scripts/sync-standards.sh` run; `TestStandardsEmbedMatchesCanonical` green; both copies committed
- [x] A-008 R7: A test fails if the description exceeds 1024 chars or the placed-skill name/frontmatter rules break; `go test ./...` passes

### Code Quality

- [x] A-009 Pattern consistency: standard edits match the page's house register (MUST/SHOULD, "rules with teeth", checklist bullets); Go edits match surrounding comment density and naming
- [x] A-010 No magic numbers: the 1024 cap is a named constant in the code/tests
- [x] A-011 No unnecessary duplication: validity checks extend existing tests where coverage exists rather than adding parallel ones

## Notes

- Check items as you review: `- [x]`
- Change type is `docs` — the review's parsimony pass and deletion-candidate prompt are skipped by rule.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | skills-ref validation as Go-test equivalents; no ci.yml change | CI already runs go test; intake assumption 7 explicitly allows equivalents; validator packaging unverified | S:70 R:90 A:75 D:70 |
| 2 | Certain | Preserve the six TestRosterProactiveHint fragments verbatim; compress connective prose only | Test contract pins them; user decision prioritizes trigger coverage | S:90 R:85 A:90 D:90 |
| 3 | Confident | Relocated prose extends the body's existing run-kit paragraph (no new body section) | Body already carries the run-kit pointer paragraph (qn14); smallest coherent placement | S:70 R:90 A:85 D:80 |
| 4 | Confident | No `ci/` memory file work — hydrate scope drops the intake's conditional `ci/…` entry | The condition (a ci.yml step) resolved to no-change per Assumption 1 | S:75 R:95 A:85 D:85 |

4 assumptions (1 certain, 3 confident, 0 tentative).

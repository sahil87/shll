# Intake: Install-Composition Toolkit Standard

**Change**: 260720-w6ay-install-composition-standard
**Created**: 2026-07-20

## Origin

One-shot `/fab-new` invocation. Raw input:

> Add a binding toolkit standard to docs/site/standards/ codifying two related policies just decided for the whole shll toolkit. Policy A — no inter-tool Homebrew dependencies: toolkit formulas MUST NOT declare depends_on on sibling toolkit formulas; a tool that invokes a sibling tool at runtime MUST probe for it (command -v in shell/skills, exec.LookPath in Go) and degrade gracefully with an actionable install hint (e.g. "wt is not installed. Install it: brew install sahil87/tap/wt"); shll install is the composition point that installs the full toolkit. Policy B — centralized install documentation: per-tool READMEs and the tap README MUST NOT carry per-formula brew install instructions; they link to https://shll.ai for install steps (the curl bootstrap / shll install). Individual formula installs remain supported (and shll install accepts a subset) — unsupported is documenting them per-repo, which drifts. Follow the existing structure and tone of the files in docs/site/standards/ (read them first), update any standards index that exists, and keep it concise. Rationale/context: fab-kit and hop previously declared depends_on wt/idea; those edges are being removed in parallel changes, and the all meta-formula is being retired in favor of shll install.

Both policies arrive pre-decided — the input specifies the exact obligations, the probe mechanisms, the example hint text, and the supported-vs-unsupported line. The intake's job is codification (where the standard lives, how it hooks into the existing standards set), not policy design.

## Why

1. **The pain point.** Two toolkit repos (`fab-kit`, `hop`) declared Homebrew `depends_on` on sibling toolkit formulas (`wt`, `idea`). Brew dependency edges between siblings force lockstep installs (installing one tool drags in others the user didn't ask for), complicate uninstalls (brew refuses to remove a dependency of an installed formula), and duplicate the roster knowledge that `shll install` already owns as the toolkit's single composition point. Separately, per-repo `brew install sahil87/tap/<formula>` instructions in tool READMEs drift — each repo restates the trust-then-install dance, and every change to the install story (e.g. Homebrew 6's tap-trust requirement) has to be chased across seven repos plus the tap.
2. **The consequence of not fixing it.** The `depends_on` edges are being removed in parallel changes and the `all` meta-formula is being retired — but without a binding standard, nothing stops the next tool from reintroducing a sibling edge or a fresh README from adding its own install snippet. The constitution's Toolkit Standards clause makes `docs/site/standards/` the enforcement surface; an undocumented policy is not binding.
3. **Why this approach.** The repo already has exactly the right home: `docs/site/standards/` is the canonical, binding, producer-facing standards tree (constitution § Toolkit Standards), rendered on shll.ai and served offline via `shll standards`. A new mechanical-contract page there — following the established `# Standard: <name>` shape with MUST obligations and a Verifying-conformance checklist — makes both policies binding on all seven repos plus the tap without amending any constitution.

## What Changes

### 1. New standard document: `docs/site/standards/install-composition.md`

A new mechanical-contract page following the established structure of the six existing companion standards (`# Standard: <name>` title; producer-facing intro naming the [toolkit CLI principles](../../docs/site/standards/principles.md) it implements and its scope; MUST/SHOULD obligation sections; closing `## Verifying conformance` checklist). Content plan — concise, sectioned:

- **Intro**: how the toolkit composes at install time. Producer-facing standard; implements principles №7 (compose, don't reinvent — sibling capability is probed, never assumed via a package edge) and №8 (graceful degradation — a missing sibling is a skip with a hint, not a crash). Scope note: Policy A binds all seven tap formulas and every binary that invokes a sibling; Policy B binds the six roster-tool repos plus the tap README — `shll`'s own README is out of Policy B's producer scope because it (with shll.ai) *is* the centralized install documentation (mirroring `update.md`'s "shll is the consumer here" scope carve-out).
- **Section: No inter-tool formula dependencies (Policy A)**:
  - Toolkit formulas MUST NOT declare `depends_on` on sibling toolkit formulas.
  - `shll install` is the composition point: it installs the full roster (and accepts a subset). A formula edge duplicates that roster knowledge in the tap and forces lockstep installs/uninstalls.
  - Context receipt: `fab-kit` and `hop` previously declared `depends_on` on `wt`/`idea`; those edges are removed, and the `all` meta-formula is retired in favor of `shll install`.
- **Section: Probe at runtime, degrade gracefully (Policy A, binary half)**:
  - A tool that invokes a sibling tool at runtime MUST probe for it — `command -v <tool>` in shell/skill code, `exec.LookPath` in Go — and MUST NOT assume presence.
  - On a missing sibling it MUST degrade gracefully with an actionable install hint. Example message, verbatim: `wt is not installed. Install it: brew install sahil87/tap/wt`
- **Section: Install documentation is centralized (Policy B)**:
  - Per-tool READMEs and the tap README MUST NOT carry per-formula `brew install` instructions.
  - They link to https://shll.ai for install steps (the curl bootstrap / `shll install`).
  - The supported-vs-unsupported line, stated explicitly: individual formula installs remain **supported** (`brew install sahil87/tap/<tool>` works, and `shll install` accepts a subset) — what is unsupported is **documenting** them per-repo, which drifts.
- **`## Verifying conformance`** checklist: no `depends_on` on a sibling in the tool's tap formula; every sibling invocation is behind a probe; missing-sibling paths emit the install hint; README/tap-README install sections link to shll.ai instead of carrying `brew install` lines.

### 2. Standards index updates: `docs/site/standards/principles.md`

`principles.md` is the standards index — three places reference the companion set:

- The "Six companion standards" intro paragraph (currently a 3+3 categorization: "Three cover documentation and help… Three cover the per-tool surfaces shll composes") — becomes seven, with `install-composition` worked into the enumeration.
- The "The contracts" table — new row: `install-composition` | Implements №7, №8 | scope `binary+repo` | one-line "what it standardizes".
- The closing "Consuming these standards" paragraph — the parenthesized companion list gains `install-composition`.

No changes to the ten principle sections themselves (their "Enforced by" receipts stay as-is; the new contract's linkage is carried by the table's Implements column, keeping the edit concise).

### 3. Embed wiring (mechanically required by this repo's architecture)

`docs/site/standards/` is canonical but also served offline by `shll standards` via a build-time embed. Adding a standard without wiring it in would leave it invisible to the command:

- `scripts/sync-standards.sh`: append `install-composition` to the `STANDARDS=(…)` array.
- `src/cmd/shll/standards.go`: append a `standardsRoster` entry — `Name: "install-composition"`, one-line `Description` (e.g. "No inter-tool formula dependencies; probe siblings at runtime; install docs centralized on shll.ai"), `Scope: "binary+repo"` (already in `TestStandardsRosterIntegrity`'s pinned vocabulary — no test change needed), `SourcePath: "docs/site/standards/install-composition.md"`, `EmbedName: "install-composition.md"`.
- Run `scripts/sync-standards.sh` and commit the refreshed embed copies (`src/cmd/shll/standards/install-composition.md` new, `principles.md` copy updated).
- All `standards_test.go` tests are roster-driven (list count, JSON, byte-identical reader, drift guard) — they pick up the new entry with no test-code changes.

### Out of scope

- Removing the `depends_on` edges in `fab-kit`/`hop` and retiring the `all` meta-formula — parallel changes in other repos, cited as context only.
- Changes to the tap README or per-tool READMEs to conform — each repo conforms on its own cadence (the phased-rollout convention `skill.md` established).
- shll code behavior — `shll install`/`shll update` already implement the composition-point role; nothing binary-behavioral changes here beyond the embed roster.

## Affected Memory

- `cli/standards-content`: (modify) the standards-documents memory gains the eighth standard — its two policies, the scope carve-out (shll README out of Policy B scope), and the naming choice (`install-composition`)
- `cli/standards`: (modify) `shll standards` roster/embed/sync-script gain an eighth entry; "all seven standards" phrasing becomes eight
- `cli/standards-conformance`: (modify) conformance state extends to the new standard (shll's formula has no sibling `depends_on`; shll probes roster tools and degrades per constitution V; shll's README install section is the central-source carve-out, not a violation)

## Impact

- `docs/site/standards/install-composition.md` — new (the deliverable)
- `docs/site/standards/principles.md` — index paragraph, contracts table, consuming-list
- `scripts/sync-standards.sh` — one array element
- `src/cmd/shll/standards.go` — one roster entry
- `src/cmd/shll/standards/` — refreshed embed copies (new file + principles.md)
- Tests: `go test ./...` in `src/` — existing roster-driven standards tests must stay green; drift guard requires the sync to have run
- Renders on shll.ai at `/shll/standards/install-composition` via the existing pull pipeline; readable offline via `shll standards install-composition`

## Open Questions

- None — the policies arrived fully specified; remaining choices (filename, scope label) are graded assumptions below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | One standard file covering both policies, not two | User input: "a binding toolkit standard… codifying two related policies" — singular, and the policies share one rationale (shll.ai/shll install as the single composition point) | S:90 R:80 A:95 D:90 |
| 2 | Confident | Filename/standard name `install-composition` | Matches the plain hyphenated-noun naming of `help-dump`/`readme-extraction` (memory: "plain filenames"); `install` alone would read as a per-tool subcommand surface (tools have none) and collides conceptually with `docs/site/install.md` | S:65 R:85 A:60 D:45 |
| 3 | Confident | Scope label `binary+repo` (existing vocabulary), with the intro noting the formula half lives in the tap repo | Reuses `TestStandardsRosterIntegrity`'s pinned scope set — no test/vocab churn; the tap formula is a repo-file obligation in spirit. A dedicated `tap` scope value was rejected as churn for one row | S:55 R:90 A:60 D:50 |
| 4 | Confident | Implements principles №7 and №8 | Probe-not-assume is №7's explicit contract ("advertised flags, probed not assumed"); skip-with-hint on missing sibling is №8 verbatim. Policy B's anti-drift rationale echoes №10 but the binding linkage is kept to the two direct principles | S:70 R:85 A:80 D:65 |
| 5 | Confident | Policy B scope excludes shll's own README; Policy A covers all seven formulas | shll's README + shll.ai *are* the centralized install docs Policy B points at — binding shll to "link to shll.ai instead" would be circular. Mirrors `update.md`'s established shll-out-of-producer-scope carve-out. Policy A has no such asymmetry: shll's formula must equally avoid sibling edges | S:65 R:85 A:80 D:70 |
| 6 | Confident | Embed wiring (sync script, Go roster, committed copies) is in scope despite the input mentioning only docs | Repo architecture makes it mechanically required: `docs/site/standards/` is canonical AND served by `shll standards`; omitting the roster entry leaves the standard invisible offline, violating the set's own principle №10 posture | S:60 R:80 A:90 D:80 |
| 7 | Confident | `change_type: docs` (explicit override from inferred `feat`) | The deliverable is a standards document; the Go-side change is a data-only roster entry with no behavioral logic | S:60 R:90 A:75 D:65 |

7 assumptions (1 certain, 6 confident, 0 tentative, 0 unresolved).

# Intake: Standards consumer-site sweep (D14 second pass)

**Change**: 260912-dk8f-standards-consumer-site-sweep
**Created**: 2026-09-12

## Origin

One-shot `/fab-new` invocation in a pre-created worktree (`x4-standards-consumer-site-sweep`), driven by the cross-repo HexoKit rebrand plan (run-kit repo, `fab/plans/sahil/26-09-10-hexokit-rebrand.md`), row **X4** — the last Phase 2 row before the announce.

> standards-consumer-site-sweep -- X4 in the hexokit-rebrand plan: sweep the standards-consuming sites (this shll repo among them) to switch from the old shll.ai install link/references to hexokit.com now that X1 (hexokit-site cutover prep, PR run-kit#959 -- wait, that is quake-terminal; correct precedent is X1 hexokit-site cutover PR sahil87/hexokit-site#9) and X2 (shll.ai redirect stub, PR sahil87/shll.ai#104) are both done at review-pr. Read fab/plans/sahil/26-09-10-hexokit-rebrand.md's X4 row for the exact scope before writing the intake -- likely mirrors the sahil87#1 companion-repo banner-sweep pattern from Phase 1 (README/install-link cosmetic fixes), applied here to standards.md or equivalent consumer docs in this repo.

**The X4 row, verbatim** (plan § Phase 2): *"D14 second pass: in `docs/site/standards/*.md` (and the embedded copies, drift-guarded) flip `shll.ai` → `hexokit.com` where it names the consuming site (32 mentions) and the nine "[shll toolkit](https://shll.ai)" intros → "[HexoKit toolkit](https://hexokit.com/toolkit/)". `shll standards` command, file names, and the standards' own names are untouched."* Depends on: X2 live.

**Pickup protocol followed**: Decision log read — D1–D4, D13–D15 are Confirmed (Certain, not re-opened); D5–D12 remain proposals (D10 product-first install is *implemented and live* via S5, whatever its log status). C1's intake (`260911-ttoa`, merged as shll v0.1.31) was read in full: its § Non-goals explicitly hands X4 the mentions C1 left alone — the nine standards intros, the consuming-site sentences, `docs/site/install.md`, the README body lines, `fab/project/config.yaml`, `fab/project/context.md`, and the `agent_setup.go` placed-skill prose ("flagged for the X4 row's scope — it is not currently listed there"). This intake honors that handoff: the plan row is the *minimum*, C1's handoff list is the *scope*.

**State of the dependencies (verified 2026-09-12)**: X1 ([hexokit-site#9](https://github.com/sahil87/hexokit-site/pull/9)) and X2 ([shll.ai#104](https://github.com/sahil87/shll.ai/pull/104)) are both open PRs at review-pr, not merged — X2's body says "do not merge until X1 is merged and `hexokit.com/shll-ai-redirects.json` answers 200" (it answers 404 today). So **X2 is not yet live**. That does not block this intake or its apply: every sentence this change writes is already true today — see § Why → *Why now is safe*.

**Live probes (2026-09-12)**, all `200` unless noted: `https://hexokit.com/toolkit/`, `/shll/`, `/shll/install/`, `/shll/commands/`, `/shll/standards/principles/`, `/shll/standards/help-dump/` (`301` from the no-slash form), `/wt/commands/`, `/fab-kit/commands/`, `/run-kit/commands/`, `/docs/commands/`, `/versions.json`, `/install`; `https://github.com/sahil87/hexokit-site/blob/main/docs/specs/help-dump-contract.md`, `…/readme-extraction-contract.md`, `…/tree/main/docs/specs`. `https://hexokit.com/install` serves shll's `scripts/install.sh` verbatim **plus a deploy-time composition block** (S5): with **no arguments** it runs `main run-kit` (installs `shll` + HexoKit only) and then prints `shll install  # fab-kit, wt, idea, tu, hop` as the next step; tool arguments pass through unchanged.

**Precedents**: C1 (`ttoa`) — same repo, same surfaces, same `just sync-standards` + drift-guard mechanics; C7 sahil87#1 — the companion-repo cosmetic sweep pattern the user cited (display-name + broken install-line fixes).

## Why

**The problem.** After X2 merges, shll.ai is a redirect host: it pulls nothing, renders nothing, validates nothing, owns nothing. Yet the nine toolkit standards — the documents every toolkit repo is checked against — still say the consuming site is shll.ai in 34 places (by today's grep; the plan's "32" was an earlier count): "shll.ai pulls that output on a schedule", "the capture timestamp is owned by shll.ai", "shll.ai vendors zero image binaries", "point at … `https://shll.ai/<tool>/commands/`", and the machine-anchor contract links point at `github.com/sahil87/shll.ai/…/docs/specs/*`, which X2 turns into tombstones. Nine of those mentions are the intro phrase "[shll toolkit](https://shll.ai)" — the family name D1 replaced with "the HexoKit toolkit". The same stale host also sits in shll's own README, install guide, workflows page, the bootstrap script's header comment, the binary's help text ("meta-CLI for the shll toolkit"), the placed `shll-toolkit` Agent Skill's prose, and `fab/project/`.

**What happens if we don't.** Every producer repo reading the standards after the announce is told to link a site that no longer exists as a site; the readme-extraction rule 8 tells seven repos to emit `https://shll.ai/<tool>/commands/` links that only work via redirect; the two contract links dead-end on tombstone pages. And the two-brand split the rebrand exists to fix survives on the highest-traffic surface of all: `shll --help`, `shll list`, and the Agent Skill every harness loads ("Use when driving any shll toolkit CLI").

**A correctness fix rides along — the install one-liner's prose.** shll's README, `docs/site/install.md`, and `docs/site/workflows.md` all say `curl -fsSL https://shll.ai/install | sh` "installs shll + the whole roster". That is true of shll.ai today and stops being true at X2: shll.ai's `/install` becomes a **byte copy** of hexokit.com's (X2, D4), and hexokit.com's is **product-first** (S5, D10) — no arguments installs `shll` + HexoKit and prints a hint for the rest. So the sentence is wrong on *either* host the moment X2 lands. This change moves the docs to hexokit.com **and** states the product-first semantics correctly: bootstrap, then `shll install` for the rest of the toolkit — the exact two-step the composed script's own hint prints.

**Why this shape.** Content-only, no renames (D14): `shll standards` stays the command, the nine files keep their names and their `docs/site/standards/` home, the `shll-toolkit` skill directory and rc sentinel keep their names, the roster's `Name`/`Formula`/`Repo` stay (R1/R2), and `versions.go`'s ordered two-URL fetch keeps its shll.ai *fallback* (D4 — shll.ai keeps serving `versions.json` forever). The consumer extractor in hexokit-site matches any leading blockquote and pulls `docs/site/**` wholesale, so nothing on the pipeline side changes.

**Why now is safe (no hard gate on X2).** hexokit.com already *is* the consuming site: S3 gave it the pull pipeline, X1's in-flight operator step re-enabled and seeded both Refresh crons on `main`, and the hexokit-site specs (the contract links' new targets) exist on `main` because the repo is a full mirror (D13). Nothing this change writes mentions shll.ai as anything but the manifest fallback, so no statement becomes false whether X2 merges before or after. The plan's X2 → X4 order is kept as a *preference* for the PR merge, not a correctness dependency.

## What Changes

Five areas. A and B are the plan row; C–E are C1's explicit handoff to X4 plus the binary/help prose the same family-name decision (D1) governs. Everything not listed is out of scope (§ Non-goals).

### A. The nine standards documents — 34 `shll.ai` mentions

Files: `docs/site/standards/{principles,help-dump,readme-extraction,skill,update,version,shell-init,install-composition,config-home}.md`.

**A1. The nine intro phrases** (one per file, line 3 in each): `[shll toolkit](https://shll.ai)` → `[HexoKit toolkit](https://hexokit.com/toolkit/)` — verbatim from the X4 row. Surrounding words unchanged ("The ten principles every CLI in the HexoKit toolkit is built against", "How every repo in the HexoKit toolkit structures…", "Where a HexoKit toolkit tool's configuration lives…").

**A2. The `shll` self-links in the three producer-surface intros** (`version.md:3`, `shell-init.md:3`, `update.md:3`): `[shll](https://shll.ai)` → `[shll](https://hexokit.com/shll/)` — shll's own tool page on hexokit.com (probed 200). Link text unchanged.

**A3. `install-composition.md:3`**: `[`shll install`](https://shll.ai)` → `[`shll install`](https://hexokit.com/shll/install/)` — the install guide page (probed 200), which is what "the single composition point" documents.

**A4. Consuming-site prose** — every remaining `shll.ai` that names the site's *role* becomes `hexokit.com`; sentence shapes unchanged:

| File:line | Before | After |
|-----------|--------|-------|
| `principles.md:31` | `README + \`docs/site/\` structure shll.ai pulls and renders` | `… structure hexokit.com pulls and renders` |
| `principles.md:61` | `That dump is pulled by shll.ai and rendered as…` | `That dump is pulled by hexokit.com and rendered as…` |
| `principles.md:65` | `and shll.ai validates every pulled dump against a Zod schema` | `and hexokit.com validates every pulled dump…` |
| `principles.md:117` | `both pulled and rendered on shll.ai without hand-copying` | `both pulled and rendered on hexokit.com without hand-copying` |
| `principles.md:121` | `The shll.ai pull pipeline is live for all seven tools` | `The hexokit.com pull pipeline is live for all seven tools` |
| `principles.md:125` | `render on [shll.ai](https://shll.ai) at \`/shll/standards/…\`` | `render on [hexokit.com](https://hexokit.com) at \`/shll/standards/…\`` |
| `help-dump.md:3` | `[shll.ai](https://shll.ai) pulls that output on a schedule` | `[hexokit.com](https://hexokit.com) pulls that output on a schedule` |
| `help-dump.md:5` | `The consumer side — … — is shll.ai's job` | `… is hexokit.com's job` |
| `help-dump.md:48` | `The capture timestamp is owned by shll.ai` | `The capture timestamp is owned by hexokit.com` |
| `readme-extraction.md:3` | `so [shll.ai](https://shll.ai) can pull and render them mechanically` | `so [hexokit.com](https://hexokit.com) can pull and render them mechanically` |
| `readme-extraction.md:5` | `The consumer mechanics — … — are shll.ai's job` | `… are hexokit.com's job` |
| `readme-extraction.md:17` | `shll.ai vendors zero image binaries` | `hexokit.com vendors zero image binaries` |
| `readme-extraction.md:27` | `` `https://shll.ai/<tool>/commands/` `` | `` `https://hexokit.com/<tool>/commands/` `` (D7: companions keep root slugs; `/wt/commands/`, `/fab-kit/commands/`, `/run-kit/commands/` all probed 200) |
| `skill.md:12` | `needs the repo checked out or a network round-trip to shll.ai` | `… or a network round-trip to hexokit.com` |
| `skill.md:49` | `defer to \`help-dump\` and the [shll.ai commands page](https://shll.ai)` | `defer to \`help-dump\` and the tool's [hexokit.com commands page](https://hexokit.com/toolkit/)` |
| `skill.md:57` | `renders at \`/<tool>/skill\` on shll.ai automatically` | `renders at \`/<tool>/skill\` on hexokit.com automatically` |
| `skill.md:64` | `renders at \`/<tool>/skill/<topic>\` on shll.ai as part of the pulled tree` | `… on hexokit.com as part of the pulled tree` |
| `skill.md:138` | `renders at \`/<tool>/skill\` on shll.ai (it is part of the pulled tree)` | `… on hexokit.com (it is part of the pulled tree)` |

**A5. The two machine-anchor contract links + the specs-tree link** — the shll.ai repo's `docs/specs/*` become tombstones at X2 (its PR body); the hexokit-site mirror carries the live copies (probed 200):

| File:line | Before | After |
|-----------|--------|-------|
| `help-dump.md:5` | `[shll.ai help-dump contract](https://github.com/sahil87/shll.ai/blob/main/docs/specs/help-dump-contract.md)` | `[hexokit-site help-dump contract](https://github.com/sahil87/hexokit-site/blob/main/docs/specs/help-dump-contract.md)` |
| `readme-extraction.md:5` | `[shll.ai README-extraction contract](https://github.com/sahil87/shll.ai/blob/main/docs/specs/readme-extraction-contract.md)` | `[hexokit-site README-extraction contract](https://github.com/sahil87/hexokit-site/blob/main/docs/specs/readme-extraction-contract.md)` |
| `principles.md:125` | `live in the [shll.ai repo's specs](https://github.com/sahil87/shll.ai/tree/main/docs/specs)` | `live in the [hexokit-site repo's specs](https://github.com/sahil87/hexokit-site/tree/main/docs/specs)` |

**Acceptance for A**: `grep -c "shll\.ai" docs/site/standards/*.md` → `0` for every file. The docs/site closure rule (readme-extraction rule 1: intra-family links stay same-directory; outbound links absolute `https://…`) is preserved — every edit swaps one absolute URL for another.

### B. Embedded copies (mechanical, drift-guarded)

Run `just sync-standards` (→ `scripts/sync-standards.sh`) after A and commit the nine regenerated files under `src/cmd/shll/standards/`. `TestStandardsEmbedMatchesCanonical` fails otherwise. `docs/site/skill.md` has no remaining `shll.ai` (C1 flipped it), so `src/cmd/shll/skill/skill.md` is unchanged by the sync — verify with `git status` that the sync touched exactly the nine standards copies.

### C. shll's own live docs — README, install guide, workflows page, bootstrap header

These are the surfaces C1 named for X4. Two kinds of edit: host flips, and the install one-liner's **product-first** semantics.

**C1. `README.md`**

| Line | Before | After |
|------|--------|-------|
| 7 | `every tool in the [shll toolkit](https://shll.ai) (\`run-kit\`, …)` | `every tool in the [HexoKit toolkit](https://hexokit.com/toolkit/) (\`run-kit\`, …)` — tool names stay (roster `Name` until R1) |
| 14 | `curl -fsSL https://shll.ai/install \| sh          # install shll + the whole roster, then auto-wire shell + agent harnesses` | two lines, see block below |
| 21 | `curl -fsSL https://shll.ai/install \| sh -s -- hop wt` | `curl -fsSL https://hexokit.com/install \| sh -s -- hop wt` (subset semantics unchanged — arguments pass through the composition) |
| 28 | `` `curl -fsSL https://shll.ai/install \| sh -s -- --no-agent-setup` `` | `` `curl -fsSL https://hexokit.com/install \| sh -s -- --no-agent-setup` `` |
| 30 | `lives in the [install guide](docs/site/install.md) on [https://shll.ai](https://shll.ai)` | `lives in the [install guide](docs/site/install.md) on [hexokit.com](https://hexokit.com/shll/install/)` |
| 93 | `fetches [shll.ai/versions.json](https://shll.ai/versions.json), the roster + notify-policy authority, once per run` | `fetches [hexokit.com/versions.json](https://hexokit.com/versions.json) (falling back to shll.ai's byte copy), the roster + notify-policy authority, once per run` — matches v0.1.31's actual behavior |
| 272 | `` fetches `shll.ai/versions.json` (or GitHub releases with `--source github`) `` | `` fetches `hexokit.com/versions.json` (shll.ai fallback; or GitHub releases with `--source github`) `` |
| 290 | `(what shll.ai pulls and renders per tool)` | `(what hexokit.com pulls and renders per tool)` |
| 296 | `**Command reference at [shll.ai/shll/commands](https://shll.ai/shll/commands/)** — … On every release, shll's CI exports its CLI help tree as a machine-readable \`help/shll.json\` and publishes it to [shll.ai](https://shll.ai), which renders it at that page. The export is produced by a hidden \`help-dump\` subcommand …` | `**Command reference at [hexokit.com/shll/commands](https://hexokit.com/shll/commands/)** — a browsable, always-current command tree. [hexokit.com](https://hexokit.com) pulls shll's CLI help tree daily as a machine-readable \`help/shll.json\` and renders it at that page. The export is produced by a hidden \`help-dump\` subcommand (internal build tooling, not a user command).` — the "CI publishes" clause was already stale (the push transport was torn down; memory `ci/release-workflow`), fixed in passing since the sentence is being edited anyway |
| 185, 237 | `the manager for the shll toolkit` (sample `shll list` / `shll version` output) | `the manager for the HexoKit toolkit` — follows the `shllSelfDescription` constant flip in § D |

The README install block (lines 13–16) becomes:

```sh
curl -fsSL https://hexokit.com/install | sh      # install shll + HexoKit (run-kit), then auto-wire shell + agent harnesses
shll install                                     # the rest of the toolkit: rk-desktop, fab-kit, wt, idea, tu, hop
exec $SHELL                                      # reload so the shell integration takes effect
```

The heading sentence above it ("From a clean machine to a fully wired toolkit:") stays true of the three-line block. Line 28's "`shll install` ends by wiring the machine automatically" stays — the second line re-runs the same idempotent wiring. The `shll-toolkit` Agent Skill *name* on line 28 stays (D14).

**C2. `docs/site/install.md`**

| Line | Before | After |
|------|--------|-------|
| 3 | `the rest of the [shll toolkit](https://shll.ai)` | `the rest of the [HexoKit toolkit](https://hexokit.com/toolkit/)` |
| 10 | `The recommended path is the one-liner — it bootstraps \`shll\` itself, then hands off to \`shll install\` for the rest of the roster and finishes with \`shll update\`, so the machine converges to *complete and current*: missing tools are installed, and already-installed tools are upgraded.` | `The recommended path is the one-liner — it bootstraps \`shll\` itself, then hands off to \`shll install\` and finishes with \`shll update\`, so what it installs converges to *complete and current*: missing tools are installed, and already-installed tools are upgraded. hexokit.com's copy is **product-first**: with no tool arguments it installs \`shll\` and HexoKit (\`run-kit\`) and prints the one command for the rest of the toolkit; name tools after \`sh -s --\` to pick exactly what you want.` |
| 14 | `curl -fsSL https://shll.ai/install \| sh` | `curl -fsSL https://hexokit.com/install \| sh    # shll + HexoKit` followed by `shll install                                    # the rest of the toolkit` |
| 20 | `curl -fsSL https://shll.ai/install \| sh -s -- hop wt` | `curl -fsSL https://hexokit.com/install \| sh -s -- hop wt` |
| 36 | `` (`curl -fsSL https://shll.ai/install \| sh -s -- --no-agent-setup`) `` | `` (`curl -fsSL https://hexokit.com/install \| sh -s -- --no-agent-setup`) `` |
| 194 | `[shll.ai](https://shll.ai) — the always-current command reference (CI publishes shll's help tree on every release).` | `[hexokit.com/shll/commands](https://hexokit.com/shll/commands/) — the always-current command reference (hexokit.com pulls shll's help tree daily).` |

Line 32's mechanics sentence ("hands off install-then-update: `shll install` with every arg verbatim, then `exec shll update` with the tool names") stays — the composition passes `run-kit` *as* the argument, so the description remains exact.

**C3. `docs/site/workflows.md`**

| Line | Before | After |
|------|--------|-------|
| 3 | `the meta-CLI for the [shll toolkit](https://shll.ai)` | `the meta-CLI for the [HexoKit toolkit](https://hexokit.com/toolkit/)` |
| 10 | `curl -fsSL https://shll.ai/install \| sh                                  # bootstrap: trust + install shll, then install the roster` | `curl -fsSL https://hexokit.com/install \| sh                              # bootstrap: trust + install shll, then HexoKit (run-kit)` followed by a new aligned line `shll install                                                             # the rest of the toolkit` (the existing `shll setup shell` / `exec $SHELL` lines follow, column alignment kept) |
| 98 | `[shll.ai](https://shll.ai) — the always-current command reference.` | `[hexokit.com/shll/commands](https://hexokit.com/shll/commands/) — the always-current command reference.` |

**C4. `scripts/install.sh` header comment (lines 2–5)** — served verbatim at the top of `hexokit.com/install`, so it is a live surface:

```sh
# before
# shll toolkit bootstrap — served at https://shll.ai/install
#
#   curl -fsSL https://shll.ai/install | sh                # converge everything
#   curl -fsSL https://shll.ai/install | sh -s -- hop wt   # converge a subset

# after
# HexoKit toolkit bootstrap — served at https://hexokit.com/install
# (shll.ai/install is a byte copy). hexokit.com appends a product-first
# default at deploy time: with no tool arguments it converges shll + HexoKit
# only; this raw script alone converges everything.
#
#   curl -fsSL https://hexokit.com/install | sh                # shll + HexoKit (raw script: everything)
#   curl -fsSL https://hexokit.com/install | sh -s -- hop wt   # converge a subset
```

Comment-only; no executable line changes. The `main()`-truncation guard, phase lines, and arg passthrough are untouched. The path `scripts/install.sh` is load-bearing (hexokit-site's deploy raw-fetches it from `main`) and stays.

### D. Binary prose — "shll toolkit" → "HexoKit toolkit" in help text and the placed skill

D1 names the family "the HexoKit toolkit"; C1 flagged the `agent_setup.go` prose for X4. No test pins any of these strings (verified: the only `_test.go` files mentioning `shll.ai` are `versions_test.go` and `help_dump_test.go`, both about the manifest fallback / `captured_at` ownership, neither asserting help text). Every `shll toolkit` occurrence in `src/**/*.go` (non-test) flips; the list:

| File | Where | Before → After |
|------|-------|----------------|
| `src/cmd/shll/root.go` | `rootLong` line 1; `Short` | `shll — meta-CLI for the shll toolkit.` → `shll — meta-CLI for the HexoKit toolkit.`; `meta-CLI for the shll toolkit` → `meta-CLI for the HexoKit toolkit`. The per-subcommand lines saying "every shll tool" stay (they mean shll-managed, not the brand) |
| `src/cmd/shll/tools.go` | `shllSelfDescription` const (line 249) + the three doc comments (20, 161, 207) | `the manager for the shll toolkit` → `the manager for the HexoKit toolkit` (renders in `shll list`, `shll version`; README samples on lines 185/237 follow) |
| `src/cmd/shll/setup.go` | `setupLong` (39), `setupAgentLong` (88), `Short` (122), `short` (154), comment (13) | `Wire this machine for the shll toolkit` → `… for the HexoKit toolkit`; `the shll toolkit bootstrap` → `the HexoKit toolkit bootstrap`; `wire this machine for the shll toolkit (shell + agent harnesses)` → HexoKit; `place the shll toolkit skill for agent harnesses` → `place the HexoKit toolkit skill for agent harnesses` |
| `src/cmd/shll/list.go:54` | `Long` | `List the shll toolkit roster shll manages` → `List the HexoKit toolkit roster shll manages` |
| `src/cmd/shll/doctor.go:103` | `Long` | `Verify the shll toolkit is correctly installed and wired.` → `Verify the HexoKit toolkit is correctly installed and wired.` |
| `src/cmd/shll/uninstall.go:90` | `Long` | `Uninstall shll toolkit tools via Homebrew` → `Uninstall HexoKit toolkit tools via Homebrew` |
| `src/cmd/shll/check_updates.go:136` | `Long` | `Check for pending shll toolkit updates` → `Check for pending HexoKit toolkit updates` |
| `src/cmd/shll/standards.go:162–163` | `Short`, `Long` | `read the shll toolkit's binding standards (offline, embedded)` → `read the HexoKit toolkit's binding standards (offline, embedded)`; `Read the shll toolkit's binding, producer-facing standards.` → HexoKit |
| `src/cmd/shll/agent_setup.go` | placed `SKILL.md` body (76, 78) and `agentSkillDescription()` (112) | `# shll toolkit` → `# HexoKit toolkit`; `This machine has the shll toolkit installed.` → `This machine has the HexoKit toolkit installed.`; `Use when driving any shll toolkit CLI or shll itself — ` → `Use when driving any HexoKit toolkit CLI or shll itself — `. **Unchanged**: `skillDirName = "shll-toolkit"` and the frontmatter `name:` (D14), the two-step body, the run-kit proactive paragraph |
| `src/cmd/shll/main.go:1`, `src/internal/changelog/changelog.go:43` | comments | `shll toolkit` → `HexoKit toolkit` |

**Unchanged in `src/`** (accurate as written, D4): `versions.go`'s `manifestURLFallback = "https://shll.ai/versions.json"` and its comments; `check_updates.go:143` `(falls back to https://shll.ai/versions.json)` and the line-200 comment; `help_dump.go:30`'s historical reference; the two test comments.

The description-length cap: `agentSkillDescription()` grows by 3 characters ("HexoKit" vs "shll"); `agentSkillDescriptionMaxLen` is the agentskills.io cap — the existing test that pins it (`TestAgentSetup_Description*`) is the check. Run `go test ./cmd/shll/` after the edit.

### E. `fab/project/` prose (repo-development context, not user-facing)

| File | Before | After |
|------|--------|-------|
| `fab/project/config.yaml:4` | `description: "Meta-CLI for the shll toolkit — update, shell-init, and version across all shll tools."` | `description: "Meta-CLI for the HexoKit toolkit — update, shell-init, and version across all shll tools."` |
| `fab/project/context.md:5` | `\`shll\` is a meta-CLI for the shll toolkit.` | `\`shll\` is a meta-CLI for the HexoKit toolkit.` |
| `fab/project/context.md:7` | `The name comes from the project's landing domain: [\`shll.ai\`](https://shll.ai). The website's repo lives at \`hop shll.ai where\` → \`/home/sahil/code/sahil87/shll.ai\`.` | `The name predates the HexoKit rebrand: it came from the toolkit's original landing domain, shll.ai, which is now a permanent redirect host for [hexokit.com](https://hexokit.com). The site's repo is \`sahil87/hexokit-site\` (\`/home/sahil/code/sahil87/hexokit-site\`); \`sahil87/shll.ai\` holds only the redirect stub.` |
| `fab/project/context.md:29` | `install.sh = curl\|sh bootstrap served at shll.ai/install` | `install.sh = curl\|sh bootstrap served at hexokit.com/install (shll.ai/install is a byte copy)` |
| `fab/project/constitution.md:37` | `Adding a new tool to the shll toolkit requires a shll release.` | `Adding a new tool to the HexoKit toolkit requires a shll release.` |
| `fab/project/constitution.md:41` | `…canonically authored in this repo's own \`docs/site/standards/\` tree and rendered on https://shll.ai.` | `…rendered on https://hexokit.com.` |
| `fab/project/constitution.md` § Governance | `**Version**: 1.1.0 \| … \| **Last Amended**: 2026-07-18` | `**Version**: 1.1.1 \| **Ratified**: 2026-05-09 \| **Last Amended**: 2026-09-12` — patch bump: two factual identity edits, no principle changes |

### Non-goals (explicitly left alone)

- **No rename** of any standard, file, the `shll standards` command, the `shll-toolkit` skill directory / frontmatter `name:`, or the rc sentinel (D14). **Roster** `Name`/`Formula`/`Repo`/`LegacyName` and every `sahil87/tap/run-kit` string: untouched (R1/R2). Tool names in prose (`run-kit`, `rk-desktop`) stay until R1.
- **`versions.go` / `check_updates.go` shll.ai fallback** — kept (D4). The binary's network behavior does not change.
- **The product-first default itself** and the hexokit-site composition block — hexokit-site's (S5). One quirk observed and *recorded, not fixed*: the composition tests `$# -eq 0`, so a **flags-only** invocation (`sh -s -- --no-agent-setup`) bypasses the product-first default and installs the whole roster. The README/install.md flag examples are still correct as passthrough demonstrations; whether the default should key on "no tool names" is a hexokit-site decision (§ Open Questions).
- **`docs/memory/` narrative** (D11) — only present-truth lines change, via hydrate; `log.md` history lines and `Introduced by` records keep "shll.ai".
- **`fab/changes/archive/**`, `fab/plans/**`** — historical (D11).
- **The plan doc's X4 row / Status line** in the run-kit repo (pickup protocol step 4) — a cross-repo operator edit recorded in § Impact, not made from this repo.
- **`docs/site/skill.md`** — already clean after C1; untouched.

## Affected Memory

Present-truth lines only (D11); history/log lines untouched.

- `cli/standards-content`: (modify) the D14 second pass has landed — the nine intros name the HexoKit toolkit, the consumer-site role is hexokit.com, the machine-anchor contract links point at `sahil87/hexokit-site/docs/specs/*`; drop the "change only once shll.ai stops being the consuming site" conditional from the D14 design decision; docs/site closure note's outbound-link statement still holds.
- `cli/standards`: (modify) `shll standards` Short/Long help says "the HexoKit toolkit's binding standards"; embed mechanism unchanged.
- `cli/standards-conformance`: (modify) Policy B row — the README install surface is now the product-first two-step pointing at hexokit.com; `docs/site/install.md` renders at hexokit.com/shll/install.
- `cli/install`: (modify) the bootstrap hand-off is served at hexokit.com/install with hexokit-site's product-first composition (no-arg = `shll install run-kit`); shll.ai/install is a byte copy after X2.
- `cli/commands`: (modify) root `Short`/`Long` and `shllSelfDescription` name the HexoKit toolkit.
- `cli/setup`: (modify) the placed `shll-toolkit` skill's H1/intro/description prose says HexoKit toolkit; directory name and frontmatter `name:` unchanged.
- `cli/skill`: (modify) "renders at shll.ai/shll/skill for free" → hexokit.com.
- `cli/help-dump-contract`: (modify) `captured_at` is stamped by hexokit.com's puller.
- `ci/install-bootstrap`: (modify) "The shll.ai raw-fetch URL contract" → hexokit-site's deploy raw-fetches `scripts/install.sh` from `main` and appends the product-first composition; shll.ai's stub copies hexokit.com's composed file (X2). Header comment wording.
- `ci/release-workflow`: (modify) the pull-based help-tree integration is hexokit-site's scheduled job.

## Impact

**Files touched** (all in this repo):

| Area | Files | Change |
|------|-------|--------|
| Standards (canonical) | `docs/site/standards/*.md` (9) | 34 mention flips (A1–A5) |
| Standards (embedded) | `src/cmd/shll/standards/*.md` (9) | regenerated by `just sync-standards` |
| shll docs | `README.md`, `docs/site/install.md`, `docs/site/workflows.md` | host flips + product-first install prose |
| Bootstrap | `scripts/install.sh` | header comment only |
| Binary prose | `src/cmd/shll/{root,tools,setup,list,doctor,uninstall,check_updates,standards,agent_setup,main}.go`, `src/internal/changelog/changelog.go` | help/description/comment strings |
| Project context | `fab/project/{config.yaml,context.md,constitution.md}` | identity prose; constitution 1.1.0 → 1.1.1 |

**Behavior-contract changes** (all string-level, no logic):
- `shll --help` and the eight affected subcommands' help text change → the pulled `help/shll.json` changes on hexokit.com's next scheduled pull. No schema change; `help_dump_test.go`'s fidelity check compares against live `-h`, so it passes by construction.
- `shll list` / `shll version` render `the manager for the HexoKit toolkit` for the self row (`--json` `description` field included). No known consumer parses it (run-kit consumes `shll check-updates --json` only).
- The placed `~/.agents/skills/shll-toolkit/SKILL.md` (and the `~/.claude/` copy) gets new H1/intro/description text on the next `shll setup agent` / `shll update`. Directory and `name:` unchanged, so harness listings keep the same key.
- Network behavior: **none**. `FetchManifest`'s ordered list, the fallback, timeouts, and `ErrUnavailable` are untouched.

**Tests to run** (scoped first): `cd src && go test ./cmd/shll/` — covers `TestStandardsEmbedMatchesCanonical`, `TestSkillEmbedMatchesCanonical`, the agent-setup description tests, and the help-dump fidelity check. Then `go test ./...`. Then `grep -rn "shll\.ai" docs/site README.md scripts/install.sh src --include='*.md' --include='*.go' --include='*.sh' | grep -v _test.go` — the only survivors must be the `versions.go` fallback constant/comments, `check_updates.go`'s two fallback mentions, `help_dump.go:30`, and `scripts/install.sh`'s "byte copy" parenthetical.

**Constitution check**: Toolkit Standards clause — this change edits `docs/site/standards/`, `README.md`, and help output; `readme-extraction` rules 1 (H1 → blockquote → badges order intact; same-directory intra-family links; absolute outbound links) and 8 (README links `docs/site/install.md` repo-relatively and the commands page absolutely) were read and stay satisfied. I (Security) — no subprocess code touched. II (No state) — nothing. VII (Minimal surface) — no new subcommands or flags. Governance — one patch amendment (1.1.1) for two factual identity lines.

**Merge timing**: recommended after X2 ([shll.ai#104](https://github.com/sahil87/shll.ai/pull/104)) merges, per the plan's X2 → X4 order — but nothing here depends on it (see § Why). If merged first, the only observable difference is that shll's docs stop mentioning shll.ai a few days before shll.ai stops being a site.

**Cross-repo follow-ups (not in this PR)**:
1. run-kit `fab/plans/sahil/26-09-10-hexokit-rebrand.md`: X4 row gets fab change `dk8f` + PR link + status; Status line → "X4 in flight" (pickup protocol step 4).
2. hexokit-site: decide whether the `/install` composition should key its product-first default on "no *tool-name* arguments" instead of `$# -eq 0` (today `sh -s -- --no-agent-setup` installs the whole roster). Not a shll change.
3. After merge, hexokit.com's daily README/docs-site pull picks up the new pages; the next shll release (or R1's) ships the new help/skill strings.

## Open Questions

- None blocking. Two judgment calls the operator may want to veto via `/fab-clarify`: (a) Assumption 7 — flipping the *binary's* help/description/placed-skill prose ("shll toolkit" → "HexoKit toolkit") is a scope extension over the plan row's `docs/site/standards/*.md`; it follows D1 and C1's explicit flag but changes `help/shll.json` and the Agent Skill text. (b) Assumption 8 — the constitution's two identity lines and the 1.1.1 patch bump.
- Cross-repo, recorded above: the hexokit-site `$# -eq 0` flags-only quirk.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | No standard, file, command, skill directory, or rc sentinel is renamed; roster fields and `sahil87/tap/run-kit` strings untouched | Plan D14 (Confirmed) and the X4 row's last sentence; R1/R2 own the renames | S:95 R:60 A:95 D:95 |
| 2 | Certain | The nine intro phrases become exactly `[HexoKit toolkit](https://hexokit.com/toolkit/)` | Verbatim from the X4 row and D14; `/toolkit/` probed 200; C1's skill bundle already uses this phrase | S:90 R:90 A:90 D:90 |
| 3 | Certain | Consuming-site prose → `hexokit.com`; the two contract links and the specs-tree link → `github.com/sahil87/hexokit-site/…/docs/specs/…` | The X4 row names the prose flip; the hexokit-site mirror carries the specs (probed 200) and X2 tombstones the shll.ai copies | S:75 R:90 A:85 D:75 |
| 4 | Confident | `[shll](https://shll.ai)` → `https://hexokit.com/shll/`; `[`shll install`](https://shll.ai)` → `https://hexokit.com/shll/install/`; `skill.md:49`'s commands-page link → `https://hexokit.com/toolkit/` | The X4 row does not name per-link targets; each chosen target is the page that describes the linked thing, all probed 200; trivially re-pointed | S:50 R:90 A:75 D:55 |
| 5 | Confident | Scope extends past `docs/site/standards/` to shll's own live docs (README, `install.md`, `workflows.md`, `scripts/install.sh` header) | C1's § Non-goals hands exactly these to X4; the user's prompt says "install link/references" and "equivalent consumer docs"; nothing else in the plan covers them | S:60 R:85 A:80 D:60 |
| 6 | Confident | Install one-liner prose adopts the product-first two-step (the hexokit.com/install curl bootstrap, then `shll install`) and stops claiming "the whole roster" | Live `hexokit.com/install` composition verified (no args → `main run-kit` + the `shll install` hint); X2 makes shll.ai/install a byte copy; no `all` target exists in `resolveTargets`; the two-step is the script's own hint | S:45 R:85 A:80 D:60 |
| 7 | Confident | Binary help/description/comment strings and the placed-skill H1/intro/description flip "shll toolkit" → "HexoKit toolkit"; `skillDirName` and `name:` stay | D1 names the family; C1 flagged `agent_setup.go` prose for X4; no test pins the strings; help-dump fidelity is against live `-h` | S:45 R:85 A:70 D:55 |
| 8 | Confident | `fab/project/` prose flips, incl. constitution §Tool Roster / §Toolkit Standards identity lines with a 1.1.1 patch bump and Last Amended 2026-09-12 | C1 listed `context.md`/`config.yaml` for X4; the constitution's `https://shll.ai` render URL is a factual line, not a principle; one-line revert | S:35 R:90 A:75 D:55 |
| 9 | Certain | `versions.go` / `check_updates.go` shll.ai *fallback* mentions stay | D4 (Confirmed): shll.ai serves `versions.json` forever; the fallback is live behavior, so its description must keep naming it | S:70 R:90 A:90 D:85 |
| 10 | Confident | No hard merge gate on X2; plan order X2 → X4 kept as a preference | hexokit.com is already the pulling/rendering site (S3 + X1's cron seed); the hexokit-site specs exist on `main`; nothing written becomes false either way | S:55 R:80 A:80 D:65 |
| 11 | Certain | README line 296's "CI publishes to shll.ai" clause is replaced with the pull-based truth | Memory `ci/release-workflow` records the push teardown; the sentence is being edited for the host flip anyway | S:60 R:95 A:95 D:90 |
| 12 | Certain | Memory: present-truth lines only; `log.md` and `Introduced by` history untouched | D11 (historical text not renamed); FKF present-truth style | S:70 R:85 A:85 D:80 |
| 13 | Certain | Cross-repo items (plan row update in run-kit; hexokit-site `$# -eq 0` quirk) are recorded as follow-ups, not done here | Different repos; C1 handled its plan-row update the same way | S:80 R:90 A:90 D:90 |

13 assumptions (7 certain, 6 confident, 0 tentative, 0 unresolved).

# Intake: HexoKit banner and policy (D14 first pass)

**Change**: 260911-ttoa-hexokit-banner-and-policy
**Created**: 2026-09-11

## Origin

One-shot `/fab-new` invocation, driven by the cross-repo HexoKit rebrand plan (run-kit repo, `fab/plans/sahil/26-09-10-hexokit-rebrand.md`), row **C1** — the first row of Phase 1 and the gating change for C3, C4, and the six C7 companion-repo changes.

> Per fab/plans/sahil/26-09-10-hexokit-rebrand.md row C1 (hexokit-banner-and-policy, shll repo), first row of Phase 1: implement the D14 first pass. Do NOT rename the standards (D14) and leave the roster's Name/Formula/Repo fields untouched -- those move together with R1/R2 in the deferred Phase 3, never ahead of it. Content-only changes: (1) the readme-extraction standard's mandated §2 blockquote -> "Part of [HexoKit](https://hexokit.com) -- see all projects there"; (2) the install-composition standard's Policy B install-docs location -> hexokit.com; (3) the config-home standard's example path -> ~/.config/hexokit; (4) the versions.json URL constant -> hexokit.com (keep shll.ai as a fallback); (5) the `shll skill` bundle prose. Leave every OTHER shll.ai / "shll toolkit" mention alone -- those wait for X4 in Phase 2. Per the plan's pickup protocol, run `shll standards` and read readme-extraction, install-composition, config-home, update before editing. This is the gating change for C3, C4, and the six C7 companion-repo changes -- keep it tight and correct.

**Pickup protocol followed**: `shll standards` run; `readme-extraction`, `install-composition`, `config-home`, and `update` read in full from `docs/site/standards/`. Plan decisions D1–D4, D13, D14 are confirmed (Certain, not re-opened); the Phase 1 gate is open (S4 landing accepted 2026-09-10). Live probes on 2026-09-11: `https://hexokit.com/versions.json` → 200 `application/json` (schema 1, carries the `run-kit` row keyed by roster Name); `https://shll.ai/versions.json` → 200; `https://hexokit.com/install` → 200; `https://hexokit.com/toolkit/` → 200.

**Decisions inherited from the plan (D14, Confirmed)**: the standards are *not* renamed — `shll standards` stays the command, the nine documents stay at `docs/site/standards/` and embedded in the binary, the `shll-toolkit` skill dir and rc sentinel stay. Only content changes, in two passes: this change (C1) edits the mandated README blockquote and Policy B's install-docs location (plus the three smaller items the row lists); **X4** (Phase 2, after shll.ai becomes a redirect host) flips the ~32 `shll.ai` mentions that name the *consuming site* and the nine "[shll toolkit](https://shll.ai)" intro phrases. The consumer extractor (`BLOCKQUOTE_RE` in hexokit-site's `extract-readme.ts`) matches any leading blockquote, so the banner text change is free on the pipeline side.

## Why

**The problem.** The product is being renamed from run-kit to HexoKit (plan Approach B: product rename, `rk` substrate kept), and hexokit.com is live. The most visible cross-repo brand surface is the README blockquote the `readme-extraction` standard mandates as the first line under the H1 in all seven toolkit repos — today it reads "Part of the [shll toolkit](https://shll.ai)". Every downstream Phase 1 change (C3 run-kit brand surfaces, C4 home migration, C7's six companion-repo banner sweeps) is *checked against the standards' text* (plan § Constitution mapping → Toolkit Standards). Until the standards say HexoKit, those changes have nothing conformant to converge on.

**What happens if we don't.** C3/C4/C7 stall, or land against stale standards and reintroduce the two-brand split (hexokit.com product, shll.ai toolkit banner) that the rebrand exists to correct. Separately, once shll.ai flips to a redirect stub at X2, every shipped `shll` binary fetching `https://shll.ai/versions.json` depends on X2's byte-copy endpoint forever; moving the primary to hexokit.com now — while keeping shll.ai as a fallback — lets the next shll release already read the canonical manifest, with the old host as the safety net for the cutover window.

**Why this shape.** Content-only, four standards documents + one binary constant + one skill page, no renames: the plan deliberately keeps the standards' names, the `shll standards` command, and the roster's `Name`/`Formula`/`Repo` untouched because those fields are *runtime-coupled* to the tap formula and the GitHub repo (`shll install`, `doctor`, `check-updates` match the manifest by roster `Name`) and move only with R1/R2 in Phase 3. Renaming a standard buys nothing (`shll` is the toolkit manager, a tool name like `hop`) and the blockquote is the one line whose staleness is visible on seven repo pages.

## What Changes

Five surfaces, all content edits; plus the mechanical embed sync and one fallback-aware fetch loop. Everything not listed here is out of scope (see § Non-goals).

### 1. `readme-extraction` standard — the mandated §2 blockquote

`docs/site/standards/readme-extraction.md`, rule **1. Head**, the fenced example that is "this exact line in all seven repos":

```markdown
# before
> Part of the [shll toolkit](https://shll.ai) — see all projects there.

# after
> Part of [HexoKit](https://hexokit.com) — see all projects there.
```

Exact text per plan D14: the article "the" is dropped, the em-dash (` — `) and trailing period are kept (the user's prompt wrote `--` as an ASCII stand-in for the em-dash the standard already uses). The surrounding prose ("canonical toolkit blockquote", ordering rule H1 → blockquote → badges) is unchanged. The page's other `shll.ai` mentions (intro "[shll toolkit](https://shll.ai)", "so [shll.ai] can pull and render", "shll.ai vendors zero image binaries", rule 8's `https://shll.ai/<tool>/commands/`, the contract link) are **consuming-site** mentions and wait for X4.

**shll's own README conforms to its own standard.** `README.md` line 3 carries the mandated line today and the constitution's Toolkit Standards clause says this repo MUST itself conform, so the same one-line flip lands in `README.md`:

```markdown
> Part of [HexoKit](https://hexokit.com) — see all projects there.
```

No other README line changes (its install one-liners, the "[shll toolkit](https://shll.ai)" intro, the `shll.ai/versions.json` table cell, and the `shll.ai/shll/commands` link are X4/S5 territory).

### 2. `install-composition` standard — Policy B's install-docs location

`docs/site/standards/install-composition.md`. Policy B says install documentation is centralized in one place; that place becomes hexokit.com. Four sentences flip, each a Policy B *location* statement (the intro's "[shll toolkit](https://shll.ai)" / "[`shll install`](https://shll.ai)" phrases on line 3 are intro/site mentions → X4):

| Where | Before | After |
|-------|--------|-------|
| Line 5, the two-halves summary | `**Policy B** — install documentation is centralized on shll.ai.` | `**Policy B** — install documentation is centralized on hexokit.com.` |
| Line 7, the scope carve-out | `because it, together with shll.ai, *is* the centralized install documentation the policy points at` | `because it, together with hexokit.com, *is* the centralized install documentation the policy points at` |
| Line 31, the Policy B MUST NOT bullet | `They link to [https://shll.ai](https://shll.ai) for install steps — the curl bootstrap or `shll install`.` | `They link to [https://hexokit.com](https://hexokit.com) for install steps — the curl bootstrap or `shll install`.` |
| Line 54, Verifying conformance | `links to https://shll.ai instead of carrying per-formula `brew install` lines.` | `links to https://hexokit.com instead of carrying per-formula `brew install` lines.` |

The `shll standards` roster line for this standard is the same Policy B location statement rendered in the binary, so it flips with it — `src/cmd/shll/standards.go`, `standardsRoster` entry `install-composition`:

```go
Description: "No sibling `depends_on` between toolkit formulas; probe siblings at runtime; install docs centralized on hexokit.com",
```

No test pins the description text (verified: no `centralized on` in any `_test.go`). `hexokit.com/install` already serves the bootstrap script (200), so the flipped text is accurate on merge; S5 later changes that script's *default* tool set, not its location.

### 3. `config-home` standard — the example config path

`docs/site/standards/config-home.md`, § Conformance, the `run-kit` row's example path — the one D9/C4 will implement:

```markdown
# before
- **`run-kit` is adopting**: its config-consolidation plan moves `~/.rk/settings.yaml` to `$HOME/.config/run-kit/config.yaml` under this standard.

# after
- **`run-kit` is adopting**: its config-consolidation plan moves `~/.rk/settings.yaml` to `$HOME/.config/hexokit/config.yaml` under this standard.
```

Only the path flips. The tool-name example on line 16 ("`<tool-name>` is the full tool name (`run-kit`, not `rk`) — the same one-string identity the update standard requires across repo, formula, and binary") stays: it is tied to the repo/formula/binary identity that moves only at R1/R2, and flipping it now would make the standard assert a name the formula does not yet carry. The "run-kit's `RK_PORT`" / "run-kit's `RK_AUTO_NAME`" prose (lines 33–34) is product-name prose (C3/X4) and substrate identifiers (never renamed).

### 4. `versions.json` URL constant — hexokit.com primary, shll.ai fallback

`src/internal/versions/versions.go`. Today a single constant feeds a single package-level seam:

```go
const manifestURLDefault = "https://shll.ai/versions.json"
var manifestURL = manifestURLDefault
```

After — an ordered list, primary first, old host as fallback:

```go
// manifestURLDefault is the production hexokit.com versions manifest — the
// roster + policy authority for the `--released` backend.
const manifestURLDefault = "https://hexokit.com/versions.json"

// manifestURLFallback is the previous host, kept as the second attempt while
// shll.ai serves a byte copy of the manifest through the site cutover.
const manifestURLFallback = "https://shll.ai/versions.json"

// manifestURLs is the ordered fetch list (package-level seam; tests collapse it
// to a single httptest URL via SetTransportForTest).
var manifestURLs = []string{manifestURLDefault, manifestURLFallback}
```

`FetchManifest` behavior:

- Iterate `manifestURLs` in order. Each attempt keeps today's per-request `requestTimeout` (10 s) context and today's failure classification — transport error, timeout, non-200, JSON decode failure, unsupported schema all count as "this URL unavailable" and move on to the next.
- First successful decode with `Schema == manifestSchema` returns immediately; the fallback is never contacted when the primary succeeds (no extra request on the happy path).
- When every URL fails, return an error wrapping `ErrUnavailable` that names the last attempt's failure (the existing `%w: …` shape), so `shll check-updates` keeps its single degradation point and its exit-1 diagnostic unchanged.
- No retries within a URL, no caching (Constitution II). Worst case is two sequential 10 s timeouts.

Test seam: `SetTransportForTest(url, client)` keeps its exported signature (consumed by `src/cmd/shll/check_updates_test.go`) and now sets `manifestURLs = []string{url}`, restoring the previous slice on the returned func — existing tests are byte-for-byte unchanged. New package-internal tests in `versions_test.go` set `manifestURLs` to two `httptest.Server` URLs and assert: primary 500 → fallback 200 → manifest returned; primary unreachable (closed server) → fallback used; both fail → `ErrUnavailable`; primary 200 → fallback server receives zero requests.

Surfaces that *describe* the constant flip with it (they state the URL/host literally):

- `src/cmd/shll/check_updates.go` line 142 long-help: `--source released   latest versions + notify policy from https://shll.ai/versions.json` → `from https://hexokit.com/versions.json (falls back to shll.ai)`.
- `src/cmd/shll/check_updates.go` line 26 `sourceFlagUsage`: `released (shll.ai versions manifest + notify policy; the default)` → `released (hexokit.com versions manifest + notify policy; the default)`.
- Package/const doc comments in `versions.go` that say "shll.ai versions manifest" → "hexokit.com versions manifest".

The manifest's tool map stays keyed by roster `Name` — the live hexokit.com manifest already carries the `run-kit` row (S3's `envelope` carry-over), so `check-updates` matching is unaffected; the `hexokit` row arrives with R1, not here.

### 5. `shll skill` bundle prose

`docs/site/skill.md` — the page `shll skill shll` serves in-process (embedded via `scripts/sync-standards.sh`, drift-guarded by `TestSkillEmbedMatchesCanonical`). Two lines:

```markdown
# line 3 — before
The agent skill bundle for **shll** — the meta-CLI that installs, updates, wires, and inspects the [shll toolkit](https://shll.ai) (`run-kit`, `rk-desktop`, `fab-kit`, `wt`, `idea`, `tu`, `hop`). …
# after
The agent skill bundle for **shll** — the meta-CLI that installs, updates, wires, and inspects the [HexoKit toolkit](https://hexokit.com/toolkit/) (`run-kit`, `rk-desktop`, `fab-kit`, `wt`, `idea`, `tu`, `hop`). …

# line 22 — before
… `--source released` (default, shll.ai versions manifest with notify policy) or `--source github` …
# after
… `--source released` (default, hexokit.com versions manifest with notify policy) or `--source github` …
```

The phrase "[HexoKit toolkit](https://hexokit.com/toolkit/)" mirrors the wording X4 will apply to the nine standards intros, so the bundle and the standards converge on one phrase. Tool names in the parenthetical stay (`run-kit` is still the roster Name until R1).

### 6. Embed sync (mechanical)

After editing `docs/site/standards/*.md` and `docs/site/skill.md`, run `just sync-standards` (→ `scripts/sync-standards.sh`) and commit the refreshed copies under `src/cmd/shll/standards/` and `src/cmd/shll/skill/skill.md`. `TestStandardsEmbedMatchesCanonical` and `TestSkillEmbedMatchesCanonical` fail otherwise.

### Non-goals (explicitly left for later rows)

- **No rename** of any standard, file, or the `shll standards` command (D14). **Roster** `Name`/`Formula`/`Repo`, `LegacyName`, and every `sahil87/tap/run-kit` string: untouched (→ R1/R2).
- Every other `shll.ai` / "shll toolkit" mention → **X4**: the nine standards intros ("[shll toolkit](https://shll.ai)"), the consuming-site mentions in `readme-extraction` ("shll.ai pulls and renders", "shll.ai vendors zero image binaries", `https://shll.ai/<tool>/commands/`, the contract link), `update`'s "[shll](https://shll.ai)", `docs/site/install.md` (5 mentions), README body lines 7/14/21/272/296, `fab/project/config.yaml` description, `fab/project/context.md`.
- `src/cmd/shll/agent_setup.go`'s placed `shll-toolkit` skill (H1 "# shll toolkit", "This machine has the shll toolkit installed", the roster-built description "Use when driving any shll toolkit CLI") — the dir name and rc sentinel stay per D14; the prose is a "shll toolkit" mention outside C1's five items. Flagged for the X4 row's scope (it is not currently listed there).
- The `hexokit.com/install` script's *default tool set* (S5) and the manifest's `hexokit` row (R1).
- `docs/memory/` narrative history (D11) — only present-truth lines change, via hydrate.

## Affected Memory

- `cli/standards-content`: (modify) the `readme-extraction` mandated blockquote text, `install-composition` Policy B's centralized location (hexokit.com), and `config-home`'s example path — the documents' present-truth content this file summarizes.
- `cli/standards`: (modify) the `install-composition` roster `Description` line now says "centralized on hexokit.com".
- `cli/standards-conformance`: (modify) shll's own README blockquote conformance line reflects the new mandated text.
- `internal/versions`: (modify) manifest fetch is now an ordered two-URL list (hexokit.com primary, shll.ai fallback) with the same `ErrUnavailable` single-degradation contract; `SetTransportForTest` collapses the list.
- `cli/check-updates`: (modify) `--source released` default now names the hexokit.com manifest with shll.ai fallback; help text wording.
- `cli/skill`: (modify) the self-bundle (`docs/site/skill.md`) intro names the HexoKit toolkit and the hexokit.com manifest.

## Impact

**Files touched** (all in this repo):

| Area | File | Change |
|------|------|--------|
| Standards (canonical) | `docs/site/standards/readme-extraction.md` | blockquote example line |
| | `docs/site/standards/install-composition.md` | 4 Policy B location sentences |
| | `docs/site/standards/config-home.md` | 1 example path |
| Standards (embedded) | `src/cmd/shll/standards/{readme-extraction,install-composition,config-home}.md` | regenerated by `just sync-standards` |
| Skill bundle | `docs/site/skill.md`, `src/cmd/shll/skill/skill.md` | 2 lines + regenerated copy |
| Binary | `src/internal/versions/versions.go` | URL constants, ordered list, fetch loop, seam |
| | `src/internal/versions/versions_test.go` | fallback tests |
| | `src/cmd/shll/check_updates.go` | 2 help strings |
| | `src/cmd/shll/standards.go` | 1 roster Description |
| README | `README.md` | line 3 blockquote |

**Behavior contract change**: `shll check-updates --source released` (the default) now GETs `https://hexokit.com/versions.json` first and `https://shll.ai/versions.json` only if the first attempt is unavailable. Exit codes, `--json` schema, and the `ErrUnavailable` diagnostic are unchanged. Consumers exec'ing `shll check-updates --json` (run-kit) see no difference.

**Tests to run** (scoped first): `cd src && go test ./internal/versions/ ./cmd/shll/` — covers the new fallback tests, `TestStandardsEmbedMatchesCanonical`, `TestSkillEmbedMatchesCanonical`, and the check-updates seam users. Then `go test ./...`.

**Cross-repo follow-ups (not in this PR)**: the plan doc's C1 row (run-kit repo) gets its fab change ID / PR / status per the pickup protocol; C3, C4, and the six C7 changes unblock on merge and are checked against the revised standards text.

**Constitution check**: Toolkit Standards clause — this change edits `docs/site/standards/` and `README.md`, and the README stays conformant with the revised `readme-extraction` (H1 → blockquote → badges order intact). I (Security) — no new subprocess code. II (No state) — no caching added to the fallback. VII (Minimal surface) — no new subcommands or flags.

## Open Questions

- None blocking. The one judgment call the operator may want to veto is the shll `README.md` blockquote flip (Assumption 3) — it follows from the constitution's self-conformance clause rather than from the C1 row text.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | No standard, file, or command is renamed; roster `Name`/`Formula`/`Repo` untouched | Plan D14 (Confirmed) and the user's explicit instruction; R1/R2 own those moves | S:95 R:60 A:95 D:95 |
| 2 | Certain | Blockquote text is exactly `> Part of [HexoKit](https://hexokit.com) — see all projects there.` — em-dash and trailing period kept, article "the" dropped | Plan D14 gives the text verbatim; the current line uses ` — `, the prompt's `--` is an ASCII stand-in | S:80 R:90 A:90 D:85 |
| 3 | Confident | shll's own `README.md` line 3 blockquote flips in this change | Constitution Toolkit Standards: this repo MUST itself conform; it is the mandated line, not a consuming-site mention; one-line, trivially reversible | S:55 R:90 A:80 D:65 |
| 4 | Confident | Policy B flips = lines 5, 7, 31, 54 of `install-composition.md` + the `standards.go` roster Description; line 3's intro phrases stay for X4 | Those five are the *location* statements the C1 row names; the intro phrases are the "[shll toolkit](https://shll.ai)" set X4 owns | S:70 R:85 A:80 D:70 |
| 5 | Confident | `config-home.md`: only the `$HOME/.config/run-kit/config.yaml` example path flips; the line-16 `(run-kit, not rk)` tool-name example and `RK_*` prose stay | The path is what C4 implements (D9); the tool-name example is tied to the repo/formula/binary identity that moves at R1/R2 | S:75 R:90 A:75 D:60 |
| 6 | Confident | Fallback = ordered URL list, sequential attempts, same per-attempt 10 s timeout and failure classes, last error wraps `ErrUnavailable`; `SetTransportForTest` collapses the list to one URL | Smallest change preserving the single-degradation contract and the exported seam; codebase pattern (package-level swappable seams) | S:60 R:80 A:85 D:60 |
| 7 | Confident | `check_updates.go` help strings (line 26 usage, line 142 long help) and `versions.go` comments flip to hexokit.com, mentioning the fallback in the long help only | They state the constant's host literally; leaving them would make `--help` describe a URL the binary no longer fetches first | S:60 R:90 A:80 D:70 |
| 8 | Confident | Skill bundle intro becomes "[HexoKit toolkit](https://hexokit.com/toolkit/)"; line 22 says "hexokit.com versions manifest" | `/toolkit/` is live (probed 200); mirrors X4's planned phrase so bundle and standards converge | S:70 R:90 A:75 D:65 |
| 9 | Certain | `agent_setup.go` placed-skill prose and the `shll-toolkit` dir name are untouched (flagged for X4's scope) | D14: skill dir + rc sentinel stay; the user's "leave every other shll toolkit mention" instruction | S:80 R:85 A:85 D:80 |
| 10 | Certain | Every other `shll.ai` / "shll toolkit" mention (standards intros, consuming-site sentences, `install.md`, README body, `context.md`, `config.yaml`) is left for X4 | Explicit user instruction and plan D14's two-pass split | S:95 R:90 A:90 D:90 |

10 assumptions (4 certain, 6 confident, 0 tentative, 0 unresolved).

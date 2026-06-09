# Intake: README clarity pass — dedupe trust-tap, orient outsiders

**Change**: 260609-oghj-readme-clarity-pass
**Created**: 2026-06-09
**Status**: Draft

## Origin

> check my README.md and suggest improvements — then: "Work on 1, and 2 for now"

Conversational. The user invoked `/fab-new` asking for a README review. The agent declined to
create a change for the *review* (read-only critique), delivered a 6-point review inline, and the
user selected items **1** and **2** to act on. Two design decisions were then resolved via a
clarifying question before this intake was written (see Assumptions #1 and #2):

- **Trust-tap canonical home** → the `#### --trust-tap` command subsection. Quick-start and
  Troubleshooting trim to a one-liner + link.
- **Tool-gloss depth** → a one-line "what it's for" gloss per roster tool (not a bare toolkit
  sentence, not a full paragraph each).

This change is a pure documentation edit to `README.md`. It does not touch `cmd/` or `internal/`,
and it does not change any command behavior — only how that behavior is described.

## Why

**Problem 1 — `--trust-tap` is explained three times.** The same concept appears in (a) the
Quick-start callout (current line 31), (b) the dedicated `#### --trust-tap` command subsection
(93–109), and (c) Troubleshooting (177–192). A reader hits a wall of Homebrew tap-trust prose
before they understand what shll *is*. Triplication also means three places to keep in sync — the
recent README-contract work (`260608-xgc0`) already had to reason about these sections
individually.

**Problem 2 — the README assumes you already know the toolkit.** Line 5 names six tools
(`fab-kit`, `rk`, `tu`, `hop`, `wt`, `idea`) with no hint of what any of them *do*. The shell-init
table (122–129) describes what each contributes *to your shell*, but never what the tool is *for*.
A cold visitor — exactly who a public README serves — can't tell whether this toolkit is relevant
to them.

**Consequence if unfixed.** The README's strong value prop ("one command to install/update/wire the
toolkit") is buried under trust-tap detail and gated behind unexplained tool names. First-time
readers bounce.

**Approach over alternatives.** Single canonical explanation + pointers (the standard docs pattern)
beats deletion (the trust-tap detail is genuinely useful where someone hits the warning) and beats
leaving it triplicated. One-line glosses beat a full per-tool paragraph (this is shll's README, not
each tool's) and beat a single vague toolkit sentence (which wouldn't tell a reader if *their* need
is covered).

## What Changes

Single file: `README.md`. Two independent edits. No behavior, flag, or command-surface changes —
every command, flag, and example stays valid. This is the scoped subset of a larger 6-point review;
items 3–6 (license/CONTRIBUTING/requirements sections, `version` example staleness, Quick-start
`all`-path note, "roster" term definition) are explicitly **out of scope** for this change.

### 1. Dedupe the `--trust-tap` explanation

Canonical home: the `#### --trust-tap — resolve the Homebrew tap-trust warning` command subsection
(currently lines 93–109). It keeps the full explanation: the `brew trust` ceremony, the
`HOMEBREW_REQUIRE_TAP_TRUST=1` export, composition with `--print`/`--uninstall`, the graceful
degradation when `brew trust` is unavailable, and how `--uninstall` does not `brew untrust`.

The other two mentions are trimmed to a one-liner + intra-page link to the canonical subsection:

- **Quick-start callout (line 31)** — currently a full paragraph explaining what `--trust-tap` does,
  the `HOMEBREW_REQUIRE_TAP_TRUST=1` side effect on *other* taps, and the drop-the-flag alternative.
  Reduce to roughly one sentence: that `--trust-tap` records genuine Homebrew trust for
  `sahil87/tap` and that you can drop it, linking to the canonical subsection (and keeping the
  existing Troubleshooting link for the lighter alternatives). Example target:

  > `--trust-tap` records genuine Homebrew trust for `sahil87/tap` so brew stops nagging about
  > non-official taps — drop it (`shll shell-setup`) to leave brew's tap-trust posture unchanged.
  > See [`--trust-tap`](#--trust-tap--resolve-the-homebrew-tap-trust-warning) for what it does and
  > the side effects.

- **Troubleshooting "Recommended fix" (177–192)** — currently re-explains what `brew trust` and
  `HOMEBREW_REQUIRE_TAP_TRUST=1` do. Reduce the *recommended fix* prose to the command plus a
  pointer to the canonical subsection; **keep** the "Lighter alternatives" env-var table and the
  "shll will not set these for you" note (those are troubleshooting-specific and not duplicated in
  the command subsection). The existing link from Troubleshooting to the subsection already exists
  (line 183) — preserve it.

**Link-integrity constraint**: the canonical subsection's heading anchor
`#--trust-tap--resolve-the-homebrew-tap-trust-warning` is already referenced from Quick-start and
Troubleshooting. Any new links must use that exact anchor. The README-contract change
(`260608-xgc0`) verified these fragment links resolve on the rendered shll.ai page — do not rename
the heading, and verify all `#--trust-tap…` anchors still match after editing.

### 2. One-line gloss per roster tool

Give a cold reader a one-line "what it's for" per tool. Placement: add a **"What it's for"**
column to the existing shell-init contribution table (currently 122–129) so each tool is described
once, in one place — no second list to keep in sync.
<!-- clarified: placement = new column in the shell-init table (not a separate list, not both) — user confirmed -->
The table's caption shifts from "what it adds to your shell" to cover both "what it is" and "what
it adds." Draft glosses (refine against each tool's own README during apply):

| Tool | One-line gloss (what it's for) |
|------|--------------------------------|
| `hop` | fast directory navigation / bookmarks (`cd` on steroids) |
| `wt`  | git worktree manager — create, switch, and clean up worktrees |
| `tu`  | terminal/task utility *(verify exact purpose against tu's README)* |
| `idea` | quick idea / note capture from the terminal *(verify)* |
| `rk`  | run-kit — tmux/terminal orchestration & visual windows *(verify)* |
| `fab-kit` | `fab` — spec-driven change workflow (this repo's own pipeline) |

The glosses for `tu`, `idea`, `rk` are placeholders — the apply step MUST verify each against the
linked per-tool repo (Reference section, lines 200–206) rather than guessing. Do not ship a wrong
description of a sibling tool.

Optionally (low priority, same edit): line 5's bare tool-name list could gain a half-clause framing
the toolkit ("small, composable Unix-y CLIs"), but the per-tool glosses are the required deliverable.

## Affected Memory

- `cli/commands`: (modify) only if the README is treated as a documented surface in memory — likely
  **no memory change**: this is a docs-presentation edit, not a behavior change. No command,
  flag, or output changes. Hydrate may confirm no memory update is needed.

## Impact

- **Files**: `README.md` only.
- **Code/APIs**: none. No `cmd/`, no `internal/`, no flags, no command behavior.
- **Cross-references**: intra-page anchor links to `#--trust-tap--resolve-the-homebrew-tap-trust-warning`
  and the existing `docs/site/install.md` / `#tap-sahil87tap-is-allowed-by-default-warning` links
  must continue to resolve.
- **shll.ai extraction contract**: `260608-xgc0` made the README conform to an extraction contract
  (some sections are pulled into shll.ai). Apply MUST NOT reintroduce content into denylisted
  sections or break the contract. Re-read that change's intake/memory before editing if the
  extraction boundaries are unclear.
- **Sibling-tool accuracy**: the tool glosses describe *other* repos — verify against their READMEs.

## Open Questions

- Should the per-tool glosses live as a new column in the existing shell-init table, or as a
  separate short list? (Either satisfies the requirement; apply picks whichever reads best and keeps
  one source of truth.)
- Do `tu`, `idea`, and `rk` have concise canonical one-liners in their own READMEs to reuse verbatim?

## Clarifications

### Session 2026-06-09

| # | Action | Detail |
|---|--------|--------|
| 7 | Confirmed | Glosses placed as a new "What it's for" column in the shell-init table (over separate-list / both) |
| 6 | Confirmed | Verify-at-apply plan for `tu`/`idea`/`rk` glosses endorsed — stays Tentative by design (agent does not yet know the siblings' purpose) |

### Session 2026-06-09 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 3 | Confirmed | — |
| 4 | Confirmed | — |
| 5 | Confirmed | — |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Canonical home for the full `--trust-tap` explanation is the `#### --trust-tap` command subsection; Quick-start + Troubleshooting trim to one-liner + link | User chose "Command subsection is canonical" over "Troubleshooting is canonical" in the clarifying question | S:98 R:80 A:90 D:95 |
| 2 | Certain | Tool orientation is a one-line gloss per roster tool (not a bare toolkit sentence, not a full paragraph each) | User chose "One-line gloss per tool" over the sentence-only and both options | S:98 R:80 A:85 D:95 |
| 3 | Certain | Scope is README.md only — items 3–6 of the review (license/CONTRIBUTING/requirements, version-example staleness, all-path note, roster definition) are excluded | Clarified — user confirmed | S:95 R:85 A:90 D:85 |
| 4 | Certain | No memory/spec change required — this is a docs-presentation edit with no behavior change | Clarified — user confirmed | S:95 R:80 A:80 D:75 |
| 5 | Certain | Keep Troubleshooting's "Lighter alternatives" env-var table and "shll will not set these" note; only the recommended-fix prose is trimmed | Clarified — user confirmed | S:95 R:75 A:80 D:78 |
| 6 | Tentative | `tu` / `idea` / `rk` glosses are placeholders; apply verifies each against the per-tool repo before shipping | Confirmed-as-Tentative — verify-at-apply plan endorsed; agent still lacks confident knowledge of these siblings | S:45 R:70 A:35 D:55 |
| 7 | Certain | Per-tool glosses placed as a new "What it's for" column in the existing shell-init table | Clarified — user confirmed (column over separate-list / both) | S:95 R:85 A:60 D:50 |

7 assumptions (6 certain, 0 confident, 1 tentative, 0 unresolved).

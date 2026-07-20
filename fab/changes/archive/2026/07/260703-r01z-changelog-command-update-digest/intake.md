# Intake: Changelog command + update release digest

**Change**: 260703-r01z-changelog-command-update-digest
**Created**: 2026-07-03

## Origin

Conversational — designed over a `/fab-discuss` session before `/fab-new` was invoked.

> When using "shll update" is it possible to show the list of updates that have happened in the respective tools from their release pages?

Follow-up decisions from the conversation, in order:

1. **Default on** — the changelog surface in `shll update` needs no opt-in flag (user: "yes default on").
2. **`net/http` direct** — fetch GitHub release data via Go's stdlib HTTP client, unauthenticated; no `gh` CLI dependency (user: "ok for net/http").
3. **Two-command shape** — user proposed a deterministic escape hatch ("shll changelog -t tu v2.1.4 v2.1.5 -t hop v1 v2 … we print a command the user can copy paste"), making changelog generation a standalone command that `shll update` composes. Agreed; the repeated `-t` flag grammar was replaced with positional `tool@old..new` specs (a 3-value-per-occurrence flag is not expressible in cobra's flag model).
4. **Promptless digest** — user initially proposed a "Show changelog?" prompt after update output; after weighing TTY-gating and the "said no by mistake" failure mode, user explicitly selected the promptless option (asked via structured question): compact digest always prints, full bodies are one copy-paste away via the printed `shll changelog` command. No interactivity anywhere.

Grounding verified during discussion: sahil87 tool releases carry auto-generated "What's Changed" bodies (checked hop v0.1.18 — PR titles + a `Full Changelog` compare link), and the roster already has a `Repo` field plus a `githubOrgBase` constant (added for `shll list`; note rk's repo is `run-kit`).

## Why

**Problem**: `shll update` upgrades up to seven tools in one run, but the only record of *what changed* is buried in streamed brew/tool output — version transitions flash by, and release notes are never shown. A user who wants to know what an update actually delivered must open up to seven GitHub release pages by hand and reconstruct the version range themselves.

**If we don't fix it**: updates stay opaque. Users either skip reading changelogs entirely (missing breaking-change notes like hop's agent-support release) or burn minutes on manual GitHub archaeology after every update.

**Why this approach**: the toolkit's GitHub Releases already carry auto-generated per-PR notes, so composing them (Constitution IV instinct applied to shll itself) beats maintaining any parallel changelog. A standalone `shll changelog` command as the primitive — with `shll update` printing a compact digest plus the exact re-runnable command — gives deterministic re-display (no interactive prompt to mis-answer, no lost output) and a genuinely useful standalone tool ("what would an update bring?") for one implementation cost.

## What Changes

### New subcommand: `shll changelog`

A new top-level cobra command (new file `src/cmd/shll/changelog.go`, wired in `root.go` like every other subcommand).

**Constitution VII justification** (required for any new top-level subcommand): changelog display cannot be a flag on `update` — its core purpose is *deterministic re-display after the fact* (the user who wants to re-read notes must not have to re-run an upgrade), and it has standalone value independent of updating (pre-update preview, historical range queries). It does not belong in a per-tool CLI because its job is cross-toolkit aggregation — exactly shll's mandate.

**Argument grammar** — positional per-tool specs, not flags:

```
shll changelog                          # all installed tools: installed → latest ("what would an update bring?")
shll changelog tu                       # one tool: installed → latest
shll changelog tu@0.6.2..0.6.4          # explicit range: releases in (0.6.2, 0.6.4]
shll changelog tu@0.6.2..0.6.4 hop@0.1.16..0.1.18   # multiple tools (this is what `shll update` prints)
```

- Valid tool names: the Roster names plus `shll` itself (reuse/extend `resolveTargets`-style validation with `allowShll=true` semantics; unknown names are a hard error listing valid targets, mirroring `update`).
- Versions accepted with or without the `v` prefix (brew reports `0.1.18`, tags are `v0.1.18`); normalize before compare. Brew revision suffixes (`0.6.4_1`) are stripped for tag matching.
- Output is in roster order regardless of argument order (matching `update`'s subset contract), with shll first when included.
- No-range form where the tool is already at the latest version prints an "up to date" line for it (plus the releases-page URL), not an error.
- Explicit range where `old == new` or no releases fall in the range prints "no releases in range".
- A named-but-not-installed tool is only an error for the *no-range* forms (they need an installed version to anchor the range); an explicit `tool@old..new` never consults brew and works regardless of install state.

**Output per tool**: a header line `{tool} {old} → {new} ({N} releases)` followed by each release in the range, newest first — tag, title, and the release body (the auto-generated "What's Changed" markdown, printed as-is). Capped at the 10 most recent releases per tool; when the range holds more, print the cap notice plus the `Full Changelog` compare URL (`https://github.com/sahil87/{Repo}/compare/v{old}...v{new}`). Color/section framing follows the existing `ui.go` conventions (TTY-gated via `colorEnabled`, per-tool headers per the per-tool-output-separation spec).

### `shll update`: version capture + digest tail

Today `probeTool` runs `brew list --formula --versions` via `isInstalled` but keeps only a boolean. Changes to `src/cmd/shll/update.go`:

1. **Capture before-versions**: extend `probeResult` to carry the installed version string parsed from the `brew list --versions` output the probe already pays for (a captured read via `proc.Run` — NOT parsing streamed foreground output, which is the anti-pattern). Same for shll-self (its before-version comes from the existing `shllSelfVersion()` / probe path).
2. **Capture after-versions**: after each *successful* upgrade (exit 0), re-query `brew list --formula --versions <formula>` (a cheap captured read) for the new version.
3. **Digest tail**: after the existing summary tail, for every tool whose version actually changed (`before != after`), print the "What changed:" digest — per-tool `{tool} {old} → {new} ({N} releases)` lines plus one title line per release (tag + release title only, NO bodies), then the copy-pasteable full-notes command. Exact shape agreed with the user:

```
Done — 3 of 3 succeeded (12.4s)

What changed:
  tu   0.6.2 → 0.6.4   (2 releases)
    v0.6.4  fix: opencode session parsing
    v0.6.3  feat: daily usage rollups
  hop  0.1.16 → 0.1.18 (2 releases)
    v0.1.18 feat: non-interactive agent support
    v0.1.17 fix: shim hardening

Full notes: shll changelog tu@0.6.2..0.6.4 hop@0.1.16..0.1.18
```

4. **Edge cases**:
   - No tool bumped (all already up-to-date or all failed) → no digest, no command line. Silence, exactly as today.
   - `--dry-run` → no digest (nothing was upgraded; the existing preview behavior is unchanged).
   - Subset runs (`shll update hop`) → digest covers only the bumped subset members; the printed command names only those tools.
   - Release fetch fails for a tool → that tool's digest entry degrades to `{tool} {old} → {new} — see {compare URL}` (Constitution V); the update exit code is NEVER affected by fetch failures.
   - A tool that upgraded but whose fetched range contains zero matching releases (tag scheme mismatch) → same compare-URL degradation.
   - The digest is presentation-only: it does not influence `anyFailed` or the exit code.

### New internal package: release fetching (`src/internal/changelog/`)

shll's first non-subprocess network I/O — isolated in its own internal package (mirroring the `internal/proc` pattern; command code never talks to `net/http` directly):

- **Fetch**: `GET https://api.github.com/repos/sahil87/{Repo}/releases?per_page=100`, unauthenticated, stdlib `net/http` only (no new module dependencies). API base URL and per-request timeout (10s) are named constants per code-quality.md. Repo slugs come from the existing `Tool.Repo` field / `shllSelf.Repo` — never derived from brew.
- **Range filter**: normalize tags and brew versions (strip `v` prefix, strip brew `_N` revision suffix), numeric dot-component compare, select releases in `(old, new]`, newest first.
- **Concurrency**: fetches for multiple tools may run concurrently (read-only HTTP, mirroring the `probeRoster` carve-out); results render in roster order.
- **Degradation contract**: any failure (network error, non-200 incl. 403 rate-limit, JSON parse failure, timeout) returns a typed "unavailable" result — callers render the compare-URL fallback and continue. No retries in v1.
- **Not in v1**: no `GITHUB_TOKEN`/auth support (7 unauthenticated requests per run vs. the 60/hr limit is ample headroom); no caching (Constitution II — stateless); no retry/backoff.
<!-- assumed: package name `internal/changelog` — plan may rename (e.g. `internal/ghrel`); the boundary (no net/http in cmd code) is the load-bearing part -->

## Affected Memory

- `cli/changelog`: (new) the `shll changelog` command — grammar, range semantics, output shape, degradation contract
- `cli/update`: (modify) version capture in probes, post-upgrade re-query, the "What changed:" digest tail + printed full-notes command
- `cli/commands`: (modify) subcommand wiring gains `changelog`
- `internal/changelog`: (new) the GitHub-releases fetch package — endpoint, timeout, range filter, unavailable-result degradation

## Impact

- `src/cmd/shll/changelog.go` (+ `changelog_test.go`) — new command
- `src/cmd/shll/update.go` (+ `update_test.go`) — probe version capture, after-version re-query, digest tail
- `src/cmd/shll/root.go` — one-line command registration
- `src/cmd/shll/tools.go` — no schema change needed (`Repo` field and `githubOrgBase` already exist); possibly a shared version-normalization helper placement
- `src/internal/changelog/` (new package + tests) — first stdlib-HTTP network code in the repo; needs a test seam analogous to `proc.Runner` (injectable HTTP transport or base-URL)
- No new module dependencies; no changes to `shell-init`/`version`/`install`/`list`/`doctor`
- Constitution touchpoints: I (no subprocess change; HTTP isolated in internal package), II (stateless — all derived per run), IV (composes GitHub releases + brew reads), V (fetch failures degrade to URLs, never break update), VII (justification above)

## Open Questions

- None — the one contested decision (prompt vs. promptless digest) was asked and resolved (promptless).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Two-command shape: `shll changelog` is the primitive; `update` composes it and prints the copy-paste command | Discussed — user proposed the standalone command explicitly | S:90 R:70 A:90 D:90 |
| 2 | Certain | Promptless digest in `update` (no "Show changelog?" prompt); compact title-line digest + printed command | Asked — user selected "Promptless digest" over TTY-gated prompt and hybrid | S:95 R:85 A:90 D:95 |
| 3 | Certain | Digest is default-on, no opt-in flag | Discussed — user: "yes default on" | S:95 R:80 A:90 D:90 |
| 4 | Certain | Fetch via stdlib `net/http`, unauthenticated GitHub API; no `gh` CLI dependency | Discussed — user: "ok for net/http" | S:90 R:80 A:85 D:85 |
| 5 | Certain | Before/after versions from captured `brew list --versions` reads (probe + post-upgrade re-query); never parse streamed brew output | code-quality.md anti-pattern rules this in directly | S:70 R:80 A:90 D:85 |
| 6 | Certain | Fetch failures degrade to the compare URL and never affect exit codes | Constitution V — graceful degradation | S:80 R:85 A:95 D:90 |
| 7 | Confident | Positional `tool@old..new` spec grammar (not repeated `-t` flags) | Recommended in discussion (cobra can't express 3-value flag tuples); user selected an option whose preview used this grammar verbatim | S:75 R:75 A:80 D:75 |
| 8 | Confident | No-range semantics: bare/`tool`-only forms show installed → latest ("what would an update bring?"); up-to-date prints a notice, not an error | Proposed in discussion, unobjected; single consistent semantic across forms | S:55 R:80 A:70 D:60 |
| 9 | Confident | Cap 10 releases per tool; overflow → compare URL | Proposed bound (~10) in discussion, unobjected; trivially tunable | S:60 R:90 A:75 D:70 |
| 10 | Confident | shll self-upgrade included in digest; `shll` valid as a changelog target | Mirrors `update`'s shllTargetToken handling; `shllSelf.Repo` already exists | S:50 R:85 A:80 D:70 |
| 11 | Confident | New stdlib-only internal package hosts fetch/filter (cmd code never imports net/http); name `internal/changelog` is a placeholder the plan may adjust | Mirrors `internal/proc` isolation pattern (Constitution I spirit, code-quality.md) | S:55 R:80 A:85 D:75 |
| 12 | Confident | v1 excludes GITHUB_TOKEN auth, caching, and retries | 7 requests/run vs. 60/hr unauthenticated limit; caching violates Constitution II; all easily added later | S:40 R:90 A:75 D:70 |
| 13 | Confident | `--dry-run` prints no digest | Nothing upgraded ⇒ no transitions to report; preview contract unchanged | S:45 R:90 A:85 D:80 |
| 14 | Confident | Version normalization: strip `v` prefix and brew `_N` revision suffix; numeric dot-component compare | Reasonable engineering default; low blast radius, fully reversible | S:35 R:85 A:60 D:50 |

14 assumptions (6 certain, 8 confident, 0 tentative, 0 unresolved).

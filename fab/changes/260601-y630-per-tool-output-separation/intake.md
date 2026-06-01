# Intake: Per-Tool Output Separation for shll update/install/shell-init

**Change**: 260601-y630-per-tool-output-separation
**Created**: 2026-06-01
**Status**: Draft

## Origin

This change was synthesized from a `/fab-discuss` session whose decisions are captured in
the committed, human-curated spec at `docs/specs/per-tool-output-separation.md`. That spec
is the **authoritative design intent** — this intake reproduces its locked decisions
verbatim so the downstream spec-stage agent has full continuity.

> **User intent**: "Per-tool output separation for shll update/install/shell-init." Today
> `shll update`, `shll install`, and `shll shell-init` foreground each sub-tool's output
> directly to the terminal (`proc.RunForeground` → inherited stdio), producing one
> undifferentiated wall of text with no visual marker for where one tool's output ends and
> the next begins. Print a labeled header before each tool's output, plus a one-line summary
> tail for the two upgrade commands.

The scope (which commands are in/out) and the two design forks (the `version` exclusion and
the `shell-init` comment-vs-header split) were resolved during the discuss session — see the
spec's "Open questions" section ("None blocking").

## Why

1. **The pain point.** Because sub-tool output is *streamed* (foregrounded, not captured),
   shll never sees the bytes — it can only print **around** each subprocess, not reformat
   what's inside. With six roster tools, `shll update` and `shll install` emit one
   continuous wall of text; the user cannot tell where `rk`'s output ends and `tu`'s begins.
   `shll shell-init` concatenates blobs with no boundary marker either.

2. **The consequence if unfixed.** Output stays unreadable as the roster grows. Triaging
   which tool failed during a multi-tool upgrade requires scrolling and guessing. There is
   no run-level summary, so a partial failure inside a long scroll is easy to miss.

3. **Why this approach.** The streamed-output constraint means the only realistic lever is a
   header/separator that shll prints *before* each tool's section, plus a tail *after* all
   tools run. This respects Constitution III/IV (shll still only prints around each
   subprocess — no sub-tool logic is reimplemented, no new subprocess is introduced). The
   alternative — capturing and reformatting sub-tool output — would break the foreground
   streaming UX (live progress bars, colors) and add buffering complexity for no real gain.

## What Changes

A single shared UI helper (TTY detection + `NO_COLOR` check + glyph choice + header
printer) is introduced and consumed by three commands. `version` is explicitly untouched.

### Scope (locked during discussion)

| Command | In scope? | Mechanism |
|---------|-----------|-----------|
| `update` | YES | Per-tool header to **stdout** + summary tail |
| `install` | YES | Per-tool header to **stdout** + summary tail |
| `shell-init` | YES | Per-tool separator as a **shell comment** in stdout |
| `version` | NO | Excluded — lines already self-label (`rk 1.5.0`); a header is redundant and a success/failure tail is meaningless for a read-only aggregation. **Do NOT touch `version`.** |

### Header style (update / install)

One labeled line per tool, printed immediately before that tool's foregrounded output:

- **TTY + color enabled** → `▸ <tool>` (bold cyan arrow + bold tool name). Sub-tool output
  keeps its native color.
- **Piped / `NO_COLOR` set / non-TTY** → `==> <tool>`, no ANSI. The degrade swaps **both**
  the glyph (`▸` → `==>`) **and** any Unicode in shll's *own* output (e.g. `→` → `->`) so
  logs and CI stay clean ASCII. (Sub-tool output is passed through untouched either way.)
  The `==>` idiom matches Homebrew's existing convention, so the plain form reads naturally
  alongside brew's own output.

For `update`, the existing instant status line `Checking installed sahil87 tools…`
(`updateStatusLine`, `src/cmd/shll/update.go`) is **unchanged** and still printed first.
`shll (self)` gets a header too when the self-upgrade step runs.

### Stream discipline (critical)

| Command | Header stream | Rationale |
|---------|---------------|-----------|
| `update` / `install` | **stdout** | Sub-tool output is foregrounded to stdout. The header MUST share that stream — printing to stderr would interleave unpredictably against stdout (different buffers, different flush timing). |
| `shell-init` | **stdout**, as a shell comment | stdout is consumed by `eval "$(shll shell-init <shell>)"`. A bare header would be eval'd as a command and break the shell. A `#`-prefixed line is a shell no-op (eval-safe) and still visible when read. |

### `shell-init` separator — the deliberate exception

`shell-init` does **NOT** use the `▸`/`==>` header. It emits a shell-comment separator
before each tool's init block:

```
# ── tu ──
export PATH=...
# ── hop ──
alias h='hop'
# ── wt ──
...
```

- **No color, no TTY-gating** — always plain ASCII comments. ANSI escapes inside eval'd
  output would corrupt the shell; the comment form is the *only* safe separator here.
- This is a **deliberate inconsistency** with the other commands' `▸`/`==>` header, driven
  by Constitution V (Graceful Degradation — `shell-init` output MUST always be eval-safe).
  It is **NOT** an oversight. A future change MUST NOT "unify" `shell-init` onto the `▸`
  header — doing so reintroduces the eval-break. **This exception MUST be documented in
  memory at hydrate** (`docs/memory/cli/shell-init.md`).

The separator MUST only be emitted for a tool whose init block actually reaches stdout. Per
the existing eval-safety invariant (`docs/memory/cli/shell-init.md`), a tool that is not
installed is silently omitted and a tool that errors has its stdout dropped — neither should
get a separator. This preserves the "stdout consists only of bytes from successful
sub-tools, concatenated, plus shll's own comment separators" property.

### Summary tail (update / install only)

After all tools run, print one honest line based on **exit codes only**:

```
Done — 6 of 6 tools succeeded.
```

or, on partial failure:

```
5 succeeded, 1 failed — see above.
```

- **Honesty constraint**: because sub-tool output is streamed (not captured), shll cannot
  distinguish "actually upgraded" from "already up-to-date" — it only knows each tool's exit
  code. The tail therefore reports **succeeded / failed counts**, **never** "updated vs.
  up-to-date". Do not claim more than the exit code proves.
- TTY-color-gated like the headers (e.g. a green `✓` on a TTY, plain on a pipe).
- `shell-init` gets **no** tail (it produces a script, not a run report). `version` gets no
  tail.
- The exit-code accounting MUST align with each command's existing `anyFailed` semantics
  (the per-tool loops in `update.go` / `install.go` already track failure best-effort and
  return `errSilent` on any failure — the tail's counts derive from the same per-tool
  success/failure facts; the tail does not change exit codes).

### Color gating

Color + Unicode glyphs are emitted only when **both**:

1. `stdout` is a TTY — `term.IsTerminal(fd)` from `golang.org/x/term`, **and**
2. `NO_COLOR` is unset (honor the [no-color.org](https://no-color.org) convention).

Otherwise: plain ASCII, no ANSI. This is shll's **first** terminal inspection, so
`golang.org/x/term` is a new (small, idiomatic) dependency — a deliberate addition, not
incidental. `shell-init` ignores color gating entirely (always plain ASCII comments).

### Implementation notes (non-binding — for the spec/plan to refine)

1. **Single shared helper.** TTY detection, the `NO_COLOR` check, the glyph choice, and a
   `printToolHeader(w, name)` should live in one place — a **new** file
   `src/cmd/shll/ui.go` — not duplicated across `update.go` / `install.go`. The shell-init
   comment-emitter (`printToolComment` or similar) is a sibling in the same file, sharing
   the file but not the color logic.
2. **Injectable color decision for tests.** Expose a `forceColor` / `forcePlain` seam (or a
   `colorEnabled bool` parameter) so tests are not TTY-dependent. `bytes.Buffer` test
   writers are not TTYs, so tests naturally exercise the **plain ASCII** branch — assert
   against the `==>` / comment forms.
3. **Golden-string churn.** Several existing tests assert verbatim stdout — e.g.
   `TestUpdate_NoToolsInstalled` expects exactly
   `Checking installed sahil87 tools…\nNo sahil87 tools installed.\n`. Adding headers and the
   tail changes these golden strings. Per the constitution's **Test Integrity** rule, update
   the spec / expected output first, then conform the tests — never bend the implementation
   to satisfy a stale fixture. (Note: the nothing-to-do short-circuits — `update`'s
   `No sahil87 tools installed.` and `install`'s `All sahil87 tools already installed.` —
   run no per-tool loop, so they should emit no per-tool header; whether they get a tail is a
   spec-level decision, but the honest default is "no tail when no tool ran".)
4. **Constitution check.** No new subprocess (still all through `internal/proc`). No sub-tool
   logic reimplemented (Constitution III/IV intact — shll still only prints *around* each
   subprocess). The one new dependency is `golang.org/x/term` for TTY detection. No new
   top-level subcommand (Constitution VII not triggered — this is behavior on existing
   commands, not a new command).

## Affected Memory

- `cli/update`: (modify) Document the per-tool header before each `upgradeTool` call, the
  summary tail, the stdout stream discipline, and TTY/`NO_COLOR` color gating. Note the
  self-upgrade step also gets a `shll (self)` header. Reconcile the verbatim golden-string
  examples (e.g. the `TestUpdate_NoToolsInstalled` expected stdout) with the new behavior.
- `cli/install`: (modify) Document the per-tool header + summary tail + stream discipline +
  color gating, mirroring `update`.
- `cli/shell-init`: (modify) Document the shell-comment separator `# ── <tool> ──` before
  each tool's init block, and **the eval-safety exception** (no `▸` header, no color, no
  TTY-gating — deliberate, MUST NOT be unified onto the header). Note the separator is only
  emitted for tools whose stdout actually reaches output (installed + non-erroring),
  preserving the existing eval-safety invariant.
- `cli/commands`: (modify, maybe) Note the new shared UI helper file (`src/cmd/shll/ui.go`)
  in the file-layout table and cross-reference it from the three touched commands.
- `internal/` (maybe): Only if the UI helper warrants its own internal note — likely not,
  since it lives under `cmd/shll/`, not `internal/`. Decide at hydrate.

## Impact

Affected code areas (from the spec's "Affected areas"):

- `src/cmd/shll/update.go` — per-tool header before each `upgradeTool` call + summary tail.
- `src/cmd/shll/install.go` — per-tool header + summary tail.
- `src/cmd/shll/shell_init.go` — shell-comment separator before each tool's init block.
- `src/cmd/shll/ui.go` *(new)* — shared header/color/TTY helper + shell-init comment emitter.
- `src/cmd/shll/*_test.go` — golden-string updates for the three touched commands
  (`update_test.go`, `install_test.go`, `shell_init_test.go`). `version_test.go` and
  `shell_install_test.go` should be unaffected.
- `src/go.mod` (+ `go.sum`) — add `golang.org/x/term`.

Dependencies / systems:

- New direct dependency: `golang.org/x/term` (TTY detection). First terminal inspection in
  the codebase. Cross-platform: builds on darwin-arm64/amd64 and linux-arm64/amd64
  (Windows unsupported — `x/term` supports all four target platforms).
- No change to subprocess invocation, the brew helpers, the tool roster, or exit codes.

## Open Questions

- ~~Does the nothing-to-do short-circuit in `update`/`install` get a summary tail?~~
  **Resolved (clarify 2026-06-01):** NO tail and NO per-tool header in the empty case — no
  per-tool loop runs, so the existing one-line message (`No sahil87 tools installed.` /
  `All sahil87 tools already installed.`) stands alone. See Assumption #12.
- ~~Exact ANSI sequences / styling for the bold-cyan `▸` and green `✓`.~~
  **Resolved (clarify 2026-06-01):** hand-rolled standard ANSI SGR codes as named constants
  in `ui.go`; no external color library. See Assumption #13.
- Whether `cli/commands` and/or an `internal/` memory note are warranted, or whether the
  three per-command memory files suffice — decide at hydrate. *(Genuinely deferred — a
  hydrate-time documentation-scope call, not a design decision.)*

## Clarifications

### Session 2026-06-01

Tentative resolution (asked one at a time):

| # | Action | Detail |
|---|--------|--------|
| 12 | Confirmed | Empty-case short-circuits get no tail and no per-tool header |
| 13 | Confirmed | Hand-rolled ANSI SGR named constants in `ui.go`; no external color library |

Bulk confirm (Confident assumptions):

| # | Action | Detail |
|---|--------|--------|
| 8 | Confirmed | — |
| 9 | Confirmed | — |
| 10 | Confirmed | — |
| 11 | Confirmed | — |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope = `update`, `install`, `shell-init`; `version` excluded entirely | Locked in the committed spec's Scope table and discuss session; constitution/spec deterministically answer it | S:98 R:80 A:95 D:95 |
| 2 | Certain | Header glyphs: `▸ <tool>` on TTY+color, `==>` plain otherwise; degrade swaps glyph AND shll's own Unicode | Verbatim from spec "Header style"; `==>` chosen to match Homebrew idiom | S:97 R:70 A:90 D:90 |
| 3 | Certain | update/install header → stdout (same stream as foregrounded output); never stderr | Spec "Stream discipline" marks this critical with explicit interleaving rationale | S:97 R:60 A:92 D:95 |
| 4 | Certain | shell-init uses `# ── <tool> ──` comment separator: no color, no TTY-gating, never the `▸` header — deliberate exception, document in memory | Spec dedicates a whole section to this; tied to Constitution V eval-safety; explicitly flagged "NOT an oversight" | S:98 R:55 A:95 D:96 |
| 5 | Certain | Summary tail (update/install only) reports succeeded/failed counts by exit code — never "updated vs up-to-date" | Spec "Honesty constraint"; streamed output means exit codes are all shll knows | S:96 R:75 A:92 D:92 |
| 6 | Certain | Color+glyphs gated on (stdout is TTY via `term.IsTerminal`) AND (`NO_COLOR` unset); shell-init ignores gating | Spec "Color gating" states both conditions and the no-color.org convention explicitly | S:97 R:70 A:92 D:93 |
| 7 | Certain | Add `golang.org/x/term` as a new direct dependency for TTY detection | Spec names the exact dependency as a deliberate addition; first terminal inspection in the repo | S:96 R:65 A:95 D:95 |
| 8 | Certain | Shared helper lives in a new `src/cmd/shll/ui.go` with an injectable color seam (`colorEnabled bool` / `forceColor`) for tests | Clarified — user confirmed (bulk) | S:95 R:80 A:80 D:70 |
| 9 | Certain | Update the three commands' golden-string tests to match new output (Test Integrity: spec first, then conform tests) | Clarified — user confirmed (bulk) | S:95 R:75 A:90 D:85 |
| 10 | Certain | shell-init separator emitted only for tools whose stdout reaches output (installed + non-erroring), preserving the eval-safety invariant | Clarified — user confirmed (bulk) | S:95 R:70 A:85 D:72 |
| 11 | Certain | Change type = `feat` (user-visible UX behavior change across three commands) | Clarified — user confirmed (bulk) | S:95 R:85 A:85 D:80 |
| 12 | Certain | Nothing-to-do short-circuits (`No sahil87 tools installed.` / `All sahil87 tools already installed.`) get NO summary tail and NO per-tool header | Clarified — user confirmed: no per-tool loop runs, so nothing to separate or count; existing one-line message stands alone | S:95 R:65 A:55 D:50 |
| 13 | Certain | Exact ANSI styling: hand-rolled standard SGR codes (named constants in `ui.go`) for bold-cyan `▸` and green `✓` — NO external color library | Clarified — user confirmed: keeps dep footprint to just `golang.org/x/term`; fits the "named constants, no magic strings" code-quality rule | S:95 R:80 A:60 D:55 |

13 assumptions (13 certain, 0 confident, 0 tentative, 0 unresolved). Run /fab-clarify to review.

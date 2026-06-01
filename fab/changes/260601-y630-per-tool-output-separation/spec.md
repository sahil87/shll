# Spec: Per-Tool Output Separation for shll update/install/shell-init

**Change**: 260601-y630-per-tool-output-separation
**Created**: 2026-06-01
**Affected memory**: `docs/memory/cli/update.md`, `docs/memory/cli/install.md`, `docs/memory/cli/shell-init.md`, `docs/memory/cli/commands.md`

<!--
  Derived from the human-curated design intent at docs/specs/per-tool-output-separation.md
  and the confirmed intake (13 Certain assumptions, 0 open design questions). All design
  forks (the `version` exclusion, the shell-init comment-vs-header split, the empty-case
  no-tail rule, hand-rolled ANSI) were resolved upstream — this spec records the locked
  contract, not new decisions.
-->

## Non-Goals

- **`shll version` output** — left entirely untouched. Version lines already self-label (`rk 1.5.0`), so a header is redundant and a success/failure tail is meaningless for a read-only aggregation. No file under `version.go` / `version_test.go` is modified.
- **Capturing or reformatting sub-tool output** — sub-tool output stays *foregrounded* (streamed via `proc.RunForeground` → inherited stdio). shll prints only *around* each subprocess; it never buffers, parses, or rewrites the bytes a sub-tool emits (preserves live progress bars/colors; respects Constitution III/IV).
- **Distinguishing "upgraded" from "already up-to-date"** — because sub-tool output is streamed, shll sees only exit codes, not a per-tool change verdict. The summary tail reports succeeded/failed counts only.
- **A new top-level subcommand** — this is behavior added to three existing commands; Constitution VII is not triggered.
- **Changing exit codes** — the per-tool header, separator, and summary tail are presentation-only. The `anyFailed` accounting and every `errSilent` / `errExitCode` return value are preserved exactly.
- **Unifying `shell-init` onto the `▸`/`==>` header** — deliberately excluded for eval-safety (see the shell-init domain section); a future change MUST NOT do this.

## CLI: Shared UI helper (`ui.go`)

### Requirement: Single shared UI helper file

A new file `src/cmd/shll/ui.go` MUST hold the shared presentation logic: TTY detection, the `NO_COLOR` check, the glyph/style choice, the per-tool header printer (`printToolHeader` or equivalent), the summary-tail printer, and the shell-init comment-separator emitter. The header/tail logic and the shell-init comment emitter MAY share the file but MUST NOT share the color/TTY-gating logic — the shell-init emitter is always plain ASCII. `update.go`, `install.go`, and `shell_init.go` MUST consume these helpers rather than duplicating the logic.

#### Scenario: Helper lives in one place
- **GIVEN** the three touched commands need a per-tool boundary marker
- **WHEN** the change is implemented
- **THEN** `src/cmd/shll/ui.go` is created and `update.go` / `install.go` / `shell_init.go` call into it
- **AND** no TTY/`NO_COLOR`/glyph logic is duplicated across `update.go` or `install.go`

### Requirement: Color and Unicode gating

Color and Unicode glyphs MUST be emitted only when BOTH conditions hold: (1) stdout is a terminal — detected via `term.IsTerminal(fd)` from `golang.org/x/term`, AND (2) the `NO_COLOR` environment variable is unset (honoring the no-color.org convention). When either condition fails, the helper MUST emit plain ASCII with no ANSI escape sequences. `golang.org/x/term` MUST be added as a new direct dependency in `src/go.mod` (and `go.sum`); it is the codebase's first terminal inspection.

#### Scenario: Interactive terminal with color allowed
- **GIVEN** stdout is a TTY AND `NO_COLOR` is unset
- **WHEN** the helper chooses the presentation mode
- **THEN** it emits the colored Unicode form (`▸`, green `✓`)

#### Scenario: NO_COLOR set on a TTY
- **GIVEN** stdout is a TTY AND `NO_COLOR` is set (to any value)
- **WHEN** the helper chooses the presentation mode
- **THEN** it emits the plain ASCII form (`==>`, no ANSI)

#### Scenario: Piped / redirected output
- **GIVEN** stdout is not a TTY (piped, redirected to a file, or captured in CI)
- **WHEN** the helper chooses the presentation mode
- **THEN** it emits the plain ASCII form regardless of `NO_COLOR`

### Requirement: Injectable color seam for tests

The color decision MUST be injectable so tests are not TTY-dependent — via a `colorEnabled bool` parameter, a `forceColor`/`forcePlain` seam, or equivalent. Because `bytes.Buffer` test writers are not terminals, tests MUST naturally exercise the plain-ASCII branch and assert against the `==>` header / `# ── … ──` comment / plain tail forms.

#### Scenario: Test writer hits the plain branch
- **GIVEN** a test drives a command with a `bytes.Buffer` stdout writer
- **WHEN** the helper evaluates the gating
- **THEN** `term.IsTerminal` reports non-terminal and the plain ASCII form is produced
- **AND** the test asserts the `==>` / comment / plain-tail strings without faking a TTY

### Requirement: Hand-rolled ANSI SGR named constants

ANSI styling MUST use hand-rolled standard SGR escape codes declared as NAMED CONSTANTS in `ui.go` (e.g. bold-cyan for `▸`, green for `✓`) — never an external color library and never inline magic escape strings (per code-quality.md "named constants, no magic strings"). The only new dependency introduced by this change is `golang.org/x/term`.

#### Scenario: No color library dependency
- **GIVEN** the colored forms are implemented
- **WHEN** `src/go.mod` is inspected after the change
- **THEN** the only added direct dependency is `golang.org/x/term`
- **AND** the SGR sequences are defined as named constants in `ui.go`

### Requirement: ASCII degrade swaps shll's own Unicode

When degrading to the plain form, the helper MUST swap BOTH the header glyph (`▸` → `==>`) AND any Unicode in shll's OWN output (e.g. `→` → `->`) so logs and CI stay clean ASCII. Sub-tool output MUST be passed through untouched in both the colored and plain forms — shll never rewrites bytes a sub-tool emits.

> **Note — spec-mandated wording literals are exempt from the glyph-degrade rule.** The degrade applies only to *swappable* glyphs (`▸`→`==>`, dropping the green `✓`). The em-dash `—` in the summary tail (`Done — N of M tools succeeded.` / `X succeeded, Y failed — see above.`) and the box-drawing `─` in the shell-init separator (`# ── <tool> ──`) are spec-defined WORDING, kept verbatim in BOTH the colored and plain branches — they are NOT degraded. They carry no eval-safety/CI risk: the em-dash lives in a human-readable run-report line that is never eval'd, and the box-drawing chars sit inside a `#` shell comment (a no-op when eval'd).

#### Scenario: Plain form is pure ASCII
- **GIVEN** the plain (non-TTY or `NO_COLOR`) branch is selected
- **WHEN** shll prints its own header/tail text
- **THEN** every character shll emits is ASCII (`==>`, `->`), with no Unicode glyphs
- **AND** any bytes streamed by a sub-tool are unchanged

## CLI: `update` and `install` headers and summary tail

### Requirement: Per-tool header before each tool's output

`shll update` and `shll install` MUST print one labeled header line immediately before each tool's foregrounded output, on the SAME tool's section. On a color-enabled TTY the header MUST read `▸ <tool>` (bold-cyan arrow + bold tool name); in the plain form it MUST read `==> <tool>` with no ANSI. The `==>` idiom matches Homebrew's existing convention so the plain form reads naturally alongside brew's own output. For `update`, the header MUST also be printed for the shll self-upgrade step, labeled `shll (self)`, when that step runs.

#### Scenario: Header precedes each roster tool (update, plain)
- **GIVEN** `shll update` runs with `hop` and `wt` installed and a non-TTY stdout
- **WHEN** the per-tool upgrade loop reaches `hop` then `wt`
- **THEN** `==> hop` is written to stdout immediately before `hop`'s foregrounded output
- **AND** `==> wt` is written immediately before `wt`'s foregrounded output

#### Scenario: Self-upgrade header (update)
- **GIVEN** `shll update` runs and shll itself is brew-installed (`isInstalled(ctx, shllFormula)` is true)
- **WHEN** the self-upgrade step (`brew upgrade shllFormula`) runs before the roster loop
- **THEN** a `shll (self)` header (`▸ shll (self)` / `==> shll (self)`) is written before that step's output

#### Scenario: Header precedes each installed tool (install, plain)
- **GIVEN** `shll install` runs with `fab-kit` and `rk` missing and a non-TTY stdout
- **WHEN** the install loop reaches each missing tool in roster order
- **THEN** `==> fab-kit` precedes `fab-kit`'s install output and `==> rk` precedes `rk`'s

### Requirement: `update` status line unchanged and printed first

The existing instant status line `Checking installed sahil87 tools…` (`updateStatusLine`, `src/cmd/shll/update.go:20`) MUST remain unchanged and MUST still be the first byte written to stdout, before any per-tool header. The header feature MUST NOT alter, reorder, or suppress it.

#### Scenario: Status line still leads
- **GIVEN** `shll update` runs with tools installed
- **WHEN** stdout is examined
- **THEN** it begins with `Checking installed sahil87 tools…\n`
- **AND** the first per-tool header appears only after that line

### Requirement: Headers and tail share the stdout stream

For `update` and `install`, the per-tool header and the summary tail MUST be written to STDOUT — the same stream the sub-tool output is foregrounded onto — never to stderr. Writing them to stderr would interleave unpredictably against stdout (different buffers, different flush timing). In production the writer passed to `runUpdate`/`runInstall` (`cmd.OutOrStdout()`) is `os.Stdout`, the same destination `proc.RunForeground` inherits, so headers and streamed output stay ordered on one stream.

#### Scenario: Header goes to stdout, not stderr
- **GIVEN** a test drives `runUpdate` / `runInstall` with separate stdout and stderr buffers
- **WHEN** a tool is processed
- **THEN** its header appears in the stdout buffer
- **AND** no header text appears in the stderr buffer

### Requirement: Summary tail on exit codes (update / install only)

After all tools have run, `shll update` and `shll install` MUST print exactly one summary line derived from EXIT CODES only. On full success it MUST read `Done — N of M tools succeeded.`; on partial failure it MUST read `X succeeded, Y failed — see above.`. The counts MUST align with the command's existing `anyFailed` semantics — the same per-tool success/failure facts the loop already tracks. The tail MUST NOT claim "updated" vs. "up-to-date" (the honesty constraint — streamed output means shll knows only exit codes). The tail MUST NOT change the process exit code. The tail MUST be color-gated like the headers (a green `✓` on a TTY, plain otherwise). `shell-init` and `version` MUST NOT print a tail.

#### Scenario: All tools succeed
- **GIVEN** `shll update` runs the self-upgrade plus M roster upgrades and none fail
- **WHEN** the run completes
- **THEN** the last stdout line reports all-succeeded in the `Done — N of M tools succeeded.` form
- **AND** the process still returns nil (exit 0), unchanged

#### Scenario: One tool fails
- **GIVEN** `shll update` runs and exactly one tool exits non-zero (so `anyFailed` is set)
- **WHEN** the run completes
- **THEN** the last stdout line reports the partial-failure form `X succeeded, Y failed — see above.` with counts matching the per-tool outcomes
- **AND** the process still returns `errSilent` (exit 1), unchanged

#### Scenario: Tail never asserts up-to-date
- **GIVEN** any `update` run regardless of whether tools actually changed
- **WHEN** the tail is composed
- **THEN** it reports only succeeded/failed counts and never the words "updated" or "up-to-date"

### Requirement: Empty-case short-circuit gets no header and no tail

When the nothing-to-do short-circuit fires — `update`'s `No sahil87 tools installed.` (no roster tool installed AND shll not brew-installed) or `install`'s `All sahil87 tools already installed.` (nothing missing) — no per-tool loop runs, so NO per-tool header and NO summary tail MUST be emitted. The existing one-line message MUST stand alone (for `update`, preceded only by the unchanged status line).

#### Scenario: update nothing-to-do
- **GIVEN** brew is present but neither shll nor any roster tool is installed
- **WHEN** `shll update` runs
- **THEN** stdout is exactly `Checking installed sahil87 tools…\nNo sahil87 tools installed.\n`
- **AND** no `▸`/`==>` header and no summary tail appear

#### Scenario: install everything already installed
- **GIVEN** every roster tool is already installed
- **WHEN** `shll install` runs
- **THEN** stdout is exactly `All sahil87 tools already installed.\n`
- **AND** no header and no summary tail appear

## CLI: `shell-init` shell-comment separator (the eval-safety exception)

### Requirement: Shell-comment separator instead of the header

`shll shell-init` MUST NOT use the `▸`/`==>` header. Before each tool's init block that reaches stdout, it MUST emit a shell-comment separator of the form `# ── <tool> ──`. This separator MUST be plain ASCII-safe shell-comment text with NO ANSI color and NO TTY-gating — it is emitted identically whether or not stdout is a terminal and regardless of `NO_COLOR`. This is a DELIBERATE inconsistency with `update`/`install`, mandated by Constitution V (eval-safety): `eval "$(shll shell-init <shell>)"` consumes stdout, so a bare `▸ <tool>` line would be eval'd as a command and break the shell, and ANSI escapes inside eval'd output would corrupt it; a `#`-prefixed line is a shell no-op. This exception MUST be documented in memory at hydrate (`docs/memory/cli/shell-init.md`) and MUST NOT be "unified" onto the header by a future change.

#### Scenario: Separator precedes each contributing tool
- **GIVEN** `tu`, `hop`, and `wt` are installed and each emits init output
- **WHEN** `shll shell-init zsh` runs
- **THEN** stdout interleaves `# ── tu ──`, then tu's block, `# ── hop ──`, then hop's block, `# ── wt ──`, then wt's block, in roster order

#### Scenario: Separator ignores color gating
- **GIVEN** `shll shell-init zsh` runs on a color-enabled TTY
- **WHEN** the separators are emitted
- **THEN** they are plain ASCII `# ── <tool> ──` comments with no ANSI escapes
- **AND** the output remains eval-safe

### Requirement: Separator emitted only for tools whose output reaches stdout

The separator MUST be emitted ONLY for a tool whose init block actually reaches stdout — i.e. the tool is installed (binary on PATH) AND its `shell-init` did not error. A tool that is not installed (`proc.ErrNotFound`, skipped silently) and a tool whose `shell-init` errors (its stdout dropped, message to stderr) MUST NOT get a separator. This preserves the existing eval-safety invariant that stdout consists only of bytes from successful sub-tools, concatenated, now plus shll's own comment separators.

#### Scenario: Uninstalled tool gets no separator
- **GIVEN** only `tu` is installed (`hop`, `wt` not on PATH)
- **WHEN** `shll shell-init zsh` runs
- **THEN** stdout contains `# ── tu ──` and tu's block only
- **AND** no `# ── hop ──` or `# ── wt ──` separator appears

#### Scenario: Erroring tool gets no separator
- **GIVEN** `tu`, `hop`, `wt` are all installed but `hop`'s `shell-init` errors (roster-middle)
- **WHEN** `shll shell-init zsh` runs
- **THEN** stdout contains `# ── tu ──` + tu's block and `# ── wt ──` + wt's block, with no `# ── hop ──` separator and no hop bytes
- **AND** the hop error goes to stderr and the command returns `errSilent` (exit 1), unchanged

#### Scenario: No integrating tools installed
- **GIVEN** none of `tu`/`hop`/`wt` is installed
- **WHEN** `shll shell-init zsh` runs
- **THEN** stdout is empty (no separators, eval-safe no-op), exit 0, unchanged

## CLI: Test integrity (golden-string conformance)

### Requirement: Existing golden-string tests conform to the new output

The existing verbatim-stdout assertions in `update_test.go`, `install_test.go`, and `shell_init_test.go` MUST be updated to match the new header/separator/tail output. Per the constitution's Test Integrity rule, the spec/expected output is authored first and the tests are then conformed to it — the implementation MUST NOT be bent to satisfy a stale fixture. Tests in `version_test.go` and `shell_install_test.go` MUST remain unaffected.

#### Scenario: Empty-case golden strings preserved
- **GIVEN** `TestUpdate_NoToolsInstalled` and `TestInstall_AllAlreadyInstalled` assert the exact short-circuit stdout
- **WHEN** the change lands
- **THEN** those golden strings are unchanged (the empty case emits no header/tail), so the tests still pass without edits to their expected strings

#### Scenario: Loop-path golden strings updated
- **GIVEN** tests that exercise a per-tool loop and assert exact stdout (e.g. `shell_init_test.go`'s concatenation tests)
- **WHEN** the separator/header is added
- **THEN** those expected strings are updated to include the `# ── <tool> ──` / `==>` markers and the tests pass against the spec-defined output

## Design Decisions

1. **`shell-init` uses a shell comment, not the `▸`/`==>` header**
   - *Why*: `shell-init` stdout is consumed by `eval`. A bare header line would be eval'd as a command and break the user's shell; ANSI escapes inside eval'd output would corrupt it. A `#`-prefixed line is a shell no-op and still readable. Constitution V makes eval-safety non-negotiable.
   - *Rejected*: reusing the same `▸`/`==>` header for consistency — it reintroduces the eval-break. This is recorded as a guarded exception so a future "consistency" refactor doesn't undo it.

2. **Summary tail reports succeeded/failed counts, never "updated vs. up-to-date"**
   - *Why*: Sub-tool output is streamed, not captured, so shll only ever knows each tool's exit code — not whether a tool actually changed. Reporting more than the exit code proves would be dishonest.
   - *Rejected*: a richer "N updated, K up-to-date" tail — would require capturing and parsing sub-tool output, breaking the foreground streaming UX and Constitution III/IV.

3. **Hand-rolled ANSI SGR named constants, no color library**
   - *Why*: Keeps the dependency footprint to exactly one new dep (`golang.org/x/term`) and fits the code-quality rule against magic strings (SGR codes become named constants).
   - *Rejected*: an external color library (e.g. fatih/color) — unnecessary weight for two styled glyphs.

4. **Headers/tail to stdout, sharing the stream with foregrounded sub-tool output**
   - *Why*: The sub-tool output is foregrounded to stdout; the header must share that exact stream so ordering is deterministic. stderr has independent buffering/flush timing and would interleave unpredictably.
   - *Rejected*: routing shll's own framing to stderr (a common "diagnostics on stderr" instinct) — here it would scramble the visual association between a header and the output it labels.

5. **Empty-case short-circuits emit no header and no tail**
   - *Why*: No per-tool loop runs in the nothing-to-do branch, so there is nothing to separate or count; the existing one-line message is the complete, honest signal.
   - *Rejected*: printing `Done — 0 of 0 tools succeeded.` — noise that implies work happened when none did.

## Assumptions

<!-- Carried forward from intake.md (13 Certain). Each confirmed at spec level against the
     source symbols, golden strings, and constitution; no design fork re-opened. The lone
     genuinely-deferred item (hydrate-time memory-scope for cli/commands and internal/) is a
     documentation-scope call, not a design decision, so it is not an SRAD assumption row. -->

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope = `update`, `install`, `shell-init`; `version` excluded entirely (not touched) | Confirmed from intake #1; locked in the curated spec's Scope table and constitution; `version.go`/`version_test.go` untouched | S:98 R:80 A:95 D:95 |
| 2 | Certain | Header glyphs: `▸ <tool>` on TTY+color, `==>` plain otherwise; degrade swaps glyph AND shll's own Unicode (`→`→`->`); sub-tool bytes untouched | Confirmed from intake #2; verbatim from curated spec "Header style"; `==>` matches Homebrew idiom | S:97 R:70 A:90 D:90 |
| 3 | Certain | update/install header + tail → stdout (same stream as foregrounded output), never stderr | Confirmed from intake #3; curated spec marks this critical with the interleaving rationale; `cmd.OutOrStdout()`==`os.Stdout` in prod | S:97 R:60 A:92 D:95 |
| 4 | Certain | shell-init uses `# ── <tool> ──` comment separator: no color, no TTY-gating, never the `▸` header — deliberate eval-safety exception, documented in memory at hydrate, MUST NOT be unified | Confirmed from intake #4; whole curated-spec section; Constitution V; flagged "NOT an oversight" | S:98 R:55 A:95 D:96 |
| 5 | Certain | Summary tail (update/install only) reports succeeded/failed counts by exit code — `Done — N of M tools succeeded.` / `X succeeded, Y failed — see above.`; never "updated vs up-to-date"; no exit-code change | Confirmed from intake #5; honesty constraint; aligns with existing `anyFailed` semantics | S:96 R:75 A:92 D:92 |
| 6 | Certain | Color+glyphs gated on (stdout is TTY via `term.IsTerminal`) AND (`NO_COLOR` unset); shell-init ignores gating | Confirmed from intake #6; curated spec states both conditions + no-color.org convention | S:97 R:70 A:92 D:93 |
| 7 | Certain | Add `golang.org/x/term` as the one new direct dependency for TTY detection; cross-platform on the four supported targets (Windows unsupported, fine) | Confirmed from intake #7; first terminal inspection in the repo; named exactly | S:96 R:65 A:95 D:95 |
| 8 | Certain | Shared helper in new `src/cmd/shll/ui.go` with an injectable color seam (`colorEnabled bool`/`forceColor`); `bytes.Buffer` writers hit the plain branch | Confirmed from intake #8 (user bulk-confirmed); matches the existing `run*` test-seam pattern | S:95 R:80 A:80 D:70 |
| 9 | Certain | Update the three commands' golden-string tests to match new output (Test Integrity: spec first, then conform); `version_test.go`/`shell_install_test.go` unaffected | Confirmed from intake #9 (user bulk-confirmed); empty-case golden strings stay verbatim, loop-path strings churn | S:95 R:75 A:90 D:85 |
| 10 | Certain | shell-init separator emitted only for tools whose stdout reaches output (installed + non-erroring), preserving the eval-safety invariant | Confirmed from intake #10 (user bulk-confirmed); maps onto the existing skip-on-`ErrNotFound` / drop-on-error loop | S:95 R:70 A:85 D:72 |
| 11 | Certain | Change type = `feat` (user-visible UX behavior change across three commands) | Confirmed from intake #11 (user bulk-confirmed); spec gate threshold for `feat` = 3.0 | S:95 R:85 A:85 D:80 |
| 12 | Certain | Empty-case short-circuits get NO summary tail and NO per-tool header | Confirmed from intake #12 (user confirmed in clarify); no per-tool loop runs, so nothing to separate or count | S:95 R:65 A:55 D:50 |
| 13 | Certain | Exact ANSI styling: hand-rolled standard SGR codes as named constants in `ui.go` (bold-cyan `▸`, green `✓`) — NO external color library | Confirmed from intake #13 (user confirmed in clarify); keeps dep footprint minimal; fits "named constants, no magic strings" | S:95 R:80 A:60 D:55 |

13 assumptions (13 certain, 0 confident, 0 tentative, 0 unresolved).

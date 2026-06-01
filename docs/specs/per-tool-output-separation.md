# Spec: Per-Tool Output Separation

> **Status**: Design intent (pre-implementation). Captured during a `/fab-discuss` session.
> Ready to feed a `/fab-new` change. No code written yet.

## Problem

`shll update` (and the other multi-tool commands) foreground each sub-tool's output
directly to the terminal (`proc.RunForeground` → inherited stdio). The result is one
undifferentiated wall of text: there is no visual marker telling the user where `rk`'s
output ends and `tu`'s begins. The same applies to `install` and `shell-init`.

Because sub-tool output is *streamed* (foregrounded, not captured), shll never sees the
bytes — it can only print **around** each subprocess, not reformat what's inside. The
realistic lever is therefore a **header/separator that shll prints before each tool's
section**.

## Goal

Make the boundary between tools obvious during `update`, `install`, and `shell-init` by
printing a labeled header before each tool's output, plus a one-line summary tail for the
upgrade commands.

## Scope

| Command | In scope? | Mechanism |
|---------|-----------|-----------|
| `update` | ✅ | Per-tool header to **stdout** + summary tail |
| `install` | ✅ | Per-tool header to **stdout** + summary tail |
| `shell-init` | ✅ | Per-tool separator as a **shell comment** in stdout |
| `version` | ❌ | Excluded — version lines already self-label (`rk 1.5.0`); whitespace grouping suffices. No header. |

`version` is deliberately out of scope: a header (`▸ rk`) before a line that already reads
`rk 1.5.0` is redundant, and a success/failure summary tail is meaningless for a
read-only aggregation.

## Design

### Header style

One labeled line per tool, immediately before that tool's foregrounded output:

- **TTY + color enabled** → `▸ <tool>` (bold cyan arrow, bold tool name). Tool output
  keeps its native color.
- **Piped / `NO_COLOR` set / non-TTY** → `==> <tool>`, no ANSI. The degrade swaps both the
  glyph (`▸` → `==>`) **and** any Unicode in shll's own output (e.g. `→` → `->`) so logs
  and CI stay clean ASCII. (Sub-tool output is passed through untouched either way.)

The `==>` idiom matches Homebrew's existing convention, so the plain form reads naturally
alongside brew's own output.

> **Note — spec-mandated wording literals are exempt from the glyph-degrade rule.** The degrade
> applies only to *swappable* glyphs (`▸`→`==>`, dropping the green `✓`). The em-dash `—` in the
> summary tail and the box-drawing `─` in the shell-init separator (`# ── <tool> ──`) are
> spec-defined wording, kept verbatim in BOTH branches — not degraded. They carry no
> eval-safety/CI risk: the em-dash sits in a human-readable run-report line that is never eval'd,
> and the box-drawing chars sit inside a `#` shell comment (a no-op when eval'd).

For `update`, the existing instant status line (`Checking installed sahil87 tools…`,
`updateStatusLine`) is unchanged and still printed first. `shll (self)` gets a header too
when the self-upgrade step runs.

### Stream discipline (critical)

| Command | Header stream | Rationale |
|---------|---------------|-----------|
| `update` / `install` | **stdout** | Sub-tool output is foregrounded to stdout. The header MUST share that stream — printing to stderr would interleave unpredictably against stdout (different buffers, different flush timing). |
| `shell-init` | **stdout**, as a shell comment | stdout is consumed by `eval "$(shll shell-init <shell>)"`. A bare header would be eval'd as a command and break the shell. A `#`-prefixed line is a shell no-op (eval-safe) and still visible when the output is read. |

### `shell-init` separator — the deliberate exception

`shell-init` does **not** use the `▸`/`==>` header. It emits a shell-comment separator:

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
  It is NOT an oversight. A future change MUST NOT "unify" shell-init onto the `▸` header —
  doing so reintroduces the eval-break. Document this exception in memory at hydrate time.

### Summary tail (`update` / `install` only)

After all tools run, print one honest line based on **exit codes**:

```
Done — 6 of 6 tools succeeded.
```

or, on partial failure:

```
5 succeeded, 1 failed — see above.
```

**Honesty constraint**: because sub-tool output is streamed (not captured), shll cannot
distinguish "actually upgraded" from "already up-to-date" — it only knows each tool's exit
code. The tail therefore reports **succeeded / failed counts**, never "updated vs.
up-to-date". Do not claim more than the exit code proves.

The tail is TTY-color-gated like the headers (e.g. a green `✓` on a TTY, plain on a pipe).
`shell-init` gets **no** tail (it produces a script, not a run report). `version` gets no
tail.

### Color gating

Color + Unicode glyphs are emitted only when **both**:

1. `stdout` is a TTY (`term.IsTerminal(fd)` from `golang.org/x/term`), **and**
2. `NO_COLOR` is unset (honor the [no-color.org](https://no-color.org) convention).

Otherwise: plain ASCII, no ANSI. This is shll's **first** terminal inspection, so
`golang.org/x/term` is a new (small, idiomatic) dependency — a deliberate addition, not
incidental.

`shell-init` ignores color gating entirely — always plain ASCII comments (see above).

## Implementation notes (non-binding — for the eventual change)

1. **Single shared helper.** TTY detection, the `NO_COLOR` check, the glyph choice, and
   `printToolHeader(w, name)` should live in one place (e.g. a new `ui.go` / `term.go` in
   `src/cmd/shll/`) — not duplicated across `update.go` / `install.go`. The shell-init
   comment-emitter (`printToolComment` or similar) is a sibling in the same file, sharing
   nothing but the file.
2. **Injectable color decision for tests.** Expose a `forceColor` / `forcePlain` seam (or a
   `colorEnabled bool` parameter) so tests are not TTY-dependent. `bytes.Buffer` test
   writers are not TTYs, so tests naturally exercise the **plain ASCII** branch — assert
   against the `==>` / comment forms.
3. **Golden-string churn.** Several existing tests assert verbatim stdout (e.g.
   `TestUpdate_NoToolsInstalled` expects exactly
   `Checking installed sahil87 tools…\nNo sahil87 tools installed.\n`). Adding headers and
   the tail changes these golden strings. Per the constitution's **Test Integrity** rule,
   update this spec / the expected output first, then conform the tests — never bend the
   implementation to satisfy a stale fixture.
4. **Constitution check.** No new subprocess (still all through `internal/proc`). No
   sub-tool logic reimplemented (Constitution III/IV intact — shll still only prints
   around each subprocess). The one new dependency is `golang.org/x/term` for TTY
   detection.

## Affected areas

- `src/cmd/shll/update.go` — per-tool header before each `upgradeTool` call + summary tail
- `src/cmd/shll/install.go` — per-tool header + summary tail
- `src/cmd/shll/shell_init.go` — shell-comment separator before each tool's init block
- `src/cmd/shll/ui.go` *(new)* — shared header/color/TTY helper
- `src/cmd/shll/*_test.go` — golden-string updates for the three touched commands
- `src/go.mod` — add `golang.org/x/term`

## Affected memory (for hydrate)

- `docs/memory/cli/update.md` — header + tail behavior, stream discipline
- `docs/memory/cli/install.md` — header + tail behavior
- `docs/memory/cli/shell-init.md` — shell-comment separator + the eval-safety exception
- `docs/memory/cli/commands.md` *(maybe)* — note the shared UI helper
- `docs/memory/internal/` *(maybe)* — if the UI helper warrants its own note

## Open questions

None blocking. The `version` exclusion and the `shell-init` comment-vs-header fork were
resolved during the discuss session.

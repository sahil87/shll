# Intake: Post-install "Next steps" nudge in `shll install`

**Change**: 260716-93r2-install-next-steps-nudge
**Created**: 2026-07-16

## Origin

Created via `/fab-proceed` promptless dispatch from a driving conversation (synthesized feature
description passed to the intake subagent; no interactive questioning — `{questioning-mode} =
promptless-defer`). The conversation had already made the load-bearing decisions; they are captured
verbatim below and encoded as graded assumptions.

> **Post-install "Next steps" nudge in `shll install`.** The shll.ai homepage one-liner
> (`curl -fsSL https://shll.ai/install | sh`) runs `scripts/install.sh`, which ends with
> `exec shll install "$@"` — so the last thing a bootstrap user sees is `shll install`'s output,
> which today ends with just the summary tail (`printSummaryTail`, src/cmd/shll/install.go). The two
> follow-on steps — `shll shell-setup` (wire shell integration) and `run-kit agent-setup` (optional,
> once per machine: agent state in run-kit's dashboard) — are documented only on shll.ai's
> getting-started/install page. Homepage copy-pasters never see them and silently miss shell
> integration. The only existing CLI nudges live in `shll doctor` (`suggestNotWired`: "not wired —
> run 'shll shell-setup' then 'exec $SHELL'"), which a fresh user has no reason to run.
>
> Decisions made in the conversation:
> 1. Add a "Next steps" block to `runInstall` (src/cmd/shll/install.go), printed after the summary tail.
> 2. shell-setup nudge is gated on actual wiring state: reuse the existing rc-file wiring detector
>    that `shll shell-setup` owns and `shll doctor` already reuses — only print the nudge (run
>    `shll shell-setup`, then `exec $SHELL`) when the shll sentinel block is NOT present in the
>    user's rc file. No nudge for already-wired users.
> 3. The shell-setup nudge also fires on the "All sahil87 tools already installed." short-circuit path.
> 4. run-kit agent-setup nudge is gated on run-kit being installed (after this run). shll cannot
>    cheaply know whether agent-setup has already run — that is run-kit-internal state, and
>    Constitution II (no state) / III (wrap, don't reinvent) forbid shll probing or parsing it — so
>    the nudge is an informational line marked "optional, once per machine", mirroring the site's
>    install-guide wording. Accepted trade-off: it prints even for users who already ran agent-setup.
>    (A stricter alternative — only when this run actually installed run-kit — was noted as a
>    fallback if the line proves too noisy.)
> 5. `--dry-run` prints no nudges — it is a command preview, not an outcome.

Related context: shll.ai PR #90 (separate repo, already merged-track) adds the same follow-on steps
under the homepage one-liner. This change is the CLI-side, more robust fix. **No shll.ai changes are
in scope here.**

## Why

1. **The pain point.** The `curl -fsSL https://shll.ai/install | sh` bootstrap terminates in
   `exec shll install "$@"` (`scripts/install.sh`), so `shll install`'s output is the entire
   first-run experience. Today that output ends at the summary tail — a bootstrap user is never told
   about `shll shell-setup` (which wires `eval "$(shll shell-init <shell>)"` into their rc file) or
   `run-kit agent-setup`. Homepage copy-pasters silently miss shell integration: the tools are
   installed but hop/wt/tu shell functions never activate.
2. **If we don't fix it.** Fresh users end up in the exact state `shll doctor`'s `suggestNotWired`
   WARN describes — but a fresh user has no reason to run `doctor`, so the miss goes undetected
   until something visibly doesn't work. The docs on shll.ai cover the steps, but the homepage
   one-liner audience never reads the getting-started page.
3. **Why this approach.** The install command is the one surface every bootstrap user is guaranteed
   to see, and shll already owns a read-only wiring detector (doctor reuses shell-setup's
   `resolveShell`/`resolveRcFile`/`locateBlock` via `resolveWiringFact`), so the shell-setup nudge
   can be precisely gated — zero noise for already-wired users, and idempotent re-runs stay clean.
   The site-side fix (shll.ai PR #90) helps but only for users who read the page; the CLI-side nudge
   is state-aware and reaches everyone.

## What Changes

### 1. "Next steps" block in `runInstall` (src/cmd/shll/install.go)

A new nudge block printed to **stdout** at the end of `runInstall`, on both non-preview outcome
paths:

- **Install-loop path**: after `printSummaryTail(...)` (which follows the per-tool install loop).
- **Short-circuit path**: after the `allInstalledMsg` line (`"All sahil87 tools already
  installed."`) — a re-runner who never wired their shell still gets nudged (decision 3).
- **Never on `--dry-run`**: the dry-run branch returns after `printInstallPreview` as today, with no
  nudges (decision 5). The brew-missing and unknown-target error paths also return before any nudge
  (unchanged early returns).

The block contains up to two lines, each independently gated:

1. **shell-setup nudge** — printed only when the shll sentinel block is NOT wired in the user's rc
   file (see §2 for the gate). Content: run `shll shell-setup`, then `exec $SHELL` — consistent with
   doctor's `suggestNotWired` wording ("not wired — run 'shll shell-setup' then 'exec $SHELL'").
2. **run-kit agent-setup line** — printed only when run-kit is installed after this run (see §3).
   Informational, marked **"optional, once per machine"**, mirroring the shll.ai install-guide
   wording.

When neither gate fires (wired user, run-kit absent), the block is omitted entirely — no empty
"Next steps:" header. Output shape (illustrative; exact wording finalized at apply, see Assumptions
#8):

```
Summary: 3/3 installed (42s)

Next steps:
  → shll shell-setup    # wire shell integration into your rc file, then: exec $SHELL
  → run-kit agent-setup # optional, once per machine — agent state in run-kit's dashboard
```

Framing conventions: the block goes to **stdout** (same stream as the per-tool headers and summary
tail), a blank line precedes it (the existing section-spacing rule), and any styling reuses the
existing `ui.go` helpers (`colorEnabled` computed once against stdout, `bold`/`arrow` etc.) so
color/TTY gating is identical to the header/tail framing. All message strings are **named
constants** per code-quality.md (no magic strings) — mirroring `allInstalledMsg`,
`shllSelfInstallNote`, and doctor's `suggestNotWired`.

Gating sketch (both nudges computed the same way on both outcome paths):

```go
// after printSummaryTail / after allInstalledMsg — never in dry-run:
w := resolveWiringFact(env) // read-only; env threaded for testability (see §4)
if w.shellResolved && !w.corrupt && !w.wired {
    // print shell-setup nudge line
}
if toolInstalled(ctx, runKitTool) { // post-run re-probe — stateless re-derive (Constitution II)
    // print run-kit agent-setup informational line
}
```

### 2. shell-setup nudge gate: reuse the existing wiring detector (read-only)

Reuse `resolveWiringFact(env func(string) string) wiringFact` (currently in `src/cmd/shll/doctor.go`,
same `main` package — no move required), which is the established read-only composition of
shell-setup's own primitives: `resolveShell` → `resolveRcFile` → `os.ReadFile` → `locateBlock` →
`hasEval` (covers both the new `# >>> shll >>>` and legacy `# >>> shll shell-init >>>` sentinels).

- **Nudge condition**: `shellResolved && !corrupt && !wired`. Quiet on the two edge states:
  unresolvable `$SHELL` (nudging toward `shll shell-setup` would fail with exit 2 for e.g. fish) and
  a corrupt (open-without-close) block (shell-setup refuses to modify it — nudging there is a dead
  end; `doctor` owns that diagnostic). This mirrors doctor's own separation of `suggestNotWired`
  vs. `suggestCorruptBlock`.
- Strictly **read-only** reuse, exactly like doctor: `os.ReadFile` + parse; `shll install` never
  writes, creates, or migrates the rc file. No new detection logic is written (Constitution III —
  one detection path).
- If code organization warrants, the detector may be acknowledged as shared infrastructure in
  comments/memory, but the function itself is reused as-is.

### 3. run-kit agent-setup line gate: run-kit installed after this run

- **Gate**: run-kit present after the run completes — re-derived via the existing shared install
  probe (`toolInstalled` / `isInstalled`, version.go/brew.go family) rather than by tracking what
  this run did (Constitution II — stateless re-derive). A post-run probe uniformly covers: run-kit
  just installed this run, run-kit pre-installed (incl. the short-circuit path), the rk→run-kit
  migration path, and subset runs (`shll install hop`) where run-kit wasn't in the install set but
  is present on the machine.
- **Why not gate on "agent-setup already ran"**: that is run-kit-internal state; Constitution II (no
  state) and III (wrap, don't reinvent) forbid shll probing or parsing it. Hence the line is
  informational and marked "optional, once per machine" — the accepted trade-off is that it prints
  even for users who already ran agent-setup (decision 4).
- **Recorded fallback** (not implemented now): if the line proves too noisy, tighten the gate to
  "only when this run actually installed run-kit".

### 4. Tests (src/cmd/shll/install_test.go, test-alongside)

`runInstall` grows an env seam for the wiring probe, mirroring `runDoctor`'s established pattern
(`runDoctor(ctx, jsonOut, env func(string) string, stdout, stderr)`; production passes `os.Getenv`,
tests pass a map-backed `envFunc`). Tests drive `runInstall` with a fake `proc.Runner`,
`bytes.Buffer` writers, and a `t.TempDir()` rc file reached via the faked `$SHELL`/`$HOME`
(/`$ZDOTDIR`) env. Required cases:

- shell-setup nudge **shown** when the rc file has no shll block (unwired)
- shell-setup nudge **hidden** when the rc block with the eval line is present (wired)
- agent-setup line **gated on run-kit presence** — shown when run-kit installed, hidden when absent
- **nothing** printed on `--dry-run` (preview output only, no nudges)
- **short-circuit path** ("All sahil87 tools already installed.") still nudges when unwired

## Affected Memory

- `cli/install`: (modify) document the post-install "Next steps" block — emission points (loop tail
  + short-circuit), the two gates, dry-run exclusion, constants, and the bootstrap-UX rationale
  (`scripts/install.sh` `exec` target)
- `cli/doctor`: (modify) note that `resolveWiringFact` is now reused by `shll install`'s nudge gate
  as well (still strictly read-only)
- `cli/shell-setup`: (modify) cross-references — the read-only reuse list (currently "doctor only")
  gains `shll install`

## Impact

- **Code**: `src/cmd/shll/install.go` (nudge block + gates + constants; `runInstall` signature gains
  an env parameter), `src/cmd/shll/install_test.go` (new cases; existing call sites updated for the
  signature). Read-only reuse of `doctor.go`'s `resolveWiringFact` and the shared install probe
  (`toolInstalled`) — no changes to `shell_setup.go`, `doctor.go` behavior, or `ui.go` expected
  (new constants live in install.go per existing convention).
- **Constitution fit**: I — no new subprocess paths (wiring probe is file I/O; install probe already
  routes through `internal/proc`); II — all gates re-derived per invocation, no state; III/IV —
  reuses existing detectors/probes, composes `shll shell-setup` and `run-kit agent-setup` by
  *pointing at them*, never absorbing them; V — nudges degrade silently (unresolvable shell, corrupt
  block, run-kit absent → line simply omitted); VII — no new subcommand, additive output on an
  existing one.
- **Out of scope**: any shll.ai repo change (PR #90 covers the site side); `shll doctor` wording;
  `shll update`/`uninstall` output.

## Open Questions

None — the driving conversation resolved all consequential decisions; residual choices are graded
below (no Unresolved rows).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Nudge block prints after `printSummaryTail` on the install-loop path AND after `allInstalledMsg` on the short-circuit path; never on `--dry-run` | Discussed — conversation decisions 1, 3, 5 verbatim | S:95 R:90 A:95 D:95 |
| 2 | Certain | shell-setup nudge gated on actual rc wiring via the existing read-only detector (`resolveWiringFact` composing shell-setup's `locateBlock`); no nudge for wired users | Discussed — decision 2; detector already exists and doctor proves the reuse pattern | S:95 R:85 A:95 D:95 |
| 3 | Certain | run-kit agent-setup line gated only on run-kit installed after the run, marked "optional, once per machine"; prints even if agent-setup already ran (accepted trade-off, stricter gate recorded as fallback) | Discussed — decision 4; Constitution II/III forbid probing run-kit-internal state | S:90 R:80 A:95 D:90 |
| 4 | Certain | Nudges to stdout with existing color/TTY framing (`colorEnabled` et al.); messages as named string constants; tests alongside in install_test.go | Constraints stated verbatim in conversation; code-quality.md and existing install.go constants confirm | S:90 R:90 A:95 D:95 |
| 5 | Confident | Nudges print regardless of per-tool install failures (`anyFailed`) — the block is informational and the tail already conveys failures | Not discussed; one-line condition, trivially reversible; wiring need is orthogonal to install outcome | S:40 R:90 A:70 D:65 |
| 6 | Confident | shell-setup nudge only when `shellResolved && !corrupt && !wired` — quiet on unresolvable `$SHELL` (shell-setup would exit 2) and corrupt block (shell-setup refuses; doctor owns that diagnostic) | Not discussed; doctor's suggestNotWired/suggestCorruptBlock separation gives the pattern | S:35 R:85 A:60 D:50 |
| 7 | Confident | run-kit presence re-derived by a post-run probe via the shared `toolInstalled` probe — uniform across loop, short-circuit, migration, and subset runs | Constitution II (stateless re-derive) plus decision 4's "installed (after this run)" wording | S:55 R:85 A:80 D:70 |
| 8 | Confident | Exact nudge wording finalized at apply, mirroring shll.ai install-guide wording and doctor's `suggestNotWired` phrasing (`run 'shll shell-setup' then 'exec $SHELL'`) | Low-stakes presentational choice; conversation fixed the semantic content, not the byte-exact text | S:50 R:95 A:75 D:60 |
| 9 | Confident | `runInstall` gains an `env func(string) string` parameter for the wiring probe (production passes `os.Getenv`), mirroring `runDoctor`'s established test seam | Not discussed; doctor.go sets the exact precedent; needed for the required rc-file test cases | S:45 R:80 A:80 D:70 |

9 assumptions (4 certain, 5 confident, 0 tentative, 0 unresolved).

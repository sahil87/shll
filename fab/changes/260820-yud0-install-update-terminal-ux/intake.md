# Intake: Two-region install/update terminal UX

**Change**: 260820-yud0-install-update-terminal-ux
**Created**: 2026-08-20

## Origin

Drafted via `/fab-draft` from the authored plan `fab/plans/sahil/desktop-roster-and-install-ux.md` (Change C section), a planning session with decisions confirmed by Sahil. One-shot draft — the plan section was written to be intake-ready (problem, scope, decisions, acceptance).

> Change C — `install-update-terminal-ux`: two-region terminal experience for `shll install`/`shll update` in Go — pinned status header, streaming child output, prompt-hang hardening, determinate OSC 9;4 progress.

## Why

1. **Pain point**: Long roster walks (`shll install`, `shll update`) interleave per-tool output with no persistent view of overall progress — the user can't see which tool is running, how many remain, or what's next without scrolling. Separately, a hypothetical child prompt would hang the run invisibly (stdin is inherited today).
2. **Consequence if unfixed**: multi-minute roster runs stay opaque; any tool that ever grows an interactive question stalls the whole walk with no visible cause.
3. **Why this approach**: a pinned status region (DECSTBM scroll region) gives a persistent header while child output streams naturally beneath — no full-screen TUI, no output buffering. Prompt-hang hardening **enforces** the toolkit's prompt-free standard (installs/updates are prompt-free by standard) rather than accommodating prompts: children get stdin from `/dev/null`, and a failure shows the captured output tail. If a genuine question is ever unavoidable, the tool reads `/dev/tty` explicitly — shll does not reopen stdin for it.

## What Changes

### Pinned status region (tty only)

For `shll install` and `shll update` on a tty:

- Top line(s) show current tool, honest `k/n` step count, and next tool — e.g. `Installing run-kit (2/7) · next: rk-desktop`.
- Child output streams beneath via a DECSTBM scroll region (margins set below the header; no alternate screen).
- Restore the scroll region on **every** exit path — `defer` on normal return and error, plus signal handling (Ctrl-C) — and re-apply the region on SIGWINCH (terminal resize).

### Degradation

- Not-a-tty (CI, pipes): keep today's sequential `printToolHeader` output **exactly** — `shll update | cat` output is byte-identical in structure to today's.
- `NO_COLOR` respected exactly as today.

### Prompt-hang hardening

- Children run with stdin from `/dev/null` (no inherited stdin).
- On child failure, print the captured output tail so the cause — including an attempted prompt — is visible.
- This enforces the prompt-free standard; no interactive accommodation is added.

### Determinate OSC 9;4 progress

- Wire determinate `pos/total` OSC 9;4 through the existing plumbing in `src/cmd/shll/progress.go` (including its tmux passthrough), replacing/extending the current indeterminate-style emission so the terminal tab and the run-kit dashboard tile show a real progress bar.
- Progress is honest `k/n` steps over roster tools — never a synthetic percent across tools (explicit plan decision).
- Clear on finish; emit error state on failure.

### Implementation seams

- Reuse/extend `src/cmd/shll/ui.go` helpers; keep everything unit-testable through the existing writer/tty seams (as `progress_test.go` and `ui_test.go` do today).
- Touches `install.go`, `update.go`, `ui.go`, `progress.go` and their tests.

## Affected Memory

- `cli/install`: (modify) tty two-region UX, stdin `/dev/null` hardening, failure-tail behavior
- `cli/update`: (modify) same UX; the existing OSC 9;4 description changes from the current emission to determinate `pos/total` with clear/error states
- `cli/commands`: (modify) only if shared UI/tty seams documented there change shape

## Impact

- **Code**: `src/cmd/shll/install.go`, `update.go`, `ui.go`, `progress.go` + `_test.go` files. Subprocess changes stay inside `internal/proc` usage (Constitution I); stdin redirection is a proc-level concern.
- **External behavior**: tty runs of `shll install`/`shll update` get a pinned header + scrolling child output and a determinate terminal progress bar; non-tty output is unchanged byte-for-byte in structure; a child that tries to prompt fails fast with visible output instead of hanging.
- **Standards**: `update` standard + prompt-free standard govern this surface — conformance check against `docs/site/standards/` before shipping.
- **Sequencing**: lands **after** Change A (`roster-desktop-entry`) — same files (`install.go`, `update.go`, `ui.go`), and the `k/n` count should reflect the final roster including rk-desktop. Queue order A → B → C.

## Open Questions

- (none — the plan resolved all decision points; see Assumptions)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Progress is honest `k/n` steps, never a synthetic percent across tools; OSC 9;4 reuses `progress.go` plumbing | Explicit plan decision ("do not re-litigate") | S:90 R:80 A:90 D:95 |
| 2 | Certain | Hardening enforces the prompt-free standard (stdin `/dev/null`, fail fast + captured tail) — no interactive accommodation; a genuinely unavoidable question reads `/dev/tty` in the tool itself | Explicit plan decision; matches the toolkit's prompt-free standard | S:90 R:80 A:90 D:90 |
| 3 | Confident | Two-region layout = DECSTBM scroll region with header above, not an alternate-screen TUI | Plan names DECSTBM explicitly; restore-on-exit + SIGWINCH re-apply specified | S:85 R:70 A:75 D:80 |
| 4 | Confident | Non-tty output stays byte-identical in structure to today's `printToolHeader` flow | Acceptance sketch states it verbatim; existing tty seams make it testable | S:85 R:80 A:85 D:90 |
| 5 | Confident | Header content is `current tool + k/n + next tool` (e.g. `Installing run-kit (2/7) · next: rk-desktop`); exact styling decided at apply within `ui.go` conventions | Plan gives the example shape; cosmetic details are trivially reversible | S:70 R:90 A:80 D:70 |

5 assumptions (2 certain, 3 confident, 0 tentative, 0 unresolved).

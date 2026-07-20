# Intake: Output polish + --dry-run for shll update and install

**Change**: 260604-6vuo-update-install-polish-dry-run
**Created**: 2026-06-04
**Status**: Draft

## Origin

This change was synthesized from a detailed `/fab-discuss` design session about improving the
during-run and pre-run experience of `shll update` and `shll install`. It builds **additively** on
the already-shipped per-tool-output framing (change y630): headers via `printToolHeader`, the
summary tail via `printSummaryTail`, and TTY/`NO_COLOR` gating via `colorEnabled` — all already
present in `src/cmd/shll/ui.go`. This intake is the sole source of the design; it does not pull from
any other draft.

The discussion locked four additive features, applied to **both** `shll update` and `shll install`:

1. Progress counters in the per-tool header (`▸ [N/M] <tool>`).
2. Section spacing (blank line before each header except the first).
3. Wall-clock run duration in the summary tail.
4. A `--dry-run` flag on both commands that previews the plan and exits without side effects.

Key decisions reached and their rationale are reproduced below (and encoded in the Assumptions
table). The direction is explicitly **"polish the current streaming model"** — NOT a TUI/dashboard,
NOT capturing-and-reframing child output, NOT adding any heavy dependency (no lipgloss/bubbletea/charm).

## Why

`shll update` and `shll install` are long, side-effectful, multi-tool operations the user starts and
then watches and waits on (often a multi-minute run). Three pain points today:

1. **No progress sense during the run.** The per-tool headers are a flat list (`==> wt`, `==> idea`, …)
   with no `[N/M]` denominator and no blank-line separation, so the user cannot tell "how far along
   am I" and each tool's streamed output runs straight into the next tool's header — exactly the
   "wall of text" the per-tool-output-separation spec set out to fix, only partially addressed.
2. **No timing feedback after.** The summary tail (`Done — 6 of 6 tools succeeded.`) says nothing
   about how long the run took, which is the single most-asked question after a long upgrade.
3. **No way to preview the plan before committing.** There is NO dry-run today, so the only way to
   learn what `shll update`/`install` will do is to run it (and incur all its side effects). This is
   the biggest DX/anxiety-reduction lever — and it is nearly free, because the probe phase
   (`probeRoster` + the install partition loop) already computes the entire plan.

Why this approach over alternatives: the streaming model (`proc.RunForeground` with inherited stdio)
is deliberate and correct — shll never sees the sub-tool bytes, it only frames *around* them
(Constitution III/IV: compose, don't absorb / wrap, don't reinvent). A TUI/dashboard would require
capturing-and-reframing child output and a heavy dependency, breaking that model. The right lever is
to improve shll's own framing (headers, spacing, tail) and add a read-only preview — all additive,
all on top of the existing `ui.go` seam.

## What Changes

ONE change covering FOUR features across BOTH `update` and `install`. `version` and `shell-init` are
**out of scope** (consistent with change y630 — `shell-init` must stay eval-safe and gets no
headers/tail/counters; `version` self-labels).

### 1. Progress counters in the per-tool header

The per-tool header gains an `[N/M]` progress counter:

- **Color TTY** → `▸ [N/M] <tool>`
- **Plain / non-TTY / `NO_COLOR`** → `==> [N/M] <tool>`

`N` is the 1-based position in the sequence of tools being acted on; `M` is the total count of tools
that will be acted on this run.

**Denominator must be known BEFORE the per-tool loop starts.** Today `total` is computed
*incrementally* inside the loop (`update.go`: `total++` per tool; `install.go`: `len(missing)` is
already known up front). For `update`, the count is `(number of installed roster tools) + (1 if shll
itself is brew-installed)`. The probe results (`probes []probeResult`) and the `shllSelfInstalled`
bool are both computed before the loop, so `M` can be derived up front by counting
`probes[i].installed == true` plus the conditional self step. For `install`, `M = len(missing)`
(already known before the loop).

**DECISION (locked in discussion): `shll (self)` counts as `[1/N]`.** It is a tool being updated like
any other, and excluding it would make the counter disagree with the summary-tail counts (the tail's
`total` already includes the self step — see `update.go:144` `total++` for self). With shll
brew-installed and the full roster present, `update` headers read:

```
==> [1/7] shll (self)
==> [2/7] wt
==> [3/7] idea
==> [4/7] tu
==> [5/7] rk
==> [6/7] hop
==> [7/7] fab-kit
```

**The header stays minimal — JUST `▸ [N/M] <tool>`.** Do NOT add a dimmed command echo
(`$ tu update --skip-brew-update`). That idea was explicitly **considered and REJECTED** in the
discussion as visual noise that duplicates `--help`.

### 2. Section spacing

Print a blank line before each per-tool header **EXCEPT the first**, so each tool's streamed output is
visually separated from the next tool's header. This is the most literal fulfillment of the per-tool-
output-separation spec's stated goal ("make tool boundaries obvious"). Concretely, the `update`
sequence becomes (blank lines shown as ``⏎``):

```
==> [1/7] shll (self)
<shll's brew upgrade output…>
⏎
==> [2/7] wt
<wt's update output…>
⏎
==> [3/7] idea
…
```

The summary tail keeps its existing placement (after the loop). **CLARIFIED (user confirmed): a blank
line DOES precede the summary tail**, extending the same between-section spacing treatment to the tail —
the final tool's streamed output is separated from the summary tail by one blank line, consistent with
the "blank line before each per-tool header except the first" rule.
<!-- clarified: a blank line precedes the summary tail too, applying the same between-section spacing as the per-tool headers (blank line before each header except the first). The final tool's streamed output is separated from the tail by one blank line. User confirmed. -->

### 3. Summary timing

Append wall-clock run duration to the summary tail. **CLARIFIED (user confirmed): duration appears on
BOTH tail forms** — the full-success form and the partial-failure form. The success tail reads:

```
Done — 6 of 6 tools succeeded in 1m12s.
```

(with the green `✓` prefix on a color TTY). The partial-failure tail places the duration **before the
em-dash**:

```
X succeeded, Y failed in 1m12s — see above.
```

Both forms stay factual and restrained (`… in 1m12s`, no hype).
<!-- clarified: duration appended to BOTH tail forms — success reads "Done — N of M tools succeeded in <dur>." (green ✓ on TTY); partial-failure reads "X succeeded, Y failed in <dur> — see above." with the duration before the em-dash. User confirmed. -->

**This does NOT violate the existing honesty constraint.** The `ui.go` comment on `printSummaryTail`
is emphatic that the tail NEVER claims "updated" vs. "up-to-date" (shll only knows exit codes because
sub-tool output is streamed, not captured). Duration is a **fact about the run**, not a claim about
outcomes — it is additive and factual. Keep the existing honesty constraint intact; duration sits
alongside it. Use Go's `time.Duration` string form rounded to a sensible unit (e.g. `1m12s`); keep the
wording restrained.

**TESTABILITY CONSTRAINT (known, flagged here per discussion):** `update_test.go` and `install_test.go`
drive `runUpdate`/`runInstall` with `bytes.Buffer` writers and fake `proc.Runner`s and assert
**verbatim golden stdout** (e.g. `TestUpdate_HeadersAndTail` asserts the exact
`==> shll (self)\n==> wt\n…\nDone — 7 of 7 tools succeeded.\n`). A real wall-clock duration is
**non-deterministic**, so it cannot appear in a verbatim golden string as-is. Two viable mechanisms:

- **(Preferred) Injectable clock seam.** Pass a clock/`now` function (or start/elapsed seam) into
  `runUpdate`/`runInstall`, consistent with how the code already injects the `proc.Runner` for
  testability (the established pattern is a package-level swappable function — see
  `proc.Runner RunnerFunc` in `src/internal/proc/proc.go` and the `installFakeRunner` t.Cleanup
  helper). Tests then inject a deterministic clock (e.g. start `t0`, end `t0 + 72s`) and assert the
  exact `in 1m12s`. Production wiring passes the real `time.Now`.
- **(Fallback) Shape assertion.** Tests assert the tail *shape* (`in \d+...`) rather than an exact
  value.

**Prefer the injectable clock seam** — it is consistent with the existing `proc.Runner` injection
pattern and keeps the golden strings deterministic and exact. Resolve the exact mechanism (function
parameter vs. package-level var vs. a small `clock` interface; how `runUpdate`/`runInstall` signatures
change; whether the cobra `RunE` wires `time.Now`) at plan/apply time. The intake flags this as a known
constraint that the plan MUST address explicitly.

### 4. `--dry-run` flag on both `update` and `install`

A `--dry-run` bool flag on each cobra command. When set, the command prints exactly what it WOULD do —
using the probe results it **already computes** — then exits **WITHOUT any side effect**.

**CRITICAL HONESTY/SAFETY constraint: dry-run means "no writes, but probes still run".**

- **Reads are allowed in dry-run.** The read-only probes the command already runs are reads, not
  writes, and dry-run still runs them: `brew list --formula --versions` (install detection,
  `isInstalled`), and for `update`, the `<tool> update --help` substring check (`toolSupportsSkipFlag`,
  which determines whether `--skip-brew-update` would be appended). These must run so the preview is
  accurate.
- **Writes are forbidden in dry-run.** `brew update --quiet` is itself a **side effect** (it mutates
  brew's local metadata), so dry-run MUST NOT run it. Instead, the preview prints a line stating that
  the real run would refresh brew metadata first (e.g. `would refresh brew metadata first`). Likewise
  no `brew upgrade`, no `<tool> update`, no `brew install` is invoked in dry-run.

**`update` preview** lists, per tool that would be acted on, the **exact command** that would run
(the same argv `upgradeTool` would build), in roster order, with `shll (self)` first when applicable.
**CLARIFIED (user confirmed): the preview uses ALIGNED COLUMNS** — tool names are left-padded to a
common width (computed from the longest tool label present, including `shll (self)`) so the commands
line up in a readable column:

```
Would update 6 tools (brew metadata refresh first):
  shll (self)  brew upgrade sahil87/tap/shll
  wt           wt update --skip-brew-update
  idea         idea update
  tu           tu update --skip-brew-update
  rk           rk update
  hop          hop update --skip-brew-update
  fab-kit      fab-kit update
```
<!-- clarified: dry-run preview uses aligned columns — tool labels left-padded to a common width (longest label present, including "shll (self)") so commands align. User confirmed. -->

The exact per-tool argv depends on probe results:
- has `Update` argv + supports the flag → `<tool> update --skip-brew-update`
- has `Update` argv, no flag (version skew) → `<tool> update`
- no `Update` argv (hypothetical future tool) → `brew upgrade sahil87/tap/<formula>`
- `shll (self)` (when brew-installed) → `brew upgrade sahil87/tap/shll`

The "brew metadata refresh first" annotation reflects that the real run would call
`brew update --quiet` once before upgrades — but dry-run does NOT run it.

**`install` preview** lists, per not-yet-installed tool, the `brew install sahil87/tap/<formula>` it
would run, in roster order (the missing subset). It uses the **same aligned-column layout** as the
`update` preview — tool names left-padded to the longest label present — with a matching header line
(e.g. "Would install N tools:"):

```
Would install 4 tools:
  idea     brew install sahil87/tap/idea
  tu       brew install sahil87/tap/tu
  rk       brew install sahil87/tap/rk
  fab-kit  brew install sahil87/tap/fab-kit
```
<!-- clarified: install preview mirrors the update preview's aligned-column layout (tool names left-padded to the longest label present), with a matching "Would install N tools:" header. User confirmed. -->

(`install` has no metadata-refresh step, so its preview has no "refresh brew metadata first" line —
consistent with the no-`brew update` Design Decision in `cli/install`.)

**Graceful degradation in dry-run (Constitution V):** the preview lists only tools that would actually
be acted on — uninstalled tools are still skipped in `update`'s preview, and already-installed tools
are still skipped in `install`'s preview. The empty cases (`update`: nothing to upgrade; `install`:
all already installed) should produce a sensible "nothing to do" preview rather than an empty plan —
exact wording is a presentation detail (see Assumptions).

The remaining dry-run output formatting details (whether a single shared preview helper lives in
`ui.go`, header/counter/spacing applicability inside the preview) are resolved at plan/apply time. The
**column-alignment** question is now settled (CLARIFIED: aligned columns, padded to the longest label
present — see feature #4 above). The contract this intake locks: **no writes, probes still run, `brew
update` NOT executed, preview lists exact per-tool argv in aligned columns, only actionable tools
listed, exit 0 with no side effect.**

### Scope decisions (locked in discussion)

- **ONE change** covering all four features across **BOTH** `update` and `install` (user explicitly
  chose "one change" and "yes, to install also").
- **Direction is "polish the current streaming model" — NOT a TUI/dashboard.** Keep streaming sub-tool
  output live via `proc.RunForeground` (inherited stdio). Do NOT capture-and-reframe child output. Do
  NOT add lipgloss/bubbletea/charm or any new heavy dependency. The point is to improve shll's OWN
  framing AROUND the live stream (Constitution III/IV).
- **`version` and `shell-init` are OUT of scope** (consistent with the shipped per-tool-output-
  separation change — `shell-init` must stay eval-safe and gets no headers/tail/counters; `version`
  self-labels).

## Affected Memory

- `cli/update`: (modify) — document `[N/M]` counters (self counts as `1`), section spacing (blank line
  before each header except the first), duration in the summary tail (and explicitly that duration
  does NOT break the honesty constraint — it is a run fact, not an outcome claim), and the `--dry-run`
  contract (no writes; probes still run; `brew update` is NOT executed; preview lists the exact
  per-tool argv `upgradeTool` would build; only actionable tools listed; exit 0).
- `cli/install`: (modify) — mirror the same for `install`: `[N/M]` counters over the missing subset,
  section spacing, duration in the tail, and the `--dry-run` contract (no writes; `isInstalled` probes
  still run; preview lists `brew install sahil87/tap/<formula>` per missing tool; no metadata-refresh
  line since install has none; exit 0).
- `cli/commands`: (modify, decide at hydrate) — note the `--dry-run` flag on the `update`/`install`
  command surface; possibly note any new shared `ui.go` preview helper / the extended
  `printToolHeader`/`printSummaryTail` signatures, and the new clock seam if one is added.

## Impact

Affected code:

- **`src/cmd/shll/update.go`** — compute the counter denominator `M` up front (count
  `probes[i].installed` + the conditional `shllSelfInstalled` self step) instead of incrementally;
  pass `[N/M]` position into the header; emit blank-line spacing before each header except the first;
  append duration to the tail; add the `--dry-run` branch + cobra flag wiring; add the dry-run preview
  printer; thread the clock/now seam through `runUpdate`.
- **`src/cmd/shll/install.go`** — mirror: `[N/M]` over `len(missing)`, spacing, duration, `--dry-run`
  branch + flag, preview printer, clock seam through `runInstall`.
- **`src/cmd/shll/ui.go`** — likely extend `printToolHeader` to accept position/total (or add a new
  variant) and `printSummaryTail` to accept a duration; keep the TTY/`NO_COLOR` gating
  (`colorEnabled`) and the em-dash/glyph-degrade rules intact. A shared dry-run preview helper may live
  here too. Keep `ui.go` presentation-only — NO subprocess calls (it makes none today).
- **`src/cmd/shll/update_test.go` / `install_test.go`** — golden-string updates for the new
  header (`[N/M]`), spacing (blank lines), and tail (duration) shape; new tests for `--dry-run` preview
  output and for the **no-writes guarantee** (assert `brew update --quiet`, `brew upgrade`, `<tool>
  update`, and `brew install` are NOT in the recorded calls in dry-run, while the read-only probes ARE).
  `version_test.go` and `shell_init_test.go` are unaffected. Per the constitution's **Test Integrity**
  rule, the golden-string churn MUST update the spec/expected output first, then conform the tests —
  never bend the implementation to a stale fixture.
- **`src/go.mod`** — NO new dependency expected. Reuse `golang.org/x/term` (already present, change
  y630). `cobra` (already present) supplies the flag. `time` is stdlib.

Constitution check:

- **Principle VII (Minimal Surface Area).** This change adds **NO new top-level subcommand**.
  `--dry-run` is a **flag** on two existing commands (`update`, `install`), and the cosmetic changes
  (counters, spacing, duration) are behavior on existing commands. The code-review rule "New top-level
  subcommands need a Constitution VII justification line" is **not triggered** — there is no new
  subcommand. The "could this be a flag on an existing subcommand?" test is satisfied: it *is* a flag.
  Recorded here explicitly per that rule.
- **Principle I (Security First).** No new subprocess patterns; all execution still routes through
  `internal/proc`. `--dry-run` REDUCES execution (skips all writes).
- **Principle V (Graceful Degradation).** Dry-run preview still skips uninstalled tools
  (`update`) / already-installed tools (`install`) — only actionable tools are listed.
- **Principle II (No DB/State)** and **Principle IV (Composition, Not Replacement)** are intact — no
  state is added, and shll still only prints *around* each subprocess.

Change type: **feat** (user-visible UX/DX behavior change across two commands; adds a new flag).

## Open Questions

- Exact mechanism for the injectable clock/now seam (function parameter on `runUpdate`/`runInstall` vs.
  package-level swappable var like `proc.Runner` vs. a small `clock` interface). Preference: mirror the
  `proc.Runner` package-level-var pattern, but the precise signature is a plan/apply decision.
- Duration rounding/format (e.g. `1m12s` via `time.Duration.Round(time.Second).String()`, or a custom
  formatter for sub-second runs). Resolve at apply; keep it restrained and factual.
- ~~Placement of the duration in the **partial-failure** tail form~~ — **RESOLVED (clarified):** the
  duration appears on BOTH tail forms; the partial-failure form reads `X succeeded, Y failed in <dur>
  — see above.` (duration before the em-dash).
- ~~Whether a blank line precedes the summary tail~~ — **RESOLVED (clarified):** yes, a blank line
  precedes the summary tail, extending the between-section spacing rule to the tail.
- Exact dry-run preview formatting: ~~column alignment of the per-tool argv table~~ (**RESOLVED
  (clarified):** aligned columns, padded to the longest label present); whether a single shared preview
  helper lives in `ui.go`; whether counters/spacing apply inside the preview; the empty-case ("nothing
  to do") preview wording for both commands.

## Clarifications

### Session 2026-06-04

| # | Question | Resolution |
|---|----------|------------|
| 15 | Does the duration append to the partial-failure tail, or only the all-succeeded form? | BOTH forms get the duration. Success: `Done — N of M tools succeeded in <dur>.` (green ✓ on TTY). Partial-failure: `X succeeded, Y failed in <dur> — see above.` (duration before the em-dash). |
| 17 | Does a blank line precede the summary tail? | Yes — extend the between-section spacing rule (blank line before each per-tool header except the first) to the tail; the final tool's output is separated from the tail by one blank line. |
| 18 | Is the dry-run argv preview column-aligned? | Yes — aligned columns; tool labels left-padded to a common width computed from the longest label present (including `shll (self)`), for both `update` and `install` previews. |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | ONE change covering all four features across BOTH `update` and `install`. | User explicitly chose "one change" and "yes, to install also" in the discussion. | S:98 R:80 A:90 D:95 |
| 2 | Certain | Direction is "polish the current streaming model" — no TUI/dashboard, no capture-and-reframe, no lipgloss/bubbletea/charm or any heavy dependency. | Explicit discussion decision; aligns with Constitution III/IV (compose/wrap, don't absorb) and the existing `proc.RunForeground` streaming model. | S:97 R:70 A:92 D:92 |
| 3 | Certain | `version` and `shell-init` are OUT of scope (no headers/tail/counters/dry-run). | Explicit discussion decision; consistent with change y630 (shell-init eval-safety; version self-labels). | S:96 R:85 A:95 D:95 |
| 4 | Certain | `shll (self)` counts as `[1/N]` in the `update` counter (a tool like any other). | Explicitly locked in discussion; excluding it would make the counter disagree with the tail's `total` (which already includes the self step at `update.go:144`). | S:97 R:75 A:90 D:96 |
| 5 | Certain | Header stays minimal — JUST `▸ [N/M] <tool>`; NO dimmed command echo (`$ tu update …`). | Explicitly considered and REJECTED in discussion as visual noise duplicating `--help`. | S:96 R:80 A:88 D:94 |
| 6 | Certain | Duration is additive/factual and does NOT violate the tail honesty constraint (never claims "updated vs up-to-date"). | Discussion rationale + `ui.go` comment: duration is a run fact, not an outcome claim; the existing exit-code-only constraint stays intact. | S:95 R:65 A:90 D:90 |
| 7 | Certain | `--dry-run` runs read-only probes but performs NO writes; `brew update --quiet` is a side effect and MUST NOT run (preview annotates "would refresh brew metadata first"). | Explicit critical safety constraint from discussion; grounded in `update.go:118` (`brew update --quiet` mutates metadata) and the read/write distinction. | S:97 R:55 A:88 D:92 |
| 8 | Certain | `--dry-run` is a FLAG on two existing commands — NO new top-level subcommand; Constitution VII is not triggered (recorded explicitly per the code-review rule). | The "could this be a flag?" test is satisfied — it IS a flag; no new `AddCommand`. Constitution VII + code-review.md rule. | S:96 R:80 A:96 D:95 |
| 9 | Certain | Change type is `feat`. | User-visible UX/DX behavior + new flag; matches the fab-new keyword inference (no fix/refactor/docs signal dominates). | S:95 R:90 A:95 D:95 |
| 10 | Confident | Use the injectable clock/now seam (preferred) over shape-assertion to keep golden strings deterministic and exact. | Discussion stated a preference for an injectable seam consistent with the existing `proc.Runner` injection; both options are viable, seam is the front-runner. Reversible at apply. | S:80 R:70 A:80 D:75 |
| 11 | Certain | Dry-run preview lists the exact per-tool argv `upgradeTool`/the install loop would build, in roster order, `shll (self)` first for `update`. | Specified in the discussion with VERBATIM worked examples (the argv contract is locked); the argv is fully derivable from probe results + `upgradeTool` dispatch. Only the formatting/alignment is open (tracked separately as #18). | S:92 R:75 A:90 D:88 |
| 12 | Certain | Counter denominator `M` computed up front: `update` = installed-roster-count + (1 if shll brew-installed); `install` = `len(missing)`. | Dictated by feature #1's explicit constraint ("M must be known BEFORE the loop") — a derived necessity, not a chosen option; probes + `shllSelfInstalled` / `missing` are already computed before their loops. | S:93 R:78 A:92 D:90 |
| 13 | Confident | `ui.go` is extended (extend `printToolHeader` for position/total + `printSummaryTail` for duration, possibly a shared preview helper) while staying presentation-only with no subprocess calls. | Stated in the Impact section; aligns with the existing `ui.go` role (change y630) and Constitution I (ui.go makes no subprocess calls). Exact signature shape reversible at apply. | S:85 R:65 A:85 D:78 |
| 14 | Confident | Duration format is the example wording `1m12s` (Go `time.Duration` form), placement appended to the all-succeeded tail as `… succeeded in 1m12s.`. | Discussion gave the exact example `in 1m12s` verbatim — the format IS specified. Only the sub-second/rounding edge is open (settled at apply); the front-runner is unambiguous. | S:85 R:72 A:78 D:80 |
| 15 | Certain | Duration appears on BOTH tail forms: success reads `Done — N of M tools succeeded in <dur>.` (green ✓ on TTY); partial-failure reads `X succeeded, Y failed in <dur> — see above.` with the duration before the em-dash. | Clarified — user confirmed. | S:95 R:72 A:55 D:50 |
| 16 | Confident | Empty-case dry-run previews print a "nothing to do" line mirroring the existing `No sahil87 tools installed.` (update) / `All sahil87 tools already installed.` (install) messages rather than an empty plan. | Graceful-degradation intent is explicit, and the existing non-dry-run empty-case messages (`update.go:108`, `install.go:69`) give an unambiguous front-runner to mirror. Easily adjusted. | S:78 R:75 A:80 D:76 |
| 17 | Certain | A blank line precedes the summary tail too, extending the between-section spacing rule (blank line before each per-tool header except the first) to the tail — the final tool's output is separated from the tail by one blank line. | Clarified — user confirmed. | S:95 R:80 A:55 D:48 |
| 18 | Certain | The per-tool argv preview uses aligned columns — tool labels left-padded to a common width computed from the longest label present (including `shll (self)`) — for both `update` and `install` previews. | Clarified — user confirmed. | S:95 R:78 A:58 D:50 |

18 assumptions (14 certain, 4 confident, 0 tentative, 0 unresolved).

# Intake: shell-install --trust-tap (Homebrew tap-trust resolution)

**Change**: 260601-l6lo-shell-install-trust-tap
**Created**: 2026-06-01
**Status**: Draft

## Origin

Initiated from a `/fab-discuss` session investigating a recurring Homebrew warning the
user kept seeing through shll:

```
Warning: Tap sahil87/tap is allowed by default.
Homebrew will require explicit trust for non-official taps in a future release.
Set `HOMEBREW_REQUIRE_TAP_TRUST=1` to require explicit trust now or
`HOMEBREW_NO_REQUIRE_TAP_TRUST=1` to keep allowing by default.
Hide these hints with `HOMEBREW_NO_ENV_HINTS=1` (see `man brew`).
```

The conversation traced the warning's source, identified that shll *amplifies* it (multiple
`brew` invocations per `shll update` → the hint prints 2–3×), and worked through the design
space conversationally. Key turns:

- User reframed the goal from "how do I silence it" to "what is shll's responsibility to its
  users" — a docs + tool question, not a personal workaround.
- User proposed `shll shell-install --add-homebrew-trust` as the surface.
- Agent pushed back on naming ambiguity and on whether a *genuine trust* mechanism even exists
  in brew. **Verified on the user's machine (Homebrew 5.1.14):** brew exposes a real, scriptable
  trust ceremony (`brew trust --tap <tap>` / `brew untrust --tap <tap>`), gated by the
  `HOMEBREW_REQUIRE_TAP_TRUST` policy env var.
- User chose: **flag on the existing `shell-install` command** (not a new top-level command,
  per Constitution VII) + **genuine trust** (the security-forward path, not the lazy
  `HOMEBREW_NO_ENV_HINTS` silence).
- Mid-intake, user asked whether an already-installed/already-shell-set-up user could just type
  `shll shell-install --trust-tap` — surfacing the requirement that `--trust-tap` be an
  **additive, standalone action** that does not disturb an existing shell-init block.

Interaction mode: conversational (discussion → new), high signal strength.

## Why

**The problem.** Homebrew nags on every operation against a non-official tap (`sahil87/tap`)
because the user is in the default "limbo" trust state — they've neither explicitly trusted the
tap nor explicitly opted to keep allowing untrusted taps. shll makes this worse: because
`shll update` shells out to `brew` several times (`brew update`, the shll self-upgrade
`brew upgrade`, per-tool fallbacks), the same advisory prints 2–3× per command. The warning is
brew's, not shll's — but shll is the surface the user experiences it through, and shll currently
offers **no path to resolve it**.

**The consequence of not fixing it.** The toolkit's flagship "one command for everything"
experience (`shll update`, `shll install`) is permanently noisy. Worse, the warning foreshadows
a future brew release that will *require* explicit trust — at which point untrusted-tap
operations may start failing outright. Giving users a first-class way to record genuine trust
now is forward-looking, not just cosmetic.

**Why this approach over alternatives.** Three exits from the limbo state were considered:

1. `HOMEBREW_NO_ENV_HINTS=1` — silence *all* brew hints. Blunt; hides future hints too.
2. `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` — keep allow-by-default, stop nagging. Lazy; punts the
   trust decision forever and doesn't align with brew's direction.
3. **Genuine trust** — `brew trust --tap sahil87/tap` + `HOMEBREW_REQUIRE_TAP_TRUST=1`. The user
   explicitly vouches for their own tap; untrusted third-party taps would then be *blocked*
   (better security posture). **Chosen.**

shll does NOT decide trust on the user's behalf (that would violate the spirit of Constitution
III — wrap, don't reinvent — and make a security call for the user). Instead it offers an
**explicit, user-invoked** flag that *persists a decision the user just made by typing it*. The
docs additionally mention escape hatches #1/#2 for users who prefer the lighter touch.

## What Changes

### `--trust-tap` flag on `shll shell-install`

A new boolean flag on the existing `shell-install` command (`src/cmd/shll/shell_install.go`).
It is **not** a mode like `--print`/`--uninstall` (those are mutually-exclusive dispatch
branches in `runShellInstall`). `--trust-tap` is an orthogonal *what-to-act-on* selector that
**composes** with the existing modes.

In its primary (non-print, non-uninstall) invocation, `shll shell-install --trust-tap` performs
**both** halves of genuine trust, atomically:

1. **The ceremony** — runs `brew trust --tap sahil87/tap` via `internal/proc` (Constitution I —
   never raw `os/exec`; route through `proc.Run`/`proc.RunForeground`). This records persistent
   trust in brew's own store.
2. **The policy** — writes `export HOMEBREW_REQUIRE_TAP_TRUST=1` into the **single shll-managed
   block** (see "Single combined block" below).

**Both halves are required together.** Trust record without the policy var = the warning
persists (the trust record is inert until `HOMEBREW_REQUIRE_TAP_TRUST` is set). Policy var
without the trust record = brew now *blocks* `sahil87/tap` (strictly worse than the warning). So
the flag must apply both, and treat partial application as a failure to repair on the next run.

### Single combined block (DECIDED)

There is exactly **one** shll-managed sentinel block in the rc file. The trust export and the
shell-init eval line live **together** inside it — not in two separate blocks:

```sh
# >>> shll >>>
export HOMEBREW_REQUIRE_TAP_TRUST=1
eval "$(shll shell-init zsh)"
# <<< shll <<<
```

Rationale (user decision during intake): one tidy shll footprint in the dotfile, and a single
unambiguous uninstall. The `export` is placed before the `eval` so the policy var is set in the
environment before any shll-driven brew invocation could read it (order is largely immaterial for
an interactive-shell env var, but this is the safe ordering).

> **Sentinel rename + migration (DECIDED — clarify 2026-06-01)**: the block adopts a new combined
> sentinel `# >>> shll >>>` / `# <<< shll <<<`, replacing the current `# >>> shll shell-init >>>` /
> `# <<< shll shell-init <<<`. Existing users have a block under the **old** sentinel, so the
> install path MUST **detect and migrate** it: recognize an old-sentinel block, and rewrite it
> under the new sentinel (carrying its eval line forward, merging in the export line when
> `--trust-tap` applies). This migration is the **most complex part of the change** — the spec
> must enumerate scenarios: old-sentinel-only present, new-sentinel present, both present
> (treat as corrupted — define resolution), and a partial/unclosed old block (current code
> short-circuits these as "already installed"; decide whether migration auto-repairs or refuses).
> The `--print` output and the idempotency scan must reference the new sentinel.

### Install is now a per-line MERGE, not a whole-block append

Because both lines share one block, install can no longer blindly append a fresh block. The
install path MUST:

1. Locate the existing shll block (if any) via `findBlock`.
2. Determine which managed lines should be present — the eval line (for a normal `shell-install`)
   and/or the export line (when `--trust-tap` is passed). The desired set is the **union** of
   what's already in the block and what this invocation adds.
3. Regenerate the block body from that union and rewrite the block in place (or append a fresh
   block if none exists).

This means:

- **Already-set-up user** (block has only the eval line) → `shll shell-install --trust-tap`
  merges the export line **into the existing block**, runs the ceremony. No second block, no
  duplicate. ✅ — this is the direct answer to "would it work for an already-set-up user."
- **Trust-first user** (block has only the export line) → later `shll shell-install` merges the
  eval line in.
- Idempotency is now **per-line**: a line already present is not duplicated; the invocation is a
  no-op for lines that already exist (and the ceremony is guarded per Open Question #1).

This per-line merge is the main new complexity versus the current whole-block model — the spec
must define the merge rules precisely. `findBlock` still locates the block range; the new part is
composing the block *body* from "which lines should be present" rather than a fixed template.

### Uninstall removes the WHOLE block (DECIDED)

`shll shell-install --uninstall` removes the **entire** shll block — both the eval line and the
export line — in one operation (user decision). There is no "uninstall only the trust line" mode.
This keeps uninstall simple and unambiguous and reuses the existing whole-block slice-out logic
in `runShellInstallUninstall` (retargeted to the new `# >>> shll >>>` sentinel; it should also
recognize and remove an old-sentinel block so users who never re-installed can still uninstall).
Uninstall does **NOT** run `brew untrust` (DECIDED — Open Question #3 resolved): the trust record
is inert without `HOMEBREW_REQUIRE_TAP_TRUST` and harmless to leave; a user can `brew untrust`
manually (verified idempotent).

### Mode composition semantics

- `shll shell-install` → ensure the eval line is present in the shll block (merge if needed).
- `shll shell-install --trust-tap` → **full setup (DECIDED)**: ensure **both** the eval line
  *and* the export line are present in the shll block (merge if needed) **and** run `brew trust`.
  One command fully wires a fresh user. (User chose this over export-line-only; the tradeoff —
  a trust-only user also gets the eval line — was accepted.)
- `shll shell-install --trust-tap --print` → dry-run: print the resulting block to stdout, run
  nothing, modify nothing (mirrors existing `--print`).
- `shll shell-install --uninstall` → remove the whole shll block. Does **NOT** run `brew untrust`
  (DECIDED — the trust record is inert without the policy var and harmless to leave; reversal is
  available manually since `brew untrust` is idempotent).

### Graceful degradation (Constitution V)

Older brew versions may not ship `brew trust` (it is newer). Before invoking the ceremony,
**capability-probe** — same pattern as the existing `--skip-brew-update` substring probe in
`update.go` (run `brew trust --help` or `brew help trust` and check it's recognized). If
`brew trust` is unavailable:

- Do **not** error out the whole command.
- Do **not** write `HOMEBREW_REQUIRE_TAP_TRUST=1` (that would block the tap with no trust record
  to back it).
- Degrade clearly: warn that genuine trust requires a newer Homebrew, and point the user at the
  lighter `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` / `HOMEBREW_NO_ENV_HINTS=1` escape hatches.

Also handle `brew` being absent entirely (the `proc.ErrNotFound` path), consistent with how
`hasBrew` is used elsewhere.

### Named constants

The tap name MUST come from a named constant. Note: the existing code has `formulaPrefix`
(`sahil87/tap/`) used to build *formula* references (`sahil87/tap/shll`). `brew trust --tap`
takes the **tap** (`sahil87/tap`), not a formula — so derive/define the tap name as a named
constant (e.g., trim the trailing `/` from `formulaPrefix`, or add a `tapName` constant). Do not
open-code `sahil87/tap` (code-quality: no magic strings).

### Documentation

- **README.md — new Troubleshooting section** explaining the warning is a brew env-hint (not a
  shll error), that shll surfaces it because it wraps `brew`, and that `shll update` may show it
  2–3×. Document `shll shell-install --trust-tap` as the recommended resolution, and mention the
  lighter env-var escape hatches (`HOMEBREW_NO_REQUIRE_TAP_TRUST=1`, `HOMEBREW_NO_ENV_HINTS=1`)
  for users who don't want genuine trust. State explicitly that shll won't set these for you —
  trusting a tap is the user's decision.
- **README.md — `shll shell-install` section** — document the new `--trust-tap` flag and how it
  composes with `--print`/`--uninstall`.

## Affected Memory

- `cli/shell-install`: (modify) document the new `--trust-tap` flag, the homebrew-trust managed
  block, ceremony + policy dual-action, and uninstall behavior.
- `internal/proc`: (modify, only if a new transport/usage pattern emerges — likely none; the
  ceremony uses existing `proc.Run`/`proc.RunForeground`). Mark tentative.
- A new troubleshooting/operations memory MAY be warranted, but spec-level behavior lives under
  `cli/shell-install` — no new domain unless the spec surfaces one.

## Impact

- **`src/cmd/shll/shell_install.go`** — primary change site: new flag, generalized block
  machinery, ceremony invocation, degradation handling.
- **`src/cmd/shll/shell_install_test.go`** — new test cases (test-alongside): trust-block
  install, idempotency, `--print --trust-tap`, `--uninstall` removing both blocks, degradation
  when `brew trust` absent, already-set-up additive behavior, `brew` absent.
- **`src/cmd/shll/brew.go`** — possibly a `tapName` constant and a `brewTrustAvailable(ctx)`
  capability probe (mirrors `hasBrew`).
- **`internal/proc`** — used as-is; no new API expected.
- **README.md** — Troubleshooting section + `shell-install` flag docs.
- **No state added** — shll wraps brew's stateful trust store, same shape as `shll install`
  wrapping `brew install` (Constitution II satisfied).
- **Cross-platform** — rc-file resolution already handles zsh/bash × darwin/linux; the trust
  block reuses it. `brew trust` is brew's concern, platform-agnostic from shll's side.

## Open Questions

1. ~~**`brew trust` idempotency**~~ — **RESOLVED** (clarify session 2026-06-01, empirically
   verified on brew 5.1.14): `brew trust --tap sahil87/tap` is idempotent — first run prints
   "Trusted tap" and exits 0, second run prints "Already trusted tap" and exits 0. `brew untrust
   --tap` is symmetrically idempotent ("Untrusted tap" / "Not trusted tap", both exit 0). **No
   "already trusted" guard is needed — shll can invoke unconditionally.**
2. **Minimum brew version shipping `brew trust`** — still open as a precise floor, but the
   mechanism is settled: capability-probe (`brew trust --help` recognized?) and degrade if absent.
   The exact oldest version is not blocking — the probe handles any version gracefully. Spec
   should note the probe is the contract, not a hardcoded version check.
3. ~~**Should `--uninstall` also `brew untrust`?**~~ — **RESOLVED** (clarify 2026-06-01): **No.**
   Uninstall removes the rc block only; the brew trust record is left intact (inert without the
   policy var, harmless, user-reversible via the verified-idempotent `brew untrust`).
4. ~~**Sentinel naming / migration**~~ — **RESOLVED** (clarify 2026-06-01): adopt the new combined
   `# >>> shll >>>` sentinel and **migrate** existing `# >>> shll shell-init >>>` blocks in place.
   Spec must enumerate the migration scenarios (old-only, new-only, both-present, partial old).
5. ~~**Does `--trust-tap` also ensure the shell-init eval line?**~~ — **RESOLVED** (clarify
   2026-06-01): **Yes.** `--trust-tap` does full setup — ensures both eval and export lines +
   runs `brew trust`. Trust-only-user-gets-eval-line tradeoff accepted.

### Remaining open (non-blocking)

6. **Both-sentinels-present resolution** — if a user somehow has *both* an old `# >>> shll
   shell-init >>>` block and a new `# >>> shll >>>` block (e.g., hand-edited), what does install
   do? Spec to define (likely: migrate/merge into one new block, or refuse with a clear message).
7. **Partial/unclosed old block** — current code short-circuits an open-without-close block as
   "already installed" (no auto-repair). Spec to decide whether migration changes that.

## Clarifications

### Session 2026-06-01

| # | Question | Resolution |
|---|----------|------------|
| Q#1 | `brew trust` / `brew untrust` idempotency on re-run | Agent-resolved by empirical probe (brew 5.1.14): both idempotent, exit 0 on re-run ("Trusted"/"Already trusted", "Untrusted"/"Not trusted"). No guard needed — invoke unconditionally. Assumption #14 upgraded Unresolved → Certain. |
| Q#2 | Minimum brew version shipping `brew trust` | Settled as a mechanism, not a version: capability-probe handles any version gracefully; no hardcoded floor check. |
| Q#3 | Should `--uninstall` also `brew untrust`? | **No** — uninstall removes the rc block only; trust record left intact (inert, user-reversible). Assumption #12 → Certain. |
| Q#4 | Sentinel naming / migration | **Rename** to `# >>> shll >>>` and **migrate** existing `# >>> shll shell-init >>>` blocks in place. Accepted the migration code path for a cleaner sentinel. Assumption #11 → Certain. |
| Q#5 | Does `--trust-tap` also ensure the eval line? | **Yes** — full setup (both lines + ceremony). Accepted that a trust-only user also gets the eval line. Assumption #13 → Certain. |

Two minor scenarios deferred to spec (non-blocking): both-sentinels-present resolution (Q#6), partial/unclosed old block during migration (Q#7).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Surface is a `--trust-tap` flag on existing `shell-install`, not a new top-level command | Discussed — user explicitly chose flag over `shll trust-tap` to respect Constitution VII (minimal surface area); reuses existing rc-edit machinery | S:95 R:80 A:90 D:90 |
| 2 | Certain | Approach is genuine trust (`brew trust` + `HOMEBREW_REQUIRE_TAP_TRUST=1`), not env-hint silencing | Discussed — user chose the security-forward path over `HOMEBREW_NO_ENV_HINTS`; verified `brew trust` exists on brew 5.1.14 | S:95 R:60 A:85 D:85 |
| 3 | Certain | Ceremony routes through `internal/proc`, never raw `os/exec` | Constitution I (non-negotiable) + code-quality | S:100 R:90 A:100 D:100 |
| 4 | Certain | ONE combined shll-managed block holds both the export and the eval line — not two separate blocks | Discussed — user decided a single tidy shll footprint in the dotfile | S:95 R:60 A:85 D:90 |
| 5 | Certain | `--uninstall` removes the WHOLE shll block (both lines) in one operation; no per-line uninstall mode | Discussed — user decided ("uninstall removes the whole shll block"); reuses existing slice-out logic | S:95 R:75 A:90 D:90 |
| 6 | Confident | Both halves (ceremony + export line) applied atomically; partial state self-repairs on re-run | Derived from verified brew semantics — either half alone is broken or worse than the warning | S:80 R:55 A:80 D:75 |
| 7 | Confident | Install becomes a per-line MERGE into the single block (union of desired lines), not a whole-block append | Forced by the single-block decision — already-set-up users must get the export merged into their existing block without duplicating it | S:80 R:50 A:80 D:75 |
| 8 | Confident | Capability-probe `brew trust` before invoking; degrade without erroring when absent or brew missing | Constitution V + existing `--skip-brew-update` probe precedent in update.go | S:75 R:65 A:80 D:70 |
| 9 | Confident | Tap name (`sahil87/tap`) comes from a named constant; note it differs from `formulaPrefix` (tap vs formula) | code-quality (no magic strings); spotted the formula-vs-tap distinction reading brew.go | S:80 R:85 A:90 D:80 |
| 10 | Confident | README gains a Troubleshooting section + documents the new flag; mentions lighter escape hatches | Discussed — explicit docs requirement; standard for a user-facing flag | S:85 R:90 A:90 D:85 |
| 11 | Certain | Adopt new combined `# >>> shll >>>` sentinel and MIGRATE existing `# >>> shll shell-init >>>` blocks in place | Clarified — user chose the cleaner sentinel and accepted the migration code path (Open Q#4); spec enumerates migration scenarios | S:95 R:45 A:70 D:75 |
| 12 | Certain | `--uninstall` removes the rc block only; does NOT run `brew untrust` (record inert + user-reversible) | Clarified — user chose minimal-blast-radius uninstall (Open Q#3); brew untrust verified idempotent so manual reversal stays open | S:95 R:60 A:75 D:80 |
| 13 | Certain | `--trust-tap` does FULL setup — ensures both eval + export lines and runs `brew trust` | Clarified — user chose one-command full setup (Open Q#5), accepting that a trust-only user also gets the eval line | S:95 R:55 A:70 D:75 |
| 14 | Certain | `brew trust`/`brew untrust` are idempotent (exit 0 on re-run) — shll invokes unconditionally, no guard | Clarified — empirically verified on brew 5.1.14 (Open Q#1 resolved); brew floor handled by capability-probe, not a version check | S:95 R:60 A:90 D:80 |

14 assumptions (9 certain, 5 confident, 0 tentative, 0 unresolved).

# Intake: Uniform shll-self representation across inspect/manage commands

**Change**: 260609-bb7r-shll-self-display-uniform
**Created**: 2026-06-09
**Status**: Draft

## Origin

This change came out of a substantive `/fab-discuss` design session — all key
decisions were resolved there, so this intake records them rather than
re-deriving them. The interaction mode was conversational design, distilled into
a single synthesized description handed to `/fab-new`.

> Represent `shll` itself uniformly across inspect/manage commands (shll-first
> ordering). Today `shll` appears in some commands' output (`version` lists
> `shll` first; `update` shows `shll (self)` as `[1/M]` first) but is absent from
> others (`doctor`, `list`, `install`). A new user inspecting the toolkit doesn't
> consistently see that `shll` is part of the same family. The goal is
> **discoverability**: surface `shll` itself, consistently, across the commands
> that inspect or manage the toolkit, so the toolkit reads as a unified family
> with `shll` as its manager-member.

Key decisions reached in the session (encoded as assumptions below): unified
shll-first ordering, a single shared `shllSelf` descriptor (NOT a `Roster`
entry), per-command rows for `doctor`/`list`/`install`, `shell-init` as the
documented exception, and the explicit reversal of change lst7's "no self-row"
decision for discoverability.

## Why

1. **Problem.** `shll` is the manager-member of the sahil87 toolkit, but its
   presence is inconsistent across the toolkit-inspection surface. `version` and
   `update` already lead with `shll`; `doctor`, `list`, and `install` omit it
   entirely. A new user running `shll list` or `shll doctor` to understand "what
   is this toolkit?" sees the six managed sub-tools but not `shll` itself — so
   the toolkit does not read as one family with a manager-member.

2. **Consequence of inaction.** The discoverability gap persists: the very
   commands a newcomer reaches for to map the toolkit under-represent its entry
   point. The family framing (manager + six leaves) stays implicit, learnable
   only by noticing that `version`/`update` differ from `list`/`doctor`/`install`.

3. **Why this approach.** A single shared `shllSelf` descriptor, *prepended* by
   each command that meaningfully shows the toolkit, gives one source of truth
   reused across `version`, `update`, `list`, `doctor`, and `install`. It makes
   the existing shll-first pattern (already in `version`/`update`) universal
   without touching `Roster`. The rejected alternative — adding `shll` to the
   `Roster` slice — would violate Constitution III (Roster is the *managed
   sub-tool* list), break the leaves-first invariant guarded by
   `TestRosterLeavesBeforeDependents`, and make `install`/`update`/`shell-init`
   try to operate on `shll` itself (e.g. `brew install` the running binary).

## What Changes

### Unified ordering — shll-first, then leaves-first roster

All 7 entries (`shll` + the 6 managed roster tools) appear in the SAME order
across every command that meaningfully shows them:

```
shll, wt, idea, tu, rk, hop, fab-kit
```

This is ALREADY what `version` and `update` do (shll first, then the leaves-first
`Roster` order). This change makes it the universal pattern across the
inspect/manage surface.

### Mechanism — a shared `shllSelf` descriptor; DO NOT add shll to `Roster`

Introduce ONE shared descriptor representing "shll as a displayable entry":

- **Name**: `shll`
- **Description**: "the manager for the shll toolkit" (or similar manager-framing line)

- **Repo**: `shll` (resolves to `https://github.com/sahil87/shll` via the existing `repoURL`/`githubOrgBase` composition)
- **Version**: sourced from the package-level `version` var in `main.go` — NOT via a self-subprocess (`shll --version`)

Each command that shows `shll` PREPENDS this shared descriptor to its
roster-derived rows. One source of truth, reused by `version`, `update`, `list`,
`doctor`, `install`.

`Roster` MUST stay exactly the 6 managed sub-tools (Constitution III).
`TestRosterLeavesBeforeDependents` stays untouched. Adding `shll` to the `Roster`
slice is explicitly REJECTED.

### Per-command changes

- **`version`**: NO change — already shll-first.
- **`update`**: NO change — already `shll (self)` first as `[1/M]`.
- **`doctor`**: ADD a shll-first row. Checks 1+2 ONLY (binary always present —
  it's the running process; version read from the package `version` var, NOT a
  self-subprocess). NO wiring check for `shll` (it ships no shell-init — same
  treatment as `idea`/`rk`/`fab-kit`, `shell_init:false`). The `shll` row is
  effectively ALWAYS OK, so it MUST NOT perturb the scriptable any-FAIL→exit-1
  contract. In `--json`, `shll` gets an object too.
- **`list` (table)**: ADD a shll-first row with a PLAIN `ok` / `✓` status marker
  (same rendering as installed tools — maximum visual uniformity was explicitly
  chosen over a distinct "self" marker), with the manager description.
- **`list` (`--json`)**: ADD a shll-first object. It MUST carry `"self": true`
  (this field is absent or false on the 6 managed tools) so scripting consumers
  driving `brew install` can filter shll out via `select(.self != true)`.
  `installed` stays `true` for `shll`.
- **`install`**: ADD a shll-first INFORMATIONAL line (e.g. "shll — already
  present / self-managed"). You cannot `brew install` the running binary, so this
  is informational only, NOT an install action.

- **`shell-init`**: EXCLUDED — deliberate, documented exception. `shll` has no
  shell-init output of its own to compose, and `shell-init`'s stdout is `eval`'d
  (Constitution V eval-safety); a stray line risks breaking the `eval`. Leave
  `shell-init` unchanged and document WHY it is the one exception.

### Reversal of change lst7's "no self-row" decision

Change lst7 documented "No `shll` self-row" in `list` ("shll is the manager, not
a managed tool"). This is NOT a constitutional rule — it lives only in three
locations:

- `docs/memory/cli/list.md:28`
- `src/cmd/shll/list.go` (the `runList` doc comment, ~lines 73-74)
- `README.md:175`

This change REVERSES that decision for discoverability. Justification line for
the reversal: **"discoverability — new users should see shll itself as part of
the toolkit family."** These three live locations get reconciled during the
change (the reversal recorded, not silently overwritten).

### Constitution VII note

This adds BEHAVIOR to existing commands (`doctor`/`list`/`install`), NOT new
top-level subcommands, so VII's new-subcommand bar is not triggered. The only
thing requiring explicit recording is the reversal of lst7's design decision
(above) — recorded here in the intake and reconciled in memory/README/code
comment, NOT promoted to a constitutional rule.

## Affected Memory

- `cli/list`: (modify) Reverse the "No `shll` self-row" decision; document the
  prepended shll-first row (plain `ok`/`✓` marker, manager description) and the
  `--json` `"self": true` field; record the lst7 reversal with its
  discoverability justification.
- `cli/doctor`: (modify) Document the prepended shll-first row — checks 1+2 only,
  version from the package `version` var, no wiring check, always-OK so it never
  perturbs the any-FAIL→exit-1 contract; `--json` gains a shll object.
- `cli/install`: (modify) Document the prepended shll-first informational line
  (self-managed, not a brew install action).
- `cli/version`: (modify) Note that version's existing shll-first row is now the
  canonical instance of the shared `shllSelf` descriptor (cross-reference the new
  single source of truth); behavior unchanged.
- `cli/update`: (modify) Note that update's existing `shll (self)` first entry is
  the canonical manage-side instance of the shared descriptor; behavior unchanged.
- `cli/commands`: (modify) Document the shared `shllSelf` descriptor in
  `tools.go` alongside the `Roster` / `Tool` struct notes — what it is, why it is
  NOT a `Roster` entry (Constitution III + leaves-first invariant), and which
  commands prepend it.
- `cli/shell-init`: (modify) Document WHY `shell-init` is the deliberate
  exception (no own shell-init output; eval-safety risk).

## Impact

- **`src/cmd/shll/tools.go`** (or a small new file): the shared `shllSelf`
  descriptor + its version-from-`main.go.version` wiring. Single source of truth.
- **`src/cmd/shll/doctor.go`**: prepend the shll-first row; ensure the always-OK
  row does not affect the exit-1-on-any-FAIL contract; add the `--json` object.
- **`src/cmd/shll/list.go`**: prepend the shll-first table row (plain marker) and
  the `--json` object with `"self": true`; update the `runList` doc comment
  (reversal reconciliation).
- **`src/cmd/shll/install.go`**: prepend the shll-first informational line.
- **`src/cmd/shll/main.go`**: package-level `version` var is the version source
  for the descriptor (read, likely no change beyond exposure).
- **Tests**: `doctor_test.go`, `list_test.go`, `install_test.go` — new
  assertions for the shll-first row in each surface; existing
  `len(Roster)`-based row-count assertions in `list_test.go`/`doctor_test.go`
  must be updated to account for the prepended row (now `len(Roster)+1` where the
  self entry is included). `tools_test.go` may guard the descriptor fields.
  `TestRosterLeavesBeforeDependents` stays UNTOUCHED.
- **Docs**: `docs/memory/cli/{list,doctor,install,version,update,commands,shell-init}.md`,
  `README.md` (line ~175 reversal), and the `list.go` code comment.
- **`shell-init`**: explicitly NOT touched (documented exception).

## Open Questions

<!-- None blocking. All core decisions were resolved in the /fab-discuss session.
     Two micro-decisions are recorded as Tentative assumptions below rather than
     asked, since both are low-blast-radius and easily reversed via /fab-clarify. -->

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Unified ordering is shll-first then leaves-first `Roster` (`shll, wt, idea, tu, rk, hop, fab-kit`) across all commands that show the toolkit | Already the established `version`/`update` pattern; this makes it universal. Determined by existing code + discussion | S:95 R:80 A:90 D:95 |
| 2 | Certain | Mechanism is ONE shared `shllSelf` descriptor that commands PREPEND; `shll` is NOT added to `Roster` | Adding to `Roster` violates Constitution III and breaks `TestRosterLeavesBeforeDependents` (leaves-first invariant) and would make install/update/shell-init operate on shll itself. Constitution-determined | S:95 R:70 A:95 D:95 |
| 3 | Certain | `shll` version for the descriptor is read from the package-level `version` var in `main.go`, NOT via a `shll --version` self-subprocess | Explicitly decided in discussion; avoids a self-spawn and matches "binary always present since it's running". Confirmed against `main.go`/`version.go` | S:95 R:80 A:90 D:90 |
| 4 | Certain | `version` and `update` get NO code change — already shll-first | Verified against current behavior (context.md + discussion); memory just cross-references the shared descriptor | S:95 R:90 A:95 D:95 |
| 5 | Certain | `doctor` shll row runs checks 1+2 only (no wiring check), is effectively always-OK, and MUST NOT perturb the any-FAIL→exit-1 scriptable contract | Discussion + Constitution V; `shll` ships no shell-init (`shell_init:false`), same as idea/rk/fab-kit | S:95 R:65 A:90 D:90 |
| 6 | Certain | `list --json` shll object carries `"self": true` (absent/false on the 6 managed tools); `installed` stays `true` | Explicit discussion decision so scripting consumers filter via `select(.self != true)` before `brew install` | S:95 R:75 A:90 D:95 |
| 7 | Certain | `list` table shll row uses the PLAIN installed marker (`ok` / `✓`), not a distinct "self" marker | Maximum visual uniformity explicitly chosen over a distinct marker in discussion | S:90 R:85 A:85 D:90 |
| 8 | Certain | `install` shll entry is an INFORMATIONAL line only (self-managed), never a `brew install` action on the running binary | Discussion + the impossibility of brew-installing the running process | S:95 R:85 A:90 D:95 |
| 9 | Certain | `shell-init` is EXCLUDED as a deliberate, documented exception (no own shell-init output; `eval`-safety per Constitution V) | Explicit discussion decision; Constitution V eval-safety makes a stray stdout line a real risk | S:95 R:75 A:95 D:95 |
| 10 | Certain | The lst7 "no self-row" reversal is recorded in the intake and reconciled in `docs/memory/cli/list.md`, `list.go` comment, and `README.md` — NOT promoted to a Constitution rule | The decision lives only in those 3 non-constitutional locations; Constitution VII bar is not triggered (behavior, not a new subcommand). Verified all 3 locations exist | S:95 R:80 A:95 D:90 |
| 11 | Confident | The shared descriptor lives in `src/cmd/shll/tools.go` (alongside `Roster`/`Tool`/`githubOrgBase`), reusing the existing `Tool` struct shape, rather than a separate new file | tools.go is where roster/repo plumbing already lives and the description names it first; a new file is offered only as a fallback. Low cost to relocate | S:75 R:80 A:75 D:70 |
| 12 | Confident | `shll`'s Repo slug is `shll` → `https://github.com/sahil87/shll` (no rk-style override needed) | Mirrors every non-rk tool whose Repo equals Name; the rk/run-kit 404 footgun does not apply to shll. HTTP-verifiable but high-confidence | S:80 R:85 A:80 D:85 |
| 13 | Confident | Descriptor Description string is "the manager for the shll toolkit" | Discussion supplied this concrete candidate ("or similar" = agent picks default); one obvious interpretation, trivially reversible — only final phrasing is open | S:80 R:90 A:80 D:70 |
| 14 | Confident | `install` informational wording is "shll — already present / self-managed" | Discussion supplied this concrete candidate ("e.g." = agent picks default); one obvious interpretation, cosmetic and reversible | S:78 R:90 A:80 D:70 |

14 assumptions (10 certain, 4 confident, 0 tentative, 0 unresolved).

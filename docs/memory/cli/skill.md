---
type: memory
description: "`shll skill [tool] [topic]` — the skill-bundle composer: bare form is an installed-only, shll-first glossary; `shll skill <tool>` streams `<tool> skill` byte-identical via proc.RunCaptured (rk→run-kit alias, child stderr suppressed); `shll skill <tool> <topic>` passes the topic through, propagating the child's stderr/exit code on failure; `shll skill shll` serves the embedded self-bundle in-process. Invocation argv resolved via the roster Skill override (skillArgv; fab-kit → `fab skill`)."
---
# cli/skill

`shll skill [tool] [topic]` — the agent-facing reader for the toolkit's per-tool skill bundles, the runtime composer for the `skill` standard shll both publishes and now adopts. It mirrors `shell-init`'s composition idea (compose by subprocess, at runtime, so bundles stay version-locked to the *installed* binary — Constitution III), but for the agent-usage briefing each tool ships instead of shell-init output.

Source: `src/cmd/shll/skill.go` (+ `skill_test.go`). (agst)

> This is the **composer** (`shll skill`). The `skill` **standard document** it adopts (the ≤150-line static-bundle contract, `run-kit context` precedent, the `skill`-not-`agent` name decision) is [cli/standards-content §the skill standard](/cli/standards-content.md#the-skill-standard); shll's audited conformance to it is [cli/standards-conformance](/cli/standards-conformance.md). The bootstrap-skill placement command that points agents *at* this two-step is [cli/agent-setup](/cli/agent-setup.md).

## The grammar (three shapes, one subcommand)

`runSkill(ctx, stdout, stderr, args)` (the test seam extracted from the cobra factory; `Args: cobra.MaximumNArgs(2)`) dispatches on arg count:

- **`shll skill`** (bare, no args) → the installed-only glossary (`writeSkillGlossary`).
- **`shll skill <tool>`** → that one tool's bundle (`writeSkillBundle`): `shll` self → embedded in-process; a Roster tool → `<tool> skill` passthrough.
- **`shll skill <tool> <topic>`** → that tool's topic page (`writeSkillTopic`): a Roster tool → `<tool> skill <topic>` passthrough; `shll` self → in-process no-topics error (shll ships zero topics).

Three or more args stays a cobra usage error (exit 2 via the shared `errExitCode`/`isUsageError` wrap) — the arg-count contract is `>2 → usage error`. A single arg is resolved as a *tool name* (a topic without a tool is indistinguishable from a tool and stays one — an unknown one-arg name is the existing usage exit 2 with the valid-tools list). No grammar ambiguity is introduced by the second arg (tp2s).

The bare form is deliberately **not** a dump of every bundle. Concatenating 7 × ~150-line bundles into agent context would violate toolkit principle №9 (the same rule that caps a single bundle at ≤150 lines); the two-step "list, then per-tool on demand" is the context-economy contract, decided in backlog `[agst]`. A large-scope tool's core bundle lists its own topic pages, and the third shape reaches them through the same front door (backlog `[tp2s]`).

## The skill-argv override (fab-kit → `fab skill`)

Both passthroughs resolve the tool's skill-bundle invocation through one helper rather than hardcoding `{tool.Name, skillSubcommand}`:

```go
// skillArgv returns the roster Skill override when set, else the default.
func skillArgv(tool Tool) []string {
	if len(tool.Skill) > 0 {
		return tool.Skill
	}
	return []string{tool.Name, skillSubcommand}
}
```

The `Tool` struct carries a `Skill []string` argv-override field (alongside `ShellInit`/`Update` — see [cli/commands §the Tool struct](/cli/commands.md#hardcoded-tool-roster)). An empty slice means the default `{Name, "skill"}`; only a tool whose skill surface diverges from its roster `Name` populates it. **Exactly one roster entry populates it — fab-kit → `{"fab", "skill"}`**, because fab-kit serves its skill bundle on the `fab` router binary, not the `fab-kit` binary (the `skill` standard's own topic-page example is `fab skill dispatch`). So `shll skill fab-kit` invokes `fab skill`, never a literal `fab-kit skill`, and `shll skill fab-kit <topic>` invokes `fab skill <topic>`.

The default is derivable from `Name` + `skillSubcommand`, so only the exception is stated (ShellInit's "empty means no divergence" semantics, not Update's "all entries populate") — see the [exception-only population Design Decision](#design-decision-exception-only-skill-population). Resolution is skill-only: `toolInstalled`'s bare-glossary PATH probe still runs on `tool.Name` (`fab-kit --version`) — installedness is a different question from skill argv, and the fab-kit formula ships both binaries — and `update`/`install`/`version`/`list`/`doctor` are untouched (they key off `Name`/`Formula`/`Update`).

## Bare `shll skill` — the installed-only glossary

`writeSkillGlossary(ctx, stdout)` prints one line per **installed** tool, column-aligned via the same `tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)` config `shll version` / `shll list` use:

- **shll first, always.** `shllSelf.Name` + `shllSelf.Description` lead the table — shll is the running binary, so it is unconditionally present (no probe). This is the [unified shll-first ordering](/cli/commands.md#unified-shll-first-ordering--the-principle) instance for `skill`.
- **Then the `Roster` in leaves-first order**, each filtered by the shared PATH-only `toolInstalled(ctx, tool)` probe (`version.go`). A tool not on PATH is skipped silently (Constitution V). No brew calls — the glossary stays cheap (`toolInstalled` is the `<tool> --version` PATH probe, not `brew list`; see [cli/version §the shared install probe](/cli/version.md#the-shared-install-probe)).
- **A trailing hint** after a blank line: `skillHintLine` = `Run 'shll skill <tool>' for that tool's full agent skill bundle ('shll skill <tool> <topic>' for a topic page).` — a single line teaching both the second step (per-tool bundle) and the third (a topic page).

It never prints a bundle H1 (`# … skill`) — the glossary and the bundles are disjoint outputs. Descriptions come from each tool's hardcoded `Description` field / `shllSelfDescription`, single-sourced on the roster (Constitution III — cannot drift from the managed set). Shape:

```
shll     the manager for the shll toolkit
wt       Git worktree management — create, list, open, delete worktrees
hop      Fast directory/project jumping across worktrees

Run 'shll skill <tool>' for that tool's full agent skill bundle ('shll skill <tool> <topic>' for a topic page).
```

(With only `wt` + `hop` on PATH besides shll — the other four roster tools are absent and omitted.)

## `shll skill <tool>` — one tool's bundle

`writeSkillBundle(ctx, stdout, stderr, name)` resolves `name` and serves the bundle:

1. **Legacy alias** — before roster resolution, `if canonical, ok := legacyAliases[name]; ok && rosterHas(canonical)` rewrites `name` (so `shll skill rk` → `run-kit skill`, never a literal `rk skill`). Same `legacyAliases` map `resolveTargets` consults — skill carries no bespoke alias logic (see [cli/commands §the legacy target alias](/cli/commands.md#the-legacy-target-alias-rk--run-kit)).
2. **`shll` self-token** (`name == shllTargetToken`) → `writeShllOwnBundle` (below), served in-process.
3. **A Roster tool** (`rosterTool(name)` hit) → the byte-identical `<tool> skill` passthrough (below).
4. **Unknown name** → `errExitCode{code: usageExitCode, msg: "shll skill: unknown tool %q (valid: %s)"}` (exit 2 — the conformance usage-error convention). The valid list is `validTargets(true)` — `allowShll=true`, so `shll` IS advertised as a valid skill target here (unlike `install`, where `shll` is rejected). No subprocess is spawned for an unknown name.

### The byte-identical passthrough (`proc.RunCaptured`)

For a Roster tool, skill resolves the invocation argv via `skillArgv(tool)` (the roster `Skill` override when set — fab-kit → `fab skill` — else the default `{tool.Name, skillSubcommand}`; see [the skill-argv override](#the-skill-argv-override-fab-kit--fab-skill)) and invokes it through the **capture-all** transport under a bounded context:

```go
argv := skillArgv(tool)                                       // fab-kit → {"fab","skill"}, else {Name,"skill"}
subCtx, cancel := context.WithTimeout(ctx, skillProbeTimeout) // 2s, mirroring version's probe bound
out, _, code, err := proc.RunCaptured(subCtx, argv[0], argv[1:]...)
```

`proc.RunCaptured` buffers **both** the child's stdout and stderr and returns its exit code, passing *neither* stream through (see [internal/proc §RunCaptured / TransportCaptureAll](/internal/proc.md#runcaptured--transportcaptureall)). That is exactly what skill needs: **stream captured stdout byte-identical on success, suppress captured stderr on failure** so only shll's own one-line notice reaches the user. The classification (invoke-and-classify — no separate `--help`-substring probe, because `skill` either prints or errors):

- **`errors.Is(err, proc.ErrNotFound)`** (not on PATH) → one stderr line `skillNotInstalledFmt` (`shll skill: %s is not installed — run 'shll install %s'`, tool name twice) + `errSilent` (exit 1). Operational, not usage.
- **`err != nil`** (pre-start I/O failure, no usable exit code) → `shll skill: %s: %v` + `errSilent` (exit 1).
- **`code != 0`** (ran to completion but failed — most likely the installed version predates `skill`, so the unknown-command exits non-zero) → `skillUnsupportedFmt` (`shll skill: %s does not support 'skill' — its installed version may predate it (try 'shll update %s')`, tool name twice) + `errSilent` (exit 1). The notice is **hedged, not imperative**: the tool may already be current (in which case updating changes nothing), so it names the likely cause without promising the remedy and scopes the update suggestion to the one tool. The child's captured stderr is suppressed in favor of this one notice.
- **`code == 0`** → `stdout.Write(out)` — byte-identical passthrough, no framing, no rendering (stdout is data per the standard's invocation contract). A write error is `errSilent`.

Notices print `tool.Name` (the user-facing roster name) regardless of which binary the resolved argv actually invokes — a fab-kit failure still reads `fab-kit`, never `fab`.

This degradation is what lets the composer ship **while zero leaf tools implement `skill`**: every `shll skill <tool>` today hits the not-installed or unsupported path with a clean notice, and starts passing through the moment a tool's `skill` subcommand lands (the per-repo standards waves).

### `shll skill shll` — the self-bundle, in-process

The composer grammar collides with the standard's bare-form contract (`shll skill` can't be both the glossary and shll's own bundle), so shll's bundle is served through the **self-target**, not the bare form (agst — see the [conformance receipt](/cli/standards-conformance.md#the-skill-standard-adopted)):

- `writeShllOwnBundle(stdout, stderr)` reads the embedded copy `skillFS.ReadFile(skillEmbedPath)` (`skillEmbedPath = "skill/skill.md"`) and writes it byte-identical to stdout. **In-process, no subprocess** — a `shll skill shll` self-invocation would recurse into the composer.
- A read failure is a build-integrity bug (the sync step / drift guard should have caught it), surfaced as a wrapped error, not treated as user error.

## `shll skill <tool> <topic>` — a topic page (verbatim passthrough)

`writeSkillTopic(ctx, stdout, stderr, name, topic)` serves one of a tool's topic pages. **Tool-arg resolution mirrors `writeSkillBundle` exactly** — legacy alias (`rk`→`run-kit`, so `shll skill rk <topic>` invokes `run-kit skill <topic>`, never a literal `rk`) → `shll` self-token → roster lookup → unknown name → `errExitCode{code: usageExitCode, msg: "shll skill: unknown tool %q (valid: %s)"}` (exit 2, `validTargets(true)`, no subprocess). For a Roster tool it resolves the same `skillArgv(tool)` (so fab-kit's topics route through `fab skill <topic>`) and appends the topic, then invokes through the same capture-all transport under the same `skillProbeTimeout` (2s):

```go
argv := skillArgv(tool)
args := append(append([]string(nil), argv[1:]...), topic) // copy-then-append — never alias into Tool.Skill
subCtx, cancel := context.WithTimeout(ctx, skillProbeTimeout)
out, childErr, code, err := proc.RunCaptured(subCtx, argv[0], args...)
```

The final args are built by **copying** the resolved tail before appending the topic (`append(append([]string(nil), argv[1:]...), topic)`), never `append(argv[1:], topic)` directly — the latter could alias into fab-kit's roster `Skill` backing array and mutate the shared roster slice. The roster `Skill` stays byte-equal `{"fab", "skill"}` after any topic invocation.

**What diverges is the failure classification.** Where the one-arg form *suppresses* the child's stderr and *rewraps* every non-zero exit into a curated `skillUnsupportedFmt` notice at exit 1, the two-arg form *propagates* — because the `skill` standard's unknown-topic contract ("non-zero exit with the valid topics on stderr") must survive the composer unmodified. The classification (order matters — first match wins):

- **`errors.Is(err, proc.ErrNotFound)`** (not on PATH) → one stderr line `skillNotInstalledFmt` + `errSilent` (exit 1). Classified **before** any exit-code question — the not-installed notice takes precedence over propagation.
- **`err != nil`** (pre-start I/O failure, no usable exit code) → `shll skill: %s: %v` + `errSilent` (exit 1).
- **`code < 0`** (the deadline/signal-kill sentinel — see below) → one curated `skillTopicTimeoutFmt` line + `errSilent` (exit 1). Operational.
- **`code > 0`** (ran to completion but failed — an unknown topic, or a version predating the topic; the two are not reliably distinguishable from exit codes and stderr-sniffing is fragile) → **write the child's captured stderr bytes verbatim** (`stderr.Write(childErr)`), then `return &errExitCode{code: code}` with an empty `msg`. `translateExit` exits with the mirrored child code and writes nothing further, so the child's own stderr is the only diagnostic. This is the deliberate divergence: **propagate, do not rewrap.**
- **`code == 0`** → `stdout.Write(out)` — byte-identical passthrough. A write error is `errSilent`.

The accepted consequence of verbatim propagation: a tool whose installed version *predates* `skill`, invoked **with a topic**, surfaces its own raw unknown-command stderr instead of the curated `skillUnsupportedFmt` notice the one-arg form would give. That divergence is intentional — the two failure shapes (predates-skill vs. unknown-topic) are indistinguishable from exit codes, so honest propagation is the design (backlog `[tp2s]`). The one-arg form's suppress-and-rewrap classification is untouched.

### The `code < 0` deadline-kill guard

`code > 0` propagation is gated so a child that **never ran to completion** is *not* propagated. `proc.RunCaptured` reports a `skillProbeTimeout` deadline (or any other signal kill) as **code `-1` with `err == nil`** — Go's `*exec.ExitError.ExitCode()` sentinel for a signal-killed process, *not* a real child exit status (see [internal/proc §RunCaptured / TransportCaptureAll](/internal/proc.md#runcaptured--transportcaptureall)). Mirroring `-1` into `errExitCode{code: -1}` would wrap to `os.Exit(-1)` → process exit **255** with **zero** stderr — a silent failure violating the toolkit's exit-code convention. So a negative code routes to the curated operational notice `skillTopicTimeoutFmt` at exit 1 instead. The `code > 0` guard (rather than `code != 0`) is what makes verbatim propagation apply only to a child that ran to completion.

### `shll skill shll <topic>` — the no-topics contract, in-process

shll ships **zero** topic pages of its own, so `shll skill shll <topic>` (any topic) is served **in-process, no subprocess** (a self-invocation would recurse into the composer): one stderr line `skillNoTopicsFmt` (`shll skill: shll ships no topic pages (unknown topic %q)`) + `errExitCode{code: usageExitCode}` (exit 2). This matches the sibling unknown-tool usage convention — the `skill` standard requires only "non-zero exit, valid topics on stderr", and shll's valid topic set is empty, so naming the unknown topic on one line is the honest diagnostic.

## The bundle: authored, embedded, drift-guarded, budget-bounded

shll's own bundle is authored canonically at **`docs/site/skill.md`** (the [standards-directory restructure](/cli/standards-content.md#the-docssitestandards-restructure) reserved this path — a tool's own bundle lives at `docs/site/skill.md`, which is why the standards *documents* moved into `docs/site/standards/` to avoid the collision). It is a ≤150-line static usage briefing per the `skill` standard, and it renders at `shll.ai/shll/skill` for free.

**The embed mechanism is the `shll standards` precedent, reused verbatim** (the Go module root is `src/`, and `docs/site/` sits above it, so `//go:embed` cannot reach the canonical file directly):

- **Committed copy** `src/cmd/shll/skill/skill.md`, embedded via `//go:embed skill/skill.md` → `skillFS embed.FS` in `skill.go`.
- **Sync step** — `scripts/sync-standards.sh` gained a second section (`docs/site/skill.md` → `src/cmd/shll/skill/skill.md`) alongside the standards copy loop; the `//go:generate ../../../scripts/sync-standards.sh` directive is also in `skill.go`. See [cli/standards §the build-time embed mechanism](/cli/standards.md#the-build-time-embed-mechanism).
- **Drift guard** — `TestSkillEmbedMatchesCanonical` (`skill_test.go`) keeps the embedded copy byte-honest against `docs/site/skill.md` on every `go test`, mirroring `TestStandardsEmbedMatchesCanonical`. A budget test asserts ≤150 lines (the bundle is 53 lines today).

The bundle's `shll agent-setup` capability line describes **skills placement** (place the `shll-toolkit` skill at the two global skill paths, then delegate run-kit hooks), NEVER stanza injection — a wording correctness requirement of the current design (the rejected stanza mechanism, see [cli/agent-setup](/cli/agent-setup.md)).

## Named constants (code-quality.md — no magic strings)

All in `skill.go`: `skillEmbedPath` (`"skill/skill.md"`), `skillSubcommand` (`"skill"` — the subcommand shll invokes on each tool, and the default `skillArgv` tail), `skillProbeTimeout` (2s, mirroring `versionTimeout`), `skillHintLine`, `skillUnsupportedFmt` (`"shll skill: %s does not support 'skill' — its installed version may predate it (try 'shll update %s')"` — hedged, takes the tool name **twice**; the `writeSkillBundle` call site passes `tool.Name, tool.Name`), `skillNotInstalledFmt`, `skillNoTopicsFmt` (the `shll skill shll <topic>` no-topics stderr line), and `skillTopicTimeoutFmt` (the two-arg deadline/kill exit-1 notice).

## Constitution VII justification

`skill` = composition of per-tool commands — the `shell-init` precedent, verbatim the pattern Constitution III/IV exist to bless (compose each installed tool's own `<tool> skill`, never embed other tools' docs). Could not be a flag on an existing subcommand: no existing subcommand reads/composes per-tool agent bundles, and the self-target embed needs its own special-casing. Recorded in [cli/commands §Constitution VII per subcommand](/cli/commands.md#constitution-vii-justification-per-subcommand).

## Test seam

`skill_test.go` drives `runSkill` with `bytes.Buffer` writers and a fake `proc.Runner` (the shared install-fake pattern — see [internal/proc §test seam](/internal/proc.md#test-seam-runner)). One-arg + bare coverage: bare glossary (installed-only, shll-first, no brew, hint, no bundle concat), byte-identical passthrough asserting the capture-all transport is used, `rk`→`run-kit` alias resolution, `shll skill shll` in-process embed (no subprocess), not-installed/unsupported → one-line stderr + exit 1 with the child's raw stderr suppressed, unknown name → usage exit 2 with no subprocess. Argv-override coverage: `TestSkillArgv_DefaultAndOverride` pins both resolver branches directly (a non-override tool like `wt` → `{"wt", "skill"}`, fab-kit → `{"fab", "skill"}`); `TestSkill_Passthrough_FabKitOverrideInvokesFabSkill` asserts `shll skill fab-kit` records exactly `fab skill` (never a literal `fab-kit skill`) with stdout byte-identical; `TestSkillTopic_FabKitOverrideInvokesFabSkillTopic` asserts `shll skill fab-kit <topic>` records exactly `fab skill <topic>` and that fab-kit's roster `Skill` slice is unmutated afterward (the copy-then-append aliasing guard). Two-arg (topic) coverage: the passthrough happy path (fake asserts the exact `<tool> skill <topic>` argv via `TransportCaptureAll`, stdout byte-identical, stderr empty, exit 0); unknown-topic propagation (child exits 2 with valid-topics stderr → shll's stderr carries the child bytes verbatim, shll's exit mirrors the child); the timed-out/killed child (fake returns `code -1`, nil err → one curated `skillTopicTimeoutFmt` line, exit 1, no negative code leaked); the 3-args-→-usage-exit-2 arg-count contract (drives real cobra); topic-form + not-on-PATH → `skillNotInstalledFmt` exit 1; `shll skill shll <topic>` → in-process error, exit 2, no subprocess; `shll skill rk <topic>` → alias resolves to `run-kit skill <topic>`. The softened `skillUnsupportedFmt` wording (and its doubled tool-name arg) is asserted in the unsupported-path test. Plus `TestSkillEmbedMatchesCanonical` (drift) and the ≤150-line budget.

## Design Decisions

### Design Decision: exception-only Skill population

**Decision**: Only fab-kit populates the roster `Skill` argv-override field; every other tool derives the default `{Name, skillSubcommand}` from `skillArgv`.
**Why**: The default is derivable from `Name` + `skillSubcommand`, so stating only the exception (ShellInit's "empty means no divergence" semantics) avoids six redundant slices carrying no information.
**Rejected**: Populating all entries (Update-style "all entries populate") — adds noise with no signal, and a wrong copy-paste in a redundant slice would be a new bug surface.
*Introduced by*: 260722-ktk5-skill-argv-override

### Design Decision: a roster argv override, not a `skill.go` branch

**Decision**: fab-kit's divergent skill binary is expressed as roster data (`Skill: {"fab", "skill"}`) resolved by `skillArgv`, not a `name == "fab-kit"` branch inside the dispatch logic.
**Why**: The `Tool` struct already solves "this tool's invocation differs from `{Name, subcommand}`" twice — `ShellInit` and `Update` are per-tool argv slices — so a third argv-override field keeps the roster the single source of truth (Constitution III) and adds no per-tool special-casing to `skill.go`.
**Rejected**: (a) hardcoding a `name == "fab-kit"` branch in `skill.go` — bespoke per-tool logic outside the roster that would drift; (b) renaming the roster entry to `fab` — `fab-kit` is the formula leaf, the binary the install probe checks, and the user-facing name across `list`/`version`/`update`, so a skill-only concern would ripple through every command.
*Introduced by*: 260722-ktk5-skill-argv-override

### Design Decision: verbatim topic propagation is unchanged by the override

**Decision**: The topic form still propagates a completed child's stderr and exit code verbatim; the argv override only swaps *which binary* is invoked, not the failure classification.
**Why**: The `skill` standard's unknown-topic contract ("non-zero exit with the valid topics on stderr") must survive the composer unmodified; fab-kit routing through `fab` does not change that requirement.
*Introduced by*: 260722-ktk5-skill-argv-override

## Cross-references

- The subprocess transport it relies on: [internal/proc §RunCaptured / TransportCaptureAll](/internal/proc.md#runcaptured--transportcaptureall).
- The bootstrap-skill placement command that teaches agents this two-step: [cli/agent-setup](/cli/agent-setup.md).
- The `skill` standard document it adopts: [cli/standards-content §the skill standard](/cli/standards-content.md#the-skill-standard), [cli/standards-conformance §the skill standard](/cli/standards-conformance.md#the-skill-standard-adopted).
- The embed/sync/drift-guard precedent: [cli/standards](/cli/standards.md).
- Root wiring (`newSkillCmd`), the shll-first ordering principle, the `legacyAliases`/`rosterTool`/`validTargets` resolver, `shllSelf`: [cli/commands](/cli/commands.md).
- The shared `toolInstalled` PATH probe: [cli/version](/cli/version.md#the-shared-install-probe).
- Constitution III (Wrap, Don't Reinvent — compose `<tool> skill`), IV (Composition, Not Replacement), V (Graceful Degradation — installed-only glossary + degrade-with-notice), VII (Minimal Surface Area — justified above).

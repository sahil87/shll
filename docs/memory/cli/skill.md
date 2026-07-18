---
type: memory
description: "`shll skill [tool]` — the runtime skill-bundle composer: bare form is an installed-only, shll-first glossary (one line per tool, no bundle concat, no brew); `shll skill <tool>` streams `<tool> skill` byte-identical via proc.RunCaptured (rk→run-kit alias); `shll skill shll` serves shll's own embedded bundle in-process; not-installed/unsupported → one-line stderr + exit 1, unknown name → usage exit 2. Bundle authored at docs/site/skill.md, sync/embed/drift-guarded."
---
# cli/skill

`shll skill [tool]` — the agent-facing reader for the toolkit's per-tool skill bundles, the runtime composer for the `skill` standard shll both publishes and now adopts. It mirrors `shell-init`'s composition idea (compose by subprocess, at runtime, so bundles stay version-locked to the *installed* binary — Constitution III), but for the agent-usage briefing each tool ships instead of shell-init output.

Source: `src/cmd/shll/skill.go` (+ `skill_test.go`). Added by change `agst` (`260718-agst-agent-setup-skill-commands`).

> This is the **composer** (`shll skill`). The `skill` **standard document** it adopts (the ≤150-line static-bundle contract, `run-kit context` precedent, the `skill`-not-`agent` name decision) is [cli/standards-content §the skill standard](/cli/standards-content.md#the-skill-standard); shll's audited conformance to it is [cli/standards-conformance](/cli/standards-conformance.md). The bootstrap-skill placement command that points agents *at* this two-step is [cli/agent-setup](/cli/agent-setup.md).

## The grammar (three shapes, one subcommand)

`runSkill(ctx, stdout, stderr, args)` (the test seam extracted from the cobra factory; `Args: cobra.MaximumNArgs(1)`) dispatches on arg count:

- **`shll skill`** (bare, no args) → the installed-only glossary (`writeSkillGlossary`).
- **`shll skill <tool>`** → that one tool's bundle (`writeSkillBundle`): `shll` self → embedded in-process; a Roster tool → `<tool> skill` passthrough.

The bare form is deliberately **not** a dump of every bundle. Concatenating 7 × ~150-line bundles into agent context would violate toolkit principle №9 (the same rule that caps a single bundle at ≤150 lines); the two-step "list, then per-tool on demand" is the context-economy contract, decided in backlog `[agst]`.

## Bare `shll skill` — the installed-only glossary

`writeSkillGlossary(ctx, stdout)` prints one line per **installed** tool, column-aligned via the same `tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)` config `shll version` / `shll list` use:

- **shll first, always.** `shllSelf.Name` + `shllSelf.Description` lead the table — shll is the running binary, so it is unconditionally present (no probe). This is the [unified shll-first ordering](/cli/commands.md#unified-shll-first-ordering--the-principle) instance for `skill`.
- **Then the `Roster` in leaves-first order**, each filtered by the shared PATH-only `toolInstalled(ctx, tool)` probe (`version.go`). A tool not on PATH is skipped silently (Constitution V). No brew calls — the glossary stays cheap (`toolInstalled` is the `<tool> --version` PATH probe, not `brew list`; see [cli/version §the shared install probe](/cli/version.md#the-shared-install-probe-change-lst7)).
- **A trailing hint** after a blank line: `skillHintLine` = `Run 'shll skill <tool>' for that tool's full agent skill bundle.` — teaching the second step.

It never prints a bundle H1 (`# … skill`) — the glossary and the bundles are disjoint outputs. Descriptions come from each tool's hardcoded `Description` field / `shllSelfDescription`, single-sourced on the roster (Constitution III — cannot drift from the managed set). Shape:

```
shll     the manager for the shll toolkit
wt       Git worktree management — create, list, open, delete worktrees
hop      Fast directory/project jumping across worktrees

Run 'shll skill <tool>' for that tool's full agent skill bundle.
```

(With only `wt` + `hop` on PATH besides shll — the other four roster tools are absent and omitted.)

## `shll skill <tool>` — one tool's bundle

`writeSkillBundle(ctx, stdout, stderr, name)` resolves `name` and serves the bundle:

1. **Legacy alias** — before roster resolution, `if canonical, ok := legacyAliases[name]; ok && rosterHas(canonical)` rewrites `name` (so `shll skill rk` → `run-kit skill`, never a literal `rk skill`). Same `legacyAliases` map `resolveTargets` consults — skill carries no bespoke alias logic (see [cli/commands §the legacy target alias](/cli/commands.md#the-legacy-target-alias-rk--run-kit)).
2. **`shll` self-token** (`name == shllTargetToken`) → `writeShllOwnBundle` (below), served in-process.
3. **A Roster tool** (`rosterTool(name)` hit) → the byte-identical `<tool> skill` passthrough (below).
4. **Unknown name** → `errExitCode{code: usageExitCode, msg: "shll skill: unknown tool %q (valid: %s)"}` (exit 2 — the conformance usage-error convention). The valid list is `validTargets(true)` — `allowShll=true`, so `shll` IS advertised as a valid skill target here (unlike `install`, where `shll` is rejected). No subprocess is spawned for an unknown name.

### The byte-identical passthrough (`proc.RunCaptured`)

For a Roster tool, skill invokes `<tool> skill` through the **capture-all** transport under a bounded context:

```go
subCtx, cancel := context.WithTimeout(ctx, skillProbeTimeout) // 2s, mirroring version's probe bound
out, _, code, err := proc.RunCaptured(subCtx, tool.Name, skillSubcommand) // skillSubcommand = "skill"
```

`proc.RunCaptured` buffers **both** the child's stdout and stderr and returns its exit code, passing *neither* stream through (see [internal/proc §RunCaptured / TransportCaptureAll](/internal/proc.md#runcaptured--transportcaptureall-change-agst)). That is exactly what skill needs: **stream captured stdout byte-identical on success, suppress captured stderr on failure** so only shll's own one-line notice reaches the user. The classification (invoke-and-classify — no separate `--help`-substring probe, because `skill` either prints or errors):

- **`errors.Is(err, proc.ErrNotFound)`** (not on PATH) → one stderr line `skillNotInstalledFmt` (`shll skill: %s is not installed — run 'shll install %s'`) + `errSilent` (exit 1). Operational, not usage.
- **`err != nil`** (pre-start I/O failure, no usable exit code) → `shll skill: %s: %v` + `errSilent` (exit 1).
- **`code != 0`** (ran to completion but failed — the installed version predates `skill`, so the unknown-command exits non-zero) → `skillUnsupportedFmt` (`shll skill: %s does not support 'skill' yet — run 'shll update'`) + `errSilent` (exit 1). The child's captured stderr is suppressed in favor of this one actionable notice.
- **`code == 0`** → `stdout.Write(out)` — byte-identical passthrough, no framing, no rendering (stdout is data per the standard's invocation contract). A write error is `errSilent`.

This degradation is what lets the composer ship **while zero leaf tools implement `skill`**: every `shll skill <tool>` today hits the not-installed or unsupported path with a clean notice, and starts passing through the moment a tool's `skill` subcommand lands (the per-repo standards waves).

### `shll skill shll` — the self-bundle, in-process

shll's own `skill`-standard adoption was deferred to change `agst` by the [conformance audit](/cli/standards-conformance.md#the-one-deferred-standard-skill), and the composer grammar collides with the standard's bare-form contract (`shll skill` can't be both the glossary and shll's own bundle), so shll's bundle is served through the **self-target**, not the bare form:

- `writeShllOwnBundle(stdout, stderr)` reads the embedded copy `skillFS.ReadFile(skillEmbedPath)` (`skillEmbedPath = "skill/skill.md"`) and writes it byte-identical to stdout. **In-process, no subprocess** — a `shll skill shll` self-invocation would recurse into the composer.
- A read failure is a build-integrity bug (the sync step / drift guard should have caught it), surfaced as a wrapped error, not treated as user error.

## The bundle: authored, embedded, drift-guarded, budget-bounded

shll's own bundle is authored canonically at **`docs/site/skill.md`** (the [standards-directory restructure](/cli/standards-content.md#the-docssitestandards-restructure-change-i70w) reserved this path — a tool's own bundle lives at `docs/site/skill.md`, which is why the standards *documents* moved into `docs/site/standards/` to avoid the collision). It is a ≤150-line static usage briefing per the `skill` standard, and it renders at `shll.ai/shll/skill` for free.

**The embed mechanism is the `shll standards` precedent, reused verbatim** (the Go module root is `src/`, and `docs/site/` sits above it, so `//go:embed` cannot reach the canonical file directly):

- **Committed copy** `src/cmd/shll/skill/skill.md`, embedded via `//go:embed skill/skill.md` → `skillFS embed.FS` in `skill.go`.
- **Sync step** — `scripts/sync-standards.sh` gained a second section (`docs/site/skill.md` → `src/cmd/shll/skill/skill.md`) alongside the standards copy loop; the `//go:generate ../../../scripts/sync-standards.sh` directive is also in `skill.go`. See [cli/standards §the build-time embed mechanism](/cli/standards.md#the-build-time-embed-mechanism).
- **Drift guard** — `TestSkillEmbedMatchesCanonical` (`skill_test.go`) keeps the embedded copy byte-honest against `docs/site/skill.md` on every `go test`, mirroring `TestStandardsEmbedMatchesCanonical`. A budget test asserts ≤150 lines (the bundle is 53 lines today).

The bundle's `shll agent-setup` capability line describes **skills placement** (place the `sahil87-toolkit` skill at the two global skill paths, then delegate run-kit hooks), NEVER stanza injection — a wording correctness requirement of the current design (the rejected stanza mechanism, see [cli/agent-setup](/cli/agent-setup.md)).

## Named constants (code-quality.md — no magic strings)

All in `skill.go`: `skillEmbedPath` (`"skill/skill.md"`), `skillSubcommand` (`"skill"` — the subcommand shll invokes on each tool), `skillProbeTimeout` (2s, mirroring `versionTimeout`), `skillHintLine`, `skillUnsupportedFmt`, `skillNotInstalledFmt`.

## Constitution VII justification

`skill` = composition of per-tool commands — the `shell-init` precedent, verbatim the pattern Constitution III/IV exist to bless (compose each installed tool's own `<tool> skill`, never embed other tools' docs). Could not be a flag on an existing subcommand: no existing subcommand reads/composes per-tool agent bundles, and the self-target embed needs its own special-casing. Recorded in [cli/commands §Constitution VII per subcommand](/cli/commands.md#constitution-vii-justification-per-subcommand).

## Test seam

`skill_test.go` drives `runSkill` with `bytes.Buffer` writers and a fake `proc.Runner` (the shared install-fake pattern — see [internal/proc §test seam](/internal/proc.md#test-seam-runner)). Coverage grounds R1–R5: bare glossary (installed-only, shll-first, no brew, hint, no bundle concat), byte-identical passthrough asserting the capture-all transport is used, `rk`→`run-kit` alias resolution, `shll skill shll` in-process embed (no subprocess), not-installed/unsupported → one-line stderr + exit 1 with the child's raw stderr suppressed, unknown name → usage exit 2 with no subprocess, plus `TestSkillEmbedMatchesCanonical` (drift) and the ≤150-line budget.

## Cross-references

- The subprocess transport it relies on: [internal/proc §RunCaptured / TransportCaptureAll](/internal/proc.md#runcaptured--transportcaptureall-change-agst).
- The bootstrap-skill placement command that teaches agents this two-step: [cli/agent-setup](/cli/agent-setup.md).
- The `skill` standard document it adopts + the deferral it resolves: [cli/standards-content §the skill standard](/cli/standards-content.md#the-skill-standard), [cli/standards-conformance §the one deferred standard](/cli/standards-conformance.md#the-one-deferred-standard-skill).
- The embed/sync/drift-guard precedent: [cli/standards](/cli/standards.md).
- Root wiring (`newSkillCmd`), the shll-first ordering principle, the `legacyAliases`/`rosterTool`/`validTargets` resolver, `shllSelf`: [cli/commands](/cli/commands.md).
- The shared `toolInstalled` PATH probe: [cli/version](/cli/version.md#the-shared-install-probe-change-lst7).
- Constitution III (Wrap, Don't Reinvent — compose `<tool> skill`), IV (Composition, Not Replacement), V (Graceful Degradation — installed-only glossary + degrade-with-notice), VII (Minimal Surface Area — justified above).

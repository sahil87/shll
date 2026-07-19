# Intake: Update, Version, and Shell-Init Toolkit Standards

**Change**: 260719-y367-update-version-shell-init-standards
**Created**: 2026-07-19

## Origin

> standards for update, version, and shell-init tool surfaces

Conversational — created via `/fab-draft` at the end of a `/fab-discuss` session (2026-07-19). The session began by diagnosing a real incident: `fab update` timed out ("brew upgrade failed: timed out after 2m0s") and SIGKILLed brew mid-keg-swap, leaving a broken `fab` binary (`zsh: permission denied: fab`). Root cause chain: Homebrew 6 makes an un-timed GitHub API call during every tap-formula upgrade; the box's GitHub connectivity stalls intermittently; fab-kit's `runWithTimeout` (fab-kit `internal/update.go`) hard-kills `brew upgrade` at 120s. The discussion then asked: should "how shll tools update" be a toolkit standard? The user agreed, asked for an inventory of all implicit contracts shll depends on, and then requested: **"Create an intake using /fab-draft for all three in a single fab pipeline"** — the three being `update`, `version`, and `shell-init`.

Key decisions from the discussion (see Assumptions):

- One change, three standards — not three changes.
- Each standard is a narrow, producer-facing page (what a tool author must uphold), mirroring how `help-dump` implements principle №3. shll-side consumer machinery (probe-first ordering, roster order, digest rendering) stays in `docs/memory/`, NOT in the standards.
- Plain filenames per the existing convention: `update.md`, `version.md`, `shell-init.md`.
- The release/naming-alignment conventions (repo == roster name == formula leaf == binary name; `v*` semver tags) are folded into the `update` standard rather than minted as a fourth standard.
- The `update` standard gains a brew-handling safety clause motivated by the incident above.

## Why

`shll` composes the toolkit by shelling out to each tool's own CLI (Constitution III & IV). Three of the per-tool surfaces it consumes are load-bearing **frozen contracts that exist nowhere as written obligations** — they live only as shll-side implementation details plus graceful degradation:

1. **`<tool> update`** — `shll update` delegates to it and probes `<tool> update --help` for the literal substring `--skip-brew-update` (`strings.Contains`, `skipBrewUpdateFlag` constant at `src/cmd/shll/update.go:31`). A tool author who rewords a help line silently degrades every `shll update` run to N redundant `brew update`s. Nothing tells them the substring is a contract. And nothing constrains *how* a tool may wrap brew: fab-kit's 120s SIGKILL was perfectly "conformant" while corrupting installs (the 2026-07-19 incident).
2. **`<tool> --version`** — `shll version`, `shll doctor`, and the shared install probe (`probeToolVersion`/`toolInstalled`, `src/cmd/shll/version.go`) parse it under a **2-second timeout** (`versionTimeout`, `version.go:21`) with a **first-non-empty-line-only** token parse (`versionTokenRE`, `version.go:31`). A banner line above the version, or a `--version` that phones home, breaks `shll version` output and makes doctor/probes misclassify the tool as not installed. fab-kit's own updater additionally parses `fab-kit --version` as last-whitespace-field-strip-`v`.
3. **`<tool> shell-init <shell>`** — `shll shell-init` concatenates the output into a blob users `eval` in every new shell. Eval-safety is enforced only by exit codes: shll drops a tool's stdout **only when the tool exits non-zero**. A tool that prints a warning to stdout and exits 0 poisons every shell startup on every machine.

If we don't standardize: drift stays invisible until an incident (we just had one); tool authors — and the AI agents doing most authoring in these repos, per the principles doc's own premise — have no page to conform to; and every new tool re-derives the rules from shll's source. The standards tree exists precisely for this ("CLI design principles plus mechanical contracts … and others over time" — Constitution, Toolkit Standards). The constitution wave (backlog std1/std2/std3 directives) already binds all six tool repos to whatever `shll standards` enumerates, so new standards bind the toolkit with **zero per-repo constitution edits** — publishing here is sufficient.

Why one change and not three: the three pages share the registration mechanics (sync script, embed, registry, cross-refs), the same authoring pattern, and one review pass keeps their voice and scope-boundary (producer-only) consistent.

## What Changes

### 1. New standard: `docs/site/standards/update.md`

Producer-facing obligations for the `update` surface, binding the six roster tools (`wt`, `idea`, `tu`, `run-kit`, `hop`, `fab-kit`). shll itself is out of producer scope (it has no `update` subcommand; `shll update` self-upgrades via `brew upgrade sahil87/tap/shll` — shll is the *consumer*). Content contract (author into RFC-2119 form matching the existing standards' voice):

- **MUST expose an `update` subcommand** that upgrades the tool in place and owns the tool's own post-upgrade side effects (e.g., run-kit's daemon restart). This is why `shll update` delegates instead of running `brew upgrade` directly — the delegation rationale from `docs/memory/cli/update.md` (change cczs), restated producer-side.
- **MUST advertise `--skip-brew-update` as a literal substring in `<tool> update --help` output, and honor it** (skip the tool's internal `brew update` metadata refresh). Document that discovery is a substring probe — the flag string is a frozen textual contract exactly like help-dump's JSON shape.
- **Exit codes**: 0 on success **including already-up-to-date**; non-zero only on genuine failure. shll's summary tail and digest treat exit codes as the only truth.
- **Brew-handling safety** (the incident clause): MUST NOT send SIGKILL to a package-manager subprocess mid-transaction; MUST NOT impose short hard timeouts on `brew upgrade` (brew can legitimately block for minutes on network — observed 2026-07-19: a stalled GitHub API call inside `brew upgrade` exceeded fab-kit's 120s kill, corrupting the keg mid-swap between unlink and link). If any bound exists it SHOULD be generous and terminate gracefully (SIGTERM + grace), never SIGKILL.
- **SHOULD self-update via brew only when brew-installed** — detect via `os.Executable()` symlink resolution containing `/Cellar/` (the hop/fab-kit convention); a non-brew install degrades with a clear message instead of erroring.
- **Naming/release alignment** (folded here — the update path is where brew/formula interaction lives): GitHub repo name == roster/tool name == tap formula leaf == binary name; releases tagged `v{semver}` matching the brew formula version (consumed by `shll changelog` and the update digest); renames MUST ship a tap `formula_renames.json` entry (the rk→run-kit precedent and its migration-guard cost).

### 2. New standard: `docs/site/standards/version.md`

Binding all seven binaries (six roster tools **plus shll itself**):

- **MUST support `--version`**, exit 0.
- **MUST respond within 2 seconds** — which implies no network I/O on the version path. (Consumers: `shll version`/`shll doctor`/install probes run it under `versionTimeout = 2s`, `src/cmd/shll/version.go:21`.)
- **The version MUST appear on the first non-empty line** of output, containing a token matching `v?\d+(\.\d+)*([.-][\w.+-]+)?` (shll's `versionTokenRE`, `version.go:31`) or the `<word> version <rest>` prefix shape (`versionPrefixRE`). The parser never scans past line 1 — a banner-first layout is non-conformant. RECOMMENDED canonical shape: `<tool> version vX.Y.Z` (cobra's stable form; also what fab-kit's self-update post-check parses as last-field-strip-`v`).
- **The binary name on PATH MUST equal the tool name** — the version probe doubles as shll's install-mechanism-agnostic install probe (`proc.ErrNotFound` == not installed), so a differently-named binary reads as "not installed" everywhere.

### 3. New standard: `docs/site/standards/shell-init.md`

Binding tools that expose shell integration (today: `tu`, `hop`, `wt`; `shll shell-init` is the consumer/composer and conforms by construction):

- **`<tool> shell-init <shell>` for `zsh` and `bash` MUST emit eval-safe shell code on stdout and exit 0.** stdout contains ONLY shell source for the named shell — no prompts, colors, banners, or warnings.
- **Diagnostics go to stderr only.**
- **On any failure the tool MUST exit non-zero** — the composer drops a tool's stdout from the blob only when the exit code signals failure. Printing junk to stdout while exiting 0 poisons every `eval "$(shll shell-init …)"` on every machine. This is the eval-safety invariant from `docs/memory/cli/shell-init.md`, restated as a producer obligation.
- **Unsupported/missing shell argument → non-zero exit with a usage message on stderr** (shll's own convention is exit 2 for usage errors, per the principles standard).

### 4. Registration + embed mechanics (this repo)

Per the established pattern (`docs/memory/cli/standards.md`):

- Add `update version shell-init` to the `STANDARDS=(…)` array in `scripts/sync-standards.sh`; run it; commit the embed copies `src/cmd/shll/standards/{update,version,shell-init}.md`.
- Register three entries in `standardsRoster` (`src/cmd/shll/standards.go`) with Name, one-line Description, `Scope: "binary"` (obligations live in each tool's CLI binary — same scope as help-dump), and repo-relative SourcePath.
- The drift guard `TestStandardsEmbedMatchesCanonical` (`standards_test.go:192`) iterates `standardsRoster`, so it extends automatically; verify no other test pins a 4-standard count (`shll standards` list/`--json` goldens).
- Update `principles.md`'s companion-standards sentence ("Three companion standards make principles №3 and №10 concrete…") to name the new pages, and add each new page's "implements principle №N" cross-reference line (exact principle numbers chosen while authoring, from the ten in `principles.md`). Note `principles.md` is itself an embedded standard — its edit re-runs the sync too.

### 5. What does NOT change

- **No shll behavior change.** The probes, timeout, and regexes stay byte-identical — the standards codify shll's existing implemented behavior as the contract; they do not redesign it.
- **No new shll subcommand** (Constitution VII untouched); `shll standards` simply enumerates more entries.
- **No conformance audits of the six tool repos** in this change — rollout/enforcement follows separately (the std1/std2 directive pattern; fab-kit's SIGKILL fix becomes that repo's conformance work, not this change's).
- shll-side consumer machinery documentation stays in `docs/memory/cli/{update,version,shell-init}.md` — not moved into the standards.

## Affected Memory

- `cli/standards`: (modify) registry/embed/sync grow from 4 to 7 standards; drift-guard coverage note
- `cli/standards-content`: (modify) document the three new standards' contracts, the naming-alignment fold into `update`, and the binary scope choice
- `cli/standards-conformance`: (modify) shll's own posture against the new standards (version: conformant; update: N/A as producer; shell-init: composer conforms by construction)

## Impact

- `docs/site/standards/update.md`, `version.md`, `shell-init.md` — new (canonical)
- `docs/site/standards/principles.md` — companion-standards cross-reference sentence
- `scripts/sync-standards.sh` — STANDARDS array
- `src/cmd/shll/standards/` — three committed embed copies (+ re-synced principles.md)
- `src/cmd/shll/standards.go` — three `standardsRoster` entries
- `src/cmd/shll/standards_test.go` — auto-extending drift guard; check for count-pinned goldens
- Downstream (out of scope here): six tool repos' conformance work; shll.ai renders the new pages from `docs/site/` automatically

## Open Questions

- None — the discussion resolved scope, packaging, and naming; remaining choices are graded below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | One change delivering all three standards | User explicitly requested "all three in a single fab pipeline" | S:95 R:70 A:95 D:95 |
| 2 | Certain | Plain filenames `update.md` / `version.md` / `shell-init.md` | Existing convention (standards-content memory: plain names, URL mirrors command) | S:85 R:90 A:95 D:90 |
| 3 | Certain | Producer-facing obligations only; shll-side machinery stays in docs/memory | Discussed — user endorsed the narrow help-dump-shaped scope to bound conformance cost | S:80 R:75 A:85 D:80 |
| 4 | Confident | Fold release/naming-alignment rules into `update.md`; no fourth standard | Recommended in discussion ("fold into whichever comes first"); update owns the brew/formula surface; easy to relocate later | S:65 R:85 A:80 D:60 |
| 5 | Certain | All three registry entries get `Scope: "binary"` | standards.go scope taxonomy: obligations live in the tool binary, same as help-dump | S:70 R:90 A:85 D:75 |
| 6 | Confident | Standards codify shll's existing probe behavior verbatim (2s, first-line regex, literal substring) rather than redesigning it | Standard-from-implementation avoids coupling a contract change with its own rollout; shll code untouched | S:75 R:70 A:90 D:75 |
| 7 | Certain | No tool-repo conformance audits in this change | Mirrors the existing standards' separate rollout (std1/std2 directives); keeps the change reviewable | S:70 R:85 A:85 D:80 |
| 8 | Confident | `update.md` includes the brew-safety MUST-NOT (no SIGKILL / no short hard timeout on brew) | Motivated by the 2026-07-19 incident diagnosed this session; user endorsed standardizing it; exact RFC-2119 phrasing left to authoring | S:70 R:80 A:80 D:70 |
| 9 | Confident | Per-page "implements principle №N" mappings chosen at authoring time | Requires a full read of principles.md's ten entries; low-risk editorial choice within an established pattern | S:60 R:90 A:75 D:65 |

9 assumptions (5 certain, 4 confident, 0 tentative, 0 unresolved).

# Intake: Scaffold shll CLI

**Change**: 260508-kvan-scaffold-cli
**Created**: 2026-05-09
**Status**: Draft

## Origin

This change initiates the shll project. The user opened a new repo at `/Users/sahil/code/sahil87/shll` after a discussion about how to update all sahil87 toolkit apps in one command — the existing pattern of per-tool `update` subcommands is inconsistent (`hop`, `rk`, `fab-kit`, `tu` have one; `idea`, `wt` don't), and there's no top-level "update everything" entry point.

The conversation evolved through several alternatives:

1. **Document a brace-expansion one-liner** in the homebrew-tap README — rejected as not discoverable enough.
2. **Add `update` to the missing tools** (`idea`, `wt`) — rejected as not solving the "single entry point" need.
3. **Ship an updater inside the `all` formula** — rejected as adding a new binary name without justification beyond `update`.
4. **A new meta-CLI named `aishll` / `ai-shell`** — rejected on naming grounds (collisions, unpronounceable).
5. **A meta-CLI named `shll`** with subcommands beyond just `update` — accepted.

The accepted scope expanded from "just update" to three subcommands once it was clear shll could host other cross-toolkit concerns:

- `shll update` — `brew upgrade` every installed sahil87 tool
- `shll shell-init <shell>` — single eval line that sets up shell integration for all sahil87 tools that expose one (today: `hop`, `wt`)
- `shll version` — versions of shll itself plus all installed sahil87 tools (useful for bug reports)

User explicitly chose to put this in a new `sahil87/shll` repo (mirroring hop/wt/etc.) rather than co-locating in `sahil87/sahil87` (the GitHub profile repo). User explicitly chose to scaffold via the fab workflow rather than dumping files raw.

User's raw prompt that initiated `/fab-new`:

> scaffold shll CLI: a meta-tool for the sahil87 toolkit with three subcommands (update, shell-init, version). Lives in /Users/sahil/code/sahil87/shll. Will be a Go + cobra binary, mirroring hop's repo shape. Distributed via sahil87/homebrew-tap as formula `shll`, and depended on by the `all` formula.

## Why

**The pain point.** The sahil87 toolkit is six CLIs (`fab-kit`, `rk`, `tu`, `hop`, `wt`, `idea`) installed via a single Homebrew tap. Today users can install them all in one command (`brew install sahil87/tap/all`) but there is no equivalent command to update them all. They must either know the brace-expansion trick (`brew upgrade sahil87/tap/{fab-kit,rk,tu,hop,wt,idea}`) or run six separate updates. Worse, `brew upgrade sahil87/tap/all` does *not* upgrade the dependencies — the `all` meta-formula only declares `depends_on`, which Homebrew uses for install but not for upgrade propagation. So the most discoverable command silently does nothing useful.

A parallel pain exists for shell integration: `hop` and `wt` each emit shell-init script (`hop shell-init zsh` and `wt shell-setup`). Users who install the full toolkit need both eval lines in their rc file, with two cold binary starts on every shell launch and two places to update if either tool changes its init.

A third pain: there's no single command to print versions of every sahil87 tool installed, which makes triage on bug reports more annoying than it should be.

**What happens if we don't fix it.** Users either (a) don't update — accumulating bit-rot across the toolkit, (b) manually run six updates — friction that probably means most users do (a), or (c) maintain their own personal aliases that diverge across users.

**Why this approach over alternatives.**
- **A new meta-CLI vs. a documented one-liner**: A discoverable subcommand (`shll update`) beats a brace-expansion incantation that users have to look up. It also gives one place to evolve the tool roster without users editing their aliases.
- **A new meta-CLI vs. a shell script in the `all` formula**: A real binary supports tab completion, structured output (`shll version` as a table), and three coherent subcommands without growing into a script soup. Once there are three subcommands, a CLI earns its keep over a script.
- **A new meta-CLI vs. extending `all`**: Treating `shll` as a peer formula (and adding `depends_on "sahil87/tap/shll"` to `all`) keeps `all` a pure meta-package. Users who want only shll can install it standalone.
- **Composition vs. absorption**: shll shells out to per-tool CLIs (`hop shell-init`, `wt shell-setup`, `<tool> --version`) rather than reimplementing their logic. Per-tool commands continue to work standalone.

## What Changes

### New repository: `sahil87/shll`

Repo shape mirrors `hop` (per Constitution VI):

```
shll/
├── src/
│   ├── go.mod
│   ├── go.sum
│   ├── cmd/shll/
│   │   ├── main.go
│   │   ├── root.go
│   │   ├── update.go
│   │   ├── shell_init.go
│   │   └── version.go
│   └── internal/
│       └── proc/
│           └── proc.go            # Run, RunForeground, ErrNotFound (copied from hop)
├── scripts/
│   ├── build.sh
│   ├── install.sh
│   └── release.sh
├── justfile                       # one-line recipes delegating to scripts/
├── README.md
├── LICENSE                        # MIT
├── .gitignore
├── .github/workflows/release.yml  # cross-compile + GitHub Release + tap PR
└── (fab/ and docs/ already in place)
```

### Subcommand: `shll update`

Updates every sahil87 tool installed via Homebrew.

**Behavior** (GIVEN/WHEN/THEN-style preview):

- GIVEN brew is on PATH AND user has installed any sahil87 tool, WHEN `shll update` runs, THEN it runs `brew update --quiet`, then for each tool in the hardcoded roster: if installed, run `brew upgrade sahil87/tap/<formula>`; if not installed, skip silently.
- GIVEN brew is NOT on PATH, WHEN `shll update` runs, THEN it prints a hint "shll update requires Homebrew. Install from https://brew.sh" to stderr and exits non-zero.
- GIVEN no sahil87 tools are installed, WHEN `shll update` runs, THEN it prints "No sahil87 tools installed." to stdout and exits zero.

**Detection of "installed"**: Resolve the binary's symlink and check for `/Cellar/` in the path (mirrors hop's `isBrewInstalled`). Alternative: `brew list --formula sahil87/tap/<formula>` exit code. Both work; hop's symlink approach is faster but only works for the running tool. For shll querying *other* tools' install status, `brew list` is the right primitive.

**Output**: Wrapper messages from shll go to stdout/stderr; brew subprocess output is inherited to the user's terminal (via `proc.RunForeground`) so colored progress works.

### Subcommand: `shll shell-init <shell>`

Emits a single shell-init blob composed from each sahil87 tool that exposes shell integration.

**Behavior**:

- GIVEN `<shell>` is `zsh` or `bash`, WHEN `shll shell-init <shell>` runs, THEN it invokes each integrating tool's shell-init command (`hop shell-init <shell>`, `wt shell-setup`) and concatenates their stdout to its own stdout.
- GIVEN a sub-tool is not installed, WHEN `shll shell-init <shell>` runs, THEN it skips that tool with no output (Constitution V — the output must be eval-safe).
- GIVEN `<shell>` is not `zsh` or `bash`, WHEN `shll shell-init <shell>` runs, THEN it exits non-zero with usage text on stderr.

**Naming**: shll uses `shell-init` (matching `hop`, `starship init`, `direnv hook`). `wt`'s `shell-setup` is invoked under the hood — shll does not require it to rename.

**Documentation**: README will instruct users to replace per-tool eval lines with one `eval "$(shll shell-init zsh)"`. Per-tool `shell-init` / `shell-setup` continue to work for users who installed individually.

### Subcommand: `shll version`

Prints versions of shll and all sahil87 tools.

**Behavior**:

- WHEN `shll version` runs, THEN it prints a table:

```
shll      v0.1.0
fab-kit   v0.4.2
rk        v0.7.1
tu        v0.2.0
hop       v0.0.3
wt        v0.1.5
idea      not installed
```

- For each tool, invoke `<tool> --version` (or the tool's canonical version flag). On failure or absence, print `not installed`.
- Output format is plain, column-aligned text. No colors. Easy to paste into bug reports.

### Tool roster (hardcoded constant)

Defined as a slice of `Tool{Name, Formula, ShellInit}` structs in `cmd/shll/root.go` or a dedicated `cmd/shll/tools.go`. Per Constitution III the list is hardcoded; adding a new tool requires a shll release.

Initial roster:

| Name | Formula | Shell-init invocation |
|------|---------|------------------------|
| `fab-kit` | `sahil87/tap/fab-kit` | none |
| `rk` | `sahil87/tap/rk` | none |
| `tu` | `sahil87/tap/tu` | none |
| `hop` | `sahil87/tap/hop` | `hop shell-init <shell>` |
| `wt` | `sahil87/tap/wt` | `wt shell-setup` |
| `idea` | `sahil87/tap/idea` | none |

### Homebrew formula

`shll.rb` added to `sahil87/homebrew-tap/Formula/`. Then the `all.rb` formula gains `depends_on "sahil87/tap/shll"` so anyone who runs `brew install sahil87/tap/all` gets shll automatically.

The formula bump in `all.rb` is a separate change in the homebrew-tap repo, not in the shll repo. It's noted here for completeness but is out of scope for the shll repo's scaffold change.

### CI / release

GitHub Actions workflow `release.yml` (mirrors hop's):

- Triggers on `v*` tag push
- Cross-compiles for `darwin/arm64`, `darwin/amd64`, `linux/arm64`, `linux/amd64`
- Creates a GitHub Release with the binaries attached
- Opens a PR to `sahil87/homebrew-tap` updating `shll.rb` to the new version + sha256

## Affected Memory

This is the first change in a fresh repo. No existing memory to modify. Hydrate will create:

- `cli/commands`: (new) — overview of shll's subcommand structure and roster
- `cli/update`: (new) — how `shll update` resolves installed tools and propagates to brew
- `cli/shell-init`: (new) — composition rules for shell-init across sub-tools
- `cli/version`: (new) — version-reporting mechanics
- `internal/proc`: (new) — subprocess wrapper conventions

These are tentative and will be confirmed during hydrate.

## Impact

**New files (this repo)**:
- All of `src/`, `scripts/`, `justfile`, `README.md`, `LICENSE`, `.gitignore`, `.github/workflows/release.yml`
- `fab/changes/260508-kvan-scaffold-cli/` artifacts

**External (out of scope for this change but documented for traceability)**:
- `sahil87/homebrew-tap`: new `Formula/shll.rb`, modified `Formula/all.rb` (add `depends_on`)
- `sahil87/sahil87` profile README: optional addition of an "Update everything" section pointing to `shll update`

**Dependencies**:
- Go ≥1.22
- `github.com/spf13/cobra` (already used by hop, wt, idea — no new dep across the toolkit)

**Risk surface**:
- Subprocess execution (mitigated by Constitution I — `internal/proc` only)
- Cross-platform binary detection (mitigated by Constitution: cross-platform behavior section)
- Tool roster drift (mitigated by Constitution III — explicit, versioned list)

## Open Questions

- **Soft-deprecate or keep per-tool `update`?** Existing per-tool `update` commands in `hop`/`rk`/`fab-kit`/`tu` would still work, but having two paths is mildly confusing. Options: (a) keep all forever — Constitution IV-aligned; (b) soft-deprecate over one release with a "use shll update" note; (c) remove. The intake assumes (a) — keep both. Spec stage can revisit.
- **Does `shll version` need `--json` for scripting?** Bug-report use case suggests plain text is fine. JSON could be added later if a real script-consumer emerges.
- **Should `shll update --check` (or `--dry-run`) show what would update without doing it?** Useful for users who want to review before pulling. Defer to v0.2.0 unless trivial.
- **`idea` shell integration in the future?** If `idea` later adds a `shell-init`, shll needs a release to pick it up (Constitution III — explicit roster). Acceptable tradeoff.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Repo lives at `sahil87/shll`, mirroring hop's structure | Discussed and explicitly chosen by user over co-locating in `sahil87/sahil87` profile repo | S:95 R:80 A:95 D:90 |
| 2 | Certain | Three initial subcommands: `update`, `shell-init`, `version` | Discussed and explicitly chosen — additional commands ruled out for v0.1.0 | S:95 R:75 A:95 D:90 |
| 3 | Certain | Tool roster is hardcoded, not dynamic | Locked in Constitution III (Tool Roster Source of Truth) and III principle | S:90 R:70 A:95 D:90 |
| 4 | Certain | Composition not absorption — shll shells out to per-tool CLIs | Locked in Constitution III (Wrap, Don't Reinvent) and IV (Composition, Not Replacement) | S:95 R:75 A:95 D:95 |
| 5 | Certain | Graceful degradation when sub-tools missing | Locked in Constitution V | S:95 R:80 A:95 D:90 |
| 6 | Certain | Go + cobra, mirroring hop | User explicitly stated; Constitution VI codifies build pattern | S:95 R:65 A:95 D:90 |
| 7 | Certain | `shll update` skips uninstalled tools silently rather than warning | Locked in Constitution V (Graceful Degradation) | S:90 R:80 A:90 D:85 |
| 8 | Certain | `shll shell-init` supports zsh and bash only (matching hop) | Toolkit-wide precedent (hop); shll matches the existing pattern | S:85 R:75 A:90 D:85 |
| 9 | Confident | Version output is plain text, not JSON | Bug-report use case is the primary driver; plain text is faster to read and easier to paste. Design call I made, not user-stated | S:65 R:85 A:80 D:75 |
| 10 | Certain | Per-tool `update` commands stay (no deprecation in this change) | Locked in Constitution IV (Composition, Not Replacement) — flagged as open question to revisit at spec | S:85 R:70 A:90 D:80 |
| 11 | Certain | `shll` formula in tap is a peer of others; `all` gains `depends_on "shll"` | User explicitly stated this in the `/fab-new` prompt | S:95 R:70 A:90 D:90 |
| 12 | Confident | Detection of "installed" uses `brew list --formula sahil87/tap/<formula>` exit code | Hop's `/Cellar/` symlink trick only works for the running tool; for *other* tools, `brew list` is the right primitive. Design call I made; alternative (parsing `brew list --json`) is heavier without benefit | S:70 R:80 A:80 D:65 |
| 13 | Confident | `shll version` invokes `<tool> --version` per tool with a 2-second timeout | Timeout protects against deadlocked tools; 2s is generous for `--version` (typical < 100ms). Easily reversed if too tight in practice | S:65 R:85 A:75 D:70 |
| 14 | Confident | shll's own version is injected via `-ldflags` at build time, mirroring hop | Standard hop pattern, codified in Constitution VI; alternatives (Go embed, `runtime/debug.ReadBuildInfo`) exist but ldflags is the established approach | S:75 R:80 A:85 D:75 |
| 15 | Confident | `shll update` runs upgrades sequentially, not in parallel | Brew serializes most internal operations behind its own lock; parallelism risks confusing output and lock contention with no real speedup. Sequential is the obvious safe default | S:70 R:75 A:80 D:75 |

15 assumptions (10 certain, 5 confident, 0 tentative, 0 unresolved).

---
type: memory
description: "The toolkit-wide standards *documents* the shll repo hosts and serves: the `docs/site/standards/` directory restructure (genre separation, URL mirrors the command), the naming decisions (plain filenames, `skill` not `agent`), the `skill` standard's contract, the help-dump `aliases` field, the three producer-surface standards (`update`/`version`/`shell-init`), and `install-composition` (no inter-tool `depends_on` + runtime probing, install docs centralized on shll.ai; shll-README carve-out)."
---
# cli/standards-content

**Domain**: cli

The toolkit-wide standards *documents* — the canonical `docs/site/standards/*.md` pages the shll repo hosts, and which `shll standards` embeds and serves. This file covers the **content and structure** of those documents: the `docs/site/standards/` directory restructure (i70w), the `skill` standard's contract, the three producer-surface standards (`update`, `version`, `shell-init`), and the `install-composition` standard. The **command** that reads them — its roster, embed mechanism, drift guard, and output shapes — is [cli/standards](/cli/standards.md).

> Placement note: these are toolkit-wide documents, not shll-CLI behavior, but the shll repo is where their canonical source lives and where the reader command consumes them, so the memory lives beside [cli/standards](/cli/standards.md) rather than in a separate single-file domain.

## The `docs/site/standards/` restructure

The eight producer-facing standards pages live in the `docs/site/standards/` subdirectory (i70w established the layout):

```
docs/site/
├── install.md          # shll's OWN tool docs — stay flat
├── workflows.md        # shll's OWN tool docs — stay flat
└── standards/          # toolkit-wide standards — genre boundary made structural
    ├── principles.md
    ├── help-dump.md
    ├── readme-extraction.md
    ├── skill.md
    ├── update.md               # producer-surface standard (y367)
    ├── version.md              # producer-surface standard (y367)
    ├── shell-init.md           # producer-surface standard (y367)
    └── install-composition.md  # install-time composition standard (w6ay)
```

### Why the subdirectory

**Decision**: toolkit-wide standards live under `docs/site/standards/`; shll's own tool docs (`install.md`, `workflows.md`) stay flat.

**Why** (three reasons):

1. **Genre separation.** A flat `docs/site/` would mix shll's *own* tool docs with *toolkit-wide* standards — a browser (human or agent) could not tell that `help-dump.md` is a 7-repo standard rather than a shll feature doc. The subdirectory makes the genre boundary structural.
2. **URL mirrors the command.** shll.ai renders nested `docs/site` trees, so the pages land at `shll.ai/shll/standards/<name>` — mirroring `shll standards <name>` exactly. The web URL and the CLI invocation become the same name.
3. **Resolves a filename collision.** Each tool's own `skill` bundle lives at `docs/site/skill.md` in that tool's repo (see the [skill standard](#the-skill-standard) below) — including shll's own (served by `shll skill shll`, agst) — which would collide with the `skill` standard *document* in a flat layout. The two coexist as `docs/site/skill.md` (shll's bundle) and `docs/site/standards/skill.md` (the standard). See [cli/skill](/cli/skill.md).

**Rejected**: keeping the standards flat and disambiguating by filename prefix (`principle-*`, `subcommand-*`/`repo-*`, `contract-*`). Filenames should be the artifact name; taxonomy is expressed via **location** (the `standards/` dir) + **list metadata** (the `scope` column — see [cli/standards §the scope field](/cli/standards.md#the-scope-field)), not baked into the filename.

*Introduced by*: `260717-i70w-standards-dir-skill-contract`.

### docs/site closure

Intra-family relative links (principles ↔ help-dump ↔ readme-extraction ↔ skill ↔ update ↔ version ↔ shell-init ↔ install-composition) all resolve inside `docs/site/standards/` with no `..` escape (readme-extraction standard, closure rule 1) — the eight files share one directory. The three producer-surface pages cross-link each other (`version` ↔ `update` on the one-string binary/formula identity; `shell-init` → `principles` for the exit-2 usage convention) and back to `principles.md`, all same-directory; `install-composition` links to `principles.md` and to `update.md` (whose shll-out-of-producer-scope carve-out it mirrors), same-directory. Links leaving the published set are absolute `https://…` URLs; there are no images. `principles.md` carries a "The contracts" section and same-directory companion links to all seven companion standards; its "Consuming these standards" URLs point at `/shll/standards/…`.

## The `help-dump` standard defines the optional `aliases` field

`docs/site/standards/help-dump.md`'s **Output shape** documents an optional `aliases` node field: producers SHOULD emit it when the framework exposes alias metadata (e.g. Cobra `cmd.Aliases`) and MUST omit the key entirely — never `[]` or `null` — for a command with no aliases; consumers MUST treat an alias-form invocation (the `path` with its `name` replaced by any listed alias) as a valid command. The Node example carries an `"aliases": ["mk"]` line with an `omitted when none` comment, and a normative sentence beside it cross-links [Schema evolution](https://shll.ai/shll/standards/help-dump#schema-evolution).

`aliases` is the **first field defined under the standard's § Schema evolution clause** — its rule that new fields MUST be optional so each tool adopts on its own release cadence (no seven-repo flag-day, no `schema_version` bump, older captures keep validating). shll emits the field (its `shell-setup`/`shell-install` command); the other six tools adopt on their own cadence. The producer-side contract and shll's implementation live in [cli/help-dump-contract](/cli/help-dump-contract.md).

*Introduced by*: `260718-whd7-help-dump-emit-aliases`.

## The `skill` standard

`docs/site/standards/skill.md` (i70w) is the fourth producer-facing standard: the **agent skill-bundle contract** for every toolkit CLI. It specifies that each tool exposes a `<tool> skill` subcommand printing a stable, one-page markdown **skill bundle** for the agent *using* the tool — embedded in the binary, versioned with it, byte-identical to that tool repo's canonical `docs/site/skill.md`. It mirrors the [help-dump standard](https://shll.ai/shll/standards/help-dump)'s register/structure (single `#` H1, invocation contract, rules with teeth, verification section) and implements toolkit principles №3 + №10 at **scope `binary+repo`**.

### The gap it fills

Nothing today serves an agent *operating* an installed tool from any repo, offline:

- **`-h` / help-dump** is flag reference — command shape, not when-to-reach-for-which or how the tool composes.
- **README / `docs/site`** needs the repo checked out or a network round-trip to shll.ai.
- **`fab/project` context** is repo-*development*-scoped — it orients a contributor, not a caller.

A `<tool> skill` bundle is offline (embedded), present on every machine with the tool, and **version-locked by construction**: the prose ships inside the same binary as the flags it describes, so it can never document a capability the installed binary lacks.

### The contract (rules with teeth)

- **Command name exactly `skill`.** Prints raw markdown to stdout, byte-identical to the repo's canonical `docs/site/skill.md`; stderr empty; exit 0; no rendering, pager, or added framing.
- **Static-only bundle.** The bytes are identical on every invocation, on every machine, for a given release — no timestamps, no environment lookups, no session state. This is the load the standard draws vs. its precedent (below).
- **≤150-line hard budget** (principle №9). Agents pull a bundle into a paying context at use time via `shll skill <tool>` (see [Landed design](#landed-design-shll-agent-setup-skills-placement-not-context-aggregation)), and the bare `shll skill` glossary lists one line per installed tool — so a bloated bundle taxes every conversation that pulls it. Over budget means it is trying to be a README.
- **Genre discipline.** A usage briefing — when-to-use, capabilities map, composition patterns, output/exit-code contracts, gotchas. NOT a second README, NOT flag reference (defer to `-h` and the shll.ai commands page).
- **Sync + drift-guard embed.** Content is embedded at build via committed copies + a sync script + a drift-guard test — the exact mechanism `shll standards` established (see [cli/standards §the build-time embed mechanism](/cli/standards.md#the-build-time-embed-mechanism)); each adopting repo reuses it.
- **Renders on the site for free** at `/<tool>/skill` (part of the pulled `docs/site/**` tree).

### Precedent: `run-kit context` (and the static/dynamic line)

The toolkit's prior art is `run-kit context` (a.k.a. `rk context`) — roughly 102 lines of agent-optimized markdown a harness loads to learn what run-kit can do. It proves the shape works. The `skill` genre draws one explicit line on it: `run-kit context` mixes **static** capability prose with a small **dynamic** Environment header (current session, pane, server URL) computed at invocation, whereas a `skill` bundle is **static-only**. Dynamic, environment-derived state stays in separate commands like `run-kit context`; a `skill` bundle never varies with where or when it runs.

### The `skill`-not-`agent` name decision

**Decision**: the subcommand is `skill`, deliberately not `agent`.

**Why**: `agent` was rejected — it collides with `fab agent` (which launches an agent *session*), reads as an imperative ("run an agent") rather than "the tool's skill bundle", and run-kit's `agent-*` family already means harness wiring. `skill` is collision-free across all seven command trees and is the [anc.dev](https://anc.dev) P8 vocabulary (SKILL.md skill bundles) that agents already recognize.

*Introduced by*: `260717-i70w-standards-dir-skill-contract`.

### Adoption is phased

Rollout is per-repo, like help-dump's. A tool without a `skill` subcommand is not yet in violation — principle №10 is a SHOULD, and the bundle is its most forward-leaning obligation.

> **shll ships `skill`** — the `shll skill` composer AND shll's own `docs/site/skill.md` bundle (served by `shll skill shll`) (agst). The **six other tools' `<tool> skill` bundles remain the per-repo waves' work.** (The `shll standards` roster's `skill` entry is the *reader-side* row for the standard document; `shll skill` is the *producer-side* command.) See [cli/skill](/cli/skill.md) and [cli/standards-conformance §the skill standard](/cli/standards-conformance.md#the-skill-standard-adopted).

### Landed design: `shll agent-setup` (skills placement, not context aggregation)

*(Recorded here because it is why bundles must stay small and static; the standard document's own `` ## Landed design: `shll agent-setup` `` section describes the skills-placement design + the runtime two-step.)*

`shll agent-setup` + `shll skill` realize the design (agst). Two load-bearing clarifications:

- **The mechanism is skills PLACEMENT, not context aggregation.** `shll agent-setup` places ONE thin bootstrap Agent Skill (`shll-toolkit`) that *points at* a runtime **two-step** (`shll skill` glossary → `shll skill <tool>` bundle on demand); the aggregation/glossary role belongs to the separate `shll skill` composer. **Rejected**: aggregating every installed tool's `<tool> skill` output into the agent's context, and placing per-tool bundles as skill files (they go stale between updates and multiply listing lines) — the thin bootstrap + runtime two-step keeps bundles version-locked by construction. See [cli/agent-setup](/cli/agent-setup.md) and [cli/skill](/cli/skill.md).
- **The ≤150-line budget and static-only rule hold.** Bundles are fetched on demand by the two-step into a paying context, so a bloated bundle is paid repeatedly. The bare `shll skill` glossary is deliberately one line per tool, never a dump of all bundles, precisely so N bundles are never concatenated at once (toolkit principle №9).
- **run-kit hook delegation:** `shll agent-setup` delegates run-kit's dashboard hooks to `run-kit agent-setup` (Constitution III/IV). The coordinated run-kit slim (removing run-kit's own context-injection stanza, moving that guidance into `run-kit skill`) is external run-kit-repo work.

*Introduced by*: `260718-agst-agent-setup-skill-commands`; the standard document's § reflects it (fw9d).

## The three producer-surface standards (`update`, `version`, `shell-init`)

`update.md`, `version.md`, and `shell-init.md` are the producer-facing standards for the three per-tool surfaces `shll` composes by shelling out (Constitution III/IV). Each **codifies shll's already-implemented probe behavior as the written contract** — the standards did not redesign anything; shll's probes, timeouts, and regexes are byte-unchanged (y367). Each page follows the house register: single `#` H1 `Standard: <name>`, an implements-principle line, "rules with teeth", and a "Verifying conformance" checklist. They document *what a tool author must uphold*; the shll-side consumer machinery (probe-first ordering, roster order, digest rendering, the composer's concatenation) stays in [cli/update](/cli/update.md), [cli/version](/cli/version.md), and [cli/shell-init](/cli/shell-init.md), not in the standards.

### `update` — the in-place upgrade contract (scope `binary`)

Binds the **six roster tools** (`wt`, `idea`, `tu`, `run-kit`, `hop`, `fab-kit`); `shll` is explicitly **out of producer scope** — inside its own delegation loop shll self-upgrades via a direct `brew upgrade sahil87/tap/shll`, so it is the *consumer* here, not an `update`-subcommand producer. Implements principle №7 (compose, don't reinvent); its brew-handling clause serves principle №6 (stateless → retry-safe). The obligations:

- **MUST expose an `update` subcommand** that upgrades the tool in place and runs its own post-upgrade side effects (e.g. run-kit's daemon restart) — the reason `shll update` delegates instead of running `brew upgrade` itself.
- **MUST advertise `--skip-brew-update` as a literal substring** in `<tool> update --help` and honor it (skip the tool's internal `brew update`). Discovery is a substring presence check (`strings.Contains`), never a regex — the flag string is a frozen textual contract, exactly like the help-dump JSON shape.
- **Exit `0` on success including already-up-to-date**; non-zero only on genuine failure. shll's summary tail reads this exit code for pass/fail; the post-upgrade digest is driven separately off the brew-read version transition, not off the exit code.
- **Brew-handling safety** — MUST NOT `SIGKILL` a package-manager subprocess mid-transaction; MUST NOT impose a short hard timeout on `brew upgrade` (brew can legitimately block for minutes on the network); any bound SHOULD be generous and terminate via `SIGTERM` + grace, never `SIGKILL`. The clause cites the concrete failure it exists to prevent: a stalled `api.github.com` call inside `brew upgrade` (Homebrew 6 makes an un-timed GitHub API call per tap-formula upgrade) exceeded a wrapper's 120-second hard kill, landing `SIGKILL` mid-swap between `unlink` and `link` and corrupting the keg (observed 2026-07-19). The page also points at `HOMEBREW_NO_GITHUB_API=1` as the sidestep.
- **SHOULD self-update via brew only when brew-installed** — detected via `os.Executable()` symlink resolution containing `/Cellar/` (the hop/fab-kit convention); a non-brew install degrades with a clear message rather than erroring.

### `version` — the `--version` shape (scope `binary`)

Binds **all seven binaries** — the six roster tools **plus shll itself** (shll is a producer here too: `shll version` prints its own ldflags-injected row through the same first-line parse). Implements principle №4 (fail fast); its stdout discipline serves principle №2. The obligations: `--version` MUST exit `0` and write to stdout; MUST respond **within 2 seconds** (consumers run it under `versionTimeout`), which implies no network I/O on the version path; the version token MUST appear on the **first non-empty line**, matching `versionTokenRE` (`v?\d+(\.\d+)*([.-][\w.+-]+)?`) or the `<word> version <rest>` prefix (`versionPrefixRE`) — the parser never scans past line 1, so a banner-first layout is non-conformant (shll falls back to the trimmed first line verbatim, printing the banner where the version belongs); RECOMMENDED canonical shape `<tool> version vX.Y.Z`; and the **binary name on PATH MUST equal the tool name** (the version probe doubles as shll's install-mechanism-agnostic install probe — a differently-named binary reads as "not installed"). The page names the one sanctioned exception: a rename in flight, where shll retries under the legacy binary name (the `rk` → `run-kit` precedent).

### `shell-init` — eval-safe shell-integration output (scope `binary`)

Binds the tools that expose shell integration — today `tu`, `hop`, `wt`; `shll shell-init` is the **consumer/composer that conforms by construction** (it drops any tool that exits non-zero and re-emits the rest). Implements principle №2 (stdout is data) in its strictest form — "data" means eval-safe shell source; its usage-error handling serves principle №4. The obligations: `<tool> shell-init <shell>` for `zsh`/`bash` MUST emit eval-safe shell source on stdout (ONLY shell source — no prompts, colors, banners, warnings) and exit `0`; diagnostics go to stderr only; on any failure the tool MUST exit non-zero, because the composer drops a tool's stdout **only when the exit code signals failure** — printing junk to stdout while exiting `0` sails straight into `eval "$(shll shell-init …)"` on every machine (the single most damaging failure mode, precisely because the exit-code gate cannot catch it); and an unsupported/missing shell argument MUST exit non-zero (convention: **exit 2** for usage errors) with a usage message on stderr and an empty stdout.

### Design Decisions

#### Fold naming/release alignment into `update.md`, not a fourth standard
**Decision**: The one-name-four-places identity (GitHub repo == roster/tool name == tap formula leaf == binary on PATH), the `v{semver}` release-tag rule, and the `formula_renames.json` rename obligation live as a section of `update.md`.
**Why**: The update path is where a tool's brew/formula identity is load-bearing (`shll update` composes `brew upgrade sahil87/tap/<formula>` and the digest consumes `v{semver}` tags), so the alignment rules bite exactly there; the intake recommended folding "into whichever comes first"; cheap to relocate to a standalone standard later.
**Rejected**: Minting a fourth `naming`/`release` standard now — premature; adds a roster entry and a page for rules that only bite on the update/brew path.
*Introduced by*: `260719-y367-update-version-shell-init-standards`.

#### Scope `binary` for all three producer-surface standards
**Decision**: `update`, `version`, and `shell-init` each carry `Scope: "binary"` in the roster.
**Why**: Their obligations are satisfied by the compiled tool at runtime (the `update` / `--version` / `shell-init` subcommand behavior), exactly like help-dump's `binary` scope — not by repo file structure.
**Rejected**: `binary+repo` (nothing in these three lives canonically as a repo file the way the `skill` bundle does); a new scope value (the four-value `foundation`/`binary`/`repo`/`binary+repo` vocabulary already fits, and `TestStandardsRosterIntegrity` pins that set).
*Introduced by*: `260719-y367-update-version-shell-init-standards`.

#### Standards codify shll's existing probe behavior verbatim
**Decision**: The three pages restate shll's implemented probe/timeout/regex behavior as the producer contract; no shll behavior changed.
**Why**: These per-tool surfaces were load-bearing frozen contracts existing nowhere as written obligations (only as shll-side implementation plus graceful degradation) — a tool author who reworded a help line or added a `--version` banner silently degraded shll with nothing telling them a contract was broken. Standard-from-implementation avoids coupling a contract change with its own rollout.
**Rejected**: Redesigning the probes alongside standardizing them (would couple two risks and force a shll behavior change in a docs change); moving the shll-side consumer machinery into the standards (they are producer-facing — consumer machinery stays in `docs/memory/cli/`).
*Introduced by*: `260719-y367-update-version-shell-init-standards`.

## The `install-composition` standard

`docs/site/standards/install-composition.md` (w6ay) is the standard for **how the toolkit composes at install time**: every tool installs as an independent tap formula, and `shll install` is the single composition point (it installs the full roster and accepts a subset). It implements principles №7 (compose, don't reinvent — sibling capability is probed, never assumed via a package edge) and №8 (graceful degradation — a missing sibling is a skip with a hint, not a crash), at scope `binary+repo`. It carries two policies:

- **Policy A — no inter-tool formula dependencies (and probe at runtime).** Toolkit formulas MUST NOT declare `depends_on` on sibling toolkit formulas — a formula edge duplicates the roster knowledge `shll install` already owns and forces lockstep installs/uninstalls. Its binary half: a tool that invokes a sibling at runtime MUST probe first (`command -v <tool>` in shell/skill code, `exec.LookPath` in Go) and degrade gracefully on a missing sibling with an actionable install hint — never crash. The hint format is verbatim: `wt is not installed. Install it: brew install sahil87/tap/wt`. The page carries the precedent receipt: `fab-kit` and `hop` previously declared `depends_on` on `wt`/`idea`; those edges are removed, and the `all` meta-formula is retired in favor of `shll install`.
- **Policy B — install documentation is centralized on shll.ai.** Per-tool READMEs and the tap README MUST NOT carry per-formula `brew install` instructions; they link to https://shll.ai (the curl bootstrap / `shll install`). The supported-vs-unsupported line is explicit: individual formula installs remain **supported** (`brew install sahil87/tap/<tool>` works, `shll install` accepts a subset) — what is unsupported is **documenting** them per-repo, which drifts (seven copies of the install dance, each chased on every install-story change).

The page follows the house register (single `#` H1 `Standard: install-composition`, implements-principle line, per-policy MUST sections, closing `## Verifying conformance` checklist), staying at or under `update.md`'s length.

### Design Decisions

#### Scope asymmetry: Policy A binds all seven formulas, Policy B excludes shll's own README
**Decision**: Policy A binds all **seven tap formulas** (including `shll`'s — its formula must equally avoid sibling edges) and every binary that invokes a sibling. Policy B binds the **six roster-tool repos plus the tap README** but explicitly **not** shll's own README.
**Why**: shll's README, together with shll.ai, *is* the centralized install documentation Policy B points at — binding shll to "link to shll.ai instead" would be circular (shll is the consumer here). Policy A has no such asymmetry: a sibling formula edge is a defect regardless of which formula declares it, shll's included.
**Rejected**: Binding shll's README under Policy B (circular); exempting shll from Policy A too (unjustified — the anti-lockstep reason applies to every formula). The carve-out mirrors `update.md`'s established shll-out-of-producer-scope phrasing.
*Introduced by*: `260720-w6ay-install-composition-standard`.

#### Filename/standard name `install-composition`, scope `binary+repo`
**Decision**: The standard is named `install-composition` (plain hyphenated noun) at roster scope `binary+repo`.
**Why**: The name matches the plain-filename convention (`help-dump`, `readme-extraction`) — taxonomy lives in location + the `scope` column, not a filename prefix; `install` alone would read as a per-tool subcommand surface and collide conceptually with `docs/site/install.md`. `binary+repo` reuses the pinned scope vocabulary (`TestStandardsRosterIntegrity`) — the formula-edge/README half is a repo-file obligation, the runtime-probe half a binary one — spanning both like `skill`.
**Rejected**: A dedicated `tap` scope value (churn for one row; the tap formula is a repo-file obligation in spirit and `binary+repo` already fits); `install` as the filename (subcommand-surface / collision ambiguity).
*Introduced by*: `260720-w6ay-install-composition-standard`.

## Cross-references

- The command that ADOPTS this `skill` standard — the `shll skill` composer + shll's own `docs/site/skill.md` bundle (agst): [cli/skill](/cli/skill.md). The harness-wiring command: [cli/agent-setup](/cli/agent-setup.md).
- shll's audited conformance against these standards documents (per-standard PASS/gap disposition): [cli/standards-conformance](/cli/standards-conformance.md).
- The **command** that reads/serves these documents (roster, embed mechanism, drift guard, output shapes, the `scope` field): [cli/standards](/cli/standards.md).
- The `standards.go` file-layout row and where `standards` sits in the subcommand surface: [cli/commands](/cli/commands.md).
- The shll-side consumer machinery these three standards codify (probe-first ordering, digest, timeout, composer concatenation): [cli/update](/cli/update.md), [cli/version](/cli/version.md), [cli/shell-init](/cli/shell-init.md).
- Live canonical documents (rendered): [principles](https://shll.ai/shll/standards/principles), [help-dump](https://shll.ai/shll/standards/help-dump), [readme-extraction](https://shll.ai/shll/standards/readme-extraction), [skill](https://shll.ai/shll/standards/skill), [update](https://shll.ai/shll/standards/update), [version](https://shll.ai/shll/standards/version), [shell-init](https://shll.ai/shll/standards/shell-init), [install-composition](https://shll.ai/shll/standards/install-composition).

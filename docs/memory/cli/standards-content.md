---
type: memory
description: "The toolkit-wide standards *documents* the shll repo hosts and serves: the `docs/site/standards/` directory restructure (genre separation, URL mirrors the command), the naming decisions (plain filenames, `skill` not `agent`), the `skill` standard's contract (static-only ≤150-line agent bundle, `run-kit context` precedent, `shll agent-setup` landed as skills placement), and the help-dump standard's optional `aliases` field (first field under its § Schema evolution clause)."
---
# cli/standards-content

**Domain**: cli

The toolkit-wide standards *documents* — the canonical `docs/site/standards/*.md` pages the shll repo hosts, and which `shll standards` embeds and serves. This file covers the **content and structure** of those documents: the `docs/site/standards/` directory restructure (i70w) and the `skill` standard's contract. The **command** that reads them — its roster, embed mechanism, drift guard, and output shapes — is [cli/standards](/cli/standards.md).

> Placement note: these are toolkit-wide documents, not shll-CLI behavior, but the shll repo is where their canonical source lives and where the reader command consumes them, so the memory lives beside [cli/standards](/cli/standards.md) rather than in a separate single-file domain.

## The `docs/site/standards/` restructure

The four producer-facing standards pages live in the `docs/site/standards/` subdirectory (i70w):

```
docs/site/
├── install.md          # shll's OWN tool docs — stay flat
├── workflows.md        # shll's OWN tool docs — stay flat
└── standards/          # toolkit-wide standards — genre boundary made structural
    ├── principles.md
    ├── help-dump.md
    ├── readme-extraction.md
    └── skill.md         # authored by change i70w
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

Intra-family relative links (principles ↔ help-dump ↔ readme-extraction ↔ skill) all resolve inside `docs/site/` with no `..` escape (readme-extraction standard, closure rule 1) — the four files share one directory. Links leaving the published set are absolute `https://…` URLs; there are no images. `principles.md` carries a "The contracts" section and same-directory companion links to all three mechanical contracts; its "Consuming these standards" URLs point at `/shll/standards/…`.

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

## Cross-references

- The command that ADOPTS this `skill` standard — the `shll skill` composer + shll's own `docs/site/skill.md` bundle (agst): [cli/skill](/cli/skill.md). The harness-wiring command: [cli/agent-setup](/cli/agent-setup.md).
- shll's audited conformance against these standards documents (per-standard PASS/gap disposition): [cli/standards-conformance](/cli/standards-conformance.md).
- The **command** that reads/serves these documents (roster, embed mechanism, drift guard, output shapes, the `scope` field): [cli/standards](/cli/standards.md).
- The `standards.go` file-layout row and where `standards` sits in the subcommand surface: [cli/commands](/cli/commands.md).
- Live canonical documents (rendered): [principles](https://shll.ai/shll/standards/principles), [help-dump](https://shll.ai/shll/standards/help-dump), [readme-extraction](https://shll.ai/shll/standards/readme-extraction), [skill](https://shll.ai/shll/standards/skill).

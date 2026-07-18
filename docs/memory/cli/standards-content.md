---
type: memory
description: "The toolkit-wide standards *documents* the shll repo hosts and serves: the `docs/site/standards/` directory restructure (genre separation, URL mirrors the command, resolves the `docs/site/skill.md` collision now realized), the naming decisions (plain filenames, `skill` not `agent`), and the `skill` standard's contract (static-only ≤150-line agent bundle, `run-kit context` precedent). The forward-designed `shll agent-setup` HAS SINCE LANDED (change agst) as skills PLACEMENT, not the context aggregation the standard first sketched."
---
# cli/standards-content

**Domain**: cli

The toolkit-wide standards *documents* — the canonical `docs/site/standards/*.md` pages the shll repo hosts, and which `shll standards` embeds and serves. This file covers the **content and structure** of those documents: the `docs/site/standards/` directory restructure (change i70w) and the `skill` standard's contract. The **command** that reads them — its roster, embed mechanism, drift guard, and output shapes — is [cli/standards](/cli/standards.md).

> Placement note: these are toolkit-wide documents, not shll-CLI behavior, but the shll repo is where their canonical source lives and where the reader command consumes them, so the memory lives beside [cli/standards](/cli/standards.md) rather than in a separate single-file domain.

## The `docs/site/standards/` restructure (change i70w)

The three producer-facing standards pages moved from a flat `docs/site/` into a `docs/site/standards/` subdirectory (via `git mv`, filenames unchanged), and a fourth (`skill.md`) was authored there:

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

**Decision**: move the toolkit-wide standards under `docs/site/standards/`; leave shll's own tool docs (`install.md`, `workflows.md`) flat.

**Why** (three reasons, all decided in the originating conversation):

1. **Genre separation.** Flat `docs/site/` mixed shll's *own* tool docs with *toolkit-wide* standards — a browser (human or agent) could not tell that `help-dump.md` is a 7-repo standard rather than a shll feature doc. The subdirectory makes the genre boundary structural.
2. **URL mirrors the command.** shll.ai renders nested `docs/site` trees, so the moved pages land at `shll.ai/shll/standards/<name>` — mirroring `shll standards <name>` exactly. The web URL and the CLI invocation become the same name.
3. **Resolves a filename collision (now realized).** Each tool's own `skill` bundle lives at `docs/site/skill.md` in that tool's repo (see the [skill standard](#the-skill-standard) below). shll is itself a toolkit tool — and change `agst` **authored shll's own `docs/site/skill.md` bundle** (served by `shll skill shll`), which would have collided with the `skill` standard *document* had standards stayed flat. The subdirectory removed the collision by construction; the two now coexist as `docs/site/skill.md` (shll's bundle) and `docs/site/standards/skill.md` (the standard). See [cli/skill](/cli/skill.md).

**Rejected**: keeping the standards flat and disambiguating by filename prefix (`principle-*`, `subcommand-*`/`repo-*`, `contract-*`). Filenames should be the artifact name; taxonomy is expressed via **location** (the `standards/` dir) + **list metadata** (the `scope` column — see [cli/standards §the scope field](/cli/standards.md#the-scope-field-change-i70w)), not baked into the filename.

*Introduced by*: change i70w (`260717-i70w-standards-dir-skill-contract`).

### docs/site closure holds across the move

Intra-family relative links (principles ↔ help-dump ↔ readme-extraction ↔ skill) survive the move unchanged because all four files stayed in the same directory. Every relative link still resolves inside `docs/site/` with no `..` escape (readme-extraction standard, closure rule 1). Links leaving the published set are absolute `https://…` URLs; there are no images. `principles.md` gained a "The contracts" section and same-directory companion links to all three mechanical contracts; its "Consuming these standards" URLs point at `/shll/standards/…`.

## The `skill` standard

`docs/site/standards/skill.md` (authored by change i70w) is the fourth producer-facing standard: the **agent skill-bundle contract** for every toolkit CLI. It specifies that each tool exposes a `<tool> skill` subcommand printing a stable, one-page markdown **skill bundle** for the agent *using* the tool — embedded in the binary, versioned with it, byte-identical to that tool repo's canonical `docs/site/skill.md`. It mirrors the [help-dump standard](https://shll.ai/shll/standards/help-dump)'s register/structure (single `#` H1, invocation contract, rules with teeth, verification section) and implements toolkit principles №3 + №10 at **scope `binary+repo`**.

### The gap it fills

Nothing today serves an agent *operating* an installed tool from any repo, offline:

- **`-h` / help-dump** is flag reference — command shape, not when-to-reach-for-which or how the tool composes.
- **README / `docs/site`** needs the repo checked out or a network round-trip to shll.ai.
- **`fab/project` context** is repo-*development*-scoped — it orients a contributor, not a caller.

A `<tool> skill` bundle is offline (embedded), present on every machine with the tool, and **version-locked by construction**: the prose ships inside the same binary as the flags it describes, so it can never document a capability the installed binary lacks.

### The contract (rules with teeth)

- **Command name exactly `skill`.** Prints raw markdown to stdout, byte-identical to the repo's canonical `docs/site/skill.md`; stderr empty; exit 0; no rendering, pager, or added framing.
- **Static-only bundle.** The bytes are identical on every invocation, on every machine, for a given release — no timestamps, no environment lookups, no session state. This is the load the standard draws vs. its precedent (below).
- **≤150-line hard budget** (principle №9). Agents load the bundle into context every session, and bundles will later be aggregated (see [Forward design](#forward-design-shll-agent-setup)) — so a bloated bundle is paid for repeatedly. Over budget means it is trying to be a README.
- **Genre discipline.** A usage briefing — when-to-use, capabilities map, composition patterns, output/exit-code contracts, gotchas. NOT a second README, NOT flag reference (defer to `-h` and the shll.ai commands page).
- **Sync + drift-guard embed.** Content is embedded at build via committed copies + a sync script + a drift-guard test — the exact mechanism `shll standards` established (see [cli/standards §the build-time embed mechanism](/cli/standards.md#the-build-time-embed-mechanism)); each adopting repo reuses it.
- **Renders on the site for free** at `/<tool>/skill` (part of the pulled `docs/site/**` tree).

### Precedent: `run-kit context` (and the static/dynamic line)

The toolkit's prior art is `run-kit context` (a.k.a. `rk context`) — roughly 102 lines of agent-optimized markdown a harness loads to learn what run-kit can do. It proves the shape works. The `skill` genre draws one explicit line on it: `run-kit context` mixes **static** capability prose with a small **dynamic** Environment header (current session, pane, server URL) computed at invocation, whereas a `skill` bundle is **static-only**. Dynamic, environment-derived state stays in separate commands like `run-kit context`; a `skill` bundle never varies with where or when it runs.

### The `skill`-not-`agent` name decision

**Decision**: the subcommand is `skill`, deliberately not `agent`.

**Why**: `agent` was rejected — it collides with `fab agent` (which launches an agent *session*), reads as an imperative ("run an agent") rather than "the tool's skill bundle", and run-kit's `agent-*` family already means harness wiring. `skill` is collision-free across all seven command trees and is the [anc.dev](https://anc.dev) P8 vocabulary (SKILL.md skill bundles) that agents already recognize.

*Introduced by*: change i70w.

### Adoption is phased; this change authored the standard only

Rollout is per-repo, like help-dump's was. Change i70w authored the **standard document only**; implementing `shll skill` and the tools' bundle content was an explicit out-of-scope per-repo follow-up wave. A tool without a `skill` subcommand is not yet in violation — principle №10 is a SHOULD, and the bundle is its most forward-leaning obligation.

> **Update (change agst): shll now ships `skill`.** shll was the first adopter — change `agst` built the `shll skill` composer AND shll's own `docs/site/skill.md` bundle (served by `shll skill shll`), resolving the deferral tracked in `[agst]`. The **six other tools' `<tool> skill` bundles remain the deferred per-repo waves' work.** (Contrast the `shll standards` roster's `skill` entry, which is the *reader-side* row for the standard document, shipped since i70w; the *producer-side* `shll skill` command is what `agst` added.) See [cli/skill](/cli/skill.md) and [cli/standards-conformance §the skill standard](/cli/standards-conformance.md#the-skill-standard-deferred-at-audit-adopted-by-change-agst).

### Forward design: `shll agent-setup` — LANDED (change agst) as skills placement, not context aggregation

*(Recorded in the standard because it is why bundles must stay small and static. The standard first sketched a **context-aggregation** mechanism; the command that actually landed uses **skills placement** — the design point below still holds, but via a different route.)*

Change `agst` built `shll skill` + `shll agent-setup`, graduating the harness wiring from `run-kit agent-setup`. Two clarifications to the original forward-design sketch:

- **The mechanism landed as skills PLACEMENT, not context aggregation.** The standard originally described `agent-setup` as "aggregate every installed tool's `<tool> skill` output into the agent's context." That is **not** what shipped: `shll agent-setup` places ONE thin bootstrap Agent Skill (`shll-toolkit`) that *points at* a runtime **two-step** (`shll skill` glossary → `shll skill <tool>` bundle on demand), and the aggregation/glossary role went to the separate `shll skill` composer. Placing per-tool bundles as skill files was explicitly **rejected** (they go stale between updates and multiply listing lines); the thin bootstrap + runtime two-step keeps bundles version-locked by construction. See [cli/agent-setup](/cli/agent-setup.md) and [cli/skill](/cli/skill.md).
- **The ≤150-line budget and static-only rule still hold, for the same reason.** Bundles are loaded into agent context every session and fetched on demand by the two-step, so a bloated bundle is still paid repeatedly. The context-economy motive survives the mechanism change — the bare `shll skill` glossary is deliberately one line per tool, never a dump of all bundles, precisely so N bundles are never concatenated at once (toolkit principle №9).
- **run-kit hook delegation landed as designed:** `shll agent-setup` delegates run-kit's dashboard hooks to `run-kit agent-setup` (Constitution III/IV). The coordinated run-kit slim (removing run-kit's own context-injection stanza, moving that guidance into `run-kit skill`) is an **external run-kit-repo change**, not part of `agst`.

## Cross-references

- The command that ADOPTS this `skill` standard — the `shll skill` composer + shll's own `docs/site/skill.md` bundle (change agst): [cli/skill](/cli/skill.md). The forward-designed harness-wiring command that landed as skills placement: [cli/agent-setup](/cli/agent-setup.md).
- shll's audited conformance against these standards documents (per-standard PASS/gap disposition, incl. the `skill` deferral this file's Adoption section explains — now resolved): [cli/standards-conformance](/cli/standards-conformance.md).
- The **command** that reads/serves these documents (roster, embed mechanism, drift guard, output shapes, the `scope` field): [cli/standards](/cli/standards.md).
- The `standards.go` file-layout row and where `standards` sits in the subcommand surface: [cli/commands](/cli/commands.md).
- Live canonical documents (rendered): [principles](https://shll.ai/shll/standards/principles), [help-dump](https://shll.ai/shll/standards/help-dump), [readme-extraction](https://shll.ai/shll/standards/readme-extraction), [skill](https://shll.ai/shll/standards/skill).

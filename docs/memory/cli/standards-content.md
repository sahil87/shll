---
type: memory
description: "The toolkit-wide standards *documents* the shll repo hosts and serves: the `docs/site/standards/` directory restructure (genre separation, URL mirrors the command, resolves the coming `docs/site/skill.md` collision), the naming decisions (plain filenames, `skill` not `agent`), and the `skill` standard's contract (static-only ≤150-line agent bundle, `run-kit context` precedent, planned `shll agent-setup` aggregation)."
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
3. **Resolves a coming filename collision.** Each tool's own `skill` bundle will live at `docs/site/skill.md` in that tool's repo (see the [skill standard](#the-skill-standard) below). shll is itself a toolkit tool — its own future `docs/site/skill.md` bundle would have collided with the `skill` standard *document* if standards had stayed flat. The subdirectory removes the collision by construction.

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

Rollout is per-repo, like help-dump's was — **no tool ships `skill` today**, including shll. Change i70w authored the **standard document only**; implementing `shll skill` and the six other tools' bundle content is an explicit out-of-scope per-repo follow-up wave. A tool without a `skill` subcommand is not yet in violation — principle №10 is a SHOULD, and the bundle is its most forward-leaning obligation. (Contrast the `shll standards` roster's `skill` entry, which is the *reader-side* row shipped now; the *producer-side* `shll skill` command is the deferred work.)

### Forward design: `shll agent-setup`

*(Planned, not yet built — recorded in the standard because it is why bundles must stay small and static.)* A future `shll agent-setup` will graduate from `run-kit agent-setup`: it will **aggregate every installed tool's `<tool> skill` output** into the agent's context, and **delegate run-kit hook installation to `run-kit agent-setup`** (whose context-injection responsibility will then be removed, leaving it to do only hook wiring). When N bundles are concatenated into one context payload, every wasted line is paid N times — the whole reason for the static-only rule and the ≤150-line budget.

## Cross-references

- shll's audited conformance against these standards documents (per-standard PASS/gap disposition, incl. the `skill` deferral this file's [Adoption section](#adoption-is-phased-this-change-authored-the-standard-only) explains): [cli/standards-conformance](/cli/standards-conformance.md).
- The **command** that reads/serves these documents (roster, embed mechanism, drift guard, output shapes, the `scope` field): [cli/standards](/cli/standards.md).
- The `standards.go` file-layout row and where `standards` sits in the subcommand surface: [cli/commands](/cli/commands.md).
- Live canonical documents (rendered): [principles](https://shll.ai/shll/standards/principles), [help-dump](https://shll.ai/shll/standards/help-dump), [readme-extraction](https://shll.ai/shll/standards/readme-extraction), [skill](https://shll.ai/shll/standards/skill).

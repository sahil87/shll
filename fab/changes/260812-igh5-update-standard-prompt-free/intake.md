# Intake: Update Standard — Prompt-Free, Unconditionally

**Change**: 260812-igh5-update-standard-prompt-free
**Created**: 2026-08-12

## Origin

> Check the shll cli standards - is it not a requirement that during the "update" flow - the tool doesn't get stuck on a "y/n" question of "do you want to update?" . Else shll update won't be able to compose on top of it - thoughts?

Conversational (`/fab-discuss` session). The user asked whether the toolkit standards require a tool's `update` to never block on an interactive y/n prompt, since `shll update` composes by delegating to each tool's `update`. Investigation found the requirement exists only at the principles layer (`principles.md` №1 "Non-interactive by default", MUST) — and №1's reconciliation contains a loophole that specifically breaks the composed case. The user approved drafting a change that adds an explicit, stricter clause to the producer-facing `update` standard.

Key decisions from the conversation:

- The gap is real: `docs/site/standards/update.md` says nothing about prompts, and its "Verifying conformance" checklist doesn't test for it.
- №1 explicitly *permits* a TTY-gated prompt (`Proceed? [y/N]` on a TTY is the blessed pattern, reference implementation `shll uninstall`). `shll update` delegates with inherited stdio in a user's terminal, so stdin **is** a TTY mid-compose — a fully №1-conformant tool can still stall the delegation loop at tool 3 of 6 waiting for a keystroke. №1 protects the headless/agent case, not the composed-interactive case.
- Fix location: `update.md` gets a clause **stricter than №1 for this one subcommand** — `update` MUST be prompt-free unconditionally, even on a TTY. `principles.md` itself is NOT changed.
- Justification to write into the standard: an in-place upgrade is not a destructive write under №5 — invoking `update` *is* the consent; and `shll update` delegates with inherited stdio, so any prompt (TTY or not) breaks the compose.
- Style: mirror the page's existing timeout/SIGKILL clause — a named **Failure mode** paragraph showing how a tool can be "conformant to every other rule on this page" while breaking composition in practice.
- A matching line is added to the "Verifying conformance" checklist.

## Why

1. **The pain point**: `shll update` composes the toolkit's upgrade flow by delegating to each installed tool's own `update` subcommand with inherited stdio. If any roster tool ships an `update` that asks "do you want to update? [y/N]" — even only when a TTY is present — the whole `shll update` run stalls mid-loop on that tool, and in the non-TTY/agent case it hangs invisibly until timeout. The producer-facing standard page a tool author actually reads before shipping `update` (`docs/site/standards/update.md`) currently says nothing about this.

2. **The consequence of not fixing it**: the only protection today is `principles.md` №1, whose reconciliation *explicitly blesses* TTY-gated prompts. A tool author following the documented `shll uninstall` pattern for their `update` command would pass every written rule while breaking `shll update` for every interactive user. The standard's own conformance checklist would green-light the regression.

3. **Why this approach**: surface-specific standards pages already carry rules stricter than (or refining) the general principles where the composed surface demands it — `update.md`'s brew-handling clause exists precisely because a wrapper can be "conformant to every other rule on this page while silently corrupting installs". Adding the prompt-free clause to `update.md` (rather than tightening №1 toolkit-wide) keeps №1's TTY reconciliation valid for genuinely destructive commands (`shll uninstall`) while closing the loophole on the one subcommand where composition makes any prompt unacceptable.

## What Changes

### 1. New clause in `docs/site/standards/update.md`: "Prompt-free, unconditionally"

Add a new `##` section immediately after `## Invocation contract` (before `## Advertise and honor --skip-brew-update`), containing:

- **The obligation (MUST)**: `<tool> update` MUST run to completion without any interactive prompt, **in every environment — including when stdin is a TTY**. No confirmation question, no pager, no "press enter to continue". This applies to the tool's own code *and* to any subprocess it wraps (a wrapped `brew` call must be invoked non-interactively).
- **Why this is stricter than principle №1**: an explicit note that this clause deliberately tightens [principles.md](principles.md) №1 for this one subcommand. №1's reconciliation permits `Proceed? [y/N]` when a TTY is present; that reconciliation does not survive composition — `shll update` delegates to each tool's `update` with inherited stdio, so stdin typically *is* a TTY mid-compose, and a №1-conformant prompt stalls the delegation loop at tool *k* of 6.
- **Why no confirmation is needed at all**: an in-place upgrade is not a destructive write in the №5 sense — invoking `update` is itself the consent. There is nothing to confirm; a tool that wants a guard can offer a `--dry-run`, never a prompt.
- **Failure mode** paragraph (house style, mirroring the brew-timeout clause): a tool whose `update` prompts only on a TTY is conformant to №1 and to every other rule on this page, yet stalls every interactive `shll update` run at its position in the delegation loop — and hangs invisibly in agent/headless runs until timeout.

Wording is drafted at apply time to match the page's voice (bold-MUST bullets, em-dash asides, "rules with teeth" register).

### 2. New line in `## Verifying conformance` (same file)

Add one checklist bullet alongside the existing ones:

- `<tool> update` runs to completion with **no interactive prompt in any environment, TTY included** — no code path reads stdin for a confirmation.

### 3. Sync the embedded copy

`docs/site/standards/` is canonical, but `shll standards` serves a build-time embed from committed copies at `src/cmd/shll/standards/*.md`, kept in sync by `scripts/sync-standards.sh` and guarded by `TestStandardsEmbedMatchesCanonical`. After editing `docs/site/standards/update.md`:

- Run `scripts/sync-standards.sh` to refresh `src/cmd/shll/standards/update.md`.
- Run the drift-guard test (`go test ./cmd/shll/ -run TestStandardsEmbedMatchesCanonical` from `src/`) to confirm the embed matches canonical.

No Go source changes — only the embedded markdown asset and its canonical source.

## Affected Memory

- `cli/standards-content`: (modify) The update standard's contract summary gains the unconditional prompt-free clause (stricter-than-№1 note, №5 non-destructive rationale, conformance-checklist line).

## Impact

- `docs/site/standards/update.md` — the substantive edit (new clause + checklist line).
- `src/cmd/shll/standards/update.md` — mechanical re-sync of the embedded copy.
- No Go code paths change; no CLI surface changes; no roster-tool code is touched by this change (the six roster tools' conformance to the new clause is verified/enforced in their own repos, out of scope here).
- `shll update` / `shll uninstall` behavior unchanged — shll is the consumer of this standard; `shll uninstall`'s TTY-gated prompt remains the №1 reference implementation and is unaffected (it is not an `update` flow).

## Open Questions

*(none — all decisions resolved in the originating discussion)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Docs-only change: edit `update.md` + re-sync embed; no Go behavior change | The clause is a standards-document addition; embed sync is a committed-asset refresh guarded by an existing test | S:85 R:90 A:95 D:90 |
| 2 | Certain | `principles.md` №1 is NOT modified; the stricter rule lives on `update.md` only | Discussed — recommended and approved; mirrors how the brew-handling clause refines №6 on this same page | S:80 R:85 A:80 D:75 |
| 3 | Certain | A matching bullet is added to `## Verifying conformance` | Discussed explicitly with the user, who approved the recommendation verbatim | S:90 R:95 A:90 D:90 |
| 4 | Certain | Embed re-sync via `scripts/sync-standards.sh` + drift-guard test is required | `cli/standards` memory documents the embed pipeline and `TestStandardsEmbedMatchesCanonical` guard over all eight standards | S:80 R:90 A:95 D:95 |
| 5 | Confident | New section placed immediately after `## Invocation contract` | Prompt-freedom is invocation-level (like the contract itself), ahead of flag/exit-code mechanics; trivially movable at apply if page flow reads better | S:60 R:90 A:75 D:65 |
| 6 | Confident | Clause scope extends to wrapped subprocesses (e.g. brew invoked non-interactively) | Natural reading of "prompt-free end to end" — a prompt from a wrapped command stalls the compose identically; not explicitly discussed but low-risk wording choice | S:55 R:85 A:75 D:70 |

6 assumptions (4 certain, 2 confident, 0 tentative, 0 unresolved).

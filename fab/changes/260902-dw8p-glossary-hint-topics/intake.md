# Intake: Glossary Hint Teaches Reserved Topics

**Change**: 260902-dw8p-glossary-hint-topics
**Created**: 2026-09-02

## Origin

Direct user request, follow-up to `260902-cxhe-skill-topic-discoverability` (merged as PR #93): the discoverability evaluation found the bare-glossary hint line is the one remaining shll surface that does not teach the reserved `topics` enumeration topic. User: "the bare-glossary hint line still doesn't mention topics → can we fix this?"

## Why

The cxhe change closed the discovery chain via `shll skill --help`, the core bundles' topic indexes, and the reserved `topics` topic — but deliberately left `skillHintLine` (the trailing line of the bare `shll skill` glossary) unchanged. An agent whose walk is glossary → bundle never misses anything, but an agent that reads only the glossary sees the `<topic>` form without learning that `topics` enumerates them. One line closes the last gap; the glossary is the two-step's front door, so it is the highest-traffic teaching surface.

## What Changes

### 1. `skillHintLine` wording (src/cmd/shll/skill.go)

Current:

```
Run 'shll skill <tool>' for that tool's full agent skill bundle ('shll skill <tool> <topic>' for a topic page).
```

New (single line, still trails the tabwriter table after a blank line):

```
Run 'shll skill <tool>' for that tool's full agent skill bundle ('shll skill <tool> topics' lists its topic pages; 'shll skill <tool> <topic>' prints one).
```

No other code changes. The glossary tests assert via the `skillHintLine` constant (`strings.Contains(out, skillHintLine)`), so they track the new wording automatically; the 5-line count assertion is unaffected (still one hint line).

### 2. Memory (docs/memory/cli/skill.md)

The bare-glossary section quotes `skillHintLine` verbatim (both the prose bullet and the example block) — update both quotes to the new string.

## Affected Memory

- `cli/skill`: (modify) the two verbatim `skillHintLine` quotes in the bare-glossary section.

## Impact

- `src/cmd/shll/skill.go` — one constant string.
- `docs/memory/cli/skill.md` — two quoted occurrences.
- No standard change (the amended skill standard mandates nothing about the composer glossary), no bundle change, no test logic change, no new surface.

## Open Questions

None.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Only `skillHintLine` and its two memory quotes change; tests self-track via the constant | Verified by grep — `skill_test.go` asserts `strings.Contains(out, skillHintLine)`, no hardcoded copy | S:90 R:95 A:95 D:95 |
| 2 | Certain | New wording teaches both the `topics` enumeration and the `<topic>` page form in one line | Directly requested; mirrors the help/bundle phrasing landed in the prior change | S:90 R:90 A:90 D:90 |
| 3 | Confident | The hint stays a single line trailing the table (no second hint line, no glossary-shape change) | The constant's own doc comment fixes the one-line contract; a second line would churn the line-count test for no gain | S:75 R:85 A:90 D:85 |

3 assumptions (2 certain, 1 confident, 0 tentative).

# Plan: Glossary Hint Teaches Reserved Topics

**Change**: 260902-dw8p-glossary-hint-topics
**Intake**: `intake.md`

## Requirements

### shll: glossary hint line

#### R1: `skillHintLine` teaches the reserved `topics` topic
The bare `shll skill` glossary's trailing hint MUST teach both the reserved `topics` enumeration and the `<topic>` page form, in one line. New value of `skillHintLine`:
`Run 'shll skill <tool>' for that tool's full agent skill bundle ('shll skill <tool> topics' lists its topic pages; 'shll skill <tool> <topic>' prints one).`

- **GIVEN** the shll binary
- **WHEN** `shll skill` runs
- **THEN** the trailing hint names `shll skill <tool> topics` as the topic-list command and `shll skill <tool> <topic>` as the page command
- **AND** the hint is still exactly one line after the blank line (glossary shape unchanged)

#### R2: Memory quotes stay verbatim-accurate
`docs/memory/cli/skill.md` quotes `skillHintLine` verbatim (prose bullet + example block); both MUST carry the new string.

- **GIVEN** the updated constant
- **WHEN** the memory file is read
- **THEN** its quoted hint matches the shipped string byte-for-byte

## Tasks

### Phase 2: Core Implementation

- [x] T001 Update the `skillHintLine` constant in `src/cmd/shll/skill.go` to the new wording; run `cd src && go test ./cmd/shll/` (tests track the constant) <!-- R1 -->
- [x] T002 Update both verbatim hint quotes in `docs/memory/cli/skill.md` <!-- R2 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `shll skill` (dev build) prints the new one-line hint naming both forms; glossary tests pass unmodified
- [x] A-002 R2: both memory quotes match the shipped constant byte-for-byte

### Code Quality

- [x] A-003 Pattern consistency: the constant's doc comment still describes what the line teaches; no other code or output shape changes

## Notes

- Check items as you review: `- [x]`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|

0 assumptions.

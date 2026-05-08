# Code Review

## Severity Definitions

- **Must-fix**: Spec mismatches, failing tests, checklist violations, constitution violations (especially Principle I — security) — always addressed during rework
- **Should-fix**: Code quality issues, pattern inconsistencies with hop, missing graceful degradation — addressed when clear and low-effort
- **Nice-to-have**: Style suggestions, minor improvements — may be skipped

## Review Scope

- Changed files only (files touched during apply)
- Skip generated code and vendor directories
- Skip binary files and assets

## False Positive Policy

- Inline `<!-- review-ignore: {reason} -->` in markdown files
- Inline `// review-ignore: {reason}` in code files
- Suppressed findings are noted in the review report but not counted as failures

## Rework Budget

- Max cycles: 3
- After 2 consecutive "fix code" attempts on the same issue, escalate to "revise plan" or "revise spec"

## Project-Specific Review Rules

- Any new subprocess invocation MUST route through `internal/proc` — flag direct `os/exec` calls in command code
- Any new sub-tool integration MUST shell out to the sub-tool's CLI — flag attempts to reimplement sub-tool logic in shll
- `shll shell-init` output MUST be eval-safe even when sub-tools are missing — flag cases that emit error messages or non-shell content to stdout when a sub-tool is absent
- New top-level subcommands need a Constitution VII justification line in the spec

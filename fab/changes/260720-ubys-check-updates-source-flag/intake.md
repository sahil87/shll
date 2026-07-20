# Intake: check-updates --source enum flag

**Change**: 260720-ubys-check-updates-source-flag
**Created**: 2026-07-20

## Origin

Promptless dispatch (`/fab-proceed` create-intake, `{questioning-mode} = promptless-defer`) from a synthesized design conversation. The user and agent discussed consolidating `shll check-updates`' backend flag surface; every material decision below was made in that conversation with explicit rationale — this intake captures them, it does not invent.

> Consolidate `shll check-updates`' backend flag surface. Replace the two mutually-exclusive bool flags `--released` and `--github` with a single enum-valued string flag `--source`, accepting `released` (the default when the flag is omitted) and `github`. Clean break: the old bool flags are removed entirely (no hidden deprecated aliases).

Key decisions reached in discussion (rationale preserved in § What Changes and § Assumptions):

- Flag named `--source`, NOT `--backend` (the user initially proposed `--backend`): the `--json` envelope already carries a `source` field with values `"released"`/`"github"`, so `--source github` makes flag name, flag value, and envelope output one vocabulary. User accepted this recommendation.
- Values stay `released|github`, NOT `shll|github` (user initially proposed `shll` for the default value's name): `shll` is ambiguous as a source name (reads as the binary, means the shll.ai versions manifest) while `released` describes the semantics; and renaming would either mismatch the envelope's `"source": "released"` or force a breaking schema-2 envelope change for zero gain (envelope evolution is additive-only under schema 1). User accepted this.
- Unknown `--source` value → usage error, exit 2, via explicit validation in the run seam (`errExitCode{code: usageExitCode}`), NOT cobra machinery.
- Clean break over deprecation aliases, justified by timing: `check-updates` shipped in v0.1.10 the same day (commit f2340f4, PR #67); the only external consumer, run-kit's updatecheck daemon, is on an unmerged run-kit branch (260720-n2ai) and its agent has been separately instructed to invoke plain `shll check-updates --json` (no backend flag — valid before and after this change since `released` is the default).

## Why

1. **The pain point**: two mutually-exclusive bool flags (`--released` / `--github`) are a worse shape than one enum-valued flag for a two-way (and potentially N-way) backend choice. The mutual-exclusion error path (`bothBackendsErrMsg`, its exit-2 check, its dedicated test) exists only because the flag shape allows an invalid combination — an enum flag makes the invalid state unrepresentable at the surface level, replacing "you combined two flags wrongly" with "you named an unknown source".
2. **The consequence of not fixing it**: the surface is a day old (shipped in v0.1.10, commit f2340f4, PR #67) with effectively zero external adopters — run-kit's updatecheck daemon (the only external consumer) is on an unmerged branch and invokes plain `shll check-updates --json` with no backend flag. Every day the bool pair survives, the cost of the clean break grows; deferring turns a free rename into a deprecation-alias dance.
3. **Why this approach**: `--source released|github` unifies flag name, flag value, and the `--json` envelope's existing `source` field (`"released"`/`"github"`) into one vocabulary — an agent or human reading the JSON can write the flag back verbatim. Alternatives rejected: `--backend shll|github` naming (vocabulary mismatch with the envelope; `shll` is ambiguous as a source name); keeping the bools as hidden deprecated aliases for one release (not warranted for a day-old surface); bumping the JSON schema (nothing in the envelope changes).

## What Changes

### Flag surface: `--released`/`--github` bools → one `--source` string flag

In `src/cmd/shll/check_updates.go`:

- **Remove** the bool flag registrations `cmd.Flags().Bool(releasedFlag, ...)` and `cmd.Flags().Bool(githubFlag, ...)` and their constants `releasedFlag`, `releasedFlagUsage`, `githubFlag`, `githubFlagUsage`.
- **Add** a string flag `--source` with default `sourceReleased` ("released"):

  ```go
  const (
      sourceFlag      = "source"
      sourceFlagUsage = "update-check backend: released (shll.ai versions manifest + notify policy; the default) or github (release tags, no notify policy)"
  )
  // in newCheckUpdatesCmd:
  cmd.Flags().String(sourceFlag, sourceReleased, sourceFlagUsage)
  ```

  (Exact usage-string wording is apply's call; the constant-naming pattern follows the existing `releasedFlag`/`releasedFlagUsage` shape per code-quality.md — no magic strings.) No shorthand (`-s`) — no existing `check-updates` flag carries one.
- The existing `source` value constants `sourceReleased = "released"` / `sourceGithub = "github"` (already the `--json` envelope values) now double as the flag's valid enum values — one vocabulary, zero new value constants.
- **Clean break**: no hidden/deprecated `--released`/`--github` aliases remain. After this change, `shll check-updates --released` is an unknown-flag error (cobra's own usage error, exit 2 via the existing `translateExit` path).

### Run-seam signature and validation

`runCheckUpdates` currently takes `(ctx, stdout, stderr, released, github, jsonOut bool)` and rejects `released && github`. It becomes:

```go
func runCheckUpdates(ctx context.Context, stdout, stderr io.Writer, source string, jsonOut bool) error
```

- The cobra `RunE` reads the string flag (`cmd.Flags().GetString(sourceFlag)`) and passes it through raw.
- **Validation lives in the run seam, NOT cobra machinery**: if `source` is neither `sourceReleased` nor `sourceGithub`, return `&errExitCode{code: usageExitCode, msg: ...}` (exit 2), with a diagnostic naming the offending value and the valid set (exact wording is apply's call, e.g. `shll check-updates: invalid --source value "x" (valid: released, github)`). pflag has no native enum type, and this keeps exit-code policy out of cobra's hands — consistent with the existing recorded design decision that rejected `MarkFlagsMutuallyExclusive` because cobra's flag-group error is a plain `fmt.Errorf` that would exit 1 (see `docs/memory/cli/check-updates.md` § Design Decisions).
- The validation MUST fire before any brew or network access (preserving the invariant the old both-flags test pinned: zero recorded subprocess calls on a usage error).
- **Delete** the both-flags mutual-exclusion path entirely: `bothBackendsErrMsg`, the `if released && github` check, and the derivation `source := sourceReleased; if github { source = sourceGithub }` (the validated `source` string is now the value directly). The "both flags" usage-error case ceases to exist.

### Help text

The cobra `Long` block's "Two backends, mutually exclusive:" section is rewritten for the enum flag, e.g.:

```
One backend, selected by --source:

  --source released   latest versions + notify policy from https://shll.ai/versions.json
                      (the default when the flag is omitted)
  --source github     latest release tag per tool from the GitHub API (unauthenticated;
                      no notify policy in this backend)

  shll check-updates                          human table: installed → latest per tool
  shll check-updates --json                   machine contract (what run-kit's daemon runs)
  shll check-updates --source github          compare against GitHub release tags
```

The exit-codes paragraph stays as-is except any "both flags" implication; "2 on a usage error" already covers the unknown-`--source`-value case. (Exact prose is apply's call; the layered structure — summary, backends, examples, exit codes — is preserved.)

### Tests (`src/cmd/shll/check_updates_test.go`)

- All `runCheckUpdates(...)` call sites change mechanically from the two-bool signature to the `source string` signature (e.g. `false, true, true` → `sourceGithub, true`; `false, false, ...` → `sourceReleased, ...` or `""`→ no: the default is applied by the flag layer, so tests pass the explicit constant).
- `TestCheckUpdates_BothBackendFlagsUsageError` becomes an **unknown-`--source`-value usage-error test**: `runCheckUpdates(ctx, &stdout, &stderr, "bogus", false)` must return `errExitCode{code: usageExitCode}`, the message must name the offending value (and SHOULD name the valid set), zero recorded subprocess calls (usage error precedes all work), empty stdout.
- All other test assertions (JSON contract, degradation, table rendering, manifest guard) are unchanged in substance — only the seam call sites move.

### Documentation collateral (clean-break coherence)

The removed flags are referenced in user-facing docs that would otherwise go stale:

- **`README.md`** — lines ~105–110 (`shll check-updates --json --released`, `shll check-updates --github` examples) and line ~281 (the "fetches ... or GitHub releases with `--github`" row) are rewritten to the `--source` form. Content-only edit; the readme-extraction structure is untouched.
- **`docs/site/skill.md`** — line ~22's check-updates entry (`--released (default, ...) or --github (release tags)`) is rewritten to the `--source` form. Because the skill bundle is embedded in the binary and drift-guarded (`TestStandardsEmbedMatchesCanonical` per `docs/memory/cli/standards.md`), run `scripts/sync-standards.sh` after editing so the committed embed copy (`src/cmd/shll/standards/skill.md`) matches the canonical file.
- **`docs/site/standards/`** is NOT touched — see § Non-goals.

### Memory (hydrate-time)

`docs/memory/cli/check-updates.md`:

- Frontmatter `description:` and § Command surface: replace the "mutually-exclusive `--released`/`--github`" flag description with the `--source released|github` enum flag (default `released`).
- § Exit codes: the usage-error row's "both backend flags" example becomes "unknown `--source` value".
- § Design Decisions: the "Mutual exclusion enforced in the run seam, not cobra flag groups" entry is superseded — rewrite it as the enum-validation-in-the-run-seam decision (same principle: exit-code policy stays out of cobra's hands; cobra/pflag has no native enum and its error shapes exit 1), noting this change as *Introduced by*. Optionally add a DD entry for the `--source`-over-`--backend` naming (envelope-vocabulary alignment) with the rejected alternatives.

## What is NOT changing (constraints)

- **The `--json` machine contract is untouched**: `schema` stays `1`; the envelope `source` field and its values (`"released"`/`"github"`) are unchanged; row shape unchanged; unresolvable-row rule unchanged; `notify`/`notable` released-rows-only rule unchanged. Envelope evolution stays additive-only under schema 1.
- **Exit-code contract otherwise unchanged**: 0 check ran, 1 check failed (manifest fetch failure / brew missing), 2 usage error. `--github`-backend per-tool degradation still exits 0.
- **Default behavior unchanged**: bare `shll check-updates` still uses the released (shll.ai manifest) backend — so run-kit's instructed invocation `shll check-updates --json` is valid before and after this change.
- **Constitution § Toolkit Standards**: this is a CLI-surface change, so it was checked against `docs/site/standards/principles.md` (read in full during intake) — the enum flag conforms (№1 non-interactive; №4 fail-fast usage error, exit 2 named in help; №3's help-dump contract regenerates from cobra automatically). No standards conflict identified.

### Non-goals

- **No toolkit-wide convention line in `docs/site/standards/principles.md`** ("source/backend choices are enum-valued flags, never bool pairs") — the user was offered this as a candidate and did not opt in; leave the standards tree untouched.
- No deprecated-alias transition period (rejected: day-old surface).
- No JSON schema bump (rejected: nothing in the envelope changes).
- No change to `shll changelog`, `internal/versions`, or `internal/changelog` — the resolver seams are untouched.

## Affected Memory

- `cli/check-updates`: (modify) command-surface section (flags), exit-codes usage-error example, frontmatter description, and the "Mutual exclusion enforced in the run seam" design decision (superseded by the enum-validation decision)

## Impact

- `src/cmd/shll/check_updates.go` — flag constants, flag registration, `Long` help block, `runCheckUpdates` signature + validation, deletion of the both-flags path (~40 lines touched).
- `src/cmd/shll/check_updates_test.go` — seam call-site signature updates throughout; `TestCheckUpdates_BothBackendFlagsUsageError` → unknown-`--source`-value test.
- `README.md` — check-updates section examples + command-table row.
- `docs/site/skill.md` + `src/cmd/shll/standards/skill.md` (via `scripts/sync-standards.sh`) — one-line flag-surface description.
- `docs/memory/cli/check-updates.md` — hydrate-stage update (see Affected Memory).
- **Breaking CLI change** for `--released`/`--github` users; blast radius assessed in discussion as effectively zero (day-old surface; sole external consumer on an unmerged branch, instructed to use the flagless default form).
- No dependency, schema, or cross-package changes.

## Open Questions

None — the design conversation resolved all material decisions (flag name, values, validation seam, exit code, clean break vs. aliases, docs scope).

## Assumptions

<!-- STATE TRANSFER: decisions from the design conversation (Certain, "Discussed — ...") plus
     intake-time inclusions the agent graded itself. Promptless dispatch: no user was asked. -->

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Flag named `--source`, not `--backend` | Discussed — user accepted: aligns flag name/value with the `--json` envelope's existing `source` field | S:95 R:85 A:95 D:95 |
| 2 | Certain | Enum values `released` and `github` (default `released`), not `shll`/`github` | Discussed — user accepted: `released` describes semantics; `shll` is ambiguous; avoids envelope mismatch or a schema-2 break | S:95 R:80 A:95 D:95 |
| 3 | Certain | Unknown `--source` value → `errExitCode{code: usageExitCode}` (exit 2) validated in the run seam, before any brew/network access — not cobra machinery | Discussed — matches the recorded DD rejecting `MarkFlagsMutuallyExclusive` (cobra errors exit 1); pflag has no native enum | S:90 R:90 A:95 D:90 |
| 4 | Certain | Clean break: bool flags, `bothBackendsErrMsg`, and the both-flags check deleted; no hidden deprecated aliases | Discussed — day-old surface (v0.1.10, PR #67 same day); sole external consumer on unmerged run-kit branch 260720-n2ai, instructed to use the flagless form | S:95 R:70 A:90 D:90 |
| 5 | Certain | `--json` contract, exit-code contract (0/1/2), and default backend behavior all unchanged | Discussed — explicit constraints; schema stays 1, envelope untouched | S:95 R:90 A:95 D:95 |
| 6 | Certain | Constitution Toolkit-Standards check satisfied by reading `docs/site/standards/principles.md`; enum flag conforms, standards tree untouched | Discussed + re-verified at intake (principles.md read in full); adding a new convention line was offered and declined | S:90 R:90 A:90 D:90 |
| 7 | Certain | Scope includes updating stale flag references in `README.md` and `docs/site/skill.md` + re-running `scripts/sync-standards.sh` for the embedded copy | Not in the synthesized list, but deterministic: clean break otherwise leaves docs referencing removed flags, and `TestStandardsEmbedMatchesCanonical` forces the embed sync | S:60 R:90 A:95 D:90 |
| 8 | Confident | `--source` gets no shorthand (`-s`) | No existing `check-updates` flag carries a shorthand; additive to add later if wanted | S:50 R:85 A:80 D:75 |
| 9 | Confident | Exact usage-error message wording and `Long`-block prose left to apply, within the stated shape (names offending value + valid set; layered help structure preserved) | Conventional apply-time latitude; the contract (exit 2, pre-work validation, both values named) is pinned above | S:50 R:90 A:85 D:70 |

9 assumptions (7 certain, 2 confident, 0 tentative, 0 unresolved).

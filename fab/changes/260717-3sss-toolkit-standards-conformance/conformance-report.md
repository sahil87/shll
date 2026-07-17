## Conformance report — audited against shll v0.0.23

**Audit method.** Standards were re-enumerated at runtime from the installed binary
(`shll standards` → 4 entries; `shll standards <name>` for each). The audited
version is **shll v0.0.23** (`shll version` shll row) — standards are versioned with
the shll release, and the installed binary's embedded standards are byte-matched to
this repo's `docs/site/standards/` by the `TestStandardsEmbedMatchesCanonical` drift
guard. **Behavioral checks ran against a dev build from repo HEAD**
(`cd src && go build -o /tmp/shll-audit ./cmd/shll`), since the repo — not the
installed binary — is what this change can fix.

Runtime enumeration (`shll standards`):

| Standard | Scope | Audit method |
|----------|-------|--------------|
| `principles` | foundation | Each of the 10 principles assessed against actual subcommand behavior |
| `help-dump` | binary | Standard's own "Verifying conformance" checklist, executed verbatim |
| `readme-extraction` | repo | Standard's own "Verifying conformance" checklist, executed verbatim |
| `skill` | binary+repo | Deferred, not yet adopted (per directive + the standard's Adoption section) |

Fixes in this change are cited by the file/test that proves them; the fixing commit is
`b36f8e404d025260d435238aa2d522447a22dad0` (this report is carried into the PR body
verbatim by `/git-pr`).

---

### principles — 1 gap (fixed here); 9 PASS

- **№1 Non-interactive by default — PASS.** `shll uninstall` is the reference receipt: `Proceed? [y/N]` on a TTY (`ui.go` `stdinIsTTY` seam), `--yes`/`-y` for automation, refusal with the flag hint on a non-TTY stdin (`uninstallNoTTYHint`), and `--dry-run` needing no consent. No other subcommand blocks on a prompt.
- **№2 stdout is data, stderr is diagnostics — PASS.** Every command writes data to `cmd.OutOrStdout()` and diagnostics to `cmd.ErrOrStderr()`. `--json` is present on the programmatic-read surfaces: `shll list --json`, `shll doctor --json`, `shll standards --json` (all with `SetEscapeHTML(false)`, single trailing newline, `schema_version`-style additive stability). `shll shell-init` stdout is eval-safe (diagnostics to stderr, ASCII shell-comment separators only). `shll version` intentionally has no `--json` — it is human bug-report paste by explicit design, and the programmatic version surface is covered by `shll doctor --json` (per-tool `version` field) and `shll list --json`; this is a deliberate design boundary, not a gap (see plan Assumption 4).
- **№3 Help is a published contract — PASS.** Layered help (short + `Long` with usage examples) on every command; hidden `help-dump` walks the live cobra tree (never parses `-h`) and is validated by `help_dump_test.go`. See the help-dump section below.
- **№4 Fail fast with actionable errors — GAP, fixed here.** Errors name what failed / why / next step (e.g. `doctor`'s `suggest*` hints, `standards`' unknown-name diagnostic listing valid names). **But cobra-level usage errors exited `1`, not the toolkit-convention `2`** (`0` success / `1` operational failure / `2` usage error) — `shll bogus`, `shll list --bogus`, `shll doctor extra-arg` all exited 1, inconsistent with `shll shell-init`'s own bad-shell path which already exited 2. **Fixed here** — `src/cmd/shll/root.go` adds a root `SetFlagErrorFunc` wrapping flag-parse errors in `errExitCode{code: 2}`; `src/cmd/shll/main.go` `translateExit` classifies cobra's arg/command usage errors (stable prefixes) to exit 2 while keeping operational failures at exit 1. Proven by `src/cmd/shll/main_test.go` (`TestTranslateExit_Contract`, `TestRootCmd_FlagErrorIsUsageExit`) and verified end-to-end on the dev build (unknown command/flag/shorthand, extra args, bad arg count → 2; `shll standards nonexistent` and other operational failures → 1; success → 0).
- **№5 Visible mutation boundaries — PASS.** Read vs. write is clear from names/help; `shll uninstall --dry-run`, `shll update --dry-run`, `shll install --dry-run` all preview via the same single-sourced argv builders the live run uses (`brewUninstallArgv`, `upgradeArgv`, the `install`/`uninstall` preview rows). `shll doctor` is read-only by contract and documents it.
- **№6 Stateless, therefore retry-safe — PASS.** Constitution II (no database/state) is honored — versions from `brew list`/`--version`, shell-init from sub-tools, at every invocation. `shll install`/`shll shell-setup` are idempotent (install only what's missing; sentinel-wrapped rc block written once).
- **№7 Compose, don't reinvent — PASS.** All subprocesses route through `internal/proc` (`exec.CommandContext`). `shll update` probes `<tool> update --help` for `--skip-brew-update` before passing it (the advertised-flag probe pattern); brew is wrapped, never formula-parsed (`--json=v2`).
- **№8 Graceful degradation — PASS.** Missing tools skip (not error): `shll version` → `not installed`, `shll shell-init` omits absent tools while staying eval-safe, `shll changelog`'s fetch layer returns a typed unavailable result and exits 0. Color/glyphs are TTY-gated (`colorEnabled`) with ASCII fallback (`arrow`/`dash`/`more`, `statusMarker`); `NO_COLOR` honored.
- **№9 Bounded, high-signal output — PASS.** `shll changelog` caps at `changelogCapPerTool` (10) releases per tool with an explicit `… N more — full changelog: <url>` notice on overflow; `shll update` prints per-tool sections + a summary tail rather than raw brew dumps. The cap is stated in the output (no silent truncation).
- **№10 Agent-discoverable documentation — PASS (SHOULD).** README structured for extraction + `docs/site/` tree (see readme-extraction below); `shll standards` serves the standards offline. The one forward-leaning `<tool> skill` obligation is a SHOULD, deferred (see skill below).

### help-dump — PASS

Executed the standard's "Verifying conformance" checklist verbatim against the dev build:

- `shll help-dump` exits **0**, writes valid JSON to **stdout only**, **stderr empty**. ✓
- Envelope is `{tool, version, schema_version, root}` with **no `captured_at`**. ✓
- `completion`, `help`, and all hidden commands (incl. `help-dump` itself) are **absent** from the tree — the visible tree is the 10 authored commands. ✓
- `version` reflects the built binary (`root.Version`, ldflags-injected; `dev` on an unstamped build) — **not a hardcoded literal**. ✓
- Minimal test present and passing: `help_dump_test.go` (`TestHelpDump_ContractShape` pins exit 0 + valid JSON + expected `tool`/`schema_version`; `TestHelpDump_VersionPassthrough` pins version-from-binary; plus byte-for-byte, self-exclusion, and determinism tests). ✓

No fix required. The command tree is unchanged by this change (R5 adds no flag/subcommand; R6 is README-only), and the checklist was re-run post-fix to confirm.

### readme-extraction — 1 gap (fixed here); rest PASS

Executed the standard's "Verifying conformance" checklist verbatim against `README.md` + `docs/site/**`:

- **README head order — GAP, fixed here.** Head was `# shll` → toolkit blockquote → prose, **missing the contiguous badge run** the standard's head structure (§1) and checklist require. All 6 sibling toolkit repos (hop, wt, tu, idea, run-kit, fab-kit) carry the identical 3-badge run (Latest release / Downloads / Stars) immediately after the blockquote. **Fixed here** — `README.md` now reads `# shll` → blockquote → the 3-badge run (pointing at `sahil87/shll`) → intro prose, byte-consistent with the sibling repos.
- Relative link targets (`](./`, `](../`, `](docs/`): every README relative link points **into `docs/site/`** (`docs/site/install.md`, `docs/site/workflows.md`, `docs/site/standards/*.md`); no `docs/site/`↔`docs/site/` relative links exist to check. ✓
- **No relative images** anywhere in `README.md` or `docs/site/**`. ✓
- **No `#gh-*-mode-only`** fragments. ✓ (The only occurrences are inside `docs/site/standards/readme-extraction.md`'s own descriptive text of the rule, not live fragments.)
- **No site-destined mermaid fences.** ✓ (Same — only in the standard's own rule text.)
- **No `docs/site/` page named `overview`, `readme`, or `commands`.** ✓ (Tree: `install.md`, `workflows.md`, `standards/{principles,help-dump,readme-extraction,skill}.md`.)
- README **cross-links its `docs/site/` pages** (Install, Reference sections) **and the absolute command-reference URL** `https://shll.ai/shll/commands/`. ✓

Footer-heading determination (intake item b): the README has **no** `Contributing`/`Development`/`Building`/`License`/`Acknowledgements` heading, so the entire README is the pulled site slice. This is **conformant** — those headings are pull-*stop* markers (§2), not required content. Everything in the README (Install, Commands, How composition works, Troubleshooting, Reference) is site-worthy; nothing maintainer-only leaks. No footer split needed.

### skill — deferred, not yet adopted (phased per-repo adoption; tracked in [agst])

shll has no `shll skill` subcommand (verified: no such command in `src/cmd/shll/`; the runtime tree lists 10 commands, none named `skill`). Per the directive and the standard's own Adoption section ("**No tool ships `skill` today** … A tool without a `skill` subcommand is not yet in violation"), this is a known, deferred gap under the toolkit's phased per-repo rollout — **not** an in-scope fix for this change. The full `shll skill` (+ `shll agent-setup`) design is tracked in backlog **`[agst]`**. No `shll skill` implemented here.

---

## Summary

| Standard | Result | Disposition |
|----------|--------|-------------|
| principles | 9 PASS, 1 gap | №4 usage-error exit code → **fixed here** (`root.go`, `main.go`, `main_test.go`) |
| help-dump | PASS | — |
| readme-extraction | 1 gap | README badge run → **fixed here** (`README.md`) |
| skill | deferred | not yet adopted — tracked in `[agst]` |

**Deferrals created:** none new. The only deferred standard (`skill`) was already tracked in `[agst]`; both other gaps were small and additive and are fixed in this change.

**Verification:** `cd src && go test ./...` passes (all packages green); `go vet ./...` clean. The command tree is unchanged, so the help-dump checklist re-run post-fix confirms continued conformance.

# Intake: Install Bootstrap Gaps

**Change**: 260817-nava-install-bootstrap-gaps
**Created**: 2026-08-17

## Origin

One-shot `/fab-new nava` invocation resolving backlog item `[nava]` (added 2026-08-17 from fresh-VM testing notes). Raw backlog text:

> install bootstrap gaps (fresh-VM testing 2026-08-17): (1) preflight — probe git (macOS via xcode-select -p, never command -v git: CLT shim false-positives), tmux, curl before the brew check; print all missing deps at once with per-platform fix commands. (2) bootstrap Homebrew when absent — NONINTERACTIVE=1 brew.sh proven headless on macOS incl CLT via softwareupdate (Linux needs git first); then use the absolute brew path for the rest of the run and print the shellenv line (kills the brew-not-on-PATH trap). (3) docs — failed download makes curl|sh silently exit 0; minimal Ubuntu lacks curl; homebrew/core now has an unrelated run-kit formula so always show tap-qualified names

The backlog entry is a distilled decision record from the user's own fresh-VM testing session — the key design choices (probe methods, NONINTERACTIVE bootstrap, absolute-path + shellenv handling, docs-only treatment of item 3) were made and validated there, so this intake encodes them as high-confidence assumptions rather than open questions.

**Design-decision reversal (deliberate)**: item (2) reverses the recorded design decision in `docs/memory/ci/install-bootstrap.md` — *"Require Homebrew; never auto-install it"* (introduced by m1zt, rationale: "auto-installing Homebrew from a piped-to-`sh` script is a large, surprising side effect"). The reversal is user-directed: fresh-VM testing proved `NONINTERACTIVE=1` brew.sh runs fully headless (macOS including CLT via `softwareupdate`), which removes the "surprising side effect" objection — the bootstrap is now deterministic and announceable. Hydrate must rewrite that design decision with this rationale, not silently contradict it.

## Why

Fresh-VM testing (2026-08-17) walked the advertised "clean machine to fully wired toolkit" path (`curl -fsSL https://shll.ai/install | sh`) on machines that actually look like clean machines, and it failed in three distinct ways:

1. **Death by a thousand missing deps.** The script's only precondition check is `command -v brew`. A genuinely fresh VM is missing several things at once (git/CLT, curl, tmux), and the current flow surfaces them one at a time, each as a downstream failure with no fix guidance: brew absent → hard stop with a brew.sh pointer; on macOS `command -v git` false-positives on the CLT shim (the shim exists at `/usr/bin/git` even when CLT is not installed, so naive probes say "git present" and then brew's git operations fail or pop a GUI dialog); tmux absence surfaces much later as a confusing run-kit runtime failure. The user retries the one-liner N times, fixing one dep per round.

2. **The brew hard-stop contradicts the product promise.** README leads with "From a clean machine to a fully wired toolkit" — but a clean machine doesn't have Homebrew, so step one is a refusal. Testing proved the refusal is no longer necessary: `NONINTERACTIVE=1` brew.sh installs headlessly on macOS (including CLT via `softwareupdate`) and on Linux (given git). And even after a successful manual brew install, brew is typically **not on PATH** in the current shell (fresh installs land in `/opt/homebrew` or `/home/linuxbrew/.linuxbrew`), so the re-run fails again — the brew-not-on-PATH trap.

3. **Docs mislead on three real-world edges.** (a) A failed download makes `curl … | sh` exit 0 silently — `sh` reads empty input and succeeds, so `&& next-step` chains proceed as if the install worked (curl's `-S` does print the error to stderr, but the exit code lies). (b) Minimal Ubuntu images ship without curl, so the one-liner itself is unrunnable until `apt-get install curl`. (c) homebrew/core now carries an **unrelated** `run-kit` formula — any doc showing a bare `brew install run-kit` (or implying bare names work) installs a stranger's software; every formula reference must be tap-qualified `sahil87/tap/<formula>`.

If we don't fix this: the first-run experience — the single most leveraged surface for toolkit adoption, and the thing shll.ai's homepage sells — fails on exactly the machines it advertises to.

Why this approach (grow the bootstrap script, not `shll install`): everything added here runs **before brew and before shll exist**, which is precisely the circularity carve-out the thin-bootstrap design already grants the script (Constitution III). `shll install` cannot probe-or-bootstrap Homebrew because the user cannot have `shll` without Homebrew having worked. The script's job description extends from "solve the shll-before-shll circularity" to "solve the pre-brew phase"; all post-brew intelligence (roster, subset, per-formula trust for the other tools) stays in `shll install`, unchanged.

## What Changes

All changes are in `scripts/install.sh` and user-facing docs. **No Go changes** — `shll install` (including its own `installBrewMissingHint`) is untouched.

### 1. Preflight: probe git, tmux, curl before the brew check

New `preflight()` step in `scripts/install.sh`, run first in `main()`:

- **Platform detection**: `uname -s` → `Darwin` / `Linux`.
- **git probe — platform-specific, never `command -v git` on macOS**:
  - macOS: `xcode-select -p >/dev/null 2>&1` (the real CLT presence check). `command -v git` is explicitly forbidden as the macOS probe — the CLT shim at `/usr/bin/git` makes it false-positive when CLT is not installed.
  - Linux: `command -v git >/dev/null 2>&1`.
- **curl probe**: `command -v curl >/dev/null 2>&1` (covers the saved-then-run invocation path, and brew.sh itself needs curl).
- **tmux probe**: `command -v tmux >/dev/null 2>&1`.
- **Report all misses at once** — collect every failed probe, then print one consolidated block with a per-platform fix command per missing dep (e.g. macOS git/CLT → `xcode-select --install`; Debian/Ubuntu → `sudo apt-get install -y git curl tmux` style hints; tmux via brew once brew exists). Never fail-on-first-dep.
- **Fatality matrix** (synthesized — the backlog specifies probes and consolidated reporting, not exit semantics; graded Confident in Assumptions row 7):
  - `curl` missing → **fatal** (exit 1 after the consolidated report; brew.sh and brew both need it).
  - `git` missing → **fatal on Linux when brew is absent** (brew.sh needs git first — backlog: "Linux needs git first"); **fatal on macOS when brew is already present** (brew's tap/git operations need real CLT); **informational-only on macOS when the brew bootstrap will run** (NONINTERACTIVE brew.sh installs CLT itself via `softwareupdate` — proven in testing; print a "the Homebrew bootstrap will install the Command Line Tools" note instead of failing).
  - `tmux` missing → **warn-only, never fatal**: tmux is run-kit's *runtime* dependency (probe + install-hint per the install-composition standard), not an install prerequisite; blocking the whole toolkit install on it would violate Constitution V. The warning includes the fix command so the fresh-VM user learns about it now instead of at first `rk` use.

### 2. Bootstrap Homebrew when absent

The current hard stop (`Homebrew is required … exit 1`) is replaced by a bootstrap:

- When `command -v brew` fails, print a clear progress line, then run the official installer headlessly:

  ```sh
  NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
  ```

  brew.sh is bash-only, so the (POSIX-sh) bootstrap invokes it via `/bin/bash -c` — bash is present on fresh macOS and Ubuntu images. Proven headless in fresh-VM testing on macOS **including CLT install via `softwareupdate`**; on Linux the preflight has already guaranteed git (fatal above), which is brew.sh's Linux prerequisite.
- **Unconditional — no opt-in flag.** The script is piped to `sh` (stdin is the pipe; nothing can prompt), and positional args pass through verbatim as the `shll install` subset, so a `--with-brew`-style flag would collide with tool names. The progress line is the announcement.
- **Absolute brew path for the rest of the run.** A fresh brew install is not on PATH in the running process. After the bootstrap (and equally when brew was already present), resolve `BREW` once and use it for every subsequent brew call (`$BREW trust …`, `$BREW install …`):
  - brew already on PATH → `BREW=$(command -v brew)`.
  - freshly bootstrapped → probe the known install prefixes for an executable: `/opt/homebrew/bin/brew` (Apple Silicon), `/usr/local/bin/brew` (Intel macOS), `/home/linuxbrew/.linuxbrew/bin/brew` (Linux).
- **PATH for the exec hand-off.** The freshly installed `shll` binary lives in the brew prefix, so before `exec shll install "$@"` the script runs `eval "$($BREW shellenv)"` in its own process — otherwise the exec fails with command-not-found on every bootstrap run.
- **Print the shellenv line for the user** (kills the brew-not-on-PATH trap for their *next* shell): after a bootstrap, print the exact rc line to persist, e.g. `eval "$(/opt/homebrew/bin/brew shellenv)"`, with a pointer that `shll shell-setup` wires shll's own init but NOT brew's (brew's shellenv is the user's/brew-installer's rc line to keep).
- **Existing invariants preserved**: POSIX sh, `set -eu`, whole body in `main() { … }` with `main "$@"` as the last line (truncation guard), fully non-interactive, file stays at `scripts/install.sh` (the shll.ai raw-fetch URL contract pins the path). Under `set -eu` a failing brew.sh aborts the script with the installer's own error output — same surface-the-error tolerance as the existing trust/install steps.
- The shll trust-then-install block and the `exec shll install "$@"` hand-off are otherwise unchanged (they just call `$BREW` instead of bare `brew`).

### 3. Docs: three fresh-VM corrections

Files: `README.md` (`## Install`), `docs/site/install.md`, `docs/site/workflows.md` (fresh-machine walkthrough). Per the constitution's Toolkit Standards clause, the edits must be checked against the governing standards in `docs/site/standards/` (at minimum `install-composition` — install docs centralized on shll.ai, shll-README carve-out) before/while applying.

- **Failed-download pitfall (document, don't change the one-liner).** Add a note where the one-liner is introduced: if the download fails, `curl -fsSL … | sh` still **exits 0** — `sh` runs empty input successfully — so an `&& next-step` chain continues as if the install worked; curl's error appears on stderr (`-S`), but the exit code cannot be trusted. The `main()` wrapper protects against *partial* execution, not *failed* download. The piped one-liner remains the recommended form; the note tells users what a silent no-op means and to check `command -v shll` / re-read stderr.
- **Minimal-Ubuntu curl prerequisite.** State that minimal Ubuntu/Debian images ship without curl and the one-liner needs `sudo apt-get install -y curl` first (the preflight also catches this for the saved-script path, but the piped path never starts without curl).
- **Tap-qualified names everywhere.** Audit README + docs/site for any bare formula reference and ensure every brew command/formula mention is tap-qualified `sahil87/tap/<formula>` — homebrew/core now has an **unrelated** `run-kit` formula, so a bare `brew install run-kit` installs the wrong software. (Docs currently look mostly qualified; the audit makes it a checked guarantee, and adds a one-line warning where run-kit install is discussed.)
- **Reframe the Homebrew requirement.** README's "the script never auto-installs Homebrew" line and docs/site/install.md's "requires Homebrew … never auto-installs it" framing become "bootstraps Homebrew headlessly when absent (official installer, `NONINTERACTIVE=1`)", including the post-bootstrap shellenv guidance. The manual `brew trust … && brew install sahil87/tap/shll` bootstrap stays documented as the by-hand alternative.

## Affected Memory

- `ci/install-bootstrap`: (modify) — behavior contract rewrite: preflight requirement (+ probe methods, consolidated reporting, fatality matrix), Homebrew-bootstrap requirement replacing the brew-required hard stop, `$BREW` absolute-path + shellenv contract, updated frontmatter description; **rewrite the "Require Homebrew; never auto-install it" design decision** to record the reversal and its why (NONINTERACTIVE proven headless, fresh-VM testing 2026-08-17).
- `cli/install`: (modify) — the "`curl | sh` upstream entry point" section and frontmatter description characterize the script as brew-required/thin; update the division-of-labor framing (script now owns the whole pre-brew phase; `shll install` contract itself unchanged).

## Impact

- `scripts/install.sh` — the entire behavioral change (preflight, brew bootstrap, `$BREW` threading, shellenv print). Stays POSIX sh at the pinned path.
- `README.md` — `## Install` section reframe + pitfall/prereq notes + name audit.
- `docs/site/install.md`, `docs/site/workflows.md` — same docs corrections at depth.
- No `src/` (Go) changes, no CI changes, no roster changes.
- Test gate (per the existing ci/install-bootstrap contract): `sh -n scripts/install.sh`; shellcheck when available; runtime paths verified by inspection. The heavy runtime claim (NONINTERACTIVE brew.sh headless incl. CLT) is already validated by the user's fresh-VM testing — the change encodes those findings rather than re-deriving them.

## Open Questions

None — the backlog item is decision-complete from the user's fresh-VM testing; the remaining latitude (exit-code semantics per missing dep, exact fix-command wording) is graded in Assumptions below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Bootstrap Homebrew when absent via `NONINTERACTIVE=1` brew.sh, reversing the m1zt "never auto-install" design decision | Backlog explicit; user-directed reversal after fresh-VM testing proved it headless (macOS incl. CLT via softwareupdate) | S:90 R:60 A:90 D:90 |
| 2 | Certain | Preflight lives in `scripts/install.sh` before the brew check; probes git, tmux, curl; macOS git via `xcode-select -p`, never `command -v git` | Backlog explicit, including the CLT-shim false-positive rationale | S:95 R:85 A:95 D:95 |
| 3 | Certain | All missing deps reported at once with per-platform fix commands — never fail-on-first | Backlog explicit ("print all missing deps at once with per-platform fix commands") | S:95 R:90 A:90 D:95 |
| 4 | Certain | Post-bootstrap the script resolves `$BREW` (absolute path) for every brew call, evals `$BREW shellenv` in-process so `exec shll install` resolves, and prints the shellenv rc line to the user | Backlog explicit ("use the absolute brew path for the rest of the run and print the shellenv line"); the in-process eval is the only way the exec hand-off can find the fresh shll | S:90 R:80 A:90 D:90 |
| 5 | Confident | Homebrew bootstrap is unconditional — no opt-in flag | Piped stdin cannot prompt (existing non-interactive requirement); positional args are the install subset so a flag would collide; backlog phrases it unconditionally | S:75 R:70 A:85 D:80 |
| 6 | Confident | tmux missing is warn-only, never fatal | run-kit *runtime* dep (probe + install-hint per install-composition standard), not an install prerequisite; Constitution V graceful degradation | S:55 R:85 A:80 D:70 |
| 7 | Confident | git/curl fatality matrix: curl fatal; git fatal on Linux pre-bootstrap and on macOS-with-brew-present, informational on macOS when the bootstrap will run (brew.sh installs CLT itself) | Synthesized from backlog hints ("Linux needs git first", CLT-via-softwareupdate proven) + Constitution V; exit semantics not spelled out in the backlog | S:50 R:85 A:55 D:50 |
| 8 | Certain | Docs fixes: document the failed-download-exits-0 pitfall, add the minimal-Ubuntu curl prerequisite, tap-qualify every formula name (homebrew/core has an unrelated run-kit) | All three explicit in the backlog under "(3) docs" | S:90 R:90 A:85 D:85 |
| 9 | Confident | The download pitfall is handled by documentation only — the piped one-liner stays the recommended install form | Backlog files it under "docs"; alternative forms (`sh -c "$(curl …)"`) fail identically on empty input, and download-then-run is a docs-note alternative, not a new recommendation | S:70 R:85 A:75 D:65 |
| 10 | Confident | No Go changes: `shll install` behavior and hints untouched; scope is script + docs (+ memory at hydrate) | Everything added runs pre-brew/pre-shll where Go cannot; the thin-bootstrap circularity carve-out extends naturally (Constitution III) | S:70 R:80 A:80 D:75 |
| 11 | Certain | Script invariants preserved: POSIX sh, `main()` truncation guard, `set -eu`, non-interactive, pinned `scripts/install.sh` path | Existing requirements in ci/install-bootstrap (truncation-safety, shll.ai raw-fetch URL contract) all still bind | S:85 R:75 A:95 D:90 |
| 12 | Confident | brew.sh is invoked via `/bin/bash -c` while the outer script stays POSIX sh | The official installer is bash-only; bash is present on fresh macOS and Ubuntu images | S:60 R:85 A:80 D:75 |

12 assumptions (6 certain, 6 confident, 0 tentative, 0 unresolved).

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

// Marker labels printed for each tool's worst-applicable check. Plain ASCII so
// they render identically on a non-TTY and inside `--json`. Named constants per
// code-quality.md (no magic strings).
const (
	markerOK   = "OK"
	markerWarn = "WARN"
	markerFail = "FAIL"
)

// versionState classifies the outcome of the per-tool `--version` probe into the
// three cases doctor distinguishes (version.go's toolVersion collapses the latter
// two into a single "not installed" label; doctor needs them apart so it can tell
// "not installed" from "stale brew link").
type versionState int

const (
	// versionMissing — binary not on PATH (proc.ErrNotFound). Drives FAIL.
	versionMissing versionState = iota
	// versionUnreportable — binary on PATH but `--version` errored, timed out, or
	// produced an empty normalized string (the stale-link case). Drives FAIL.
	versionUnreportable
	// versionOK — binary on PATH and reported a non-empty normalized version.
	versionOK
)

// suggestionFmt* are the actionable hints printed on each non-OK line and carried
// in the JSON `suggestion` field. Named format strings per code-quality.md — the
// exact wording is part of the user contract, so it lives in one place.
const (
	// suggestMissingFmt takes the brew formula (tool.Formula, e.g.
	// "sahil87/tap/hop").
	suggestMissingFmt = "run 'brew install %s'"
	// suggestUnreportableFmt takes (tool name, tool formula).
	suggestUnreportableFmt = "installed but '%s --version' failed — try 'brew reinstall %s'"
	// suggestNotWired is fixed text (no interpolation).
	suggestNotWired = "not wired — run 'shll shell-setup' then 'exec $SHELL'"
	// suggestShellUnresolvableFmt takes the raw $SHELL value.
	suggestShellUnresolvableFmt = "cannot verify shell wiring — $SHELL is %q; pass a supported shell environment or run 'shll shell-setup zsh'"
	// suggestCorruptBlock is fixed text for a corrupted shll block (open sentinel
	// without a matching close). doctor must NOT tell the user to run
	// `shll shell-setup` here, because shell-setup refuses to modify a corrupted
	// block (exit 2) — the actionable fix is manual cleanup first.
	suggestCorruptBlock = "shll block in your rc file is corrupted (unclosed sentinel) — fix or remove it manually, then run 'shll shell-setup'"
	// suggestNotTrustedFmt takes the tool's formula (e.g. "sahil87/tap/hop"). An
	// installed-but-untrusted tool still runs today, but its next `brew upgrade`
	// (via `shll update` or plain brew) is refused on Homebrew 6.0+, so doctor
	// WARNs with an actionable fix.
	suggestNotTrustedFmt = "formula not trusted — run 'shll install' (or 'brew trust --formula %s'); future upgrades will fail without it"
)

// doctorResult is the typed per-tool record. It is the single source for BOTH the
// text and JSON renderings (Design Decision: text + JSON derive from one struct so
// they cannot drift). The JSON tags fix the machine-readable field contract.
type doctorResult struct {
	Tool       string `json:"tool"`
	Status     string `json:"status"`
	Version    string `json:"version"`
	OnPath     bool   `json:"on_path"`
	VersionOK  bool   `json:"version_ok"`
	ShellInit  bool   `json:"shell_init"`
	Wired      bool   `json:"wired"`
	Suggestion string `json:"suggestion"`
}

func newDoctorCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "verify every sahil87 tool is installed, runnable, and wired",
		Long: `Verify the sahil87 toolkit is correctly installed and wired. For every roster
tool, doctor checks that (1) the binary is on PATH, (2) it reports a version (so
a half-installed/stale brew link is caught), (3) its Homebrew formula is trusted
(so a future 'brew upgrade' won't be refused on Homebrew 6.0+), and (4) — for
tools that ship shell integration (wt, tu, hop) — shll's composed shell-init eval
block is present in your rc file.

Each tool gets one line with an OK / WARN / FAIL marker; non-OK lines carry an
actionable suggestion. A missing or non-running binary is FAIL; an installed tool
that simply isn't wired into your shell — or whose formula isn't trusted — is
WARN (it still works when invoked directly). doctor exits non-zero if ANY tool is
FAIL, so it is scriptable in CI.

The trust sub-check queries 'brew trust --json=v1' read-only; if your Homebrew is
too old to ship 'brew trust' (where trust isn't required anyway), it is skipped
silently. doctor is strictly read-only — it never installs, upgrades, trusts, or
edits your rc file.

Use --json to emit a machine-readable array (one object per tool) instead of the
aligned text table; the same checks and the same exit contract apply.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd.Context(), jsonOut, os.Getenv, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit a machine-readable JSON array instead of the aligned text table")
	return cmd
}

// runDoctor is the implementation seam invoked by the cobra factory's RunE.
// Extracted so doctor_test.go can drive it with bytes.Buffer writers, a fake
// proc.Runner, and a map-backed env (mirroring resolveShell/resolveRcFile, which
// take an env func for the same reason). Production passes os.Getenv.
//
// It walks the Roster in order, evaluates each tool, renders (text or JSON), and
// returns errSilent (→ exit 1) iff any tool's worst check is FAIL. WARN never
// affects the exit. The exit logic is identical for text and --json.
func runDoctor(ctx context.Context, jsonOut bool, env func(string) string, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// The wiring fact is a single rc-file fact shared by every shell-init tool
	// (shll's composed eval block covers them all), so resolve it ONCE up front.
	fact := resolveWiringFact(env)

	// The trust fact is a single brew-wide fact (one `brew trust --json=v1`
	// query), so resolve it ONCE up front too — mirroring the single-rc-read
	// wiring fact. Gated by brewTrustAvailable: a brew absent or too old to ship
	// `brew trust` (pre-6.0, where trust isn't required anyway) yields
	// available=false and the per-tool trust sub-check is skipped silently
	// (Constitution V — never WARN on a state we can't determine).
	trust := resolveTrustFact(ctx)

	results := make([]doctorResult, 0, len(Roster)+1)
	anyFail := false
	// shll-first row: shll is the running process, so its binary is always
	// present and its version comes from the package var (shllSelfVersion, NOT a
	// `shll --version` self-subprocess). shll ships no shell-init, so — like
	// idea/rk/fab-kit — no wiring check applies (ShellInit:false). The row is thus
	// effectively always OK and is built directly (never via evaluateTool, which
	// would spawn a subprocess); it deliberately does NOT touch anyFail, so it
	// cannot perturb the scriptable any-FAIL→exit-1 contract.
	results = append(results, shllDoctorResult())
	for _, tool := range Roster {
		res := evaluateTool(ctx, tool, fact, trust)
		if res.Status == markerFail {
			anyFail = true
		}
		results = append(results, res)
	}

	var renderErr error
	if jsonOut {
		renderErr = renderDoctorJSON(stdout, results)
	} else {
		renderErr = renderDoctorText(stdout, results)
	}
	if renderErr != nil {
		fmt.Fprintf(stderr, "shll doctor: %v\n", renderErr)
		return errSilent
	}

	if anyFail {
		// Per-tool diagnostics are already on stdout (text) or in the JSON
		// suggestion fields, so errSilent suppresses a redundant stderr line while
		// still mapping to exit 1 via translateExit.
		return errSilent
	}
	return nil
}

// wiringFact captures the single shll-block rc-file fact, resolved once per run.
// shellResolved is false when $SHELL is unset/unsupported (no rc path to read) —
// in which case wiring degrades to WARN for shell-init tools. rawShell is the
// unresolved $SHELL value, carried for the explanatory suggestion.
type wiringFact struct {
	shellResolved bool
	wired         bool
	// corrupt is true when the rc file holds an shll block with an opening
	// sentinel but no matching close (locateBlock's partial signal). shell-setup
	// refuses to touch such a block, so doctor surfaces a distinct WARN rather
	// than the plain "not wired, run shll shell-setup" hint that would mislead.
	corrupt  bool
	rawShell string
}

// resolveWiringFact computes the shared wiring fact: resolve the shell from $SHELL,
// derive the rc path, read it, and report whether shll's composed eval block is
// present (blockMatch.hasEval — covers both the new and legacy sentinels). It is
// strictly READ-ONLY (os.ReadFile only); doctor never writes to the rc file. A
// missing/unreadable rc file is treated as "not wired" (shellResolved stays true —
// the shell resolved fine, the wiring simply isn't there yet).
func resolveWiringFact(env func(string) string) wiringFact {
	shell, err := resolveShell([]string{}, env)
	if err != nil {
		return wiringFact{shellResolved: false, rawShell: env("SHELL")}
	}
	rcPath := resolveRcFile(shell, env)
	content, readErr := os.ReadFile(rcPath)
	if readErr != nil {
		// No rc file (or unreadable) → no wiring detected, but the shell resolved,
		// so this is a plain "not wired" rather than the unresolvable-shell case.
		return wiringFact{shellResolved: true, wired: false}
	}
	m, newOK, legacyM, legacyOK, partial := locateBlock(content)
	if partial {
		// Open-without-close sentinel — shell-setup would refuse to modify it, so
		// "not wired, run shll shell-setup" would send the user down a dead end.
		return wiringFact{shellResolved: true, corrupt: true}
	}
	wired := (newOK && m.hasEval) || (legacyOK && legacyM.hasEval)
	return wiringFact{shellResolved: true, wired: wired}
}

// trustFact captures Homebrew's current trust state, resolved once per doctor run
// via `brew trust --json=v1`. available is false when brew is absent or too old to
// ship `brew trust` — in which case the per-tool trust sub-check is skipped
// silently (doctor never WARNs on a trust state it cannot determine). tapTrusted
// is true when the whole sahil87 tap is trusted (which trusts every formula under
// it); formulae is the set of individually-trusted fully-qualified formula names.
type trustFact struct {
	available  bool
	tapTrusted bool
	formulae   map[string]bool
}

// resolveTrustFact queries Homebrew's trust state once. It is gated by
// brewTrustAvailable (the same capability probe `shll install` uses) so a pre-6.0
// brew lacking `brew trust` yields available=false and the trust sub-check is
// skipped. The query itself (`brew trust --json=v1`, via brewTrustList) is
// strictly read-only and routes through internal/proc — doctor NEVER reads
// ~/.homebrew/trust.json directly (Constitution III).
func resolveTrustFact(ctx context.Context) trustFact {
	if !brewTrustAvailable(ctx) {
		return trustFact{available: false}
	}
	taps, formulae, ok := brewTrustList(ctx)
	if !ok {
		// brew ships `trust` but the JSON query failed/garbled — degrade to
		// "cannot determine" rather than WARN on a guess.
		return trustFact{available: false}
	}
	tapTrusted := false
	for _, tp := range taps {
		if tp == tapName {
			tapTrusted = true
			break
		}
	}
	set := make(map[string]bool, len(formulae))
	for _, fm := range formulae {
		set[fm] = true
	}
	return trustFact{available: true, tapTrusted: tapTrusted, formulae: set}
}

// trusts reports whether the given roster formula counts as trusted: either the
// whole tap is trusted, or the fully-qualified formula appears in the trusted set
// (tap- and formula-level trust both count). Only meaningful when available.
func (tf trustFact) trusts(formula string) bool {
	return tf.tapTrusted || tf.formulae[formula]
}

// evaluateTool composes the per-tool checks into a doctorResult with the
// worst-applicable marker (FAIL > WARN > OK) and the matching suggestion.
func evaluateTool(ctx context.Context, tool Tool, fact wiringFact, trust trustFact) doctorResult {
	res := doctorResult{
		Tool:      tool.Name,
		ShellInit: len(tool.ShellInit) > 0,
	}

	version, state := probeVersion(ctx, tool)
	switch state {
	case versionMissing:
		res.Status = markerFail
		res.Suggestion = fmt.Sprintf(suggestMissingFmt, tool.Formula)
		return res
	case versionUnreportable:
		res.OnPath = true
		res.Status = markerFail
		res.Suggestion = fmt.Sprintf(suggestUnreportableFmt, tool.Name, tool.Formula)
		return res
	}
	// versionOK — binary checks pass.
	res.OnPath = true
	res.VersionOK = true
	res.Version = version

	// Trust sub-check (worst-check-wins WARN tier, alongside the wiring WARN).
	// Applies to ALL installed roster tools — not just shell-init ones — since
	// every formula needs trust to upgrade on Homebrew 6.0+. A binary FAIL already
	// returned above, so it dominates. The trust WARN is checked before wiring:
	// an untrusted tool's next upgrade is refused outright (higher impact than an
	// unwired but functional tool), and both are WARN so the exit code is identical
	// either way. Skipped silently when trust state is unavailable (Constitution V).
	if trust.available && !trust.trusts(tool.Formula) {
		res.Status = markerWarn
		res.Suggestion = fmt.Sprintf(suggestNotTrustedFmt, tool.Formula)
		return res
	}

	if !res.ShellInit {
		// Non-shell-init tool: no wiring check applies — OK.
		res.Status = markerOK
		return res
	}

	// Shell-init tool: the wiring check applies.
	if !fact.shellResolved {
		res.Status = markerWarn
		res.Suggestion = fmt.Sprintf(suggestShellUnresolvableFmt, fact.rawShell)
		return res
	}
	if fact.corrupt {
		// Corrupted shll block — distinct from plain "not wired": shell-setup
		// would refuse to repair it, so the suggestion points at manual cleanup.
		res.Status = markerWarn
		res.Suggestion = suggestCorruptBlock
		return res
	}
	if !fact.wired {
		res.Status = markerWarn
		res.Suggestion = suggestNotWired
		return res
	}
	res.Wired = true
	res.Status = markerOK
	return res
}

// shllDoctorResult builds the always-OK shll-first record for `shll doctor`. It
// is NOT produced by evaluateTool: shll is the running process, so its binary is
// definitionally on PATH and its version is read from the package var
// (shllSelfVersion) rather than probed via a `shll --version` self-subprocess.
// shll ships no shell-init (ShellInit:false), so no wiring check applies — like
// idea/rk/fab-kit. The row is therefore always OK, and the caller deliberately
// keeps it out of the anyFail tally so it cannot perturb the exit-1-on-any-FAIL
// contract. Both the text and --json renderers consume it through the normal
// results walk.
func shllDoctorResult() doctorResult {
	return doctorResult{
		Tool:      shllSelf.Name,
		Status:    markerOK,
		Version:   shllSelfVersion(),
		OnPath:    true,
		VersionOK: true,
		ShellInit: false,
		Wired:     false,
	}
}

// probeVersion runs a single `<tool> --version` probe (bounded by versionTimeout)
// and classifies the outcome into the three-way versionState. It reuses the SAME
// primitives as version.go's toolVersion (proc.Run + normalizeVersion) so the
// version-reporting behavior stays single-sourced; the only difference is that
// doctor keeps missing and unreportable apart (toolVersion folds both into
// notInstalledLabel). Constitution I: subprocess execution routes through proc.
func probeVersion(ctx context.Context, tool Tool) (string, versionState) {
	subCtx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	out, err := proc.Run(subCtx, tool.Name, "--version")
	if err != nil {
		if errors.Is(err, proc.ErrNotFound) {
			return "", versionMissing
		}
		return "", versionUnreportable
	}
	v := normalizeVersion(string(out))
	if v == "" {
		return "", versionUnreportable
	}
	return v, versionOK
}

// renderDoctorText prints one tabwriter-aligned line per tool in roster order,
// followed by a problem-count summary tail when any tool is non-OK. The OK marker
// MAY be colored green on a real TTY (ui.go's colorEnabled — doctor is
// human-facing, not eval'd, so the shell-init eval-safety exception does not
// apply); markers are plain ASCII otherwise.
func renderDoctorText(stdout io.Writer, results []doctorResult) error {
	color := colorEnabled(stdout)
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	problems := 0
	for _, r := range results {
		if r.Status != markerOK {
			problems++
		}
		detail := r.Suggestion
		if r.Status == markerOK && r.ShellInit && r.Wired {
			detail = "wired"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Tool, markerGlyph(r.Status, color), r.Version, detail)
	}
	// Propagate Flush errors (e.g. broken pipe when piping to `head`) — mirrors
	// runVersion's Flush handling so a write failure surfaces as exit 1 rather
	// than being silently swallowed under a success return.
	if err := w.Flush(); err != nil {
		return err
	}
	if problems > 0 {
		// Denominator is the count of *checkable* tools — the managed roster —
		// NOT len(results), which includes the prepended always-OK shll row.
		// Including it would mis-report "N of 7" when only the 6 roster tools can
		// ever register a problem (the shll row never increments `problems`).
		if _, err := fmt.Fprintf(stdout, "\n%d of %d tools have problems. Run the suggested commands above, then re-run shll doctor.\n", problems, len(Roster)); err != nil {
			return err
		}
	}
	return nil
}

// markerGlyph renders a status marker, optionally colorizing the OK marker green
// on a TTY. WARN/FAIL are left plain in both modes — the wording carries the
// signal and there is no green-equivalent affordance for them in ui.go's palette.
func markerGlyph(status string, color bool) string {
	if color && status == markerOK {
		return ansiGreen + markerOK + ansiReset
	}
	return status
}

// renderDoctorJSON marshals the results to a JSON array (typed-struct marshal, no
// hand-built strings) with a trailing newline and no ANSI color, regardless of
// TTY — machine consumers must get clean JSON.
func renderDoctorJSON(stdout io.Writer, results []doctorResult) error {
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(results)
}

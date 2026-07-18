package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/changelog"
)

// specToolSep separates a tool name from its explicit version range in a
// changelog spec (`tool@old..new`). specRangeSep separates the old and new
// versions (`old..new`). Named constants per code-quality.md (no magic strings).
const (
	specToolSep  = "@"
	specRangeSep = ".."
)

// changelogCapPerTool caps how many releases `shll changelog` prints per tool;
// when the range holds more, a cap notice plus the Full Changelog compare URL is
// printed instead of an unbounded dump. Named constant per code-quality.md.
const changelogCapPerTool = 10

// changelogSpec is one parsed positional argument. tool identifies the roster
// tool (or shll-self via self=true); when explicit is true, old/new are the
// user-supplied range and brew is never consulted for it; when explicit is
// false, the range is resolved installed → latest.
type changelogSpec struct {
	name     string // the arg's tool name (roster name or "shll")
	self     bool   // shll itself was named
	rosterIx int    // position in Roster for ordering; -1 for shll-self (sorts first)
	explicit bool   // an explicit @old..new range was given
	old      string // explicit-range low bound (exclusive)
	new      string // explicit-range high bound (inclusive)
}

func newChangelogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "changelog [tool[@old..new]]...",
		Short: "show release notes for shll tools (what an update would bring)",
		Long: `Show GitHub release notes for shll tools.

With no arguments, shll changelog shows the pending releases for every installed
tool (its installed version → the latest release) — "what would an update bring?".
Name one or more tools to scope it; add an explicit range with ` + "`tool@old..new`" + `
to show the releases in ` + "`(old, new]`" + ` regardless of what is installed.

  shll changelog                          all installed tools: installed → latest
  shll changelog tu                       one tool: installed → latest
  shll changelog tu@0.6.2..0.6.4          explicit range (releases in (0.6.2, 0.6.4])
  shll changelog tu@0.6.2..0.6.4 hop@0.1.16..0.1.18

Valid tool names are the roster names plus shll itself. Versions are accepted with
or without a leading v. Release data is fetched from GitHub, unauthenticated; if a
fetch fails the entry degrades to a compare URL and the command still exits 0.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runChangelog(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args)
		},
	}
}

// runChangelog is the implementation seam for `shll changelog`, extracted from
// the cobra factory so changelog_test.go can drive it directly with bytes.Buffer
// writers, a fake proc.Runner, and the internal/changelog baseURL seam.
//
// It parses the positional specs (tool[@old..new]), validates tool names via the
// shared resolveTargets, resolves no-range specs to installed → latest (a brew
// captured read + a GitHub latest-tag fetch), then fetches every tool's range
// concurrently and renders each in roster order (shll first when named). Fetch
// failures degrade to a compare-URL line and NEVER change the exit code
// (Constitution V). An unknown tool name, or a named-but-not-installed tool in a
// no-range form, is a hard error (errSilent).
func runChangelog(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	specs, err := parseChangelogSpecs(args)
	if err != nil {
		fmt.Fprintf(stderr, "shll changelog: %v\n", err)
		return errSilent
	}

	// No args → the whole installed roster, each installed → latest, WITH shll
	// itself included first (symmetry with bare `shll update`, which self-upgrades
	// shll; the intake's "shll first when included"). A bare sweep gracefully SKIPS
	// uninstalled tools; an explicitly-named no-range tool ERRORS when missing — so
	// bareness is tracked from the arg count, not inferred from the spec set.
	bare := len(specs) == 0
	if bare {
		specs = defaultChangelogSpecs()
	}

	// The no-range forms (bare sweep, or a `tool`-only spec) need brew to anchor
	// the installed→latest range, so brew must be present. Explicit `tool@old..new`
	// specs never consult brew, so a run made up ENTIRELY of explicit ranges skips
	// the precondition (mirroring how `shll install`/`update` gate on brew only
	// when they will actually read it).
	if specsNeedBrew(specs) && !hasBrew(ctx) {
		fmt.Fprintln(stderr, brewMissingHint)
		return errSilent
	}

	// Resolve every spec CONCURRENTLY (mirroring FetchAll / probeRoster): each
	// explicit spec fetches its range once; each no-range spec probes brew for the
	// installed anchor then fetches the repo's releases ONCE (LatestTag) and filters
	// locally — never a second FetchAll round-trip. Results stay in spec order
	// (roster order, shll first) by indexing.
	resolved := resolveChangelog(ctx, specs, bare)

	// Named-but-not-installed is an error only for no-range forms; collect all such
	// names (in spec/roster order) and report them at once before rendering.
	var missing []string
	for _, r := range resolved {
		if r.missing {
			missing = append(missing, r.name)
		}
	}
	if len(missing) > 0 {
		for _, name := range missing {
			fmt.Fprintf(stderr, "shll changelog: %s: not installed\n", name)
		}
		return errSilent
	}

	// A bare sweep with zero installed tools prints the same nothing-to-do line as
	// `shll update`, rather than silent empty output.
	rendered := make([]resolvedChangelog, 0, len(resolved))
	for _, r := range resolved {
		if r.skip {
			continue
		}
		rendered = append(rendered, r)
	}
	if bare && len(rendered) == 0 {
		fmt.Fprintln(stdout, noToolsInstalledMsg)
		return nil
	}

	color := colorEnabled(stdout)
	for i, r := range rendered {
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		printToolHeader(stdout, r.result.Tool, i+1, len(rendered), color)
		switch r.note {
		case noteUpToDate:
			// Already at latest → notice + releases URL (dash ASCII-degrades).
			fmt.Fprintf(stdout, "up to date at %s %s %s\n", r.version, dash(color), changelog.ReleasesURL(r.result.Repo))
		case noteUnavailable:
			// No-range latest fetch failed → explicit unavailable notice.
			fmt.Fprintf(stdout, "changelog unavailable %s %s\n", dash(color), changelog.ReleasesURL(r.result.Repo))
		default:
			renderChangelogResult(stdout, r.result, color)
		}
	}

	return nil
}

// defaultChangelogSpecs returns the no-range spec set for a bare-command run:
// shll itself FIRST (symmetry with bare `shll update`), then one spec per roster
// tool. Uninstalled tools are filtered out later during resolution (a bare run
// gracefully skips them — only an explicitly named no-range tool errors when
// missing).
func defaultChangelogSpecs() []changelogSpec {
	specs := make([]changelogSpec, 0, len(Roster)+1)
	specs = append(specs, changelogSpec{name: shllTargetToken, self: true, rosterIx: -1})
	for i, t := range Roster {
		specs = append(specs, changelogSpec{name: t.Name, rosterIx: i})
	}
	return specs
}

// specsNeedBrew reports whether any spec is a no-range form (which anchors its
// range on the brew-installed version). A run of only explicit `tool@old..new`
// specs needs no brew.
func specsNeedBrew(specs []changelogSpec) bool {
	for _, s := range specs {
		if !s.explicit {
			return true
		}
	}
	return false
}

// parseChangelogSpecs parses the positional args into changelogSpecs, validating
// tool names against the roster (plus shll) via the shared resolveTargets. It
// returns specs in roster order (shll first when named), deduped by tool name
// (last spec wins for a repeated name). An unknown name yields an error naming
// all unknowns and the valid targets, matching `update`'s diagnostic.
func parseChangelogSpecs(args []string) ([]changelogSpec, error) {
	if len(args) == 0 {
		return nil, nil
	}

	// Split each arg into name + optional range, and collect the bare names for
	// the shared validator (which reports all unknowns at once). A legacy alias
	// token (e.g. `rk`) is CANONICALIZED to its Roster name here so the roster-order
	// emit below finds it and the release fetch uses the canonical Repo — `shll
	// changelog rk` / `rk@old..new` work identically to `run-kit`. The alias is
	// resolved via the same legacyAliases map resolveTargets consults, so changelog
	// never carries bespoke alias logic (intake: in scope because name-matching
	// reuses the shared helper).
	byName := make(map[string]changelogSpec)
	names := make([]string, 0, len(args))
	for _, a := range args {
		name := a
		var explicit bool
		var old, new string
		if at := strings.Index(a, specToolSep); at >= 0 {
			name = a[:at]
			rng := a[at+len(specToolSep):]
			o, n, ok := splitRange(rng)
			if !ok {
				return nil, fmt.Errorf("invalid range %q (want tool@old..new)", a)
			}
			explicit, old, new = true, o, n
		}
		if canonical, ok := legacyAliases[name]; ok && rosterHas(canonical) {
			name = canonical
		}
		names = append(names, name)
		byName[name] = changelogSpec{name: name, explicit: explicit, old: old, new: new}
	}

	// Validate names via the shared resolver (allowShll=true). It reports all
	// unknowns at once and lists valid targets; we ignore its returned ordering
	// and re-derive order + ranges from byName below. Names are already
	// canonicalized above, so the resolver sees canonical tokens.
	if _, _, _, err := resolveTargets(names, true); err != nil {
		return nil, err
	}

	// Emit in roster order, shll first when named.
	out := make([]changelogSpec, 0, len(byName))
	if s, ok := byName[shllTargetToken]; ok {
		s.self = true
		s.rosterIx = -1
		out = append(out, s)
	}
	for i, t := range Roster {
		if s, ok := byName[t.Name]; ok {
			s.rosterIx = i
			out = append(out, s)
		}
	}
	return out, nil
}

// splitRange splits an `old..new` range body into its two versions. Returns
// ok=false when the separator is absent or either side is empty.
func splitRange(rng string) (old, new string, ok bool) {
	i := strings.Index(rng, specRangeSep)
	if i < 0 {
		return "", "", false
	}
	old = rng[:i]
	new = rng[i+len(specRangeSep):]
	if old == "" || new == "" {
		return "", "", false
	}
	return old, new, true
}

// noteKind classifies a no-range resolution that renders a one-line notice
// instead of a release range. The notice text is built at RENDER time (not
// resolution time) so its `—` glyph can ASCII-degrade with the stream's color
// decision (per-tool-output-separation spec).
type noteKind int

const (
	noteNone        noteKind = iota // render result as a range
	noteUpToDate                    // installed already at latest → "up to date at X"
	noteUnavailable                 // no-range latest fetch failed → "changelog unavailable"
)

// resolvedChangelog is one spec's resolution outcome, indexed to preserve spec
// (roster) order. Exactly one render signal applies:
//   - skip: a bare-sweep uninstalled tool → dropped silently.
//   - missing: an explicitly-named no-range tool not installed → the caller errors.
//   - note != noteNone: render the color-aware notice built from note (+ version).
//   - otherwise: render result (a fetched/filtered range, possibly Unavailable).
type resolvedChangelog struct {
	name    string
	skip    bool
	missing bool
	note    noteKind
	version string // the up-to-date "at X" version (noteUpToDate only)
	result  changelog.Result
}

// resolveChangelog resolves every spec CONCURRENTLY (one goroutine per spec,
// results indexed by position so spec/roster order is preserved) into a
// resolvedChangelog. Each explicit-range spec fetches its range once via
// FetchRange; each no-range spec probes brew for the installed anchor, then — when
// installed — fetches the repo's releases ONCE (LatestTag) and filters the
// returned list LOCALLY into (installed, latest], never a second network fetch.
// bare reports the implicit whole-roster sweep (uninstalled tools skip silently
// rather than erroring).
func resolveChangelog(ctx context.Context, specs []changelogSpec, bare bool) []resolvedChangelog {
	out := make([]resolvedChangelog, len(specs))
	var wg sync.WaitGroup
	for i, s := range specs {
		wg.Add(1)
		go func(i int, s changelogSpec) {
			defer wg.Done()
			out[i] = resolveOneSpec(ctx, s, bare)
		}(i, s)
	}
	wg.Wait()
	return out
}

// resolveOneSpec resolves a single spec (see resolveChangelog). It is the unit of
// the concurrent fan-out and makes at most one brew read + one GitHub fetch.
func resolveOneSpec(ctx context.Context, s changelogSpec, bare bool) resolvedChangelog {
	repo := repoForSpec(s)
	if s.explicit {
		// Explicit range: never consults brew; normalize the displayed bounds to a
		// single (v-stripped) form so a `tu@v0.6.2..v0.6.4` spec renders identically
		// to the unprefixed form and never echoes the user's raw `v` prefix.
		old, new := changelog.NormalizeVer(s.old), changelog.NormalizeVer(s.new)
		return resolvedChangelog{name: s.name, result: changelog.FetchRange(ctx, changelog.RangeReq{Tool: s.name, Repo: repo, Old: old, New: new})}
	}

	// No-range: installed → latest.
	installed := installedVersionForSpec(ctx, s)
	if installed == "" {
		// Not installed. Bare sweep → skip silently (graceful degradation);
		// explicitly named → error.
		if bare {
			return resolvedChangelog{name: s.name, skip: true}
		}
		return resolvedChangelog{name: s.name, missing: true}
	}
	installed = changelog.NormalizeVer(installed)

	latest, rels, err := changelog.LatestTag(ctx, repo)
	if err != nil {
		// Fetch failed for a no-range spec: the latest version is UNKNOWN, so there
		// is no genuine range to show. Emit an explicit "unavailable" notice pointing
		// at the releases page — NOT an `installed → installed — see <compareURL>`
		// self-compare (which would misread as "no change" and build a degenerate
		// compare link). result carries Tool/Repo for the header + notice URL.
		return resolvedChangelog{name: s.name, note: noteUnavailable, result: changelog.Result{Tool: s.name, Repo: repo}}
	}
	latest = changelog.NormalizeVer(latest)
	if latest == "" || changelog.CompareVer(latest, installed) <= 0 {
		// Already at (or ahead of) the latest release → up-to-date notice.
		return resolvedChangelog{
			name:    s.name,
			note:    noteUpToDate,
			version: installed,
			result:  changelog.Result{Tool: s.name, Repo: repo},
		}
	}
	// Filter the ALREADY-FETCHED releases locally into (installed, latest] — one GET
	// per repo, no second round-trip.
	return resolvedChangelog{name: s.name, result: changelog.Result{
		Tool: s.name, Repo: repo, Old: installed, New: latest,
		Releases: changelog.ReleasesInRange(rels, installed, latest),
	}}
}

// repoForSpec returns the GitHub repo slug for a spec (shll-self → shll; roster
// tool → its explicit Repo, which is not always the name — rk's is run-kit).
func repoForSpec(s changelogSpec) string {
	if s.self {
		return shllSelf.Repo
	}
	return Roster[s.rosterIx].Repo
}

// installedVersionForSpec returns the installed version for a no-range spec's
// tool from BREW: shll-self reads its brew-formula version (symmetric with the
// roster tools and with `shll update`'s shll-self anchor — NOT the running
// process's ldflags shllSelfVersion(), which reports the live binary, not the
// on-disk brew formula the changelog range should span). A roster tool reads brew
// via installedVersion. "" means not installed (or brew could not report it).
func installedVersionForSpec(ctx context.Context, s changelogSpec) string {
	if s.self {
		return installedVersion(ctx, shllFormula)
	}
	return installedVersion(ctx, Roster[s.rosterIx].Formula)
}

// renderChangelogResult renders one tool's full changelog BODY (the per-tool
// header is printed by the caller): either the unavailable/compare-URL fallback,
// the "no releases in range" line, or — via the shared renderReleases helper —
// each release (tag + title + full body) newest-first, capped at
// changelogCapPerTool with a cap notice + compare URL on overflow. res.Old/res.New
// are already normalized (v-stripped) by resolution, so both sides of the
// transition read in one form. The transition line is a navigational ANCHOR, so
// it is bold when color is enabled (bold, NOT bold-cyan — bold-cyan is reserved
// for the per-tool header). On a non-color stream the `→`/`—`/`…` glyphs
// ASCII-degrade to `->`/`--`/`...` (per-tool-output-separation spec) and no ANSI
// is emitted; the color decision is threaded in from runChangelog.
func renderChangelogResult(w io.Writer, res changelog.Result, color bool) {
	arr := arrow(color)
	if res.Unavailable {
		fmt.Fprintf(w, "%s %s %s %s see %s\n", res.Old, arr, res.New, dash(color), changelog.CompareURL(res.Repo, res.Old, res.New))
		return
	}

	n := len(res.Releases)
	fmt.Fprintln(w, bold(color, fmt.Sprintf("%s %s %s (%d release%s)", res.Old, arr, res.New, n, plural(n))))

	if n == 0 {
		fmt.Fprintln(w, "no releases in range")
		return
	}

	renderReleases(w, res, color)
}

// renderReleases renders the release BLOCKS of a fetched range — each release's
// `{tag}  {title}` line (a navigational ANCHOR, so bold when color is enabled)
// followed by the full body markdown (trailing newlines trimmed; an empty body is
// skipped), newest-first — capped at changelogCapPerTool with a
// `… {N-cap} more — full changelog: {compareURL}` notice on overflow. It is the
// single source of truth for release-block rendering, shared by
// renderChangelogResult (`shll changelog`) and printUpdateDigest (the update
// "What changed:" digest) so the two surfaces cannot drift (intake requirement).
// The caller has already emitted the surface-specific transition line (the
// changelog one, or the digest's tool-name-bearing one) and guaranteed
// len(res.Releases) > 0. Each release block is preceded by a leading blank line
// (preserved from the pre-extraction renderChangelogResult layout). On a
// non-color stream no ANSI is emitted; the color decision is threaded in.
func renderReleases(w io.Writer, res changelog.Result, color bool) {
	n := len(res.Releases)
	shown := res.Releases
	capped := false
	if n > changelogCapPerTool {
		shown = res.Releases[:changelogCapPerTool]
		capped = true
	}
	for _, r := range shown {
		fmt.Fprintf(w, "\n%s\n", bold(color, fmt.Sprintf("%s  %s", r.Tag, r.Title)))
		body := strings.TrimRight(r.Body, "\n")
		if body != "" {
			fmt.Fprintln(w, body)
		}
	}
	if capped {
		fmt.Fprintf(w, "\n%s %d more %s full changelog: %s\n", more(color), n-changelogCapPerTool, dash(color), changelog.CompareURL(res.Repo, res.Old, res.New))
	}
}

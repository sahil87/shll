package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/changelog"
	"github.com/sahil87/shll/internal/versions"
)

// The backend selector flag for `shll check-updates`. One enum-valued flag —
// valid values are the envelope source constants (sourceReleased/sourceGithub)
// below, so flag name, flag value, and the --json envelope's `source` field
// share one vocabulary. The github backend is deliberately named github, NOT
// homebrew: its source is GitHub releases, not brew. Named constants per
// code-quality.md (no magic strings).
const (
	sourceFlag      = "source"
	sourceFlagUsage = "update-check backend: released (shll.ai versions manifest + notify policy; the default) or github (release tags, no notify policy)"
)

// checkUpdatesJSONFlagUsage is the --json usage string for `shll check-updates`
// (the flag NAME is the shared jsonFlag constant from list.go).
const checkUpdatesJSONFlagUsage = "emit the machine contract as JSON (for scripting; run-kit's consumer surface)"

// checkUpdatesSchema is the version tag of the `--json` machine contract's
// envelope. Evolution is additive-only (consumers tolerate unknown fields), so
// this bumps only on a breaking shape change.
const checkUpdatesSchema = 1

// The `source` values emitted in the --json envelope, naming which backend
// produced the data — doubling as the --source flag's valid enum values (one
// vocabulary, zero new value constants). Named constants per code-quality.md.
const (
	sourceReleased = "released"
	sourceGithub   = "github"
)

// invalidSourceErrFmt is the usage-error diagnostic for an unknown --source
// value: names the offending value and the valid set. Exits usageExitCode (2)
// via errExitCode.
const invalidSourceErrFmt = "shll check-updates: invalid --source value %q (valid: %s, %s)"

// Human-output status labels (the third table column). notInstalledLabel
// (version.go) is reused for the not-installed rows. Named constants per
// code-quality.md.
const (
	checkStatusUpToDate      = "up to date"
	checkStatusUpdate        = "update available"
	checkStatusNotableSuffix = " (notable)"
	checkStatusUnavailable   = "unavailable"     // github backend: per-tool fetch failed
	checkStatusNotInManifest = "not in manifest" // released backend: name absent from the manifest
)

// checkUpdateItem is one row of the `--json` machine contract. Field rules:
//
//   - A row is emitted only when BOTH installed and latest resolve (the
//     unresolvable-row rule): a tool that is not installed, missing from the
//     manifest (released backend), or whose fetch failed (github backend) is
//     omitted from tools[] — an absent row never matches for consumers. Human
//     output still reports those tools (Constitution V), so nothing is hidden
//     from humans.
//   - Notify/Notable are present on released rows (the manifest is the policy
//     authority) and omitted entirely on github rows (no policy source exists
//     there — honest omission over invented defaults). Notable is a *bool so
//     the released form emits an explicit "notable": false while the github
//     form omits the key — a plain bool with omitempty would wrongly drop the
//     false value everywhere. Notify stays string,omitempty BY DESIGN: in the
//     edge case of an empty manifest notify value the key is omitted from the
//     released row too (honest omission again — "" is not a policy) while
//     notable stays present, computed treating empty/unknown as minor
//     (versions.Notable). Consumers distinguish backends via the envelope's
//     source field, never by per-row key presence.
//   - Versions are the normalized forms (v prefix + brew _N revision stripped)
//     so both sides share one comparable shape.
//
// Evolution rule (external contract): consumers tolerate unknown fields, so
// additions are safe; nothing is removed or renamed under schema 1.
type checkUpdateItem struct {
	Name            string `json:"name"`
	Formula         string `json:"formula"`
	Installed       string `json:"installed"`
	Latest          string `json:"latest"`
	Notify          string `json:"notify,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Notable         *bool  `json:"notable,omitempty"`
}

// checkUpdatesReport is the `--json` envelope: the contract schema tag, the
// backend that produced the data, and the resolved rows.
type checkUpdatesReport struct {
	Schema int               `json:"schema"`
	Source string            `json:"source"`
	Tools  []checkUpdateItem `json:"tools"`
}

// checkTarget is one tool the sweep covers: shll itself first (anchored on its
// brew formula, mirroring `shll changelog`'s bare-sweep precedent), then every
// Roster tool in roster order. formulaLeaf is the tap-relative formula
// name (`run-kit`, `shll`) emitted as the JSON `formula` field, matching the
// manifest's own formula values. A DELEGATED (non-brew) roster tool carries
// its Probe instead: brewFormula stays empty and its installed anchor comes
// from the probe spec (`rk desktop status`), never a brew read.
type checkTarget struct {
	name        string
	formulaLeaf string
	brewFormula string
	repo        string
	tool        Tool // the roster entry (zero for shll-self)
}

// checkUpdateRow is one target's resolution outcome. installed/latest are
// normalized versions; "" means unresolved. inManifest and fetchFailed
// distinguish the two backend-specific unresolved causes for human rendering.
type checkUpdateRow struct {
	name        string
	formulaLeaf string
	installed   string // "" = not installed (brew probe)
	latest      string // "" = unresolved (not in manifest / fetch failed)
	notify      string // released backend only: the manifest's raw notify value
	inManifest  bool   // released backend: name present in the manifest
	fetchFailed bool   // github backend: the per-tool releases fetch failed
}

func newCheckUpdatesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-updates",
		Short: "check which shll tools have updates available (read-only, never updates)",
		Long: `Check for pending shll toolkit updates — installed version vs latest available —
for shll itself plus every roster tool. Read-only: nothing is upgraded, installed,
or written. To apply updates, run ` + "`shll update`" + `.

One backend, selected by --source:

  --source released   latest versions + notify policy from https://shll.ai/versions.json
                      (the default when the flag is omitted)
  --source github     latest release tag per tool from the GitHub API (unauthenticated;
                      no notify policy in this backend)

  shll check-updates                          human table: installed → latest per tool
  shll check-updates --json                   machine contract (what run-kit's daemon runs)
  shll check-updates --source github          compare against GitHub release tags

Installed versions are read from Homebrew, so brew must be present. Exit codes:
0 when the check ran (whether or not updates are pending — verdicts live in the
output), 1 when the check itself failed (manifest unreachable, brew missing),
2 on a usage error. A github-backend per-tool fetch failure degrades that tool
only (omitted from --json, noted in the table) and the run still exits 0.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			source, _ := cmd.Flags().GetString(sourceFlag)
			jsonOut, _ := cmd.Flags().GetBool(jsonFlag)
			return runCheckUpdates(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), source, jsonOut)
		},
	}
	cmd.Flags().String(sourceFlag, sourceReleased, sourceFlagUsage)
	cmd.Flags().Bool(jsonFlag, false, checkUpdatesJSONFlagUsage)
	return cmd
}

// runCheckUpdates is the implementation seam for `shll check-updates`,
// extracted from the cobra factory so check_updates_test.go can drive it
// directly with bytes.Buffer writers, a fake proc.Runner, and the
// internal/versions + internal/changelog transport seams.
//
// Flow: validate the --source value (unknown → usage error, exit 2, before
// ANY brew or network access), gate on brew (the installed anchors are brew
// reads — brewMissingHint + errSilent when absent, exactly like changelog's
// no-range forms), fetch the manifest once for the released backend (its
// failure fails the whole check: one fetch, exit 1), resolve every target
// concurrently (brew probe + per-tool GitHub fetch on the github backend,
// indexed by position so output stays shll-first roster order), then render
// the human table or the --json machine contract. github-backend per-tool
// fetch failures degrade per-tool and never change the exit code
// (Constitution V — the changelog degradation precedent).
func runCheckUpdates(ctx context.Context, stdout, stderr io.Writer, source string, jsonOut bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if source != sourceReleased && source != sourceGithub {
		return &errExitCode{code: usageExitCode, msg: fmt.Sprintf(invalidSourceErrFmt, source, sourceReleased, sourceGithub)}
	}

	// The installed anchors are brew reads (shll-self included), so brew must be
	// present regardless of backend — mirroring changelog's no-range gate.
	if !hasBrew(ctx) {
		fmt.Fprintln(stderr, brewMissingHint)
		return errSilent
	}

	// Released backend: exactly one manifest GET per invocation (Constitution
	// II — no caching). It is the single latest+policy source, so its failure
	// fails the whole check (unlike the github backend's per-tool degradation).
	var manifest versions.Manifest
	if source == sourceReleased {
		m, err := versions.FetchManifest(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "shll check-updates: %v\n", err)
			return errSilent
		}
		manifest = m
	}

	rows := resolveCheckUpdates(ctx, checkUpdateTargets(), source, manifest)

	if jsonOut {
		return writeCheckUpdatesJSON(stdout, rows, source)
	}
	return writeCheckUpdatesTable(stdout, rows, source, colorEnabled(stdout))
}

// checkUpdateTargets returns the sweep set: shll itself FIRST (the unified
// shll-first ordering principle; installed anchor = its brew formula, never
// the running binary's ldflags version — the changelog bare-sweep precedent),
// then every Roster tool in roster order. shll is NOT added to Roster
// (Constitution III). A delegated (non-brew) roster tool carries no
// brewFormula/formulaLeaf — its installed anchor resolves through its Probe
// spec (resolveOneTarget) and the JSON `formula` field falls back to its name.
func checkUpdateTargets() []checkTarget {
	targets := make([]checkTarget, 0, len(Roster)+1)
	targets = append(targets, checkTarget{
		name:        shllSelf.Name,
		formulaLeaf: strings.TrimPrefix(shllFormula, formulaPrefix),
		brewFormula: shllFormula,
		repo:        shllSelf.Repo,
	})
	for _, t := range Roster {
		tgt := checkTarget{name: t.Name, repo: t.Repo, tool: t}
		if t.brewManaged() {
			tgt.formulaLeaf = strings.TrimPrefix(t.Formula, formulaPrefix)
			tgt.brewFormula = t.Formula
		} else {
			// No formula to name — the name is the identifier (never a bare
			// `brew install rk-desktop` hint; it is not a formula).
			tgt.formulaLeaf = t.Name
		}
		targets = append(targets, tgt)
	}
	return targets
}

// resolveCheckUpdates resolves every target CONCURRENTLY (one goroutine per
// target, results indexed by position so output order stays shll-first roster
// order — the resolveChangelog/probeInstalled pattern). Each goroutine makes
// one brew read (the installed anchor) plus, on the github backend, one
// GitHub releases fetch; the released backend looks its target up in the
// already-fetched manifest with no further network access.
func resolveCheckUpdates(ctx context.Context, targets []checkTarget, source string, manifest versions.Manifest) []checkUpdateRow {
	rows := make([]checkUpdateRow, len(targets))
	var wg sync.WaitGroup
	for i, tgt := range targets {
		wg.Add(1)
		go func(i int, tgt checkTarget) {
			defer wg.Done()
			rows[i] = resolveOneTarget(ctx, tgt, source, manifest)
		}(i, tgt)
	}
	wg.Wait()
	return rows
}

// resolveOneTarget resolves a single target (see resolveCheckUpdates): the
// installed anchor, then the backend's latest (+ notify on released).
// All versions are normalized (v prefix + brew _N revision stripped) so both
// sides share one comparable form. The anchor is a brew read for brew-managed
// tools (shll-self included — tool is the zero value there, brewManaged on a
// zero Tool is false, so shll-self takes the explicit brewFormula branch) and
// the delegated Probe spec for a non-brew tool.
func resolveOneTarget(ctx context.Context, tgt checkTarget, source string, manifest versions.Manifest) checkUpdateRow {
	row := checkUpdateRow{name: tgt.name, formulaLeaf: tgt.formulaLeaf}
	if tgt.tool.Probe != nil {
		_, v := probeToolInstalledVersion(ctx, tgt.tool)
		row.installed = changelog.NormalizeVer(v)
	} else {
		row.installed = changelog.NormalizeVer(installedVersion(ctx, tgt.brewFormula))
	}

	switch source {
	case sourceReleased:
		mt, ok := manifest.Tools[tgt.name]
		row.inManifest = ok
		if ok {
			row.latest = changelog.NormalizeVer(mt.Latest)
			row.notify = mt.Notify
		}
	case sourceGithub:
		latest, _, err := versions.LatestGitHub(ctx, tgt.repo)
		if err != nil {
			row.fetchFailed = true
			break
		}
		row.latest = changelog.NormalizeVer(latest)
	}
	return row
}

// rowResolved reports whether both sides of a row resolved — the JSON
// unresolvable-row rule's emit condition.
func rowResolved(r checkUpdateRow) bool {
	return r.installed != "" && r.latest != ""
}

// rowUpdateAvailable reports installed < latest via the toolkit's version
// compare (changelog.CompareVer). False for unresolved rows.
func rowUpdateAvailable(r checkUpdateRow) bool {
	return rowResolved(r) && changelog.CompareVer(r.latest, r.installed) > 0
}

// writeCheckUpdatesJSON emits the machine contract: the envelope with the
// contract schema, the producing backend, and one row per RESOLVED tool (the
// unresolvable-row rule — see checkUpdateItem). Encoding follows the
// list/doctor precedent: json.Encoder with SetEscapeHTML(false), 2-space
// indent, trailing newline. An empty resolved set emits "tools": [] — never
// null — so consumers can index unconditionally.
func writeCheckUpdatesJSON(w io.Writer, rows []checkUpdateRow, source string) error {
	items := make([]checkUpdateItem, 0, len(rows))
	for _, r := range rows {
		if !rowResolved(r) {
			continue
		}
		item := checkUpdateItem{
			Name:            r.name,
			Formula:         r.formulaLeaf,
			Installed:       r.installed,
			Latest:          r.latest,
			UpdateAvailable: rowUpdateAvailable(r),
		}
		if source == sourceReleased {
			// The manifest is the policy authority: echo its raw notify value and
			// emit an EXPLICIT notable (including false). github rows carry
			// neither key — no policy source exists in that backend.
			item.Notify = r.notify
			notable := versions.Notable(r.notify, r.installed, r.latest)
			item.Notable = &notable
		}
		items = append(items, item)
	}
	report := checkUpdatesReport{Schema: checkUpdatesSchema, Source: source, Tools: items}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(report); err != nil {
		return fmt.Errorf("shll check-updates: encode: %w", err)
	}
	return nil
}

// writeCheckUpdatesTable renders the human output: a column-aligned,
// self-labeling table in the `shll version` style (same tabwriter config) —
// name, version transition, status. Deliberately NO ▸/==> per-tool headers and
// NO summary tail: the per-tool-output-separation spec scopes those to
// commands that stream sub-tool output, and check-updates is a read-only
// self-labeling aggregation (the version precedent). The transition arrow
// ASCII-degrades on a non-TTY/NO_COLOR stream via the shared arrow helper.
// Unresolved rows still render (Constitution V — nothing hidden from humans):
// not installed, unavailable (github-backend fetch failure), not in manifest
// (released backend).
func writeCheckUpdatesTable(w io.Writer, rows []checkUpdateRow, source string, color bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, r := range rows {
		version, status := checkUpdateCells(r, source, color)
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.name, version, status)
	}
	return tw.Flush()
}

// checkUpdateCells derives one row's version and status cells. An
// update-available row shows the `installed → latest` transition (arrow
// degrades to `->` off-TTY) with the notable suffix when the pending bump
// crosses the tool's notify threshold (released backend only — github has no
// policy). Unresolved rows show what IS known: `not installed` in the version
// column (per the intake sketch), or the installed version with an
// unavailable / not-in-manifest note.
func checkUpdateCells(r checkUpdateRow, source string, color bool) (version, status string) {
	if r.installed == "" {
		return notInstalledLabel, ""
	}
	if r.latest == "" {
		if r.fetchFailed {
			return r.installed, checkStatusUnavailable
		}
		if source == sourceReleased && !r.inManifest {
			return r.installed, checkStatusNotInManifest
		}
		return r.installed, checkStatusUnavailable
	}
	if !rowUpdateAvailable(r) {
		return r.installed, checkStatusUpToDate
	}
	status = checkStatusUpdate
	if source == sourceReleased && versions.Notable(r.notify, r.installed, r.latest) {
		status += checkStatusNotableSuffix
	}
	return fmt.Sprintf("%s %s %s", r.installed, arrow(color), r.latest), status
}

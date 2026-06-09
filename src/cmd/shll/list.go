package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// jsonFlag is the bool flag on `shll list` that switches output from the
// human-readable aligned table to a plain JSON array (for scripting). Named
// constant per code-quality.md (no magic strings).
const jsonFlag = "json"

// jsonFlagUsage is the cobra usage string for the --json flag.
const jsonFlagUsage = "emit the roster as a JSON array (no color, for scripting)"

// Status markers for the default table. With color/TTY the markers are the
// Unicode glyphs (green ✓ installed / red ✗ missing); the plain branch uses
// ASCII tokens so non-TTY output and NO_COLOR stay escape-free and paste-safe,
// mirroring ui.go's glyph-vs-ASCII split. Named constants per code-quality.md.
const (
	statusGlyphInstalled = "✓"
	statusGlyphMissing   = "✗"
	statusASCIIInstalled = "ok"
	statusASCIIMissing   = "--"
)

// listItem is one roster row as emitted by `shll list --json`. Field names are a
// lightweight, stable contract: name, description, repo (the FULL resolved URL,
// not the bare slug, so consumers don't re-derive it and it matches the table
// column), and installed (the version-style PATH probe result).
type listItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Repo        string `json:"repo"`
	Installed   bool   `json:"installed"`
}

func newListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "list the sahil87 tools shll manages, with install status and repo links",
		Long: `List the sahil87 toolkit roster shll manages — one row per tool with an
install-status indicator, a one-line description, and its GitHub repo URL.

Install status reuses the same PATH probe as ` + "`shll version`" + ` (` + "`<tool> --version`" + `,
any error means missing) — install-mechanism agnostic, not a Homebrew check. A
missing tool is shown as missing, never an error: ` + "`shll list`" + ` always exits 0.

Default output is a column-aligned table (with color when writing to a terminal).
Pass --json for a plain JSON array suitable for scripting (` + "`shll list --json | jq`" + `).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			jsonOut, _ := cmd.Flags().GetBool(jsonFlag)
			return runList(cmd.Context(), cmd.OutOrStdout(), jsonOut)
		},
	}
	cmd.Flags().Bool(jsonFlag, false, jsonFlagUsage)
	return cmd
}

// runList is the implementation seam for `shll list`. Extracted from the cobra
// factory so list_test.go can drive it directly with a bytes.Buffer and a fake
// proc.Runner. It probes the whole roster's install status (concurrently),
// then renders either the aligned table (default) or a plain JSON array
// (jsonOut). A missing tool is reported as missing, never an error, and runList
// returns nil regardless of install status (Constitution V — Graceful
// Degradation). No `shll` self-row: list enumerates the managed sub-tools; shll
// itself is the manager, not a managed tool.
func runList(ctx context.Context, stdout io.Writer, jsonOut bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	installed := probeInstalled(ctx)
	if jsonOut {
		return writeListJSON(stdout, installed)
	}
	return writeListTable(stdout, installed)
}

// probeInstalled runs the version-style install probe for every roster tool
// concurrently and returns the results indexed by roster position (mirroring
// update.go's probeRoster/sync.WaitGroup pattern). Indexing by position keeps
// output deterministically in roster order regardless of probe completion
// order. Every subprocess call still routes through internal/proc (Constitution
// I) — only the dispatch is concurrent.
func probeInstalled(ctx context.Context) []bool {
	results := make([]bool, len(Roster))
	var wg sync.WaitGroup
	for i, t := range Roster {
		wg.Add(1)
		go func(i int, t Tool) {
			defer wg.Done()
			results[i] = toolInstalled(ctx, t)
		}(i, t)
	}
	wg.Wait()
	return results
}

// repoURL returns the full source-repo URL for a tool, built from the named
// githubOrgBase constant and the tool's explicit Repo slug (which is not always
// equal to Name — rk's repo is run-kit). Single place the URL is composed, so
// the table column and the JSON repo field never drift.
func repoURL(t Tool) string {
	return githubOrgBase + t.Repo
}

// writeListTable renders the default aligned table to w: one row per roster tool
// in roster order, columns status · name · description · repo-URL, aligned via
// text/tabwriter using the same writer config as `shll version` (minwidth 0,
// tabwidth 0, padding 2, padchar space). The status indicator uses color glyphs
// only when colorEnabled(w); otherwise plain-ASCII markers with no ANSI.
func writeListTable(w io.Writer, installed []bool) error {
	color := colorEnabled(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for i, t := range Roster {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", statusMarker(installed[i], color), t.Name, t.Description, repoURL(t))
	}
	return tw.Flush()
}

// statusMarker returns the install-status cell for a tool. With color it is the
// green ✓ / red ✗ glyph (ANSI-wrapped); plain it is the ASCII ok / -- token. The
// color decision is passed in (computed once by the caller) so this is trivially
// testable and the non-TTY/NO_COLOR path is guaranteed escape-free.
func statusMarker(installed, color bool) string {
	if color {
		if installed {
			return ansiGreen + statusGlyphInstalled + ansiReset
		}
		return ansiRed + statusGlyphMissing + ansiReset
	}
	if installed {
		return statusASCIIInstalled
	}
	return statusASCIIMissing
}

// writeListJSON emits the roster as a bare JSON array — one object per tool in
// roster order — 2-space-indented with a single trailing newline (the Encoder
// appends it) so it diffs cleanly and pipes into jq. Plain JSON only: no ANSI,
// no table framing, regardless of TTY. The repo field is the full resolved URL.
//
// HTML escaping is disabled (SetEscapeHTML(false)) so a description containing
// `&`, `<`, or `>` (e.g. fab-kit's "workspace & workflow toolkit") serializes as
// the literal character rather than `&` — keeping the --json output byte-for-
// byte legible and matching what the default table column shows. The output stays
// valid JSON either way; this is purely about the human-readable scripting form.
func writeListJSON(w io.Writer, installed []bool) error {
	items := make([]listItem, len(Roster))
	for i, t := range Roster {
		items[i] = listItem{
			Name:        t.Name,
			Description: t.Description,
			Repo:        repoURL(t),
			Installed:   installed[i],
		}
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(items); err != nil {
		return fmt.Errorf("shll list: encode: %w", err)
	}
	return nil
}

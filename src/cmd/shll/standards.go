package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

//go:generate ../../../scripts/sync-standards.sh

// standardsFS holds the canonical docs/site standards, copied into this package
// dir by scripts/sync-standards.sh and embedded at build time. The Go module
// root is src/ and docs/site/ sits above it, so //go:embed cannot reach the
// canonical files directly — the sync step copies them here first (see
// scripts/sync-standards.sh). The committed copies are what a clean
// `go build ./...` compiles; TestStandardsEmbedMatchesCanonical keeps them
// byte-honest against docs/site/ on every `go test`.
//
//go:embed standards/*.md
var standardsFS embed.FS

// standardsEmbedDir is the directory prefix under which standards documents are
// embedded (matching the //go:embed pattern above). Named constant so the
// embed-path composition is single-sourced (no magic strings, code-quality.md).
const standardsEmbedDir = "standards"

// standardsJSONFlag is the bool flag on the bare `shll standards` list form that
// switches output from the human-readable aligned table to a plain JSON array
// (mirroring `shll list --json`). Named constant per code-quality.md.
const standardsJSONFlag = "json"

// standardsJSONFlagUsage is the cobra usage string for the --json flag.
const standardsJSONFlagUsage = "emit the standards roster as a JSON array (no color, for scripting)"

// standard describes one entry in the hardcoded standards roster. It mirrors the
// tools.go Roster pattern: the roster is the source of truth for names,
// descriptions, and canonical source paths (Constitution: explicit versioned
// lists are the contract). Descriptions are hardcoded here, NOT parsed from the
// markdown at runtime.
type standard struct {
	// Name is the standard's identifier — the `<name>` argument to
	// `shll standards <name>` and the `name` field in --json output.
	Name string
	// Description is the one-line summary of what the standard governs and when
	// it applies, printed by the bare list form. It is the glossary contract: an
	// agent told only "run `shll standards`" must be able to pick the right
	// document from this line alone.
	Description string
	// Scope names where the standard's obligations live: `foundation` (the
	// principles every tool is built against), `binary` (satisfied by the
	// compiled tool at runtime), `repo` (satisfied by the repo's file
	// structure), or `binary+repo` (spans both). Printed as the middle column of
	// the bare list form and emitted as the `scope` field in --json. Named
	// vocabulary from the principles "The contracts" table, single-sourced here.
	Scope string
	// SourcePath is the repo-relative canonical path of the document (e.g.
	// `docs/site/standards/principles.md`), emitted as the `source_path` field in --json.
	// It is also the source the drift-guard test compares embedded bytes against.
	SourcePath string
	// EmbedName is the base filename of the embedded copy under standardsEmbedDir
	// (e.g. `principles.md`). The full embed path is standardsEmbedDir/EmbedName.
	EmbedName string
}

// standardsRoster is the hardcoded list of standards shll serves. Order is the
// output order for both the table and --json. Adding a standard means adding a
// row here AND its canonical docs/site file (synced in by scripts/sync-standards.sh)
// — an explicit, versioned list, exactly like the tool Roster.
var standardsRoster = []standard{
	{
		Name:        "principles",
		Description: "The ten toolkit CLI principles every tool is built against",
		Scope:       "foundation",
		SourcePath:  "docs/site/standards/principles.md",
		EmbedName:   "principles.md",
	},
	{
		Name:        "help-dump",
		Description: "Machine-readable help contract every tool must emit",
		Scope:       "binary",
		SourcePath:  "docs/site/standards/help-dump.md",
		EmbedName:   "help-dump.md",
	},
	{
		Name:        "readme-extraction",
		Description: "README + docs/site structure standard for toolkit repos",
		Scope:       "repo",
		SourcePath:  "docs/site/standards/readme-extraction.md",
		EmbedName:   "readme-extraction.md",
	},
	{
		Name:        "skill",
		Description: "Agent skill bundle standard: docs/site/skill.md served by `<tool> skill`",
		Scope:       "binary+repo",
		SourcePath:  "docs/site/standards/skill.md",
		EmbedName:   "skill.md",
	},
	{
		Name:        "update",
		Description: "In-place `update` upgrade contract: `--skip-brew-update` probe, exit codes, brew-handling safety",
		Scope:       "binary",
		SourcePath:  "docs/site/standards/update.md",
		EmbedName:   "update.md",
	},
	{
		Name:        "version",
		Description: "`--version` shape shll probes: 2s budget, first-line token, binary-name install probe",
		Scope:       "binary",
		SourcePath:  "docs/site/standards/version.md",
		EmbedName:   "version.md",
	},
	{
		Name:        "shell-init",
		Description: "Eval-safe `shell-init` output every shell-integration tool emits on stdout",
		Scope:       "binary",
		SourcePath:  "docs/site/standards/shell-init.md",
		EmbedName:   "shell-init.md",
	},
}

// standardJSONItem is one roster row as emitted by `shll standards --json`. Field
// names are a lightweight, stable contract mirroring `shll list --json`: name,
// description, scope, and source_path (the repo-relative canonical path, so a
// consumer can locate the doc in the shll repo without re-deriving it). The
// `scope` field is additive — new optional fields are allowed by the toolkit's
// format-stability rule (principle №2), so a consumer keyed on the older
// {name, description, source_path} shape never breaks on it.
type standardJSONItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
	SourcePath  string `json:"source_path"`
}

// newStandardsCmd builds the `shll standards` subcommand — the agent-facing
// reader for the toolkit's binding standards. Bare form lists every standard with
// a one-line scope description (the self-describing glossary); `shll standards
// <name>` prints the full canonical markdown to stdout. Content is embedded at
// build time from docs/site/, so output is offline and versioned with the release.
func newStandardsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "standards [name]",
		Short: "read the shll toolkit's binding standards (offline, embedded)",
		Long: `Read the shll toolkit's binding, producer-facing standards.

Bare ` + "`shll standards`" + ` lists every available standard with its scope and a
one-line description of what it governs and when it applies (name, scope, description) —
self-describing so an agent told only to "run shll standards" can pick the right document.
Pass --json for a machine-readable array of {name, description, scope, source_path} objects
(` + "`shll standards --json | jq`" + `).

` + "`shll standards <name>`" + ` prints the full markdown document to stdout, byte-identical
to its canonical docs/site/standards source. Raw markdown, no rendering, no pager — agents
consume it directly. An unknown name is an actionable error on stderr (exit non-zero).

The content is embedded into the binary at build time, so it is offline and versioned
with the release — when a canonical doc changes, the next shll release picks it up.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOut, _ := cmd.Flags().GetBool(standardsJSONFlag)
			return runStandards(cmd.OutOrStdout(), cmd.ErrOrStderr(), args, jsonOut)
		},
	}
	cmd.Flags().Bool(standardsJSONFlag, false, standardsJSONFlagUsage)
	return cmd
}

// runStandards is the implementation seam for `shll standards`, extracted from the
// cobra factory so standards_test.go can drive it with bytes.Buffers. With no
// positional arg it renders the roster list (table or --json); with one arg it
// prints that standard's embedded document. An unknown name writes an actionable
// diagnostic to stderr and returns errSilent (→ exit 1) with nothing on stdout.
func runStandards(stdout, stderr io.Writer, args []string, jsonOut bool) error {
	if len(args) == 0 {
		if jsonOut {
			return writeStandardsJSON(stdout)
		}
		return writeStandardsTable(stdout)
	}
	return writeStandardDoc(stdout, stderr, args[0])
}

// writeStandardsTable renders the roster as an aligned three-column table (name ·
// scope · description) in roster order, using the same text/tabwriter config as
// `shll list` / `shll version` (minwidth 0, tabwidth 0, padding 2, padchar
// space). No color glyphs and no status column — the standards list is a static
// glossary, not an install-status view — so the output is escape-free on every
// writer.
func writeStandardsTable(w io.Writer) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, s := range standardsRoster {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, s.Scope, s.Description)
	}
	return tw.Flush()
}

// writeStandardsJSON emits the roster as a bare JSON array — one
// {name, description, scope, source_path} object per standard in roster order,
// 2-space-indented with a single trailing newline (the Encoder appends it), so it
// diffs cleanly and pipes into jq. HTML escaping is disabled (SetEscapeHTML(false))
// so descriptions containing `&`/`<`/`>` serialize as the literal character,
// matching the table column — the same rationale as `shll list --json`.
func writeStandardsJSON(w io.Writer) error {
	items := make([]standardJSONItem, 0, len(standardsRoster))
	for _, s := range standardsRoster {
		items = append(items, standardJSONItem{
			Name:        s.Name,
			Description: s.Description,
			Scope:       s.Scope,
			SourcePath:  s.SourcePath,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(items); err != nil {
		return fmt.Errorf("shll standards: encode: %w", err)
	}
	return nil
}

// writeStandardDoc writes the full embedded markdown for the named standard to
// stdout, byte-identical to its canonical docs/site source. An unknown name is an
// actionable error: it names the valid standards on stderr, writes nothing to
// stdout, and returns errSilent so main.go's translateExit exits 1 without
// double-printing (principle №4 — fail fast with actionable errors).
func writeStandardDoc(stdout, stderr io.Writer, name string) error {
	s, ok := standardByName(name)
	if !ok {
		fmt.Fprintf(stderr, "shll standards: unknown standard %q (valid: %s)\n", name, validStandards())
		return errSilent
	}
	data, err := standardsFS.ReadFile(standardsEmbedDir + "/" + s.EmbedName)
	if err != nil {
		// A roster entry whose embed file is missing is a build-integrity bug
		// (the sync step / drift guard should have caught it), not user error.
		return fmt.Errorf("shll standards: read embedded %s: %w", s.EmbedName, err)
	}
	if _, err := stdout.Write(data); err != nil {
		return fmt.Errorf("shll standards: write: %w", err)
	}
	return nil
}

// standardByName returns the roster standard with the given name and true, or the
// zero standard and false when name is not a known standard. Source of truth is
// the live standardsRoster, so the valid-name list never drifts from it.
func standardByName(name string) (standard, bool) {
	for _, s := range standardsRoster {
		if s.Name == name {
			return s, true
		}
	}
	return standard{}, false
}

// validStandards returns the comma-separated list of valid standard names for the
// unknown-name diagnostic, derived from the live roster so it stays in sync. Uses
// strings.Join, matching tools.go's validTargets diagnostic-building idiom.
func validStandards() string {
	names := make([]string, len(standardsRoster))
	for i, s := range standardsRoster {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
)

// helpDumpTool is the literal `tool` field of the emitted document. Named
// constant rather than a magic string (code-quality.md).
const helpDumpTool = "shll"

// helpDumpSchemaVersion is the frozen schema version of the help/<tool>.json
// contract shared across the 7-tool rollout. Bump only on a breaking shape
// change to the JSON contract.
const helpDumpSchemaVersion = 1

// Cobra auto-generates `completion` and `help` subcommands; both are excluded
// from the dump (they are not part of shll's authored CLI surface).
const (
	cmdNameCompletion = "completion"
	cmdNameHelp       = "help"
)

// helpDoc is the top-level help-dump document. Field order and JSON tags mirror
// the frozen help/<tool>.json contract (reference: shll.ai's help/wt.json).
type helpDoc struct {
	Tool          string   `json:"tool"`
	Version       string   `json:"version"`
	SchemaVersion int      `json:"schema_version"`
	Root          helpNode `json:"root"`
}

// helpNode is one command in the recursive tree. Commands is always serialized
// as a (possibly empty) array, never null — see buildNode.
type helpNode struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Short    string     `json:"short"`
	Usage    string     `json:"usage"`
	Text     string     `json:"text"`
	Commands []helpNode `json:"commands"`
}

// newHelpDumpCmd builds the hidden `shll help-dump` subcommand. It is build
// tooling — Hidden keeps it off the user-facing help surface, and the Hidden
// flag also makes the command self-exclude from its own dump (see shouldSkip).
func newHelpDumpCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "help-dump",
		Short:  "emit the shll CLI help tree as JSON (build tooling)",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Anchor the walk to the actual root, not a captured variable, so
			// the dump is correct regardless of how the tree was assembled.
			return runHelpDump(cmd.Root(), cmd.OutOrStdout())
		},
	}
}

// runHelpDump walks the live cobra command tree rooted at root and writes the
// frozen JSON contract to w. The walk reads cobra's own data model (never
// regex-parses -h), so it cannot drift from the real CLI. Output is the JSON
// document only (2-space indent + a single trailing newline) — no diagnostics —
// so CI can redirect it straight into help/shll.json.
func runHelpDump(root *cobra.Command, w io.Writer) error {
	// Prune cobra's auto-generated completion/help (and any other skip-listed)
	// commands from the LIVE tree before walking. The real binary invokes
	// help-dump via rootCmd.Execute(), which lazily registers `completion` and
	// `help` BEFORE our RunE fires — so at walk time they exist as children of
	// root. They are correctly excluded from each node's `commands` array (via
	// shouldSkip), but nodeText renders cmd.UsageString(), whose "Available
	// Commands:" block reflects the live children. Without pruning, the root's
	// `text` would list completion/help while its `commands` array omits them —
	// internally incoherent and divergent from the frozen reference. Pruning
	// first keeps text ↔ commands coherent for every node. See Assumptions #1.
	pruneSkipped(root)

	doc := helpDoc{
		Tool:          helpDumpTool,
		Version:       root.Version,
		SchemaVersion: helpDumpSchemaVersion,
		Root:          buildNode(root),
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("shll help-dump: marshal: %w", err)
	}
	if _, err := w.Write(encoded); err != nil {
		return fmt.Errorf("shll help-dump: write: %w", err)
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return fmt.Errorf("shll help-dump: write: %w", err)
	}
	return nil
}

// pruneSkipped recursively removes skip-listed children (completion, help, and
// any Hidden/unavailable command — see shouldSkip) from the live tree rooted at
// cmd, so cmd.UsageString() (rendered into each node's `text`) lists exactly the
// commands that survive into the `commands` array.
//
// It first triggers cobra's lazy registration of `help` and `completion` so
// there is a command object to remove regardless of whether the binary has
// already run Execute() (which registers them). Both initializers no-op when the
// command already exists or the command has no subcommands, so calling them here
// is safe and idempotent.
func pruneSkipped(cmd *cobra.Command) {
	cmd.InitDefaultHelpCmd()
	cmd.InitDefaultCompletionCmd()

	var doomed []*cobra.Command
	for _, child := range cmd.Commands() {
		if shouldSkip(child) {
			doomed = append(doomed, child)
			continue
		}
		// Recurse only into survivors — skipped subtrees are removed wholesale.
		pruneSkipped(child)
	}
	cmd.RemoveCommand(doomed...)
}

// buildNode produces a Node for cmd and recurses into its visible children.
// Children is initialized non-nil so leaves serialize as `[]`, never `null`.
// Child order is whatever cobra's Commands() returns (its default alphabetical
// sort); the dump preserves it rather than re-sorting.
func buildNode(cmd *cobra.Command) helpNode {
	// Cobra adds the `-h`/`--help` (and root `-v`/`--version`) flags lazily, at
	// Execute time, via InitDefaultHelpFlag/InitDefaultVersionFlag. A bare tree
	// walk runs before Execute, so UsageString() would omit those flags and the
	// `[flags]` UseLine suffix — diverging from real `-h`. Initialize them here
	// so usage/text match the binary's actual help byte-for-byte.
	// (InitDefaultVersionFlag is a no-op unless cmd.Version != "".)
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()

	children := []helpNode{}
	for _, child := range cmd.Commands() {
		if shouldSkip(child) {
			continue
		}
		children = append(children, buildNode(child))
	}
	return helpNode{
		Name:     cmd.Name(),
		Path:     cmd.CommandPath(),
		Short:    cmd.Short,
		Usage:    cmd.UseLine(),
		Text:     nodeText(cmd),
		Commands: children,
	}
}

// shouldSkip reports whether a child command is excluded from the dump:
// cobra's auto-generated completion/help commands, any Hidden command (this
// self-excludes help-dump), and any unavailable/deprecated command.
func shouldSkip(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case cmdNameCompletion, cmdNameHelp:
		return true
	}
	return cmd.Hidden || !cmd.IsAvailableCommand()
}

// nodeText returns cmd's raw `-h` output byte-for-byte. It reproduces cobra's
// default help func (cobra v1.10.2 defaultHelpFunc): the right-trimmed Long
// blurb (falling back to Short), then a blank line, then UsageString(). When
// both Long and Short are empty, only UsageString() is emitted (the blurb and
// its trailing blank line are omitted entirely) — matching cobra. The
// help_dump_test byte-for-byte comparison against real `-h` enforces this.
func nodeText(cmd *cobra.Command) string {
	blurb := cmd.Long
	if blurb == "" {
		blurb = cmd.Short
	}
	blurb = strings.TrimRightFunc(blurb, unicode.IsSpace)
	usage := cmd.UsageString()
	if blurb == "" {
		return usage
	}
	return blurb + "\n\n" + usage
}

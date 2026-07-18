package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// dump runs runHelpDump against root and returns both the raw bytes and the
// decoded document. It fails the test on any error or malformed JSON.
func dump(t *testing.T, root *cobra.Command) ([]byte, helpDoc) {
	t.Helper()
	var buf bytes.Buffer
	if err := runHelpDump(root, &buf); err != nil {
		t.Fatalf("runHelpDump err = %v", err)
	}
	var doc helpDoc
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("dump output is not valid JSON: %v\noutput:\n%s", err, buf.String())
	}
	return buf.Bytes(), doc
}

// syntheticRoot builds a small tree: a root with one plain visible leaf, one
// aliased visible leaf, one hidden leaf, and stand-ins for cobra's
// auto-generated completion/help commands. The aliased leaf carries two aliases
// in a fixed order so the dump's declared-order emission is observable.
func syntheticRoot() *cobra.Command {
	root := &cobra.Command{Use: "shll", Short: "root short", Run: func(*cobra.Command, []string) {}}
	root.Version = "v0.0.0-test"
	root.AddCommand(&cobra.Command{Use: "visible", Short: "a visible leaf", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "aliased", Short: "a visible aliased leaf", Aliases: []string{"alias-one", "alias-two"}, Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "secret", Short: "a hidden leaf", Hidden: true, Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "completion", Short: "gen completions", Run: func(*cobra.Command, []string) {}})
	root.AddCommand(&cobra.Command{Use: "help", Short: "help about any command", Run: func(*cobra.Command, []string) {}})
	return root
}

// childByName returns the first child node of node with the given name, or a
// zero helpNode and false when absent.
func childByName(node helpNode, name string) (helpNode, bool) {
	for _, c := range node.Commands {
		if c.Name == name {
			return c, true
		}
	}
	return helpNode{}, false
}

func TestHelpDump_ContractShape(t *testing.T) {
	raw, doc := dump(t, syntheticRoot())

	if doc.Tool != helpDumpTool {
		t.Errorf("tool = %q, want %q", doc.Tool, helpDumpTool)
	}
	if doc.SchemaVersion != helpDumpSchemaVersion {
		t.Errorf("schema_version = %d, want %d", doc.SchemaVersion, helpDumpSchemaVersion)
	}
	if doc.Root.Name != "shll" {
		t.Errorf("root.name = %q, want %q", doc.Root.Name, "shll")
	}

	// Exactly the two visible children should survive filtering, in cobra's
	// (alphabetical) order: "aliased", then "visible".
	if len(doc.Root.Commands) != 2 {
		t.Fatalf("root.commands len = %d, want 2 (visible children only). commands: %+v", len(doc.Root.Commands), doc.Root.Commands)
	}
	if got := []string{doc.Root.Commands[0].Name, doc.Root.Commands[1].Name}; got[0] != "aliased" || got[1] != "visible" {
		t.Errorf("surviving children = %v, want [aliased visible]", got)
	}
	for _, excluded := range []string{"secret", "completion", "help"} {
		if bytes.Contains(raw, []byte(`"name": "`+excluded+`"`)) {
			t.Errorf("dump must not contain filtered child %q; output:\n%s", excluded, raw)
		}
	}

	// Optional `aliases` field: the aliased child carries its aliases in declared
	// order; the root and unaliased nodes carry NO `aliases` key at all (absence,
	// not `[]`/`null`).
	aliased, ok := childByName(doc.Root, "aliased")
	if !ok {
		t.Fatalf("aliased child missing from dump; commands: %+v", doc.Root.Commands)
	}
	if len(aliased.Aliases) != 2 || aliased.Aliases[0] != "alias-one" || aliased.Aliases[1] != "alias-two" {
		t.Errorf("aliased.aliases = %v, want [alias-one alias-two] (declared order)", aliased.Aliases)
	}
	// Decode into raw objects so we can assert on KEY PRESENCE (omitempty), which
	// a typed helpNode (always-present zero slice) cannot distinguish.
	var rawDoc struct {
		Root struct {
			Aliases  json.RawMessage `json:"aliases"`
			Commands []struct {
				Name    string          `json:"name"`
				Aliases json.RawMessage `json:"aliases"`
			} `json:"commands"`
		} `json:"root"`
	}
	if err := json.Unmarshal(raw, &rawDoc); err != nil {
		t.Fatalf("raw key-presence unmarshal err = %v; output:\n%s", err, raw)
	}
	if rawDoc.Root.Aliases != nil {
		t.Errorf("root must emit no `aliases` key (has no aliases); got %s", rawDoc.Root.Aliases)
	}
	for _, c := range rawDoc.Root.Commands {
		if c.Name == "visible" && c.Aliases != nil {
			t.Errorf("unaliased node %q must emit no `aliases` key; got %s", c.Name, c.Aliases)
		}
		if c.Name == "aliased" && c.Aliases == nil {
			t.Errorf("aliased node %q must emit an `aliases` key", c.Name)
		}
	}

	// captured_at was removed from the envelope (shll.ai stamps it on pull);
	// the key must be absent from the emitted document. Assert on the parsed
	// top-level object keys rather than a raw substring search, so a string
	// value that happened to contain "captured_at" can't mask a regression.
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(raw, &topLevel); err != nil {
		t.Fatalf("top-level unmarshal err = %v; output:\n%s", err, raw)
	}
	if _, ok := topLevel["captured_at"]; ok {
		t.Errorf("envelope must not contain captured_at (shll.ai-owned on pull); output:\n%s", raw)
	}
	wantKeys := map[string]bool{"tool": true, "version": true, "schema_version": true, "root": true}
	for k := range topLevel {
		if !wantKeys[k] {
			t.Errorf("unexpected top-level key %q; envelope must be {tool, version, schema_version, root}", k)
		}
	}

	// Leaf commands serialize as [], never null.
	if !bytes.Contains(raw, []byte(`"commands": []`)) {
		t.Errorf("expected at least one `\"commands\": []` (leaf), output:\n%s", raw)
	}
	if bytes.Contains(raw, []byte(`"commands": null`)) {
		t.Errorf("commands must never serialize as null, output:\n%s", raw)
	}
}

func TestHelpDump_TextByteForByte(t *testing.T) {
	root := newRootCmd()
	root.Version = "v1.2.3-test"
	_, doc := dump(t, root)

	// Walk the dumped tree and the live tree in lockstep. For each node assert
	// node.text equals the command's actual help-template output (cmd.Help()) —
	// the enforceable form of "RAW -h output, byte-for-byte". runHelpDump has
	// already initialized each command's -h/--version flags (buildNode calls
	// InitDefaultHelpFlag/InitDefaultVersionFlag), so cmd.Help() here renders
	// the `[flags]` UseLine + `Flags:` block exactly as the binary's `-h` does.
	//
	// Note: cmd.Help() (not a full Execute) is the reference because the dump is
	// an in-process tree walk — cobra's auto `completion`/`help` SUBcommands are
	// registered lazily only during Execute, so they appear neither in the
	// dumped `commands` arrays nor in any node's rendered `text`. This keeps a
	// parent's `Available Commands:` block consistent with its `commands` array
	// (see TestHelpDump_RootTextExcludesAutoCommands and Assumptions #1 in plan).
	var check func(node helpNode, cmd *cobra.Command)
	check = func(node helpNode, cmd *cobra.Command) {
		var help bytes.Buffer
		cmd.SetOut(&help)
		if err := cmd.Help(); err != nil {
			t.Fatalf("%s: Help() err = %v", node.Path, err)
		}
		if node.Text != help.String() {
			t.Errorf("text mismatch for %q:\n--- dump ---\n%q\n--- cmd.Help() ---\n%q", node.Path, node.Text, help.String())
		}
		byName := map[string]*cobra.Command{}
		for _, c := range cmd.Commands() {
			byName[c.Name()] = c
		}
		for _, childNode := range node.Commands {
			child, ok := byName[childNode.Name]
			if !ok {
				t.Fatalf("dumped child %q has no live counterpart under %q", childNode.Name, node.Path)
			}
			check(childNode, child)
		}
	}
	check(doc.Root, root)
}

// dumpViaExecute drives the dump through the REAL invocation path the shipped
// binary uses: rootCmd.Execute() with args ["help-dump"]. This matters because
// cobra lazily registers its auto-generated `completion` and `help` SUBcommands
// during Execute() — BEFORE the matched help-dump RunE fires. A test that calls
// runHelpDump(newRootCmd(), …) directly never registers them, so it cannot catch
// a regression where those commands leak into a node's rendered `text`. This
// helper reproduces the binary's tree exactly.
func dumpViaExecute(t *testing.T) ([]byte, helpDoc) {
	t.Helper()
	root := newRootCmd()
	root.Version = "v1.2.3-test"
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"help-dump"})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute(help-dump) err = %v\noutput:\n%s", err, buf.String())
	}
	var doc helpDoc
	if err := json.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("Execute dump output is not valid JSON: %v\noutput:\n%s", err, buf.String())
	}
	return buf.Bytes(), doc
}

// TestHelpDump_RootTextExcludesAutoCommands pins the deliberate decision that a
// parent node's `text` lists exactly its dumped children — cobra's lazily
// registered `completion`/`help` subcommands appear in NEITHER the `commands`
// array NOR the rendered `Available Commands:` text. This makes the tree
// internally consistent (text ↔ commands agree).
//
// It drives the dump through Execute() (via dumpViaExecute) so completion/help
// are registered exactly as on the shipped binary — the regression this test
// guards is invisible if the dump is invoked directly on newRootCmd().
func TestHelpDump_RootTextExcludesAutoCommands(t *testing.T) {
	_, doc := dumpViaExecute(t)
	available := availableCommandsBlock(doc.Root.Text)
	if available == "" {
		t.Fatalf("root.text has no Available Commands block:\n%s", doc.Root.Text)
	}
	for _, name := range []string{"completion", "help"} {
		if strings.Contains(available, name) {
			t.Errorf("Available Commands must not list auto command %q:\n%s", name, available)
		}
	}
	// Sanity: every dumped child IS listed in the Available Commands block.
	for _, child := range doc.Root.Commands {
		if !strings.Contains(available, child.Name) {
			t.Errorf("Available Commands should list dumped child %q:\n%s", child.Name, available)
		}
	}
}

// TestHelpDump_ExcludesAutoCommandsEverywhere is the regression guard for the
// text↔commands incoherence found in review: under the real Execute() path,
// cobra registers `completion`/`help` on the live tree, and the root's rendered
// `text` listed them even though the `commands` array excluded them. This test
// drives the dump through Execute() and asserts, for EVERY node, that
// completion/help appear in neither the `commands` array nor the rendered
// `Available Commands:` block. It FAILS against the pre-fix producer (which did
// not prune the live tree before rendering) and PASSES after the fix.
func TestHelpDump_ExcludesAutoCommandsEverywhere(t *testing.T) {
	_, doc := dumpViaExecute(t)

	var check func(node helpNode)
	check = func(node helpNode) {
		for _, name := range []string{"completion", "help"} {
			for _, child := range node.Commands {
				if child.Name == name {
					t.Errorf("node %q commands must not include auto command %q", node.Path, name)
				}
			}
			if block := availableCommandsBlock(node.Text); strings.Contains(block, name) {
				t.Errorf("node %q text Available Commands must not list auto command %q:\n%s", node.Path, name, block)
			}
		}
		for _, child := range node.Commands {
			check(child)
		}
	}
	check(doc.Root)
}

// TestHelpDump_EmitsAliasesRealTree pins the real-tree behavior: shll's one
// aliased command, `shell-setup` (alias `shell-install`, src/cmd/shll/shell_setup.go),
// carries exactly ["shell-install"] in the dumped node. It drives the dump
// through the real rootCmd.Execute() path (dumpViaExecute) per the
// prune-before-render design decision, so the assertion reflects the shipped
// binary's tree, not a bare walk.
func TestHelpDump_EmitsAliasesRealTree(t *testing.T) {
	_, doc := dumpViaExecute(t)

	node, ok := childByName(doc.Root, "shell-setup")
	if !ok {
		t.Fatalf("shell-setup node missing from real-tree dump; commands: %+v", doc.Root.Commands)
	}
	if len(node.Aliases) != 1 || node.Aliases[0] != "shell-install" {
		t.Errorf("shell-setup.aliases = %v, want [shell-install]", node.Aliases)
	}
}

// availableCommandsBlock returns the lines of the `Available Commands:` section
// of a rendered help text (up to the next blank line), or "" if absent.
func availableCommandsBlock(text string) string {
	const header = "Available Commands:"
	idx := strings.Index(text, header)
	if idx < 0 {
		return ""
	}
	rest := text[idx+len(header):]
	if end := strings.Index(rest, "\n\n"); end >= 0 {
		return rest[:end]
	}
	return rest
}

func TestHelpDump_SelfExclusion(t *testing.T) {
	root := newRootCmd()
	raw, doc := dump(t, root)

	if bytes.Contains(raw, []byte(`"name": "help-dump"`)) {
		t.Errorf("help-dump must self-exclude (it is Hidden); output:\n%s", raw)
	}
	for _, child := range doc.Root.Commands {
		if child.Name == "help-dump" {
			t.Errorf("help-dump present in dumped tree, want excluded")
		}
	}
}

func TestHelpDump_VersionPassthrough(t *testing.T) {
	root := newRootCmd()
	root.Version = "v9.9.9"
	_, doc := dump(t, root)
	if doc.Version != "v9.9.9" {
		t.Errorf("version = %q, want %q (must read root.Version, not hardcode)", doc.Version, "v9.9.9")
	}
}

func TestHelpDump_StructuralDeterminism(t *testing.T) {
	root := newRootCmd()
	root.Version = "v1.0.0"
	first, _ := dump(t, root)
	second, _ := dump(t, root)
	// The envelope carries no time-varying field, so two successive dumps of the
	// same tree are byte-identical.
	if !bytes.Equal(first, second) {
		t.Errorf("two successive dumps differ:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

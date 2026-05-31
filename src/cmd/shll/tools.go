package main

// formulaPrefix is the brew tap qualifier used for every roster formula. Named
// constant per code-quality.md (no magic strings).
const formulaPrefix = "sahil87/tap/"

// Tool describes one entry in the hardcoded sahil87 toolkit roster. The list is
// the source of truth for `shll update`, `shll shell-init`, and `shll version`
// (Constitution III — Tool Roster Source of Truth). Adding a new tool requires
// a shll release; no runtime discovery.
type Tool struct {
	// Name is the binary name (also used as the brew formula leaf and as the
	// label printed by `shll version`).
	Name string
	// Formula is the fully-qualified Homebrew formula name passed to brew.
	Formula string
	// ShellInit is the argv of the tool's shell-init invocation, with `<shell>`
	// substituted at composition time. An empty slice means the tool has no
	// shell integration — it is skipped during `shll shell-init`.
	//
	// Use the literal token `<shell>` to indicate where the user-supplied shell
	// name (zsh, bash) should be substituted at composition time. Every current
	// integrator (`tu`, `hop`, `wt`) takes a shell argument; if a future tool
	// shipped a no-arg shell-init, its argv would simply omit the placeholder.
	ShellInit []string
	// Update is the argv of the tool's own update invocation (e.g. `{"rk",
	// "update"}`). `shll update` delegates to this rather than calling `brew
	// upgrade <formula>` directly, so each tool's post-upgrade side effects
	// (e.g. rk's daemon restart) are preserved (Constitution IV — Composition).
	// An empty slice means the tool exposes no `update` subcommand — `shll
	// update` falls back to `brew upgrade <formula>` for it. Every current
	// roster tool ships an `update`, so all entries populate this field.
	Update []string
}

// shellPlaceholder is the literal substituted with the requested shell at
// composition time inside ShellInit argv. Named constant so callers do not
// open-code the string.
const shellPlaceholder = "<shell>"

// Roster is the hardcoded sahil87 toolkit list. Order matters — `shll
// shell-init` concatenates output in roster order so users can reason about
// init sequencing (spec: Composition Order requirement).
var Roster = []Tool{
	{Name: "fab-kit", Formula: formulaPrefix + "fab-kit", Update: []string{"fab-kit", "update"}},
	{Name: "rk", Formula: formulaPrefix + "rk", Update: []string{"rk", "update"}},
	{Name: "tu", Formula: formulaPrefix + "tu", ShellInit: []string{"tu", "shell-init", shellPlaceholder}, Update: []string{"tu", "update"}},
	{Name: "hop", Formula: formulaPrefix + "hop", ShellInit: []string{"hop", "shell-init", shellPlaceholder}, Update: []string{"hop", "update"}},
	{Name: "wt", Formula: formulaPrefix + "wt", ShellInit: []string{"wt", "shell-init", shellPlaceholder}, Update: []string{"wt", "update"}},
	{Name: "idea", Formula: formulaPrefix + "idea", Update: []string{"idea", "update"}},
}

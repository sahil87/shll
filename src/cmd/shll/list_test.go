package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

// listFake constructs a fakeRunner for `shll list`'s install probe. The probe is
// the same `<tool> --version` invocation `shll version` uses, so the fake keys
// off req.Name and the "--version" arg: a tool present in installed responds
// with success; an absent tool returns proc.ErrNotFound (mirroring exec.LookPath
// when the binary is missing from PATH).
func listFake(installed map[string]bool) *fakeRunner {
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		if len(req.Args) == 1 && req.Args[0] == "--version" {
			if installed[req.Name] {
				return proc.Result{Stdout: []byte(req.Name + " v0.1.0\n")}
			}
			return proc.Result{Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
}

// allInstalled returns an install map marking every roster tool installed.
func allInstalled() map[string]bool {
	m := make(map[string]bool, len(Roster))
	for _, t := range Roster {
		m[t.Name] = true
	}
	return m
}

func TestList_AllInstalled(t *testing.T) {
	installFakeRunner(t, listFake(allInstalled()))

	var stdout bytes.Buffer
	if err := runList(context.Background(), &stdout, false); err != nil {
		t.Fatalf("runList err = %v", err)
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != len(Roster) {
		t.Fatalf("line count = %d, want %d. output:\n%s", len(lines), len(Roster), stdout.String())
	}
	// Rows follow roster order; each carries the installed ASCII marker (non-TTY).
	for i, tool := range Roster {
		if !strings.Contains(lines[i], tool.Name) {
			t.Errorf("line %d = %q, want to contain %q", i, lines[i], tool.Name)
		}
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), statusASCIIInstalled) {
			t.Errorf("line %d = %q, want installed marker %q", i, lines[i], statusASCIIInstalled)
		}
	}
}

func TestList_SomeMissing(t *testing.T) {
	// Everything installed except rk.
	installed := allInstalled()
	installed["rk"] = false
	installFakeRunner(t, listFake(installed))

	var stdout bytes.Buffer
	if err := runList(context.Background(), &stdout, false); err != nil {
		t.Fatalf("runList err = %v (must never error on a missing tool)", err)
	}
	for _, line := range strings.Split(stdout.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		// The name is the second field (after the status marker).
		if len(fields) >= 2 && fields[1] == "rk" {
			if fields[0] != statusASCIIMissing {
				t.Fatalf("rk row = %q, want missing marker %q", line, statusASCIIMissing)
			}
		}
		if len(fields) >= 2 && fields[1] == "hop" {
			if fields[0] != statusASCIIInstalled {
				t.Fatalf("hop row = %q, want installed marker %q", line, statusASCIIInstalled)
			}
		}
	}
}

func TestList_RepoLinks(t *testing.T) {
	installFakeRunner(t, listFake(allInstalled()))

	var stdout bytes.Buffer
	if err := runList(context.Background(), &stdout, false); err != nil {
		t.Fatalf("runList err = %v", err)
	}
	out := stdout.String()
	for _, tool := range Roster {
		want := githubOrgBase + tool.Repo
		if !strings.Contains(out, want) {
			t.Errorf("output missing repo URL %q for %s. output:\n%s", want, tool.Name, out)
		}
	}
	// Regression guard for the rk/run-kit 404 footgun: rk MUST resolve to
	// .../run-kit, never .../rk.
	if !strings.Contains(out, githubOrgBase+"run-kit") {
		t.Errorf("rk row must resolve to %s. output:\n%s", githubOrgBase+"run-kit", out)
	}
	if strings.Contains(out, githubOrgBase+"rk") {
		t.Errorf("output must NOT contain the dead %s link. output:\n%s", githubOrgBase+"rk", out)
	}
}

func TestList_JSON(t *testing.T) {
	// rk missing, the rest installed, so `installed` is exercised in both states.
	installed := allInstalled()
	installed["rk"] = false
	installFakeRunner(t, listFake(installed))

	var stdout bytes.Buffer
	if err := runList(context.Background(), &stdout, true); err != nil {
		t.Fatalf("runList(json) err = %v", err)
	}
	out := stdout.String()

	// Trailing newline, no ANSI escapes.
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("JSON output must end with a trailing newline")
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("JSON output must contain no ANSI escapes, got:\n%s", out)
	}

	// HTML escaping is disabled: a description with `&`/`<`/`>` (fab-kit's
	// "workspace & workflow toolkit") must serialize as the literal character,
	// not the \uXXXX form — so raw --json bytes stay legible and match the table
	// column. Guards against a regression back to the default-escaping encoder.
	// htmlEscapedAmp is the 6-byte sequence the default encoder emits for `&`.
	const htmlEscapedAmp = "\\u0026"
	if strings.Contains(out, htmlEscapedAmp) {
		t.Errorf("JSON must not HTML-escape `&` to %s (want literal `&`), got:\n%s", htmlEscapedAmp, out)
	}
	if !strings.Contains(out, "workspace & workflow") {
		t.Errorf("JSON should contain the literal `&` from fab-kit's description, got:\n%s", out)
	}

	var items []listItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	if len(items) != len(Roster) {
		t.Fatalf("JSON array len = %d, want %d", len(items), len(Roster))
	}
	for i, tool := range Roster {
		got := items[i]
		if got.Name != tool.Name {
			t.Errorf("item %d name = %q, want %q (roster order)", i, got.Name, tool.Name)
		}
		if got.Description != tool.Description {
			t.Errorf("item %d description = %q, want %q", i, got.Description, tool.Description)
		}
		if got.Repo != githubOrgBase+tool.Repo {
			t.Errorf("item %d repo = %q, want full URL %q", i, got.Repo, githubOrgBase+tool.Repo)
		}
		wantInstalled := tool.Name != "rk"
		if got.Installed != wantInstalled {
			t.Errorf("item %d (%s) installed = %v, want %v", i, tool.Name, got.Installed, wantInstalled)
		}
	}
}

func TestList_NoANSI_Plain(t *testing.T) {
	installFakeRunner(t, listFake(allInstalled()))

	var stdout bytes.Buffer
	if err := runList(context.Background(), &stdout, false); err != nil {
		t.Fatalf("runList err = %v", err)
	}
	// A bytes.Buffer is never a TTY, so the ASCII status-marker path runs — no
	// ANSI escapes anywhere in the default output.
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("non-TTY output must contain no ANSI escape, got:\n%s", stdout.String())
	}
}

func TestList_Order(t *testing.T) {
	installFakeRunner(t, listFake(allInstalled()))

	// JSON path: index-paired to the live Roster, so a future reorder moves
	// expected and actual in lockstep (no edit needed).
	var stdout bytes.Buffer
	if err := runList(context.Background(), &stdout, true); err != nil {
		t.Fatalf("runList(json) err = %v", err)
	}
	var items []listItem
	if err := json.Unmarshal(stdout.Bytes(), &items); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(items) != len(Roster) {
		t.Fatalf("len = %d, want %d", len(items), len(Roster))
	}
	for i, tool := range Roster {
		if items[i].Name != tool.Name {
			t.Errorf("position %d = %q, want %q (Roster order)", i, items[i].Name, tool.Name)
		}
	}
}

func TestList_RosterFieldsNonEmpty(t *testing.T) {
	// Guard against adding a tool to Roster without filling the new fields:
	// every Description and every Repo must be non-empty.
	for _, tool := range Roster {
		if strings.TrimSpace(tool.Description) == "" {
			t.Errorf("tool %q has an empty Description", tool.Name)
		}
		if strings.TrimSpace(tool.Repo) == "" {
			t.Errorf("tool %q has an empty Repo", tool.Name)
		}
	}
}

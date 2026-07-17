package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runStandards is driven directly with bytes.Buffers (the testable seam extracted
// from the cobra factory, mirroring list_test.go's runList calls). No fake proc
// runner is needed — `shll standards` reads embedded bytes only, no subprocess.

// --- List form (T009) --------------------------------------------------------

func TestStandards_ListTable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runStandards(&stdout, &stderr, nil, false); err != nil {
		t.Fatalf("runStandards(list) err = %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("list form wrote to stderr: %q", stderr.String())
	}
	// A bytes.Buffer is never a TTY and the table carries no status glyphs, so the
	// output must be escape-free.
	if strings.Contains(stdout.String(), "\x1b[") {
		t.Errorf("list table must contain no ANSI escapes, got:\n%s", stdout.String())
	}
	lines := strings.Split(strings.TrimRight(stdout.String(), "\n"), "\n")
	if len(lines) != len(standardsRoster) {
		t.Fatalf("line count = %d, want %d (one row per standard). output:\n%s",
			len(lines), len(standardsRoster), stdout.String())
	}
	// Rows follow roster order; each row is name + description.
	for i, s := range standardsRoster {
		line := lines[i]
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != s.Name {
			t.Errorf("line %d = %q, want name %q as the first field", i, line, s.Name)
		}
		if !strings.Contains(line, s.Description) {
			t.Errorf("line %d = %q, want to contain description %q", i, line, s.Description)
		}
	}
}

func TestStandards_ListJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runStandards(&stdout, &stderr, nil, true); err != nil {
		t.Fatalf("runStandards(list json) err = %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("list --json wrote to stderr: %q", stderr.String())
	}
	out := stdout.String()

	// Trailing newline, no ANSI escapes (parity with `shll list --json`).
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("JSON output must end with a trailing newline, got:\n%q", out)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("JSON output must contain no ANSI escapes, got:\n%s", out)
	}

	var items []standardJSONItem
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput:\n%s", err, out)
	}
	if len(items) != len(standardsRoster) {
		t.Fatalf("JSON array len = %d, want %d", len(items), len(standardsRoster))
	}
	// Index-paired to the live roster, so a future reorder moves expected and
	// actual in lockstep. Assert every field incl. source_path.
	for i, s := range standardsRoster {
		got := items[i]
		if got.Name != s.Name {
			t.Errorf("item %d name = %q, want %q (roster order)", i, got.Name, s.Name)
		}
		if got.Description != s.Description {
			t.Errorf("item %d description = %q, want %q", i, got.Description, s.Description)
		}
		if got.SourcePath != s.SourcePath {
			t.Errorf("item %d source_path = %q, want %q", i, got.SourcePath, s.SourcePath)
		}
	}
}

func TestStandards_ListJSONFieldNames(t *testing.T) {
	// The raw JSON field names are a stable contract mirroring `shll list --json`.
	var stdout, stderr bytes.Buffer
	if err := runStandards(&stdout, &stderr, nil, true); err != nil {
		t.Fatalf("runStandards(list json) err = %v", err)
	}
	out := stdout.String()
	for _, key := range []string{`"name"`, `"description"`, `"source_path"`} {
		if !strings.Contains(out, key) {
			t.Errorf("JSON must carry the %s field, got:\n%s", key, out)
		}
	}
}

// --- Reader form (T010) ------------------------------------------------------

func TestStandards_ReadDoc_ByteIdentical(t *testing.T) {
	// For every roster standard, `shll standards <name>` stdout must equal the
	// embedded bytes byte-for-byte (raw markdown, no framing).
	for _, s := range standardsRoster {
		t.Run(s.Name, func(t *testing.T) {
			want, err := standardsFS.ReadFile(standardsEmbedDir + "/" + s.EmbedName)
			if err != nil {
				t.Fatalf("read embedded %s: %v", s.EmbedName, err)
			}
			var stdout, stderr bytes.Buffer
			if err := runStandards(&stdout, &stderr, []string{s.Name}, false); err != nil {
				t.Fatalf("runStandards(%q) err = %v", s.Name, err)
			}
			if stderr.Len() != 0 {
				t.Errorf("reading %q wrote to stderr: %q", s.Name, stderr.String())
			}
			if !bytes.Equal(stdout.Bytes(), want) {
				t.Errorf("stdout for %q is not byte-identical to the embedded document (len got=%d want=%d)",
					s.Name, stdout.Len(), len(want))
			}
		})
	}
}

func TestStandards_ReadDoc_JSONFlagIgnoredForReader(t *testing.T) {
	// --json is a list-form flag; with a name argument the reader path runs and
	// emits raw markdown regardless of the flag.
	s := standardsRoster[0]
	want, err := standardsFS.ReadFile(standardsEmbedDir + "/" + s.EmbedName)
	if err != nil {
		t.Fatalf("read embedded %s: %v", s.EmbedName, err)
	}
	var stdout, stderr bytes.Buffer
	if err := runStandards(&stdout, &stderr, []string{s.Name}, true); err != nil {
		t.Fatalf("runStandards(%q, json=true) err = %v", s.Name, err)
	}
	if !bytes.Equal(stdout.Bytes(), want) {
		t.Errorf("reader output changed under --json; want raw embedded markdown for %q", s.Name)
	}
}

func TestStandards_UnknownName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := runStandards(&stdout, &stderr, []string{"bogus"}, false)
	// Unknown name → errSilent (main.go translateExit maps it to exit 1), with the
	// diagnostic already on stderr.
	if !errors.Is(err, errSilent) {
		t.Fatalf("unknown name err = %v, want errSilent", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("unknown name must write nothing to stdout, got:\n%s", stdout.String())
	}
	diag := stderr.String()
	if !strings.Contains(diag, "bogus") {
		t.Errorf("stderr should name the offending input, got:\n%s", diag)
	}
	// The diagnostic must name every valid standard so an agent can recover.
	for _, s := range standardsRoster {
		if !strings.Contains(diag, s.Name) {
			t.Errorf("stderr must list valid standard %q, got:\n%s", s.Name, diag)
		}
	}
}

// --- Drift guard + roster integrity (T011) -----------------------------------

// TestStandardsEmbedMatchesCanonical is the drift guard: each embedded standard's
// bytes MUST equal its canonical docs/site source. The test file lives at
// src/cmd/shll/, so the canonical source is three levels up under docs/site/.
// Runs on every `go test ./...` and in the existing CI PR workflow — when a
// canonical doc drifts from the committed copy (someone edits docs/site/ without
// re-running scripts/sync-standards.sh), this fails, naming the drifted file.
func TestStandardsEmbedMatchesCanonical(t *testing.T) {
	for _, s := range standardsRoster {
		t.Run(s.Name, func(t *testing.T) {
			embedded, err := standardsFS.ReadFile(standardsEmbedDir + "/" + s.EmbedName)
			if err != nil {
				t.Fatalf("read embedded %s: %v", s.EmbedName, err)
			}
			// SourcePath is repo-relative (docs/site/<name>.md); the test runs from
			// src/cmd/shll/, so canonical = ../../../<SourcePath>.
			canonicalPath := filepath.Join("..", "..", "..", s.SourcePath)
			canonical, err := os.ReadFile(canonicalPath)
			if err != nil {
				t.Fatalf("read canonical %s: %v", canonicalPath, err)
			}
			if !bytes.Equal(embedded, canonical) {
				t.Errorf("embedded %s has drifted from canonical %s — run `just sync-standards` (or scripts/sync-standards.sh) and commit the refreshed copy",
					s.EmbedName, s.SourcePath)
			}
		})
	}
}

func TestStandardsRosterIntegrity(t *testing.T) {
	// Guard against adding a standard with an empty/misshapen field.
	if len(standardsRoster) == 0 {
		t.Fatal("standardsRoster must not be empty")
	}
	seen := make(map[string]bool, len(standardsRoster))
	for _, s := range standardsRoster {
		if strings.TrimSpace(s.Name) == "" {
			t.Errorf("standard has an empty Name: %+v", s)
		}
		if seen[s.Name] {
			t.Errorf("duplicate standard name %q", s.Name)
		}
		seen[s.Name] = true
		if strings.TrimSpace(s.Description) == "" {
			t.Errorf("standard %q has an empty Description", s.Name)
		}
		if !strings.HasPrefix(s.SourcePath, "docs/site/") {
			t.Errorf("standard %q SourcePath = %q, want a docs/site/ path", s.Name, s.SourcePath)
		}
		if strings.TrimSpace(s.EmbedName) == "" {
			t.Errorf("standard %q has an empty EmbedName", s.Name)
		}
		// SourcePath's basename must equal EmbedName so JSON's source_path and the
		// embedded copy refer to the same document.
		if base := filepath.Base(s.SourcePath); base != s.EmbedName {
			t.Errorf("standard %q: SourcePath base %q != EmbedName %q", s.Name, base, s.EmbedName)
		}
	}
}

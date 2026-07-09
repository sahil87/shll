package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

// --- brewTrustAvailable -------------------------------------------------------

func TestBrewTrustAvailable_True(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--help" {
			return proc.Result{Stdout: []byte("Usage: brew trust --formula <formula>\n")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	if !brewTrustAvailable(context.Background()) {
		t.Fatal("brewTrustAvailable = false, want true when `brew trust --help` is recognized")
	}
}

func TestBrewTrustAvailable_UnrecognizedSubcommand(t *testing.T) {
	// Older brew: `trust` is unknown, so brew exits non-zero → captured error.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: errors.New("Error: Unknown command: trust")}
	}}
	installFakeRunner(t, f)
	if brewTrustAvailable(context.Background()) {
		t.Fatal("brewTrustAvailable = true, want false when `trust` is unrecognized")
	}
}

func TestBrewTrustAvailable_BrewMissing(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: proc.ErrNotFound}
	}}
	installFakeRunner(t, f)
	if brewTrustAvailable(context.Background()) {
		t.Fatal("brewTrustAvailable = true, want false when brew is absent (ErrNotFound)")
	}
}

// --- brewTrustFormula ---------------------------------------------------------

func TestBrewTrustFormula_BuildsFormulaArg(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{ExitCode: 0}
	}}
	installFakeRunner(t, f)
	formula := formulaPrefix + "hop"
	code, err := brewTrustFormula(context.Background(), formula)
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, brewBinary, "trust", "--formula", formula) {
		t.Fatalf("calls = %+v, want `brew trust --formula %s`", calls, formula)
	}
	// Guard the per-formula-vs-tap distinction: the ceremony must use --formula
	// with a fully-qualified formula reference, NEVER --tap.
	for _, c := range calls {
		for _, a := range c.Args {
			if a == "--tap" || a == "--taps" {
				t.Fatalf("ceremony used a whole-tap flag %q; want per-formula --formula", a)
			}
		}
	}
}

func TestBrewTrustFormula_SurfacesNonZeroExit(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{ExitCode: 1}
	}}
	installFakeRunner(t, f)
	code, err := brewTrustFormula(context.Background(), formulaPrefix+"hop")
	if err != nil {
		t.Fatalf("err = %v, want nil (non-zero exit is reported via code)", err)
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
}

func TestBrewTrustFormula_SurfacesError(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{ExitCode: -1, Err: proc.ErrNotFound}
	}}
	installFakeRunner(t, f)
	code, err := brewTrustFormula(context.Background(), formulaPrefix+"hop")
	if err == nil {
		t.Fatal("err = nil, want non-nil for transport failure")
	}
	if code != -1 {
		t.Fatalf("code = %d, want -1 on transport error", code)
	}
}

// --- brewTrustList ------------------------------------------------------------

func TestBrewTrustList_ParsesTapsAndFormulae(t *testing.T) {
	jsonOut := `{
  "taps": ["sahil87/tap"],
  "formulae": ["sahil87/tap/hop", "sahil87/tap/wt"],
  "casks": [],
  "commands": []
}`
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--json=v1" {
			return proc.Result{Stdout: []byte(jsonOut)}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	taps, formulae, ok := brewTrustList(context.Background())
	if !ok {
		t.Fatal("ok = false, want true on a clean JSON response")
	}
	if len(taps) != 1 || taps[0] != tapName {
		t.Fatalf("taps = %v, want [%s]", taps, tapName)
	}
	want := map[string]bool{formulaPrefix + "hop": false, formulaPrefix + "wt": false}
	for _, fm := range formulae {
		if _, named := want[fm]; named {
			want[fm] = true
		}
	}
	for fm, seen := range want {
		if !seen {
			t.Errorf("formulae = %v, missing %s", formulae, fm)
		}
	}
}

func TestBrewTrustList_DegradesOnError(t *testing.T) {
	// brew present but `trust --json=v1` errors (e.g. unrecognized) → ok=false.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: errors.New("Error: Unknown command: trust")}
	}}
	installFakeRunner(t, f)
	if _, _, ok := brewTrustList(context.Background()); ok {
		t.Fatal("ok = true, want false when `brew trust --json=v1` errors")
	}
}

func TestBrewTrustList_DegradesOnBrewMissing(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: proc.ErrNotFound}
	}}
	installFakeRunner(t, f)
	if _, _, ok := brewTrustList(context.Background()); ok {
		t.Fatal("ok = true, want false when brew is absent")
	}
}

func TestBrewTrustList_DegradesOnGarbageJSON(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--json=v1" {
			return proc.Result{Stdout: []byte("not json at all")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	if _, _, ok := brewTrustList(context.Background()); ok {
		t.Fatal("ok = true, want false when the JSON cannot be decoded")
	}
}

// --- parseBrewLeaf / probeInstalledLeaf (rk→run-kit migration gate, change 9bak) -

func TestParseBrewLeaf(t *testing.T) {
	// The leaf is the FIRST whitespace field of the first non-empty line, matching
	// real `brew list --formula --versions` output (`<leaf> <version...>`).
	cases := map[string]string{
		"rk 2.5.13":            "rk",      // legacy keg
		"run-kit 3.0.0":        "run-kit", // migrated keg
		"run-kit 3.0.0 2.5.16": "run-kit", // multi-keg — leaf is still the first field
		"":                     "",        // not installed / empty
		"   ":                  "",        // whitespace-only
		"\nrun-kit 3.0.0\n":    "run-kit", // leading blank line skipped
	}
	for in, want := range cases {
		if got := parseBrewLeaf(in); got != want {
			t.Errorf("parseBrewLeaf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProbeInstalledLeaf_ReturnsLeafAndVersion(t *testing.T) {
	// State B is the load-bearing case: `brew list sahil87/tap/rk` EXITS 0 yet reports
	// leaf `run-kit` (rename resolution). probeInstalledLeaf must surface that leaf so
	// the migration gate can classify it migrated, not legacy.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" && req.Args[3] == formulaPrefix+"rk" {
			return proc.Result{Stdout: []byte("run-kit 3.0.0\n")}
		}
		return proc.Result{Err: errors.New("not installed")}
	}}
	installFakeRunner(t, f)

	installed, leaf, version := probeInstalledLeaf(context.Background(), formulaPrefix+"rk")
	if !installed {
		t.Fatal("installed = false, want true (state B: legacy formula probe exits 0)")
	}
	if leaf != "run-kit" {
		t.Errorf("leaf = %q, want run-kit (rename resolution — NOT the legacy leaf)", leaf)
	}
	if version != "3.0.0" {
		t.Errorf("version = %q, want 3.0.0", version)
	}

	// Not-installed → all zero values.
	installed, leaf, version = probeInstalledLeaf(context.Background(), formulaPrefix+"run-kit")
	if installed || leaf != "" || version != "" {
		t.Errorf("not-installed probe = (%v, %q, %q), want (false, \"\", \"\")", installed, leaf, version)
	}
}

// Sanity guard kept from the prior ceremony tests: tapName is the bare tap, no
// trailing slash (distinct from formulaPrefix). Still referenced by doctor's
// tap-level trust check and the trust-JSON parse.
func TestTapName_NoTrailingSlash(t *testing.T) {
	if tapName != "sahil87/tap" {
		t.Fatalf("tapName = %q, want \"sahil87/tap\" (no trailing slash)", tapName)
	}
	if strings.HasSuffix(tapName, "/") {
		t.Fatalf("tapName = %q must not carry a trailing slash (that is formulaPrefix's job)", tapName)
	}
}

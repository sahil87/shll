package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

// agentHomeEnv returns an env func whose $HOME points at a fresh t.TempDir(), so the
// skill-path derivation never touches the real ~. Returns the env func and the temp
// HOME so a test can assert against the placed files.
func agentHomeEnv(t *testing.T) (func(string) string, string) {
	t.Helper()
	home := t.TempDir()
	return envFunc(map[string]string{"HOME": home}), home
}

// runKitAbsentFake fails any run-kit / rk invocation with ErrNotFound (delegation
// skipped silently); everything else succeeds. Isolates the skill placement from the
// run-kit delegation.
func runKitAbsentFake() *fakeRunner {
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "run-kit" || req.Name == "rk" {
			return proc.Result{ExitCode: -1, Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
}

// skillPaths returns the two absolute SKILL.md paths agent-setup writes under home.
func skillPaths(home string) []string {
	return []string{
		filepath.Join(home, ".agents", "skills", skillDirName, skillFileName),
		filepath.Join(home, ".claude", "skills", skillDirName, skillFileName),
	}
}

// --- Install / placement (T004 / R6) -----------------------------------------

func TestAgentSetup_InstallPlacesBothSkills(t *testing.T) {
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false, false); err != nil {
		t.Fatalf("runAgentSetup err = %v", err)
	}
	for _, p := range skillPaths(home) {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("expected SKILL.md at %s: %v", p, err)
		}
		if string(data) != agentSkillContent {
			t.Errorf("placed content at %s is not the canonical skill:\n%s", p, data)
		}
	}
	// A per-path written summary is printed (first placement → "wrote").
	if c := strings.Count(stdout.String(), "wrote"); c != len(skillPaths(home)) {
		t.Errorf("expected a per-path 'wrote' summary line for each target, got:\n%s", stdout.String())
	}
}

func TestAgentSetup_Idempotent(t *testing.T) {
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())

	var o1, e1 bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &o1, &e1, false, false, false); err != nil {
		t.Fatalf("first run err = %v", err)
	}
	paths := skillPaths(home)
	before := make([][]byte, len(paths))
	for i, p := range paths {
		before[i], _ = os.ReadFile(p)
	}

	var o2, e2 bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &o2, &e2, false, false, false); err != nil {
		t.Fatalf("second run err = %v", err)
	}
	// Files are byte-identical after the re-run.
	for i, p := range paths {
		after, _ := os.ReadFile(p)
		if !bytes.Equal(before[i], after) {
			t.Errorf("re-run must be a byte-identical no-op at %s", p)
		}
	}
	// The second run reports every path as unchanged (no write performed).
	if c := strings.Count(o2.String(), "unchanged"); c != len(paths) {
		t.Errorf("re-run must report each path as 'unchanged', got:\n%s", o2.String())
	}
}

func TestAgentSetup_OverwritesDivergedContent(t *testing.T) {
	// A stale SKILL.md (wrong bytes) is overwritten and reported as "updated".
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())
	claudePath := filepath.Join(home, ".claude", "skills", skillDirName, skillFileName)
	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(claudePath, []byte("# stale\n"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false, false); err != nil {
		t.Fatalf("runAgentSetup err = %v", err)
	}
	data, _ := os.ReadFile(claudePath)
	if string(data) != agentSkillContent {
		t.Errorf("stale content must be overwritten with the canonical skill, got:\n%s", data)
	}
	if !strings.Contains(stdout.String(), "updated") {
		t.Errorf("a diverged existing file must be reported as 'updated', got:\n%s", stdout.String())
	}
}

// --- --print (T004 / R8) -----------------------------------------------------

func TestAgentSetup_Print(t *testing.T) {
	env, home := agentHomeEnv(t)
	f := runKitAbsentFake()
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, true /*print*/, false, false); err != nil {
		t.Fatalf("--print err = %v", err)
	}
	out := stdout.String()
	// The canonical content is printed verbatim …
	if !strings.HasPrefix(out, agentSkillContent) {
		t.Errorf("--print must lead with the canonical SKILL.md content, got:\n%s", out)
	}
	// … followed by both target paths.
	for _, p := range skillPaths(home) {
		if !strings.Contains(out, p) {
			t.Errorf("--print must list target path %s, got:\n%s", p, out)
		}
	}
	// No file is written.
	for _, p := range skillPaths(home) {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("--print must write nothing, but %s was created", p)
		}
	}
	// And no run-kit delegation is triggered.
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName {
			t.Errorf("--print must NOT delegate to run-kit, but recorded %+v", c)
		}
	}
}

// --- --uninstall (T004 / R8, R9) ---------------------------------------------

func TestAgentSetup_Uninstall(t *testing.T) {
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())

	// Place first, then uninstall.
	var o1, e1 bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &o1, &e1, false, false, false); err != nil {
		t.Fatalf("place err = %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, true /*uninstall*/, false); err != nil {
		t.Fatalf("--uninstall err = %v", err)
	}
	// Both skill DIRECTORIES are removed (not just the SKILL.md file).
	for _, p := range skillPaths(home) {
		dir := filepath.Dir(p)
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("--uninstall must remove the skill directory %s", dir)
		}
	}
}

func TestAgentSetup_PrintAndUninstallExit2(t *testing.T) {
	env, _ := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())

	var stdout, stderr bytes.Buffer
	err := runAgentSetup(context.Background(), env, &stdout, &stderr, true, true, false)
	var ec *errExitCode
	if !errors.As(err, &ec) {
		t.Fatalf("--print --uninstall err = %v, want *errExitCode", err)
	}
	if ec.code != usageExitCode {
		t.Errorf("exit code = %d, want %d (usage)", ec.code, usageExitCode)
	}
}

// --- run-kit delegation (T004 / R9) ------------------------------------------

func TestAgentSetup_DelegatesToRunKitWhenPresent(t *testing.T) {
	env, _ := agentHomeEnv(t)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{ExitCode: 0} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false, false); err != nil {
		t.Fatalf("run err = %v", err)
	}
	var delegated bool
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName && len(c.Args) == 2 && c.Args[0] == "agent" && c.Args[1] == "setup" {
			delegated = true
		}
	}
	if !delegated {
		t.Errorf("expected a `run-kit agent setup` delegation, calls: %+v", f.recordedCalls())
	}
}

func TestAgentSetup_RunKitAbsentSkipsSilently(t *testing.T) {
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false, false); err != nil {
		t.Fatalf("run-kit-absent run err = %v, want nil (delegation skipped silently)", err)
	}
	// The skills were still placed.
	for _, p := range skillPaths(home) {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("placement must succeed even when run-kit is absent: %s missing (%v)", p, err)
		}
	}
	// The absent-run-kit case must not surface a delegation error.
	if strings.Contains(stderr.String(), "run-kit agent setup:") {
		t.Errorf("run-kit absent must be a silent skip, but stderr carried a delegation error: %q", stderr.String())
	}
}

func TestAgentSetup_RunKitNonZeroExitWarnsAndContinues(t *testing.T) {
	env, home := agentHomeEnv(t)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == runKitToolName {
			return proc.Result{ExitCode: 3} // child ran and failed; RunForeground → (3, nil)
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false, false); err != nil {
		t.Fatalf("a failed run-kit delegation must not fail the placement, err = %v", err)
	}
	// Placement is the core work — both skills still land.
	for _, p := range skillPaths(home) {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("placement must succeed despite the delegation exit: %s missing (%v)", p, err)
		}
	}
	// The non-zero exit is surfaced as a warn-and-continue, not swallowed.
	if want := "run-kit agent setup exited 3 (continuing)"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
	}
}

func TestAgentSetup_UninstallDelegatesUninstall(t *testing.T) {
	env, _ := agentHomeEnv(t)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{ExitCode: 0} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, true /*uninstall*/, false); err != nil {
		t.Fatalf("run err = %v", err)
	}
	var delegatedUninstall bool
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName && len(c.Args) == 3 && c.Args[0] == "agent" && c.Args[1] == "setup" && c.Args[2] == "--uninstall" {
			delegatedUninstall = true
		}
	}
	if !delegatedUninstall {
		t.Errorf("expected a `run-kit agent setup --uninstall` delegation, calls: %+v", f.recordedCalls())
	}
}

// --- Canonical content / self-consistency (T004 / R7) ------------------------

func TestAgentSetup_ContentHasPortableFrontmatterOnly(t *testing.T) {
	lines := strings.Split(agentSkillContent, "\n")
	if lines[0] != "---" {
		t.Fatalf("SKILL.md must open with a frontmatter fence, got %q", lines[0])
	}
	// Collect the frontmatter keys (until the closing fence).
	var keys []string
	for _, ln := range lines[1:] {
		if ln == "---" {
			break
		}
		if i := strings.Index(ln, ":"); i > 0 && !strings.HasPrefix(ln, " ") {
			keys = append(keys, strings.TrimSpace(ln[:i]))
		}
	}
	// Portable subset: exactly `name` + `description`, nothing else.
	want := map[string]bool{"name": true, "description": true}
	if len(keys) != 2 {
		t.Fatalf("frontmatter must carry exactly name + description, got keys %v", keys)
	}
	for _, k := range keys {
		if !want[k] {
			t.Errorf("frontmatter key %q is not in the portable subset (name, description)", k)
		}
	}
}

func TestAgentSetup_NameMatchesDirAndPortableRegex(t *testing.T) {
	// The frontmatter `name:` must equal skillDirName …
	var name string
	for _, ln := range strings.Split(agentSkillContent, "\n") {
		if strings.HasPrefix(ln, "name:") {
			name = strings.TrimSpace(strings.TrimPrefix(ln, "name:"))
			break
		}
	}
	if name != skillDirName {
		t.Errorf("frontmatter name %q must equal the skill directory name %q", name, skillDirName)
	}
	// … and match the agentskills.io portable-name regex.
	re := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	if !re.MatchString(skillDirName) {
		t.Errorf("skill name %q must match ^[a-z0-9]+(-[a-z0-9]+)*$", skillDirName)
	}
}

func TestAgentSetup_BodyTeachesTwoStepAndStandards(t *testing.T) {
	if !strings.Contains(agentSkillContent, "shll skill") {
		t.Errorf("SKILL.md body must teach `shll skill`")
	}
	if !strings.Contains(agentSkillContent, "shll skill <tool>") {
		t.Errorf("SKILL.md body must teach the per-tool `shll skill <tool>` step")
	}
	if !strings.Contains(agentSkillContent, "shll skill <tool> <topic>") {
		t.Errorf("SKILL.md body must teach the topic form `shll skill <tool> <topic>`")
	}
	if !strings.Contains(agentSkillContent, "shll standards") {
		t.Errorf("SKILL.md body must carry the `shll standards` pointer")
	}
	// It must NOT reintroduce stanza/sentinel wording.
	if strings.Contains(agentSkillContent, "stanza") || strings.Contains(agentSkillContent, "sentinel") {
		t.Errorf("SKILL.md must not describe a stanza/sentinel mechanism, got:\n%s", agentSkillContent)
	}
}

// TestAgentSetup_DescriptionSingleLine pins the generated frontmatter description to
// the YAML-safe shape the builder promises: exactly one line, with no `: ` sequence
// (which would need quoting in an unquoted YAML scalar).
func TestAgentSetup_DescriptionSingleLine(t *testing.T) {
	desc := agentSkillDescription()
	if strings.Contains(desc, "\n") {
		t.Errorf("description must be a single line, got:\n%s", desc)
	}
	if strings.Contains(desc, ": ") {
		t.Errorf("description must not contain ': ' (unquoted YAML scalar), got: %s", desc)
	}
}

// TestRosterSkillHints enforces the SkillHint roster contract: every tool carries a
// non-empty task-domain phrase, and the generated description weaves each in as a
// `hint (name)` trigger clause (with the legacy alias appended for run-kit) — the
// task-vocabulary activation surface the skill description exists to provide.
func TestRosterSkillHints(t *testing.T) {
	desc := agentSkillDescription()
	for _, tool := range Roster {
		if tool.SkillHint == "" {
			t.Errorf("roster tool %q must declare a SkillHint", tool.Name)
			continue
		}
		name := tool.Name
		if tool.LegacyName != "" {
			name += "/" + tool.LegacyName
		}
		clause := tool.SkillHint + " (" + name + ")"
		if !strings.Contains(desc, clause) {
			t.Errorf("description missing clause %q, got: %s", clause, desc)
		}
	}
	// The description must still carry the two-step teaching pointer.
	if !strings.Contains(desc, "shll skill <tool>") {
		t.Errorf("description must keep the `shll skill <tool>` pointer, got: %s", desc)
	}
}

// TestRosterProactiveHint pins the ProactiveHint contract, which — unlike SkillHint —
// is optional-by-design: it is populated ONLY on run-kit (the sprawl guard) and rendered
// as additional sentence(s) AFTER the tool clauses and BEFORE the two-step pointer. It
// gets its own test rather than extending TestRosterSkillHints (which enforces an
// every-tool required field).
//
// Beyond the dynamic verbatim-containment check, it pins the hint's three load-bearing
// functions as fragment containment checks, so a future rewording cannot silently drop
// any of: (a) the proxy trigger vocabulary ("to proxy a local http port" — matches
// requests that name proxying/dev servers), (b) the skill-shadowing
// counter-instruction ("before opening any file or local port in a browser, read" —
// fires when a competing skill's local `open`/`xdg-open` delivery step is about to run,
// routing the agent to `shll skill run-kit` for the proxied-iframe recipe instead), and
// (c) the hosted-artifact counter-instruction ("publishing an artifact" — fires when an
// Artifact-style hosted-publishing delivery step, which opens no file and touches no
// local port, is about to route visuals off the run-kit dashboard).
func TestRosterProactiveHint(t *testing.T) {
	// Exactly run-kit carries a ProactiveHint; every other tool leaves it empty
	// (the sprawl guard — only agent-proactive capabilities earn description space).
	var withHint []string
	for _, tool := range Roster {
		if tool.ProactiveHint != "" {
			withHint = append(withHint, tool.Name)
		}
	}
	if len(withHint) != 1 || withHint[0] != "run-kit" {
		t.Fatalf("exactly run-kit must declare a ProactiveHint, got %v", withHint)
	}

	rk, ok := rosterTool("run-kit")
	if !ok {
		t.Fatal("run-kit must be in the roster")
	}
	desc := agentSkillDescription()

	// The run-kit ProactiveHint sentence(s) appear verbatim in the rendered description …
	if !strings.Contains(desc, rk.ProactiveHint) {
		t.Errorf("description must contain run-kit's ProactiveHint verbatim.\nhint: %q\ndesc: %s", rk.ProactiveHint, desc)
	}
	// … carrying all three load-bearing functions — the proxy trigger vocabulary, the
	// skill-shadowing counter-instruction, and the hosted-artifact counter-instruction —
	// pinned as independent fragments so a future rewording cannot silently drop any
	// (the verbatim check above is dynamic and would pass mechanically with any Roster
	// value).
	for _, fragment := range []string{
		"to proxy a local http port",                               // (a) proxy trigger vocabulary
		"before opening any file or local port in a browser, read", // (b) shadowing counter-instruction
		"publishing an artifact",                                   // (c) hosted-artifact counter-instruction
	} {
		if !strings.Contains(desc, fragment) {
			t.Errorf("description must contain the load-bearing fragment %q, got: %s", fragment, desc)
		}
	}
	// … positioned AFTER the tool clauses and BEFORE the two-step pointer. Anchor
	// the "after clauses" check on the END of the LAST rendered clause (not the start
	// of the "Use when driving" preamble): the hint must fall after every clause, so a
	// hint mistakenly emitted between the preamble and the clause list must still fail.
	last := Roster[len(Roster)-1]
	lastName := last.Name
	if last.LegacyName != "" {
		lastName += "/" + last.LegacyName
	}
	lastClause := last.SkillHint + " (" + lastName + ")"
	lastClauseIdx := strings.Index(desc, lastClause)
	hintIdx := strings.Index(desc, rk.ProactiveHint)
	pointerIdx := strings.Index(desc, "Run `shll skill`")
	if lastClauseIdx < 0 || pointerIdx < 0 || hintIdx < 0 {
		t.Fatalf("description missing an expected segment (lastClause=%d hint=%d pointer=%d): %s", lastClauseIdx, hintIdx, pointerIdx, desc)
	}
	clausesEnd := lastClauseIdx + len(lastClause)
	if !(clausesEnd <= hintIdx && hintIdx < pointerIdx) {
		t.Errorf("ProactiveHint must fall after the last tool clause and before the two-step pointer (clausesEnd=%d hint=%d pointer=%d): %s", clausesEnd, hintIdx, pointerIdx, desc)
	}
}

// TestAgentSetup_FlagsWiredThroughCobra drives the REAL cobra command so it catches a
// flag-binding regression: --print must reach runAgentSetup and print (not write). It
// runs against a t.TempDir() HOME via $HOME (os.Getenv in the factory).
func TestAgentSetup_FlagsWiredThroughCobra(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	installFakeRunner(t, runKitAbsentFake())

	cmd := newAgentSetupCmd()
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"--print"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent-setup --print err = %v (flag not wired?)", err)
	}
	if !strings.HasPrefix(out.String(), agentSkillContent) {
		t.Errorf("--print must print the canonical content, got:\n%s", out.String())
	}
	// --print writes nothing.
	for _, p := range skillPaths(home) {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("--print via cobra must write nothing, but %s was created", p)
		}
	}
}

// --- --yes forwarding to the run-kit delegation (R1/R2) -----------------------

func TestAgentSetup_YesForwardsToDelegation(t *testing.T) {
	env, _ := agentHomeEnv(t)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{ExitCode: 0} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false, true /*yes*/); err != nil {
		t.Fatalf("run err = %v", err)
	}
	var yesDelegated bool
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName && len(c.Args) == 3 && c.Args[0] == "agent" && c.Args[1] == "setup" && c.Args[2] == "--"+yesFlag {
			yesDelegated = true
		}
	}
	if !yesDelegated {
		t.Errorf("expected a `run-kit agent setup --yes` delegation, calls: %+v", f.recordedCalls())
	}
}

func TestAgentSetup_YesRidesUninstallDelegation(t *testing.T) {
	env, _ := agentHomeEnv(t)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{ExitCode: 0} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, true /*uninstall*/, true /*yes*/); err != nil {
		t.Fatalf("run err = %v", err)
	}
	var yesDelegated bool
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName && len(c.Args) == 4 && c.Args[0] == "agent" && c.Args[1] == "setup" && c.Args[2] == "--uninstall" && c.Args[3] == "--"+yesFlag {
			yesDelegated = true
		}
	}
	if !yesDelegated {
		t.Errorf("expected a `run-kit agent setup --uninstall --yes` delegation, calls: %+v", f.recordedCalls())
	}
}

func TestAgentSetup_PrintWithYesIsNoOp(t *testing.T) {
	env, home := agentHomeEnv(t)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{ExitCode: 0} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	// --yes with --print is deliberately NOT a usage error (unlike --print+--uninstall):
	// print never delegates, so there is no prompt for the flag to skip.
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, true /*print*/, false, true /*yes*/); err != nil {
		t.Fatalf("--print --yes must be a harmless no-op, err = %v", err)
	}
	if !strings.HasPrefix(stdout.String(), agentSkillContent) {
		t.Errorf("--print --yes must still print the canonical content, got:\n%s", stdout.String())
	}
	for _, p := range skillPaths(home) {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("--print --yes must write nothing, but %s was created", p)
		}
	}
	if len(f.recordedCalls()) != 0 {
		t.Errorf("--print --yes must not delegate, calls: %+v", f.recordedCalls())
	}
}

func TestAgentSetup_YesFlagWiredThroughCobra(t *testing.T) {
	cmd := newAgentSetupCmd()
	fl := cmd.Flags().Lookup(yesFlag)
	if fl == nil {
		t.Fatal("agent-setup must register a --yes flag")
	}
	if fl.Shorthand != yesFlagShorthand {
		t.Errorf("--yes shorthand = %q, want %q", fl.Shorthand, yesFlagShorthand)
	}
	if fl.Usage != agentSetupYesUsage {
		t.Errorf("--yes usage = %q, want the agent-setup-specific string %q", fl.Usage, agentSetupYesUsage)
	}

	// End-to-end through cobra Execute — the flag value must actually reach the
	// delegation argv (a registered-but-unbound flag would pass Lookup above and
	// still silently drop --yes; caught in PR #79 review).
	home := t.TempDir()
	t.Setenv("HOME", home)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{ExitCode: 0} }}
	installFakeRunner(t, f)

	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	cmd.SetArgs([]string{"--yes"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agent-setup --yes err = %v", err)
	}
	var yesDelegated bool
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName && len(c.Args) == 3 && c.Args[0] == "agent" && c.Args[1] == "setup" && c.Args[2] == "--"+yesFlag {
			yesDelegated = true
		}
	}
	if !yesDelegated {
		t.Errorf("cobra --yes must reach the delegation argv (`run-kit agent setup --yes`), calls: %+v", f.recordedCalls())
	}
}

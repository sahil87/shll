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

// skillPaths returns the two absolute SKILL.md candidate paths under home — the
// ALL-CANDIDATES view (both tiers). Which of them an install actually writes
// depends on the claude gate (forceClaudeGate); --uninstall and the staleness
// probe always cover both.
func skillPaths(home string) []string {
	return []string{
		filepath.Join(home, ".agents", "skills", skillDirName, skillFileName),
		filepath.Join(home, ".claude", "skills", skillDirName, skillFileName),
	}
}

// forceClaudeGate swaps the proc.LookPath seam for the duration of t so the
// ~/.claude/skills/ placement gate is deterministically open (present=true) or
// closed (present=false) without a real `claude` binary on PATH — the same
// package-level-variable injection style as installFakeRunner.
func forceClaudeGate(t *testing.T, present bool) {
	t.Helper()
	prev := proc.LookPath
	t.Cleanup(func() { proc.LookPath = prev })
	proc.LookPath = func(name string) bool {
		if name == claudeToolName {
			return present
		}
		return prev(name)
	}
}

// --- Install / placement (T004 / R6) -----------------------------------------

func TestAgentSetup_InstallPlacesBothSkills(t *testing.T) {
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())
	forceClaudeGate(t, true)

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
	forceClaudeGate(t, true)

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
	forceClaudeGate(t, true)
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

// --- the claude gate (two-tier placement) --------------------------------------

func TestAgentSetup_InstallGateClosedWritesAgentsOnly(t *testing.T) {
	// claude NOT on PATH → only the unconditional ~/.agents/skills/ target is
	// written, and no ~/.claude/ directory is created AT ALL (dir absence, not
	// just file absence) — a machine without Claude Code gets no litter.
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())
	forceClaudeGate(t, false)

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false, false); err != nil {
		t.Fatalf("runAgentSetup err = %v", err)
	}
	agentsPath := filepath.Join(home, ".agents", "skills", skillDirName, skillFileName)
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("expected SKILL.md at %s: %v", agentsPath, err)
	}
	if string(data) != agentSkillContent {
		t.Errorf("placed content at %s is not the canonical skill:\n%s", agentsPath, data)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("gate-closed install must create no ~/.claude/ directory at all, stat err = %v", err)
	}
	// The per-path summary covers exactly the one gated target — the skipped
	// surface is silent (Constitution V), no warning, no output noise.
	if c := strings.Count(stdout.String(), "wrote"); c != 1 {
		t.Errorf("expected exactly one 'wrote' line (the ~/.agents target), got:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), ".claude") {
		t.Errorf("a gated-off target must not appear in the summary, got:\n%s", stdout.String())
	}
}

func TestAgentSetup_GateNeverDeletes(t *testing.T) {
	// A pre-existing ~/.claude placement survives a gate-closed install run
	// untouched — the gate suppresses ALL writes to the gated surface while
	// closed (including refresh rewrites); only --uninstall deletes.
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())
	forceClaudeGate(t, false)
	claudePath := filepath.Join(home, ".claude", "skills", skillDirName, skillFileName)
	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stale := []byte("# stale\n")
	if err := os.WriteFile(claudePath, stale, 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false, false); err != nil {
		t.Fatalf("runAgentSetup err = %v", err)
	}
	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("the gate must never delete a pre-existing placement: %v", err)
	}
	if !bytes.Equal(data, stale) {
		t.Errorf("a gated-off pre-existing placement must be left byte-untouched, got:\n%s", data)
	}
}

// --- --print (T004 / R8) -----------------------------------------------------

func TestAgentSetup_Print(t *testing.T) {
	env, home := agentHomeEnv(t)
	f := runKitAbsentFake()
	installFakeRunner(t, f)
	forceClaudeGate(t, true)

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

func TestAgentSetup_PrintReflectsGate(t *testing.T) {
	// Gate closed → --print lists only the path a real run would write
	// (~/.agents/skills/…), not the gated ~/.claude/skills/… path — dry-run
	// truthfulness — and still writes nothing.
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())
	forceClaudeGate(t, false)

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, true /*print*/, false, false); err != nil {
		t.Fatalf("--print err = %v", err)
	}
	out := stdout.String()
	if !strings.HasPrefix(out, agentSkillContent) {
		t.Errorf("--print must lead with the canonical SKILL.md content, got:\n%s", out)
	}
	agentsPath := filepath.Join(home, ".agents", "skills", skillDirName, skillFileName)
	if !strings.Contains(out, agentsPath) {
		t.Errorf("--print must list the unconditional target %s, got:\n%s", agentsPath, out)
	}
	if strings.Contains(out, ".claude") {
		t.Errorf("gate-closed --print must not list the ~/.claude target, got:\n%s", out)
	}
	for _, p := range skillPaths(home) {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("--print must write nothing, but %s was created", p)
		}
	}
}

// --- --uninstall (T004 / R8, R9) ---------------------------------------------

func TestAgentSetup_Uninstall(t *testing.T) {
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())
	forceClaudeGate(t, true)

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

func TestAgentSetup_UninstallIgnoresGate(t *testing.T) {
	// Place with the gate open, then close the gate (claude has since
	// disappeared): --uninstall must STILL remove both skill directories.
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())
	forceClaudeGate(t, true)

	var o1, e1 bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &o1, &e1, false, false, false); err != nil {
		t.Fatalf("place err = %v", err)
	}
	forceClaudeGate(t, false)
	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, true /*uninstall*/, false); err != nil {
		t.Fatalf("--uninstall err = %v", err)
	}
	for _, p := range skillPaths(home) {
		dir := filepath.Dir(p)
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("--uninstall must remove the skill directory %s regardless of the gate", dir)
		}
	}
}

func TestAgentSetup_PlacementStateIgnoresGate(t *testing.T) {
	// A stale pre-existing ~/.claude copy on a no-claude machine still reports
	// placed + stale — the probe covers BOTH candidate paths regardless of the
	// gate, so `shll update`'s conditional refresh and `shll doctor` keep working.
	env, home := agentHomeEnv(t)
	forceClaudeGate(t, false)
	claudePath := filepath.Join(home, ".claude", "skills", skillDirName, skillFileName)
	if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(claudePath, []byte("# stale\n"), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	placed, stale := agentSkillPlacementState(env)
	if !placed {
		t.Errorf("a pre-existing ~/.claude placement must report placed under a closed gate")
	}
	if !stale {
		t.Errorf("a stale ~/.claude placement must report stale under a closed gate")
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
	forceClaudeGate(t, true)

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
	forceClaudeGate(t, true)

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
	// … and match the agentskills.io portable-name regex …
	re := regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	if !re.MatchString(skillDirName) {
		t.Errorf("skill name %q must match ^[a-z0-9]+(-[a-z0-9]+)*$", skillDirName)
	}
	// … within the spec's 1–64 character bound.
	if len(skillDirName) < 1 || len(skillDirName) > 64 {
		t.Errorf("skill name %q must be 1-64 characters, got %d", skillDirName, len(skillDirName))
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
	// The proactive-capabilities pointer line names the code bridge and its topic page.
	if !strings.Contains(agentSkillContent, "rk code exec") {
		t.Errorf("SKILL.md body must name the `rk code exec` editor-command capability")
	}
	if !strings.Contains(agentSkillContent, "shll skill run-kit code") {
		t.Errorf("SKILL.md body must point at the `shll skill run-kit code` topic page")
	}
	// The tutorial/onboarding routing line points at the tutorial topic page.
	if !strings.Contains(agentSkillContent, "tutorial, tour, or onboarding") {
		t.Errorf("SKILL.md body must carry the tutorial/tour/onboarding routing trigger words")
	}
	if !strings.Contains(agentSkillContent, "shll skill run-kit tutorial") {
		t.Errorf("SKILL.md body must point at the `shll skill run-kit tutorial` topic page")
	}
	// It must NOT reintroduce stanza/sentinel wording.
	if strings.Contains(agentSkillContent, "stanza") || strings.Contains(agentSkillContent, "sentinel") {
		t.Errorf("SKILL.md must not describe a stanza/sentinel mechanism, got:\n%s", agentSkillContent)
	}
}

// TestAgentSetup_DescriptionWithinAgentSkillsCap pins the generated description to the
// agentskills.io ≤1024-character cap (the skill standard's placed-skill conformance
// rule). Measured in bytes — stricter than a rune count, so a pass here is a pass under
// any reading of the spec. The headroom this leaves is the budget future trigger
// vocabulary must fit in: compression prioritizes trigger coverage over prose
// completeness, and operational prose belongs in the skill body (compress + relocate).
func TestAgentSetup_DescriptionWithinAgentSkillsCap(t *testing.T) {
	desc := agentSkillDescription()
	if len(desc) > agentSkillDescriptionMaxLen {
		t.Errorf("description is %d bytes, over the agentskills.io %d-character cap", len(desc), agentSkillDescriptionMaxLen)
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
// Beyond the dynamic verbatim-containment check, it pins the hint's four load-bearing
// functions as fragment containment checks, so a future rewording cannot silently drop
// any of: (a) the proxy trigger vocabulary ("to proxy a local http port" — matches
// requests that name proxying/dev servers), (b) the skill-shadowing
// counter-instruction ("before opening any file or local port in a browser, read" —
// fires when a competing skill's local `open`/`xdg-open` delivery step is about to run,
// routing the agent to `shll skill run-kit` for the proxied-iframe recipe instead), and
// (c) the hosted-artifact counter-instruction ("publishing an artifact" — fires when an
// Artifact-style hosted-publishing delivery step, which opens no file and touches no
// local port, is about to route visuals off the run-kit dashboard), (d) the
// editor-command trigger vocabulary ("rk code exec" — matches requests to act inside the
// user's code editor, e.g. refresh a PR list or open a diff, via the run-kit code bridge),
// and (e) the tutorial/onboarding routing ("tutorial, tour, or onboarding" → read
// `shll skill run-kit tutorial` — matches requests like "Onboard me to run-kit" that
// would otherwise pattern-match a harness's generic onboarding flow).
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
	// … carrying all five load-bearing functions — the proxy trigger vocabulary, the
	// skill-shadowing counter-instruction, the hosted-artifact counter-instruction, the
	// editor-command trigger vocabulary, and the tutorial/onboarding routing — pinned as
	// independent fragments so a future rewording cannot silently drop any (the verbatim
	// check above is dynamic and would pass mechanically with any Roster value).
	for _, fragment := range []string{
		"to proxy a local http port",                               // (a) proxy trigger vocabulary
		"before opening any file or local port in a browser, read", // (b) shadowing counter-instruction
		"publishing an artifact",                                   // (c) hosted-artifact counter-instruction
		"rk code exec",                                             // (d) editor-command trigger vocabulary
		"tutorial, tour, or onboarding",                            // (e) tutorial/onboarding routing triggers
		"shll skill run-kit tutorial",                              // (e) …and its topic-page destination
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

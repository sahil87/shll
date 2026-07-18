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
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false); err != nil {
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
	if err := runAgentSetup(context.Background(), env, &o1, &e1, false, false); err != nil {
		t.Fatalf("first run err = %v", err)
	}
	paths := skillPaths(home)
	before := make([][]byte, len(paths))
	for i, p := range paths {
		before[i], _ = os.ReadFile(p)
	}

	var o2, e2 bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &o2, &e2, false, false); err != nil {
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
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false); err != nil {
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
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, true /*print*/, false); err != nil {
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
	if err := runAgentSetup(context.Background(), env, &o1, &e1, false, false); err != nil {
		t.Fatalf("place err = %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, true /*uninstall*/); err != nil {
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
	err := runAgentSetup(context.Background(), env, &stdout, &stderr, true, true)
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
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false); err != nil {
		t.Fatalf("run err = %v", err)
	}
	var delegated bool
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName && len(c.Args) == 1 && c.Args[0] == agentSetupSub {
			delegated = true
		}
	}
	if !delegated {
		t.Errorf("expected a `run-kit agent-setup` delegation, calls: %+v", f.recordedCalls())
	}
}

func TestAgentSetup_RunKitAbsentSkipsSilently(t *testing.T) {
	env, home := agentHomeEnv(t)
	installFakeRunner(t, runKitAbsentFake())

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false); err != nil {
		t.Fatalf("run-kit-absent run err = %v, want nil (delegation skipped silently)", err)
	}
	// The skills were still placed.
	for _, p := range skillPaths(home) {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("placement must succeed even when run-kit is absent: %s missing (%v)", p, err)
		}
	}
	// The absent-run-kit case must not surface a delegation error.
	if strings.Contains(stderr.String(), "run-kit agent-setup:") {
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
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, false); err != nil {
		t.Fatalf("a failed run-kit delegation must not fail the placement, err = %v", err)
	}
	// Placement is the core work — both skills still land.
	for _, p := range skillPaths(home) {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("placement must succeed despite the delegation exit: %s missing (%v)", p, err)
		}
	}
	// The non-zero exit is surfaced as a warn-and-continue, not swallowed.
	if want := "run-kit agent-setup exited 3 (continuing)"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
	}
}

func TestAgentSetup_UninstallDelegatesUninstall(t *testing.T) {
	env, _ := agentHomeEnv(t)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{ExitCode: 0} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runAgentSetup(context.Background(), env, &stdout, &stderr, false, true /*uninstall*/); err != nil {
		t.Fatalf("run err = %v", err)
	}
	var delegatedUninstall bool
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName && len(c.Args) == 2 && c.Args[0] == agentSetupSub && c.Args[1] == "--uninstall" {
			delegatedUninstall = true
		}
	}
	if !delegatedUninstall {
		t.Errorf("expected a `run-kit agent-setup --uninstall` delegation, calls: %+v", f.recordedCalls())
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

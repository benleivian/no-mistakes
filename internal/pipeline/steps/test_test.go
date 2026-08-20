package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/kunchenguid/no-mistakes/internal/agent"
	"github.com/kunchenguid/no-mistakes/internal/config"
	"github.com/kunchenguid/no-mistakes/internal/types"
)

func TestTestStep_CleansOwnedTemporaryDDEVRegistrationAfterTestFailure(t *testing.T) {
	dir, baseSHA, headSHA := setupGitRepo(t)
	sctx := newTestContextWithDBRecords(t, &mockAgent{name: "test"}, dir, baseSHA, headSHA, config.Commands{})
	sctx.Repo.ID = "1289b3"
	generated := "source-project-no-mistakes-" + sctx.Repo.ID

	if err := os.Mkdir(filepath.Join(dir, ".ddev"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ddev", "config.yaml"), []byte("name: source-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sctx.EvidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(sctx.EvidenceDir, "evidence.txt")
	if err := os.WriteFile(artifact, []byte("retain me"), 0o644); err != nil {
		t.Fatal(err)
	}

	logFile := filepath.Join(t.TempDir(), "ddev.log")
	listFile := filepath.Join(t.TempDir(), "ddev-list.json")
	if err := os.WriteFile(listFile, []byte(fmt.Sprintf(`{"level":"info","raw":[{"name":%q,"approot":%q},{"name":"source-project","approot":%q}]}`, generated, dir, t.TempDir())), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := fakeCLIBinDir(t)
	if err := os.WriteFile(filepath.Join(binDir, "ddev"), []byte(`#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_DDEV_LOG"
if [ "$1" = list ]; then cat "$FAKE_DDEV_LIST"; exit 0; fi
if [ "$1" = delete ] && [ "$2" = -Oy ]; then exit 0; fi
exit 1
`), 0o755); err != nil {
		t.Fatal(err)
	}
	sctx.Env = fakeCLIEnv(binDir, map[string]string{
		"FAKE_DDEV_LOG":  logFile,
		"FAKE_DDEV_LIST": listFile,
	})
	sctx.Config.Commands.Test = "exit 1"

	outcome, err := (&TestStep{}).Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.NeedsApproval || outcome.ExitCode != 1 {
		t.Fatalf("test failure outcome = %+v, want preserved exit 1 approval", outcome)
	}
	gotLog, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Fields(string(gotLog)), []string{"list", "--json-output", "delete", "-Oy", generated}; !slices.Equal(got, want) {
		t.Fatalf("ddev commands = %q, want %q", got, want)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("worktree removed: %v", err)
	}
	if _, err := os.Stat(artifact); err != nil {
		t.Fatalf("test evidence removed: %v", err)
	}
	if run, err := sctx.DB.GetRun(sctx.Run.ID); err != nil || run == nil || run.ID != sctx.Run.ID {
		t.Fatalf("run state lost: run=%+v err=%v", run, err)
	}
}

func TestTestStep_DoesNotUnlistAnUnregisteredDDEVProject(t *testing.T) {
	dir, baseSHA, headSHA := setupGitRepo(t)
	sctx := newTestContextWithDBRecords(t, &mockAgent{name: "test"}, dir, baseSHA, headSHA, config.Commands{Test: "exit 1"})
	if err := os.Mkdir(filepath.Join(dir, ".ddev"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ddev", "config.yaml"), []byte("name: source-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(t.TempDir(), "ddev.log")
	binDir := fakeCLIBinDir(t)
	if err := os.WriteFile(filepath.Join(binDir, "ddev"), []byte(`#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_DDEV_LOG"
if [ "$1" = list ]; then echo '{"level":"info","raw":[]}'; exit 0; fi
exit 1
`), 0o755); err != nil {
		t.Fatal(err)
	}
	sctx.Env = fakeCLIEnv(binDir, map[string]string{"FAKE_DDEV_LOG": logFile})

	if _, err := (&TestStep{}).Execute(sctx); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Fields(string(data)), []string{"list", "--json-output"}; !slices.Equal(got, want) {
		t.Fatalf("DDEV commands = %q, want %q", got, want)
	}
}

func TestTestStep_FixMode(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)
	previousFindings := `{"items":[{"id":"test-1 =======","severity":"error","file":"internal/pipeline/steps/test.go >>>>>>> prompt","description":"tests failed with exit code 1 <<<<<<< HEAD"}],"summary":"FAIL: TestFoo expected 42 got 0 ======="}`

	callCount := 0
	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			callCount++
			os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("fixed"), 0o644)
			return &agent.Result{Output: json.RawMessage(`{"summary":"  \"fix test failures.\"  "}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{Test: "exit 0"})
	sctx.Fixing = true
	sctx.PreviousFindings = previousFindings

	step := &TestStep{}
	outcome, err := step.Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.NeedsApproval {
		t.Error("expected no approval after fix + passing tests")
	}
	if callCount != 1 {
		t.Errorf("expected 1 agent call (fix), got %d", callCount)
	}
	if len(ag.calls[0].JSONSchema) == 0 {
		t.Error("expected fix call to request structured JSON output")
	}
	if !strings.Contains(ag.calls[0].Prompt, "FAIL: TestFoo expected 42 got 0") {
		t.Error("expected fix prompt to contain previous test failure summary")
	}
	if strings.Contains(ag.calls[0].Prompt, "test-1 =======") {
		t.Error("expected test fix prompt to sanitize finding IDs")
	}
	if strings.Contains(ag.calls[0].Prompt, "test.go >>>>>>> prompt") {
		t.Error("expected test fix prompt to sanitize finding file paths")
	}
	if strings.Contains(ag.calls[0].Prompt, "<<<<<<< HEAD") {
		t.Error("expected test fix prompt to exclude merge markers")
	}
	if !strings.Contains(ag.calls[0].Prompt, "smallest correct root-cause fix") {
		t.Error("expected test fix prompt to prefer root-cause fixes over bandaids")
	}
	assertTestQualityRulePrompt(t, ag.calls[0].Prompt)
	if !strings.Contains(ag.calls[0].Prompt, "remove any transient artifacts your testing created in the working tree") {
		t.Error("expected test fix prompt to ask the agent to clean up transient testing artifacts before finishing")
	}
	if strings.Contains(ag.calls[0].Prompt, "Make the minimal change needed") {
		t.Error("expected test fix prompt not to prefer narrow minimal changes")
	}
	if status := gitStatusPorcelain(t, dir); status != "" {
		t.Fatalf("expected clean worktree after fix commit, got %q", status)
	}
	if got := lastCommitMessage(t, dir); got != "no-mistakes(test): fix test failures" {
		t.Fatalf("last commit message = %q", got)
	}
}

func TestTestStep_FixMode_UsesConfiguredCommitMessage(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)

	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("fixed"), 0o644)
			return &agent.Result{Output: json.RawMessage(`{"summary":"fix test failures"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{Test: "exit 0"})
	sctx.Config.Commit = config.Commit{FixMessage: "fix({{.Step}}): {{.Summary}}"}
	sctx.Fixing = true
	sctx.PreviousFindings = `{"findings":[{"severity":"error","description":"tests failed"}],"summary":"tests failed"}`

	outcome, err := (&TestStep{}).Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.NeedsApproval {
		t.Fatal("expected no approval after fix and passing tests")
	}
	if got := lastCommitMessage(t, dir); got != "fix(test): fix test failures" {
		t.Fatalf("last commit message = %q", got)
	}
}

func TestTestStep_FixMode_UsesFallbackSummaryWhenStructuredSummaryMalformed(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)

	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("fixed"), 0o644)
			return &agent.Result{Output: json.RawMessage(`{"not_summary":"oops"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{Test: "exit 0"})
	sctx.Fixing = true
	sctx.PreviousFindings = `{"findings":[{"severity":"error","description":"tests failed"}],"summary":"tests failed"}`

	step := &TestStep{}
	outcome, err := step.Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.NeedsApproval {
		t.Fatal("expected no approval after fallback summary commit and passing tests")
	}
	if got := lastCommitMessage(t, dir); got != "no-mistakes(test): fix test failures" {
		t.Fatalf("last commit message = %q", got)
	}
}

func TestTestStep_FixMode_AgentWritesNewTests_ProceedsAutomatically(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)

	callCount := 0
	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			callCount++
			// Simulate agent creating a new test file during fix in another supported language
			os.WriteFile(filepath.Join(dir, "component.spec.tsx"), []byte("export {}\n"), 0o644)
			return &agent.Result{Output: json.RawMessage(`{"summary":"add regression test"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{Test: "exit 0"})
	sctx.Fixing = true

	step := &TestStep{}
	outcome, err := step.Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	// Issue #140: a passing test run whose only finding is an informational
	// "new test file written by agent" note must not require approval.
	if outcome.NeedsApproval {
		t.Error("expected no approval for an informational new-test-file finding when tests pass")
	}
	if callCount != 1 {
		t.Errorf("expected 1 agent call in fix mode, got %d", callCount)
	}

	var f Findings
	json.Unmarshal([]byte(outcome.Findings), &f)
	foundTestFile := false
	for _, item := range f.Items {
		if strings.Contains(item.Description, "component.spec.tsx") {
			foundTestFile = true
			if item.Action != types.ActionNoOp {
				t.Errorf("expected new-test-file finding action %q, got %q", types.ActionNoOp, item.Action)
			}
		}
	}
	if !foundTestFile {
		t.Errorf("expected finding mentioning component.spec.tsx, got findings: %+v", f.Items)
	}
}

func TestTestStep_UserIntentRunsConfiguredCommandThenEvidenceAgent(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	baselineLog := filepath.Join(dir, "baseline.log")
	testCmd := "go env GOOS > baseline.log"

	callCount := 0
	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			callCount++
			return &agent.Result{Output: json.RawMessage(`{"findings":[],"summary":"evidence demonstrates intent","tested":["manual screenshot review"],"testing_summary":"captured screenshot evidence"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{Test: testCmd})
	sctx.UserIntent = "Show users a success screen after checkout"

	step := &TestStep{}
	outcome, err := step.Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.NeedsApproval {
		t.Fatal("expected no approval when evidence-oriented agent testing passes")
	}
	if callCount != 1 {
		t.Fatalf("expected evidence agent to run after configured test command, got %d calls", callCount)
	}
	data, err := os.ReadFile(baselineLog)
	if err != nil {
		t.Fatalf("expected configured test command to run: %v", err)
	}
	if strings.TrimSpace(string(data)) != runtime.GOOS {
		t.Fatalf("configured test command output = %q, want %s", string(data), runtime.GOOS)
	}
	prompt := ag.calls[0].Prompt
	for _, want := range []string{
		"Show users a success screen after checkout",
		"Decide what evidence or artifacts would clearly demonstrate the user intent is satisfied",
		"Unit tests passing is not sufficient evidence by itself",
		"Demonstrate the user intent working end-to-end in a way consistent with how an end user would actually experience it",
		"Prefer product-level artifacts",
		"Only use command output as an artifact when that output directly demonstrates the end-user experience or requested behavior",
		"Configured test command already ran successfully as baseline",
		testCmd,
		"The \"testing_summary\" must account for the complete test step: baseline commands that already ran, automated tests, manual or evidence-producing checks, artifacts gathered, and the overall result",
		"screenshots, GIFs, videos, rendered UI, CLI transcripts",
		"For UI, HTML, CSS, Electron renderer, browser, visual layout, or copy-placement changes, attempt to capture reviewer-visible visual evidence",
		"DOM snapshots, selector assertions, and text-only render summaries are not substitutes for visual evidence when a rendered surface is available",
		"If a UI-facing change has no screenshot, image, video, GIF, or rendered HTML artifact, state why in testing_summary",
		"Write new evidence files into this evidence directory, never into the worktree:",
		sctx.EvidenceDir,
		"Do not move, commit, or modify source files only to make evidence linkable",
		"If no existing test produces sufficient evidence, write or improve a focused test",
		"If automated testing cannot produce the needed evidence, execute manual verification steps",
		"Always include an \"artifacts\" array",
		"If sufficient evidence is not possible, report a warning finding",
		"remove any transient artifacts your testing created in the working tree",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("expected prompt to contain %q, got:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "will be available from the pushed commit") || strings.Contains(prompt, "files that already exist in the repository") {
		t.Fatalf("expected prompt not to make the testing agent worry about committed evidence files, got:\n%s", prompt)
	}
	if _, err := os.Stat(sctx.EvidenceDir); err != nil {
		t.Fatalf("expected temporary evidence directory to exist: %v", err)
	}

	var findings Findings
	if err := json.Unmarshal([]byte(outcome.Findings), &findings); err != nil {
		t.Fatal(err)
	}
	t.Logf("evidence findings JSON: %s", outcome.Findings)
	if len(findings.Tested) != 2 || findings.Tested[0] != testCmd || findings.Tested[1] != "manual screenshot review" {
		t.Fatalf("expected baseline command and agent-tested evidence to be recorded, got %+v", findings.Tested)
	}
}

func TestTestStep_EvidenceDirectoryIsAlwaysOutsideTheWorktree(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)

	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			return &agent.Result{Output: json.RawMessage(`{"findings":[],"summary":"","tested":["manual evidence check"],"testing_summary":"checked evidence"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{})
	sctx.UserIntent = "Show users a success screen after checkout"

	step := &TestStep{}
	if _, err := step.Execute(sctx); err != nil {
		t.Fatal(err)
	}

	prompt := ag.calls[0].Prompt
	wantDir := sctx.EvidenceDir
	if !strings.Contains(prompt, "Write new evidence files into this evidence directory, never into the worktree: "+wantDir) {
		t.Fatalf("expected evidence guidance to point outside the worktree, got:\n%s", prompt)
	}
	if _, err := os.Stat(filepath.Join(dir, ".no-mistakes")); err == nil {
		t.Fatal("test step created an in-repo evidence directory")
	}
}

func TestTestStep_PublishedEvidenceGuidanceNamesTheEvidenceBranch(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)

	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			return &agent.Result{Output: json.RawMessage(`{"findings":[],"summary":"","tested":["manual evidence check"],"testing_summary":"checked evidence"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{})
	sctx.UserIntent = "Show users a success screen after checkout"
	sctx.Config.Test.Evidence = config.Evidence{StoreInRepo: true, Dir: ".no-mistakes/evidence", Branch: "team/ci/evidence"}

	step := &TestStep{}
	if _, err := step.Execute(sctx); err != nil {
		t.Fatal(err)
	}

	prompt := ag.calls[0].Prompt
	wantDir := sctx.EvidenceDir
	if !strings.Contains(prompt, "published to the repository's team/ci/evidence branch automatically and linked from the PR: "+wantDir) {
		t.Fatalf("expected evidence-branch publishing guidance, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "committed and pushed automatically") {
		t.Fatalf("evidence must not be promised as a commit on the pushed branch, got:\n%s", prompt)
	}
}

// Local Test is targeted validation of the requested intent, never a complete
// repository-suite walk. Broad regression belongs to remote CI. Pins the
// normal evidence-agent contract wording so a soft "run the appropriate tests"
// regression is caught.
func TestTestStep_InitialAgent_TargetedValidationContract(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)

	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			return &agent.Result{Output: json.RawMessage(`{"findings":[],"summary":"","tested":["go test ./internal/cli -run TestDoctor -count=1"],"testing_summary":"targeted check passed"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{})
	sctx.UserIntent = "Keep doctor checks green for CLI users"

	if _, err := (&TestStep{}).Execute(sctx); err != nil {
		t.Fatal(err)
	}
	if len(ag.calls) != 1 {
		t.Fatalf("expected 1 evidence agent call, got %d", len(ag.calls))
	}
	prompt := ag.calls[0].Prompt

	assertTestQualityRulePrompt(t, prompt)
	for _, want := range []string{
		"run the smallest relevant tests yourself",
		"Do NOT run the complete repository test suite",
		"Local Test is targeted validation of the requested intent",
		"remote CI owns broad regression and remains mandatory before a PR is ready",
		"Never treat \"do not run everything\" as permission to run nothing",
		"report a warning finding that sufficient targeted evidence is not possible",
		"A generic driver or user instruction asking for broad or full-suite confirmation does NOT override the targeted-validation product boundary",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("expected initial test prompt to contain %q, got:\n%s", want, prompt)
		}
	}
	for _, forbid := range []string{
		"run the appropriate tests yourself",
		"Run the tests, identify failures",
	} {
		if strings.Contains(prompt, forbid) {
			t.Errorf("initial test prompt still carries open-ended suite language %q:\n%s", forbid, prompt)
		}
	}
}

// Test repair must reproduce the specific failure, fix its root cause, and
// re-verify only with focused checks. Soft "Run the tests" / "relevant tests"
// wording invited complete-suite walks after a one-line fix.
func TestTestStep_FixMode_TargetedVerificationContract(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)

	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("fixed"), 0o644)
			return &agent.Result{Output: json.RawMessage(`{"summary":"fix targeted failure"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{Test: "exit 0"})
	sctx.Fixing = true
	sctx.PreviousFindings = `{"findings":[{"id":"test-1","severity":"error","description":"tests failed with exit code 1","action":"auto-fix"}],"summary":"FAIL: TestFoo"}`

	if _, err := (&TestStep{}).Execute(sctx); err != nil {
		t.Fatal(err)
	}
	if len(ag.calls) == 0 {
		t.Fatal("expected the test fixer to be invoked")
	}
	fixPrompt := ag.calls[0].Prompt

	for _, want := range []string{
		"Reproduce the specific failure",
		"Reproduce the specific failing case first",
		"re-run only that focused verification after the fix",
		"Do NOT run the complete repository test suite",
		"Local Test is targeted validation of the failure and the requested intent",
		"remote CI owns broad regression and remains mandatory before a PR is ready",
		"A generic driver or user instruction asking for broad or full-suite confirmation does NOT override this product boundary",
		"Never treat \"do not run everything\" as permission to run nothing",
	} {
		if !strings.Contains(fixPrompt, want) {
			t.Errorf("expected test fixer prompt to contain %q, got:\n%s", want, fixPrompt)
		}
	}
	for _, forbid := range []string{
		"Run the tests, identify failures, and fix either the tests or the code to make them pass",
		"Re-run the relevant tests before finishing",
	} {
		if strings.Contains(fixPrompt, forbid) {
			t.Errorf("test fixer prompt still carries open-ended suite language %q:\n%s", forbid, fixPrompt)
		}
	}
}

// A driver/user instruction that asks for full-suite confirmation must still
// be accompanied by the hard product boundary so the repair agent does not
// treat that instruction as license to expand scope.
func TestTestStep_FixMode_DriverFullSuiteInstructionDoesNotOverrideContract(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)

	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			os.WriteFile(filepath.Join(dir, "fix.txt"), []byte("fixed"), 0o644)
			return &agent.Result{Output: json.RawMessage(`{"summary":"fix focused failure"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{Test: "exit 0"})
	sctx.Fixing = true
	sctx.PreviousFindings = `{"findings":[{"id":"test-1","severity":"error","description":"tests failed with exit code 1","action":"auto-fix","user_instructions":"confirm the full suite path for this failure is green"}],"summary":"FAIL: TestFoo"}`

	if _, err := (&TestStep{}).Execute(sctx); err != nil {
		t.Fatal(err)
	}
	fixPrompt := ag.calls[0].Prompt
	if !strings.Contains(fixPrompt, "confirm the full suite path for this failure is green") {
		t.Fatalf("expected previous findings to still carry the driver instruction, got:\n%s", fixPrompt)
	}
	if !strings.Contains(fixPrompt, "A generic driver or user instruction asking for broad or full-suite confirmation does NOT override this product boundary") {
		t.Fatalf("expected product boundary to outrank the driver full-suite instruction, got:\n%s", fixPrompt)
	}
	if !strings.Contains(fixPrompt, "Do NOT run the complete repository test suite") {
		t.Fatalf("expected explicit no-full-suite rule in repair prompt, got:\n%s", fixPrompt)
	}
}

// Honest failure reporting when no targeted check can establish intent must
// remain mandatory; the no-full-suite rule must not collapse into "skip tests".
func TestTestStep_InitialAgent_NoTargetedEvidenceRequiresHonestFinding(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)

	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			return &agent.Result{Output: json.RawMessage(`{"findings":[{"severity":"warning","description":"no targeted test can prove the intent","action":"ask-user"}],"summary":"missing evidence","tested":["manual review of changed packages"],"testing_summary":"could not produce targeted evidence"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{})
	sctx.UserIntent = "Prove the checkout success screen works end-to-end"

	outcome, err := (&TestStep{}).Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.NeedsApproval {
		t.Fatal("expected missing targeted evidence to require approval")
	}
	prompt := ag.calls[0].Prompt
	for _, want := range []string{
		"Never treat \"do not run everything\" as permission to run nothing",
		"write or improve a focused test",
		"perform manual verification with evidence",
		"report a warning finding that sufficient targeted evidence is not possible",
		"If sufficient evidence is not possible, report a warning finding",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("expected no-targeted-evidence guidance %q in prompt:\n%s", want, prompt)
		}
	}
}

package steps

// Fork-only (see FORK.md): TestStep's temporary DDEV project cleanup.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kunchenguid/no-mistakes/internal/agent"
	"github.com/kunchenguid/no-mistakes/internal/config"
)

// parkingTestAgent never answers, so a failing configured test command parks
// at the evidence-turn budget instead of needing a scripted analyzer reply.
func parkingTestAgent() *mockAgent {
	return &mockAgent{name: "test", runFn: func(ctx context.Context, _ agent.RunOpts) (*agent.Result, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
}

func TestTestStep_CleansOwnedTemporaryDDEVRegistrationAfterTestFailure(t *testing.T) {
	dir, baseSHA, headSHA := setupGitRepo(t)
	sctx := newTestContextWithDBRecords(t, parkingTestAgent(), dir, baseSHA, headSHA, config.Commands{})
	sctx.Config.TestAgentTimeout = 20 * time.Millisecond
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
	linkTestBinary(t, binDir, "ddev")
	sctx.Env = fakeCLIEnv(binDir, map[string]string{
		"FAKE_CLI_MODE":  "ddev",
		"FAKE_CLI_LOG":   logFile,
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

func TestTestStep_DoesNotDeleteAnUnregisteredDDEVProject(t *testing.T) {
	dir, baseSHA, headSHA := setupGitRepo(t)
	sctx := newTestContextWithDBRecords(t, parkingTestAgent(), dir, baseSHA, headSHA, config.Commands{Test: "exit 1"})
	sctx.Config.TestAgentTimeout = 20 * time.Millisecond
	if err := os.Mkdir(filepath.Join(dir, ".ddev"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".ddev", "config.yaml"), []byte("name: source-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(t.TempDir(), "ddev.log")
	listFile := filepath.Join(t.TempDir(), "ddev-list.json")
	if err := os.WriteFile(listFile, []byte(`{"level":"info","raw":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	binDir := fakeCLIBinDir(t)
	linkTestBinary(t, binDir, "ddev")
	sctx.Env = fakeCLIEnv(binDir, map[string]string{
		"FAKE_CLI_MODE":  "ddev",
		"FAKE_CLI_LOG":   logFile,
		"FAKE_DDEV_LIST": listFile,
	})

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

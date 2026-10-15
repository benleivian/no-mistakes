package gate

// Fork-only (see FORK.md): eject's DDEV project cleanup.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kunchenguid/no-mistakes/internal/paths"
)

// init, not TestMain, dispatches the re-executed test binary as a fake ddev so
// upstream's gate_test.go TestMain stays untouched.
func init() {
	if os.Getenv("FAKE_DDEV_PROCESS") == "1" {
		handleFakeDDEV()
		os.Exit(0)
	}
}

func handleFakeDDEV() {
	args := os.Args[1:]
	if len(args) == 2 && args[0] == "list" && args[1] == "--json-output" {
		fmt.Printf(`{"raw":[{"name":"eject-addon","approot":%q}]}`, os.Getenv("FAKE_DDEV_APPROOT"))
		return
	}
	if len(args) != 3 || args[0] != "delete" || args[1] != "-Oy" {
		os.Exit(1)
	}
	if _, err := os.Stat(filepath.Join(".ddev")); err != nil {
		os.Exit(17)
	}
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(1)
	}
	f, err := os.OpenFile(os.Getenv("FAKE_DDEV_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		os.Exit(1)
	}
	_, _ = fmt.Fprintf(f, "%s cwd=%s\n", strings.Join(args, " "), cwd)
	_ = f.Close()
	// A failing delete must not stop eject from removing the worktree.
	os.Exit(23)
}

func linkFakeDDEVExecutable(t *testing.T, binDir string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "ddev"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	dst := filepath.Join(binDir, name)
	if err := os.Link(exe, dst); err == nil {
		return
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

// Unlike t.TempDir, cleanup tolerates transient locks on recently executed
// binaries on Windows.
func fakeDDEVBinDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "fake-ddev")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for i := 0; i < 10; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	})
	return dir
}

func TestEjectCleansUpDDEVProjectsBeforeRemovingWorktrees(t *testing.T) {
	workDir := setupTestRepo(t)
	nmRoot := t.TempDir()
	p := paths.WithRoot(nmRoot)
	if err := p.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	d := openTestDB(t, p)
	ctx := context.Background()

	repo, _, err := Init(ctx, d, p, workDir)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	wtDir := p.WorktreeDir(repo.ID, "fake-run-id")
	if err := os.MkdirAll(filepath.Join(wtDir, ".ddev"), 0o755); err != nil {
		t.Fatalf("create worktree dir: %v", err)
	}
	ddevLog := filepath.Join(t.TempDir(), "ddev.log")
	binDir := fakeDDEVBinDir(t)
	linkFakeDDEVExecutable(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DDEV_PROCESS", "1")
	t.Setenv("FAKE_DDEV_APPROOT", wtDir)
	t.Setenv("FAKE_DDEV_LOG", ddevLog)

	if _, err := Eject(ctx, d, p, workDir); err != nil {
		t.Fatalf("eject: %v", err)
	}

	if fileExists(filepath.Join(p.WorktreesDir(), repo.ID)) {
		t.Error("expected worktree directory to be cleaned up")
	}
	got, err := os.ReadFile(ddevLog)
	if err != nil {
		t.Fatalf("read DDEV cleanup log: %v", err)
	}
	want := fmt.Sprintf("delete -Oy eject-addon cwd=%s", wtDir)
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("DDEV cleanup commands = %q, want %q", got, want)
	}
}

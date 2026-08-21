package ddev

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCleanupDeletesOnlyScopedProjectsBeforeWorkspaceRemoval(t *testing.T) {
	workDir := t.TempDir()
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, ".ddev"), 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(t.TempDir(), "ddev.log")
	projects := fmt.Sprintf(`{"raw":[{"name":"owned-addon","approot":%q},{"name":"external","approot":%q}]}`, workDir, external)
	binDir := t.TempDir()
	linkFakeExecutable(t, binDir, "ddev")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DDEV_PROCESS", "1")
	t.Setenv("FAKE_DDEV_LOG", logFile)
	t.Setenv("FAKE_DDEV_PROJECTS", projects)

	Cleanup(workDir)

	got, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if want := "delete -Oy owned-addon"; strings.TrimSpace(string(got)) != want {
		t.Fatalf("DDEV commands = %q, want %q", got, want)
	}
	t.Logf("scoped cleanup: %s; external project at %s was not deleted", strings.TrimSpace(string(got)), external)
}

func TestMain(m *testing.M) {
	if os.Getenv("FAKE_DDEV_PROCESS") == "1" {
		handleFakeDDEV()
		return
	}
	os.Exit(m.Run())
}

func handleFakeDDEV() {
	args := os.Args[1:]
	if len(args) == 2 && args[0] == "list" && args[1] == "--json-output" {
		_, _ = os.Stdout.WriteString(os.Getenv("FAKE_DDEV_PROJECTS"))
		return
	}
	if len(args) != 3 || args[0] != "delete" || args[1] != "-Oy" {
		os.Exit(1)
	}
	if _, err := os.Stat(".ddev"); err != nil {
		os.Exit(1)
	}
	f, err := os.OpenFile(os.Getenv("FAKE_DDEV_LOG"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		os.Exit(1)
	}
	_, _ = fmt.Fprintln(f, strings.Join(args, " "))
	_ = f.Close()
}

func linkFakeExecutable(t *testing.T, binDir, name string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
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

func TestCleanupWithoutDDEVLeavesWorkspaceUntouched(t *testing.T) {
	workDir := t.TempDir()
	marker := filepath.Join(workDir, "keep")
	if err := os.WriteFile(marker, []byte("unchanged"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())

	Cleanup(workDir)

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("workspace marker was disturbed without DDEV: %v", err)
	}
	if string(got) != "unchanged" {
		t.Fatalf("workspace marker = %q, want unchanged", got)
	}
}

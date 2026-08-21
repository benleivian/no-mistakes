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
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	workDir := t.TempDir()
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workDir, ".ddev"), 0o755); err != nil {
		t.Fatal(err)
	}
	logFile := filepath.Join(t.TempDir(), "ddev.log")
	projects := fmt.Sprintf(`{"raw":[{"name":"owned-addon","approot":%q},{"name":"external","approot":%q}]}`, workDir, external)
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "ddev"), []byte(`#!/bin/sh
if [ "$1" = list ]; then printf '%s' "$FAKE_DDEV_PROJECTS"; exit 0; fi
test -d "$PWD/.ddev" || exit 1
printf '%s\n' "$*" >> "$FAKE_DDEV_LOG"
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
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

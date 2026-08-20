package ddev

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCleanupBoundsAllDeletesAndLogsSkippedProjects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	oldCleanupTimeout := cleanupTimeout
	oldCommandTimeout := cleanupCommandTimeout
	cleanupTimeout = 1500 * time.Millisecond
	cleanupCommandTimeout = 500 * time.Millisecond
	t.Cleanup(func() {
		cleanupTimeout = oldCleanupTimeout
		cleanupCommandTimeout = oldCommandTimeout
	})

	workDir := t.TempDir()
	logFile := filepath.Join(t.TempDir(), "ddev.log")
	if err := os.WriteFile(logFile, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var projects strings.Builder
	projects.WriteString(`{"raw":[`)
	for i := range 10 {
		if i > 0 {
			projects.WriteByte(',')
		}
		fmt.Fprintf(&projects, `{"name":"slow-%d","approot":%q}`, i, workDir)
	}
	projects.WriteString(`]}`)
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "ddev"), []byte(`#!/bin/sh
if [ "$1" = list ]; then printf '%s' "$FAKE_DDEV_PROJECTS"; exit 0; fi
printf '%s\n' "$3" >> "$FAKE_DDEV_LOG"
sleep 1
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DDEV_LOG", logFile)
	t.Setenv("FAKE_DDEV_PROJECTS", projects.String())

	var logs bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	started := time.Now()
	Cleanup(workDir)
	if elapsed := time.Since(started); elapsed > 2500*time.Millisecond {
		t.Fatalf("Cleanup took %s, want aggregate timeout", elapsed)
	}
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Fields(string(data)); len(got) == 0 || len(got) >= 10 {
		t.Fatalf("attempted projects = %q, want a bounded subset; logs: %s", got, logs.String())
	}
	if output := logs.String(); !strings.Contains(output, "cleanup budget exhausted") {
		t.Fatalf("cleanup logs = %q, want skipped-project budget warning", output)
	}
}

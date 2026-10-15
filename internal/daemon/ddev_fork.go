package daemon

import "github.com/kunchenguid/no-mistakes/internal/ddev"

// Fork-only (see FORK.md): wrappers keep the upstream call site to one line
// with no import change.

// cleanupRunWorktreeDDEV deletes DDEV projects rooted in a run worktree just
// before removeRunWorktree removes it.
func cleanupRunWorktreeDDEV(wtDir string) { ddev.Cleanup(wtDir) }

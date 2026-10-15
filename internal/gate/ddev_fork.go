package gate

import (
	"os"
	"path/filepath"

	"github.com/kunchenguid/no-mistakes/internal/ddev"
)

// Fork-only (see FORK.md): wrappers keep the upstream call sites in
// removeRepoWorktrees to one line each with no import change.

// cleanupDDEVWorktree deletes DDEV projects rooted in one recorded run
// worktree before eject removes it.
func cleanupDDEVWorktree(dir string) { ddev.Cleanup(dir) }

// cleanupDDEVWorktrees runs cleanupDDEVWorktree for every run worktree under
// the repository's default worktree tree before eject removes the tree.
func cleanupDDEVWorktrees(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			cleanupDDEVWorktree(filepath.Join(dir, entry.Name()))
		}
	}
}

// Package ddev cleans up DDEV projects registered for disposable worktrees.
package ddev

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kunchenguid/no-mistakes/internal/shellenv"
)

const cleanupTimeout = 30 * time.Second

type Project struct {
	Name    string `json:"name"`
	AppRoot string `json:"approot"`
}

type listOutput struct {
	Raw []Project `json:"raw"`
}

func ParseListOutput(output []byte) ([]Project, error) {
	var result listOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, err
	}
	return result.Raw, nil
}

// Cleanup deletes DDEV projects whose app root is inside workDir. It is best
// effort: a missing DDEV binary or any cleanup failure never blocks deletion
// of the worktree.
func Cleanup(workDir string) {
	listCtx, listCancel := context.WithTimeout(context.Background(), cleanupTimeout)

	cmd := exec.CommandContext(listCtx, "ddev", "list", "--json-output")
	shellenv.ConfigureShellCommand(cmd)
	output, err := shellenv.OutputShellCommand(cmd)
	listCancel()
	if errors.Is(err, exec.ErrNotFound) {
		return
	}
	if err != nil {
		slog.Warn("failed to list DDEV projects for worktree cleanup", "path", workDir, "error", err)
		return
	}
	projects, err := ParseListOutput(output)
	if err != nil {
		slog.Warn("failed to parse DDEV project list for worktree cleanup", "path", workDir, "error", err)
		return
	}
	for _, project := range projects {
		if project.Name == "" || !appRootWithin(project.AppRoot, workDir) {
			continue
		}
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), cleanupTimeout)
		cmd := exec.CommandContext(deleteCtx, "ddev", "delete", "-Oy", project.Name)
		cmd.Dir = workDir
		shellenv.ConfigureShellCommand(cmd)
		err := shellenv.RunShellCommand(cmd)
		deleteCancel()
		if err != nil {
			slog.Warn("failed to delete DDEV project during worktree cleanup", "path", workDir, "project", project.Name, "error", err)
		}
	}
}

func appRootWithin(appRoot, workDir string) bool {
	appRoot, err := filepath.EvalSymlinks(appRoot)
	if err != nil {
		return false
	}
	workDir, err = filepath.EvalSymlinks(workDir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(workDir, appRoot)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

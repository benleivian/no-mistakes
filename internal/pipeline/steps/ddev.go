package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kunchenguid/no-mistakes/internal/pipeline"
	"github.com/kunchenguid/no-mistakes/internal/shellenv"
	"gopkg.in/yaml.v3"
)

const ddevCleanupTimeout = 30 * time.Second

type ddevConfig struct {
	Name string `yaml:"name"`
}

type ddevProject struct {
	Name    string `json:"name"`
	AppRoot string `json:"approot"`
}

// cleanupTemporaryDDEVProject releases only the DDEV registration created for
// this run's isolated worktree. It deliberately leaves the worktree intact:
// later review, CI, and approval still need it.
func cleanupTemporaryDDEVProject(sctx *pipeline.StepContext) error {
	name, err := temporaryDDEVProjectName(sctx)
	if err != nil || name == "" {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), ddevCleanupTimeout)
	defer cancel()
	projects, err := listDDEVProjects(ctx, sctx)
	if errors.Is(err, exec.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list DDEV projects: %w", err)
	}
	for _, project := range projects {
		if project.Name != name || !sameDDEVAppRoot(project.AppRoot, sctx.WorkDir) {
			continue
		}
		cmd := stepCmdContext(ctx, sctx, "ddev", "delete", "-Oy", name)
		shellenv.ConfigureShellCommand(cmd)
		if err := shellenv.RunShellCommand(cmd); err != nil {
			return fmt.Errorf("delete temporary DDEV project %q: %w", name, err)
		}
		return nil
	}
	return nil
}

func temporaryDDEVProjectName(sctx *pipeline.StepContext) (string, error) {
	if sctx == nil || sctx.Repo == nil || strings.TrimSpace(sctx.Repo.ID) == "" {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(sctx.WorkDir, ".ddev", "config.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var cfg ddevConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("parse .ddev/config.yaml: %w", err)
	}
	name := strings.TrimSpace(cfg.Name)
	if name == "" {
		return "", nil
	}
	suffix := "-no-mistakes-" + sctx.Repo.ID
	if strings.HasSuffix(name, suffix) {
		return name, nil
	}
	return name + suffix, nil
}

func listDDEVProjects(ctx context.Context, sctx *pipeline.StepContext) ([]ddevProject, error) {
	cmd := stepCmdContext(ctx, sctx, "ddev", "list", "--json-output")
	shellenv.ConfigureShellCommand(cmd)
	output, err := shellenv.OutputShellCommand(cmd)
	if err != nil {
		return nil, err
	}
	var projects []ddevProject
	if err := json.Unmarshal(output, &projects); err != nil {
		return nil, fmt.Errorf("parse DDEV project list: %w", err)
	}
	return projects, nil
}

func sameDDEVAppRoot(projectRoot, workDir string) bool {
	projectRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return false
	}
	workDir, err = filepath.EvalSymlinks(workDir)
	if err != nil {
		return false
	}
	return filepath.Clean(projectRoot) == filepath.Clean(workDir)
}

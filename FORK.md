# Fork notes

`benleivian/no-mistakes` is a fork of [`kunchenguid/no-mistakes`](https://github.com/kunchenguid/no-mistakes).
It carries a small patch on top of upstream. DDEV cleanup logic lives in fork-owned files with small call sites in upstream files to reduce merge conflicts; the Git compatibility change below also modifies an upstream function.

## The patch

**DDEV cleanup.** Behavior and lifecycle exceptions: [`docs/src/content/docs/guides/ddev-cleanup.md`](docs/src/content/docs/guides/ddev-cleanup.md).

Fork-owned files:

- `internal/ddev/cleanup.go`, `internal/ddev/cleanup_test.go` - `ddev.Cleanup(dir)` and its tests
- `internal/daemon/ddev_fork.go` - `cleanupRunWorktreeDDEV`
- `internal/gate/ddev_fork.go`, `internal/gate/ddev_fork_test.go` - eject cleanup and its test
- `internal/pipeline/steps/ddev.go`, `internal/pipeline/steps/ddev_test.go` - Test-step temporary project cleanup and its tests
- `internal/pipeline/fakecli/ddev_fork.go` - fake `ddev` for step tests
- `docs/src/content/docs/guides/ddev-cleanup.md` - docs page (not in the sidebar, so `docs/astro.config.mjs` stays upstream)

Call sites in upstream files:

- `internal/daemon/manager.go` `removeRunWorktree`: `cleanupRunWorktreeDDEV(wtDir)` just before `git.WorktreeRemove`
- `internal/gate/gate.go` `removeRepoWorktrees`: `cleanupDDEVWorktrees(defaultDir)` before both `os.RemoveAll(defaultDir)` calls, and `cleanupDDEVWorktree(path)` in the recorded-worktree loop
- `internal/pipeline/steps/test.go` `TestStep.Execute`: `defer cleanupTemporaryDDEVProjectAfterTest(sctx)`
- `internal/pipeline/fakecli/main.go` `handleFakeCLI`: `case "ddev": fakeDDEVHandler(args)`

**Apple Git 2.39 compatibility.** `internal/branchsync/sync.go` `mergeTreePreservesFinalHead` ignores its `base` argument and drops `--merge-base`, which needs Git 2.40; `--write-tree` picks the merge base itself. Drop this once the host Git is 2.40 or newer.

When a merge conflicts in one of these spots, keep upstream's code and re-add the call line.

## Syncing with upstream

The fork's history is shared, so sync by merging, never by rebasing or force-pushing.

```sh
git config rerere.enabled true   # once per clone; replays earlier conflict resolutions
git fetch upstream --tags
git checkout -b sync-upstream origin/main
git merge upstream/main          # or a release tag, e.g. v1.97.1
go build ./... && go vet ./... && go test ./...
```

Then open a PR from the sync branch into `main`.

## Installing the fork's binary

`no-mistakes update` installs the official upstream release and drops this patch. Build from the fork instead:

```sh
make build                                        # writes bin/no-mistakes with version info
install -m 755 bin/no-mistakes "$(command -v no-mistakes)"
no-mistakes daemon restart
```

`make install` also works, but it installs to `$(go env GOPATH)/bin`, which may not be the `no-mistakes` first on your `PATH`.
Follow the [daemon lifecycle guard](docs/src/content/docs/concepts/daemon.md#starting-and-stopping) when restarting; active runs can prevent the restart.

---
title: DDEV Cleanup (fork)
description: How this fork removes DDEV projects that belong to no-mistakes worktrees.
---

This page documents a fork-only behavior of `benleivian/no-mistakes`; upstream no-mistakes does not do this. See `FORK.md` at the repository root for the patch layout and sync procedure.

## Worktree cleanup

During run cleanup and gate ejection, before no-mistakes removes an isolated worktree, it runs `ddev delete -Oy <project>` for every registered DDEV project whose app root resolves inside that worktree. Cleanup is best-effort and runs while the worktree and its DDEV Compose files still exist: on normal run completion, setup failure or cancellation, recovered-run completion, and `no-mistakes eject`. This removes project containers, volumes, networks, and add-on services created by project hooks.

A worktree the daemon preserves (for example a protected-path refusal) keeps its DDEV project too, because cleanup only runs when the worktree is actually about to be removed. Startup orphan cleanup and the retention reaper for leftover worktrees remain directory-only and never invoke DDEV.

No-mistakes does not start DDEV itself. If `ddev` is unavailable or no registered project belongs to the worktree, cleanup is silent. A DDEV command failure is logged but never fails the run or prevents worktree removal, and projects whose app roots are outside the worktree are never selected.

## Temporary test projects

When the Test step finishes, however it ends, it deletes the run's temporary DDEV project: the project named `<name>-no-mistakes-<repoID>` (from the worktree's `.ddev/config.yaml` `name`) whose app root is exactly the run worktree. The worktree and test evidence stay in place for the later review, CI, and approval steps. A failure here is logged as a step warning.

## Leftover worktrees

If a worktree is still on disk after a run (see [troubleshooting](/no-mistakes/guides/troubleshooting/)), remove any DDEV projects rooted in it before deleting the worktree manually:

```sh
ddev list
ddev delete -Oy <project>
git worktree remove --force <path>
```

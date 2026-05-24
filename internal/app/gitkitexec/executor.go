package gitkitexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gitkit/ghcli"
	"github.com/134130/gitkit/gitcmd"
	"github.com/134130/gitkit/gitrepo"
)

const stashMessage = "gh-domino: preserve working tree"

type Executor struct {
	git gitrepo.Client
	gh  ghcli.Client
}

func New(r gitcmd.Runner) Executor {
	return Executor{
		git: gitrepo.New(r),
		gh:  ghcli.New(r),
	}
}

func (e Executor) Execute(ctx context.Context, plan *app.Plan, opts app.ExecuteOptions) (*app.RunResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan is nil")
	}
	opts = normalizeOptions(opts)
	if opts.Parallel != 1 {
		return nil, fmt.Errorf("parallel execution is not implemented yet")
	}

	result := &app.RunResult{
		Warnings: slices.Clone(plan.Warnings),
	}

	restore := func(context.Context) error { return nil }
	if requiresPreparation(plan.Actions) {
		var err error
		restore, err = e.prepareRun(ctx, opts)
		if err != nil {
			return result, err
		}
	}

	runErr := e.runSequential(ctx, result, plan.Actions, opts)
	restoreErr := restore(ctx)
	if runErr != nil || restoreErr != nil {
		return result, errors.Join(runErr, restoreErr)
	}
	return result, nil
}

func normalizeOptions(opts app.ExecuteOptions) app.ExecuteOptions {
	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	if opts.Parallel == 0 {
		opts.Parallel = 1
	}
	return opts
}

func requiresPreparation(actions []app.Action) bool {
	for _, action := range actions {
		if action.Kind == app.ActionRepairPR {
			return true
		}
	}
	return false
}

func (e Executor) prepareRun(ctx context.Context, opts app.ExecuteOptions) (func(context.Context) error, error) {
	if opts.WorktreeDir != "" {
		if err := os.MkdirAll(opts.WorktreeDir, 0755); err != nil {
			return nil, fmt.Errorf("create worktree dir: %w", err)
		}
		return func(context.Context) error { return nil }, nil
	}

	branch, err := e.git.CurrentBranch(ctx)
	if err != nil {
		return nil, fmt.Errorf("get current branch: %w", err)
	}
	dirty, err := e.git.IsDirty(ctx)
	if err != nil {
		return nil, fmt.Errorf("check working tree: %w", err)
	}

	stashRef := ""
	if dirty {
		stashRef, err = e.git.StashPush(ctx, stashMessage, true)
		if err != nil {
			return nil, fmt.Errorf("stash working tree: %w", err)
		}
	}

	return func(ctx context.Context) error {
		if err := e.git.Switch(ctx, branch); err != nil {
			return fmt.Errorf("restore branch %s: %w", branch, err)
		}
		if stashRef == "" {
			return nil
		}
		if err := e.git.StashApply(ctx, stashRef); err != nil {
			return fmt.Errorf("restore stashed changes %s: %w", stashRef, err)
		}
		if err := e.git.StashDrop(ctx, stashRef); err != nil {
			return fmt.Errorf("drop restored stash %s: %w", stashRef, err)
		}
		return nil
	}, nil
}

func (e Executor) runSequential(ctx context.Context, result *app.RunResult, actions []app.Action, opts app.ExecuteOptions) error {
	selectedIDs := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		selectedIDs[action.ID] = struct{}{}
	}

	resultsByID := make(map[string]app.ActionStatus, len(actions))
	for _, action := range actions {
		skipReason := ""
		for _, dependencyID := range action.DependsOn {
			if _, selected := selectedIDs[dependencyID]; !selected {
				continue
			}
			if resultsByID[dependencyID] != app.ActionStatusSuccess {
				skipReason = fmt.Sprintf("dependency %s did not succeed", dependencyID)
				break
			}
		}
		if skipReason != "" {
			actionResult := app.ActionResult{
				Action: action,
				Status: app.ActionStatusSkipped,
				Error:  skipReason,
			}
			result.Actions = append(result.Actions, actionResult)
			resultsByID[action.ID] = actionResult.Status
			continue
		}

		if action.Kind != app.ActionRepairPR && action.Kind != app.ActionUpdateBranch {
			return fmt.Errorf("unsupported action kind: %s", action.Kind)
		}

		err := e.executeAction(ctx, action, opts)
		actionResult := app.ActionResult{
			Action: action,
			Status: app.ActionStatusSuccess,
		}
		if err != nil {
			actionResult.Status = app.ActionStatusFailed
			actionResult.Error = err.Error()
		}
		result.Actions = append(result.Actions, actionResult)
		resultsByID[action.ID] = actionResult.Status
	}
	return nil
}

func (e Executor) executeAction(ctx context.Context, action app.Action, opts app.ExecuteOptions) error {
	switch action.Kind {
	case app.ActionRepairPR:
		if opts.WorktreeDir != "" {
			return e.executeRepairInWorktree(ctx, action, opts)
		}
		return e.executeRepairInCurrentWorktree(ctx, action, opts)
	case app.ActionUpdateBranch:
		_, err := e.gh.Run(ctx, "pr", "update-branch", "--rebase", fmt.Sprint(action.PR.Number))
		return err
	default:
		return fmt.Errorf("unsupported action kind: %s", action.Kind)
	}
}

func (e Executor) executeRepairInCurrentWorktree(ctx context.Context, action app.Action, opts app.ExecuteOptions) error {
	head := action.PR.HeadRefName
	remoteHead := opts.Remote + "/" + head
	if err := e.git.Switch(ctx, head); err != nil {
		if err := e.git.SwitchCreateOrReset(ctx, head, remoteHead); err != nil {
			return fmt.Errorf("switch to %s: %w", head, err)
		}
	}
	relationship, err := e.git.AheadBehind(ctx, remoteHead, "HEAD")
	if err != nil {
		return fmt.Errorf("compare %s with HEAD: %w", remoteHead, err)
	}
	if relationship.Ahead > 0 {
		return fmt.Errorf("local branch %s has %d unpushed commit(s); refusing to force-push", head, relationship.Ahead)
	}
	if err := e.git.PullRebase(ctx, opts.Remote, head); err != nil {
		return fmt.Errorf("pull %s: %w", head, err)
	}
	if err := e.rebase(ctx, e.git, action, opts.Remote, head); err != nil {
		return err
	}
	if err := e.git.PushForceWithLease(ctx, opts.Remote, head); err != nil {
		return fmt.Errorf("push %s: %w", head, err)
	}
	return e.updateBase(ctx, e.gh, action)
}

func (e Executor) executeRepairInWorktree(ctx context.Context, action app.Action, opts app.ExecuteOptions) (err error) {
	head := action.PR.HeadRefName
	remoteHead := opts.Remote + "/" + head
	path := filepath.Join(opts.WorktreeDir, worktreeName(action))

	if err := e.git.WorktreeAdd(ctx, path, remoteHead, "--detach"); err != nil {
		return fmt.Errorf("create worktree for %s: %w", head, err)
	}
	defer func() {
		if cleanupErr := e.git.WorktreeRemove(context.Background(), path, true); cleanupErr != nil && err == nil {
			err = fmt.Errorf("remove worktree %s: %w", path, cleanupErr)
		}
	}()

	git := e.git.InDir(path)
	gh := e.gh.InDir(path)
	if err := e.rebase(ctx, git, action, opts.Remote, ""); err != nil {
		return err
	}
	if err := git.PushForceWithLeaseRefspec(ctx, opts.Remote, "HEAD:"+head); err != nil {
		return fmt.Errorf("push %s: %w", head, err)
	}
	return e.updateBase(ctx, gh, action)
}

func (e Executor) rebase(ctx context.Context, git gitrepo.Client, action app.Action, remote, branch string) error {
	newBase := action.NewBase
	if newBase == "" {
		newBase = action.PR.BaseRefName
	}
	err := git.Rebase(ctx, gitrepo.RebaseOptions{
		Onto:     remote + "/" + newBase,
		Upstream: action.Upstream,
		Branch:   branch,
	})
	if err == nil {
		return nil
	}
	if errors.Is(err, gitrepo.ErrRebaseConflict) {
		if abortErr := git.AbortRebase(ctx); abortErr != nil {
			return fmt.Errorf("rebase conflict; abort rebase: %w", abortErr)
		}
		return fmt.Errorf("rebase conflict")
	}
	return fmt.Errorf("rebase %s: %w", action.PR.HeadRefName, err)
}

func (e Executor) updateBase(ctx context.Context, gh ghcli.Client, action app.Action) error {
	newBase := action.NewBase
	if newBase == "" || newBase == action.PR.BaseRefName {
		return nil
	}
	_, err := gh.Run(ctx, "pr", "edit", fmt.Sprint(action.PR.Number), "--base", newBase)
	if err != nil {
		return fmt.Errorf("update base for #%d: %w", action.PR.Number, err)
	}
	return nil
}

func worktreeName(action app.Action) string {
	return fmt.Sprintf("gh-domino-%d-%s", action.PR.Number, sanitizePathComponent(action.PR.HeadRefName))
}

func sanitizePathComponent(value string) string {
	var b strings.Builder
	dash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "branch"
	}
	return out
}

var _ app.Executor = Executor{}

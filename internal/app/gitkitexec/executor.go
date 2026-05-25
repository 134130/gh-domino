package gitkitexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gitkit/ghcli"
	"github.com/134130/gitkit/gitcmd"
	"github.com/134130/gitkit/gitrepo"
)

const stashMessage = "gh-domino: preserve working tree"

const (
	settleAttempts = 5
	settleDelay    = 100 * time.Millisecond
)

var worktreeMu sync.Mutex

type Executor struct {
	git gitrepo.Client
	gh  ghcli.Client
}

type runOptions struct {
	Remote      string
	Parallel    int
	WorktreeDir string
	Progress    app.ProgressSink
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
	if opts.Parallel < 1 {
		return nil, fmt.Errorf("parallel must be greater than 0")
	}
	runOpts := runOptions{
		Remote:   opts.Remote,
		Parallel: opts.Parallel,
		Progress: opts.Progress,
	}

	result := &app.RunResult{
		Warnings: slices.Clone(plan.Warnings),
	}

	restore := func(context.Context) error { return nil }
	cleanup := func() error { return nil }
	emitProgress(opts.Progress, app.ProgressStart, "execute", fmt.Sprintf("Preparing %d actions", len(plan.Actions)))
	if requiresPreparation(plan.Actions) && opts.Parallel == 1 {
		var err error
		emitProgress(opts.Progress, app.ProgressStart, "prepare", "Preparing current worktree")
		restore, err = e.prepareRun(ctx)
		if err != nil {
			emitProgress(opts.Progress, app.ProgressFailure, "prepare", "Prepare current worktree failed")
			return result, err
		}
		emitProgress(opts.Progress, app.ProgressSuccess, "prepare", "Prepared current worktree")
	} else if requiresPreparation(plan.Actions) {
		emitProgress(opts.Progress, app.ProgressStart, "prepare", "Preparing temporary worktrees")
		worktreeDir, err := os.MkdirTemp("", "gh-domino-worktrees-*")
		if err != nil {
			emitProgress(opts.Progress, app.ProgressFailure, "prepare", "Prepare temporary worktrees failed")
			return result, fmt.Errorf("create worktree dir: %w", err)
		}
		runOpts.WorktreeDir = worktreeDir
		cleanup = func() error {
			emitProgress(opts.Progress, app.ProgressStart, "cleanup", "Removing temporary worktrees")
			if err := os.RemoveAll(worktreeDir); err != nil {
				emitProgress(opts.Progress, app.ProgressFailure, "cleanup", "Remove temporary worktrees failed")
				return fmt.Errorf("remove worktree dir %s: %w", worktreeDir, err)
			}
			emitProgress(opts.Progress, app.ProgressSuccess, "cleanup", "Removed temporary worktrees")
			return nil
		}
		emitProgress(opts.Progress, app.ProgressSuccess, "prepare", "Prepared temporary worktrees")
	}

	runErr := e.runActions(ctx, result, plan.Actions, runOpts)
	if requiresPreparation(plan.Actions) && opts.Parallel == 1 {
		emitProgress(opts.Progress, app.ProgressStart, "restore", "Restoring current worktree")
	}
	restoreErr := restore(ctx)
	if restoreErr != nil {
		emitProgress(opts.Progress, app.ProgressFailure, "restore", "Restore current worktree failed")
	} else if requiresPreparation(plan.Actions) && opts.Parallel == 1 {
		emitProgress(opts.Progress, app.ProgressSuccess, "restore", "Restored current worktree")
	}
	cleanupErr := cleanup()
	if runErr != nil || restoreErr != nil || cleanupErr != nil {
		emitProgress(opts.Progress, app.ProgressFailure, "execute", "Execution failed")
		return result, errors.Join(runErr, restoreErr, cleanupErr)
	}
	emitProgress(opts.Progress, app.ProgressSuccess, "execute", "Execution finished")
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

func (e Executor) prepareRun(ctx context.Context) (func(context.Context) error, error) {
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

type actionNode struct {
	action     app.Action
	dependents []int
	pending    int
	blocked    string
}

type actionCompletion struct {
	index  int
	result app.ActionResult
}

func (e Executor) runActions(ctx context.Context, result *app.RunResult, actions []app.Action, opts runOptions) error {
	nodes, hasDependents, err := buildActionGraph(actions)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		emitProgress(opts.Progress, app.ProgressSuccess, "actions", "No actions to execute")
		return nil
	}
	emitProgress(opts.Progress, app.ProgressStart, "actions", fmt.Sprintf("Executing %d actions", len(nodes)))

	results := make([]app.ActionResult, len(nodes))
	ready := make([]int, 0, len(nodes))
	for i := range nodes {
		if nodes[i].pending == 0 {
			ready = append(ready, i)
		}
	}

	completions := make(chan actionCompletion)
	running := 0
	completed := 0

	var complete func(actionCompletion)
	complete = func(completion actionCompletion) {
		if results[completion.index].Status != "" {
			return
		}
		results[completion.index] = completion.result
		completed++

		for _, dependent := range nodes[completion.index].dependents {
			if completion.result.Status != app.ActionStatusSuccess && nodes[dependent].blocked == "" {
				nodes[dependent].blocked = fmt.Sprintf("dependency %s did not succeed", completion.result.Action.ID)
			}
			nodes[dependent].pending--
			if nodes[dependent].pending != 0 {
				continue
			}
			if nodes[dependent].blocked != "" {
				completion := actionCompletion{
					index: dependent,
					result: app.ActionResult{
						Action: nodes[dependent].action,
						Status: app.ActionStatusSkipped,
						Error:  nodes[dependent].blocked,
					},
				}
				emitActionProgress(opts.Progress, app.ProgressSkipped, nodes[dependent].action, nodes[dependent].blocked)
				complete(completion)
				continue
			}
			ready = append(ready, dependent)
		}
	}

	for completed < len(nodes) {
		for running < opts.Parallel && len(ready) > 0 {
			index := ready[0]
			ready = ready[1:]
			running++
			emitActionProgress(opts.Progress, app.ProgressStart, nodes[index].action, actionRunningMessage(nodes[index].action))
			go func() {
				completions <- actionCompletion{
					index:  index,
					result: e.executeActionResult(ctx, nodes[index].action, opts, hasDependents[index]),
				}
			}()
		}
		if running == 0 {
			return fmt.Errorf("action dependency cycle detected")
		}
		completion := <-completions
		running--
		complete(completion)
	}

	result.Actions = results
	emitProgress(opts.Progress, app.ProgressSuccess, "actions", fmt.Sprintf("Executed %d actions", len(nodes)))
	return nil
}

func buildActionGraph(actions []app.Action) ([]actionNode, []bool, error) {
	nodes := make([]actionNode, len(actions))
	indexByID := make(map[string]int, len(actions))
	for i, action := range actions {
		if action.Kind != app.ActionRepairPR && action.Kind != app.ActionUpdateBranch {
			return nil, nil, fmt.Errorf("unsupported action kind: %s", action.Kind)
		}
		if _, exists := indexByID[action.ID]; exists {
			return nil, nil, fmt.Errorf("duplicate action ID: %s", action.ID)
		}
		indexByID[action.ID] = i
		nodes[i] = actionNode{action: action}
	}

	hasDependents := make([]bool, len(actions))
	for i, action := range actions {
		for _, dependencyID := range action.DependsOn {
			dependency, selected := indexByID[dependencyID]
			if !selected {
				continue
			}
			nodes[i].pending++
			nodes[dependency].dependents = append(nodes[dependency].dependents, i)
			hasDependents[dependency] = true
		}
	}
	return nodes, hasDependents, nil
}

func (e Executor) executeActionResult(ctx context.Context, action app.Action, opts runOptions, hasDependents bool) app.ActionResult {
	actionResult := app.ActionResult{
		Action: action,
		Status: app.ActionStatusSuccess,
	}
	if err := e.executeAction(ctx, action, opts); err != nil {
		actionResult.Status = app.ActionStatusFailed
		actionResult.Error = err.Error()
		emitActionProgress(opts.Progress, app.ProgressFailure, action, err.Error())
		return actionResult
	}
	if hasDependents {
		if err := e.settleAction(ctx, action, opts); err != nil {
			actionResult.Status = app.ActionStatusFailed
			actionResult.Error = err.Error()
			emitActionProgress(opts.Progress, app.ProgressFailure, action, err.Error())
			return actionResult
		}
	}
	emitActionProgress(opts.Progress, app.ProgressSuccess, action, actionSuccessMessage(action))
	return actionResult
}

func (e Executor) executeAction(ctx context.Context, action app.Action, opts runOptions) error {
	switch action.Kind {
	case app.ActionRepairPR:
		if opts.WorktreeDir != "" {
			return e.executeRepairInWorktree(ctx, action, opts)
		}
		return e.executeRepairInCurrentWorktree(ctx, action, opts)
	case app.ActionUpdateBranch:
		emitActionProgress(opts.Progress, app.ProgressLog, action, fmt.Sprintf("Updating branch for #%d", action.PR.Number))
		_, err := e.gh.Run(ctx, "pr", "update-branch", "--rebase", fmt.Sprint(action.PR.Number))
		return err
	default:
		return fmt.Errorf("unsupported action kind: %s", action.Kind)
	}
}

func (e Executor) executeRepairInCurrentWorktree(ctx context.Context, action app.Action, opts runOptions) error {
	head := action.PR.HeadRefName
	remoteHead := opts.Remote + "/" + head
	emitActionProgress(opts.Progress, app.ProgressLog, action, fmt.Sprintf("Switching to %s", head))
	if err := e.git.Switch(ctx, head); err != nil {
		if err := e.git.SwitchCreateOrReset(ctx, head, remoteHead); err != nil {
			return fmt.Errorf("switch to %s: %w", head, err)
		}
	}
	emitActionProgress(opts.Progress, app.ProgressLog, action, fmt.Sprintf("Checking %s against %s", head, remoteHead))
	relationship, err := e.git.AheadBehind(ctx, remoteHead, "HEAD")
	if err != nil {
		return fmt.Errorf("compare %s with HEAD: %w", remoteHead, err)
	}
	if relationship.Ahead > 0 {
		return fmt.Errorf("local branch %s has %d unpushed commit(s); refusing to force-push", head, relationship.Ahead)
	}
	emitActionProgress(opts.Progress, app.ProgressLog, action, fmt.Sprintf("Pulling %s", head))
	if err := e.git.PullRebase(ctx, opts.Remote, head); err != nil {
		return fmt.Errorf("pull %s: %w", head, err)
	}
	if err := e.rebase(ctx, e.git, action, opts.Remote, head, opts.Progress); err != nil {
		return err
	}
	emitActionProgress(opts.Progress, app.ProgressLog, action, fmt.Sprintf("Pushing %s", head))
	if err := e.git.PushForceWithLease(ctx, opts.Remote, head); err != nil {
		return fmt.Errorf("push %s: %w", head, err)
	}
	return e.updateBase(ctx, e.gh, action, opts.Progress)
}

func (e Executor) executeRepairInWorktree(ctx context.Context, action app.Action, opts runOptions) (err error) {
	head := action.PR.HeadRefName
	remoteHead := opts.Remote + "/" + head
	path := filepath.Join(opts.WorktreeDir, worktreeName(action))

	emitActionProgress(opts.Progress, app.ProgressLog, action, fmt.Sprintf("Creating worktree for %s", head))
	worktreeMu.Lock()
	if err := e.git.WorktreeAdd(ctx, path, remoteHead, "--detach"); err != nil {
		worktreeMu.Unlock()
		return fmt.Errorf("create worktree for %s: %w", head, err)
	}
	worktreeMu.Unlock()
	defer func() {
		worktreeMu.Lock()
		defer worktreeMu.Unlock()
		if cleanupErr := e.git.WorktreeRemove(context.Background(), path, true); cleanupErr != nil && err == nil {
			err = fmt.Errorf("remove worktree %s: %w", path, cleanupErr)
		}
	}()

	git := e.git.InDir(path)
	gh := e.gh.InDir(path)
	if err := e.rebase(ctx, git, action, opts.Remote, "", opts.Progress); err != nil {
		return err
	}
	emitActionProgress(opts.Progress, app.ProgressLog, action, fmt.Sprintf("Pushing %s", head))
	if err := git.PushForceWithLeaseRefspec(ctx, opts.Remote, "HEAD:"+head); err != nil {
		return fmt.Errorf("push %s: %w", head, err)
	}
	return e.updateBase(ctx, gh, action, opts.Progress)
}

func (e Executor) settleAction(ctx context.Context, action app.Action, opts runOptions) error {
	headRef := opts.Remote + "/" + action.PR.HeadRefName
	base := action.NewBase
	if base == "" {
		base = action.PR.BaseRefName
	}
	baseRef := opts.Remote + "/" + base
	refspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", action.PR.HeadRefName, opts.Remote, action.PR.HeadRefName)

	var lastErr error
	for attempt := range settleAttempts {
		emitActionProgress(opts.Progress, app.ProgressLog, action, fmt.Sprintf("Settling %s (%d/%d)", headRef, attempt+1, settleAttempts))
		if err := e.git.Fetch(ctx, opts.Remote, refspec); err != nil {
			lastErr = err
		} else if ok, err := e.git.IsAncestor(ctx, baseRef, headRef); err != nil {
			lastErr = err
		} else if ok {
			return nil
		} else {
			lastErr = fmt.Errorf("%s is not an ancestor of %s", baseRef, headRef)
		}

		if attempt+1 < settleAttempts {
			timer := time.NewTimer(settleDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return fmt.Errorf("settle %s: %w", headRef, lastErr)
}

func (e Executor) rebase(ctx context.Context, git gitrepo.Client, action app.Action, remote, branch string, progress app.ProgressSink) error {
	newBase := action.NewBase
	if newBase == "" {
		newBase = action.PR.BaseRefName
	}
	emitActionProgress(progress, app.ProgressLog, action, fmt.Sprintf("Rebasing %s onto %s/%s", action.PR.HeadRefName, remote, newBase))
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

func (e Executor) updateBase(ctx context.Context, gh ghcli.Client, action app.Action, progress app.ProgressSink) error {
	newBase := action.NewBase
	if newBase == "" || newBase == action.PR.BaseRefName {
		return nil
	}
	emitActionProgress(progress, app.ProgressLog, action, fmt.Sprintf("Updating base for #%d to %s", action.PR.Number, newBase))
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

func emitProgress(sink app.ProgressSink, kind app.ProgressKind, phase, message string) {
	app.EmitProgress(sink, app.ProgressEvent{
		Kind:    kind,
		Phase:   phase,
		Message: message,
	})
}

func emitActionProgress(sink app.ProgressSink, kind app.ProgressKind, action app.Action, message string) {
	app.EmitProgress(sink, app.ProgressEvent{
		Kind:     kind,
		Phase:    "action",
		Message:  message,
		PRNumber: action.PR.Number,
		ActionID: action.ID,
	})
}

func actionRunningMessage(action app.Action) string {
	switch action.Kind {
	case app.ActionRepairPR:
		target := action.NewBase
		if target == "" {
			target = action.PR.BaseRefName
		}
		return fmt.Sprintf("Repairing #%d %s onto %s", action.PR.Number, action.PR.HeadRefName, target)
	case app.ActionUpdateBranch:
		return fmt.Sprintf("Updating #%d %s", action.PR.Number, action.PR.HeadRefName)
	default:
		return fmt.Sprintf("Running %s for #%d", action.Kind, action.PR.Number)
	}
}

func actionSuccessMessage(action app.Action) string {
	switch action.Kind {
	case app.ActionRepairPR:
		return fmt.Sprintf("Repaired #%d %s", action.PR.Number, action.PR.HeadRefName)
	case app.ActionUpdateBranch:
		return fmt.Sprintf("Updated #%d %s", action.PR.Number, action.PR.HeadRefName)
	default:
		return fmt.Sprintf("Finished %s for #%d", action.Kind, action.PR.Number)
	}
}

var _ app.Executor = Executor{}

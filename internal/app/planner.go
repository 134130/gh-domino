package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/stackedpr"
)

type Planner struct {
	store RepositoryStore
}

func NewPlanner(store RepositoryStore) Planner {
	return Planner{store: store}
}

func (p Planner) BuildPlan(ctx context.Context, opts PlanOptions) (*Plan, error) {
	opts = opts.normalized()
	progress := opts.Progress

	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressStart,
		Phase:   "fetch",
		Message: fmt.Sprintf("Fetching %s", opts.Remote),
	})
	if err := p.store.Fetch(ctx, opts.Remote); err != nil {
		EmitProgress(progress, ProgressEvent{
			Kind:    ProgressFailure,
			Phase:   "fetch",
			Message: fmt.Sprintf("Fetch %s failed", opts.Remote),
		})
		return nil, fmt.Errorf("fetch %s: %w", opts.Remote, err)
	}
	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressSuccess,
		Phase:   "fetch",
		Message: fmt.Sprintf("Fetched %s", opts.Remote),
	})

	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressStart,
		Phase:   "open-prs",
		Message: "Loading open pull requests",
	})
	openPRs, err := p.store.OpenPullRequests(ctx, opts.Author)
	if err != nil {
		EmitProgress(progress, ProgressEvent{
			Kind:    ProgressFailure,
			Phase:   "open-prs",
			Message: "Load open pull requests failed",
		})
		return nil, fmt.Errorf("list pull requests: %w", err)
	}
	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressSuccess,
		Phase:   "open-prs",
		Message: fmt.Sprintf("Loaded %d open pull requests", len(openPRs)),
	})

	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressStart,
		Phase:   "merged-prs",
		Message: "Loading merged pull requests",
	})
	mergedPRs, err := p.store.MergedPullRequests(ctx, opts.Author, opts.MergedLimit)
	if err != nil {
		EmitProgress(progress, ProgressEvent{
			Kind:    ProgressFailure,
			Phase:   "merged-prs",
			Message: "Load merged pull requests failed",
		})
		return nil, fmt.Errorf("list merged pull requests: %w", err)
	}
	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressSuccess,
		Phase:   "merged-prs",
		Message: fmt.Sprintf("Loaded %d merged pull requests", len(mergedPRs)),
	})

	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressStart,
		Phase:   "head-shas",
		Message: fmt.Sprintf("Resolving %d branch heads", len(openPRs)),
	})
	headSHAs := make(map[string]string, len(openPRs))
	for _, pr := range openPRs {
		ref := fmt.Sprintf("%s/%s", opts.Remote, pr.HeadRefName)
		EmitProgress(progress, ProgressEvent{
			Kind:     ProgressLog,
			Phase:    "head-shas",
			Message:  fmt.Sprintf("Resolving %s", ref),
			PRNumber: pr.Number,
		})
		sha, err := p.store.RefSHA(ctx, ref)
		if err != nil {
			EmitProgress(progress, ProgressEvent{
				Kind:     ProgressFailure,
				Phase:    "head-shas",
				Message:  fmt.Sprintf("Resolve %s failed", ref),
				PRNumber: pr.Number,
			})
			return nil, fmt.Errorf("get SHA for %s: %w", pr.HeadRefName, err)
		}
		headSHAs[pr.HeadRefName] = sha
	}
	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressSuccess,
		Phase:   "head-shas",
		Message: fmt.Sprintf("Resolved %d branch heads", len(headSHAs)),
	})

	return p.Build(ctx, Snapshot{
		OpenPullRequests:   openPRs,
		MergedPullRequests: mergedPRs,
		HeadSHAs:           headSHAs,
	}, opts)
}

func (p Planner) Build(ctx context.Context, snapshot Snapshot, opts PlanOptions) (*Plan, error) {
	opts = opts.normalized()
	progress := opts.Progress

	headSHAs := cloneStringMap(snapshot.HeadSHAs)
	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressStart,
		Phase:   "tree",
		Message: "Building dependency tree",
	})
	defaultBranch, err := p.store.DefaultBranch(ctx, opts.Remote)
	if err != nil {
		EmitProgress(progress, ProgressEvent{
			Kind:    ProgressFailure,
			Phase:   "tree",
			Message: "Build dependency tree failed",
		})
		return nil, fmt.Errorf("get default branch: %w", err)
	}
	roots, err := p.buildDependencyTree(ctx, snapshot.OpenPullRequests, snapshot.MergedPullRequests, headSHAs, defaultBranch, opts)
	if err != nil {
		EmitProgress(progress, ProgressEvent{
			Kind:    ProgressFailure,
			Phase:   "tree",
			Message: "Build dependency tree failed",
		})
		return nil, err
	}
	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressSuccess,
		Phase:   "tree",
		Message: "Built dependency tree",
	})

	prByHead := make(map[string]gitobj.PullRequest, len(snapshot.OpenPullRequests))
	for _, pr := range snapshot.OpenPullRequests {
		if !isStackableHead(pr, defaultBranch) {
			continue
		}
		prByHead[pr.HeadRefName] = pr
	}

	mergedByHead := make(map[string]gitobj.PullRequest, len(snapshot.MergedPullRequests))
	for _, pr := range snapshot.MergedPullRequests {
		if !isStackableHead(pr, defaultBranch) {
			continue
		}
		mergedByHead[pr.HeadRefName] = pr
	}

	plan := &Plan{
		Roots:    roots,
		HeadSHAs: headSHAs,
	}

	actionByHead := map[string]string{}
	processed := map[int]bool{}

	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressStart,
		Phase:   "classify",
		Message: fmt.Sprintf("Classifying %d pull requests", len(snapshot.OpenPullRequests)),
	})
	var walk func(*stackedpr.Node) error
	walk = func(node *stackedpr.Node) error {
		if node == nil {
			return nil
		}

		pr := node.Value
		if processed[pr.Number] {
			return nil
		}
		processed[pr.Number] = true

		status, err := p.classify(ctx, pr, node.OriginalBase, prByHead, mergedByHead, headSHAs, snapshot.MergedPullRequests, defaultBranch, opts)
		if err != nil {
			return err
		}
		EmitProgress(progress, ProgressEvent{
			Kind:     ProgressLog,
			Phase:    "classify",
			Message:  fmt.Sprintf("Classified #%d as %s", pr.Number, status.State),
			PRNumber: pr.Number,
		})
		plan.Pulls = append(plan.Pulls, status)

		var action *Action
		switch status.State {
		case PullStateBroken:
			action = &Action{
				ID:       fmt.Sprintf("repair-pr-%d", pr.Number),
				Kind:     ActionRepairPR,
				PR:       pr,
				NewBase:  status.NewBase,
				Upstream: status.Upstream,
				Reason:   status.Reason,
			}

		case PullStateUpdateable:
			if opts.IncludeClean {
				action = &Action{
					ID:     fmt.Sprintf("update-branch-%d", pr.Number),
					Kind:   ActionUpdateBranch,
					PR:     pr,
					Reason: status.Reason,
				}
			}
		}

		if action != nil {
			if parentActionID := actionByHead[pr.BaseRefName]; parentActionID != "" {
				action.DependsOn = append(action.DependsOn, parentActionID)
			}
			plan.Actions = append(plan.Actions, *action)
			actionByHead[pr.HeadRefName] = action.ID
		}

		for _, child := range node.Children {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}

	for _, root := range roots {
		if err := walk(root); err != nil {
			EmitProgress(progress, ProgressEvent{
				Kind:    ProgressFailure,
				Phase:   "classify",
				Message: "Classify pull requests failed",
			})
			return nil, err
		}
	}
	EmitProgress(progress, ProgressEvent{
		Kind:    ProgressSuccess,
		Phase:   "classify",
		Message: fmt.Sprintf("Classified %d pull requests", len(plan.Pulls)),
	})

	return plan, nil
}

func (p Planner) classify(
	ctx context.Context,
	pr gitobj.PullRequest,
	originalBase *gitobj.PullRequest,
	prByHead map[string]gitobj.PullRequest,
	mergedByHead map[string]gitobj.PullRequest,
	headSHAs map[string]string,
	mergedPRs []gitobj.PullRequest,
	defaultBranch string,
	opts PlanOptions,
) (PullStatus, error) {
	status := PullStatus{
		PR:           pr,
		OriginalBase: originalBase,
		State:        PullStateClean,
	}

	state, reason, newBase, upstream, err := p.determinePRState(
		ctx,
		pr,
		originalBase,
		prByHead,
		mergedByHead,
		headSHAs,
		mergedPRs,
		defaultBranch,
		opts,
	)
	if err != nil {
		return status, err
	}

	switch state {
	case PullStateBroken:
		if newBase == "" {
			newBase = pr.BaseRefName
		}
		status.State = PullStateBroken
		status.Reason = reason
		status.NewBase = newBase
		status.Upstream = upstream
		return status, nil

	case PullStateUpdateable:
		if reason == "" {
			reason = ReasonBaseStale
		}
		status.State = PullStateUpdateable
		status.Reason = reason
		return status, nil
	}

	if opts.IncludeClean {
		status.State = PullStateUpdateable
		status.Reason = ReasonRebaseAll
	}

	return status, nil
}

func (p Planner) determinePRState(
	ctx context.Context,
	pr gitobj.PullRequest,
	originalBase *gitobj.PullRequest,
	prByHead map[string]gitobj.PullRequest,
	mergedByHead map[string]gitobj.PullRequest,
	headSHAs map[string]string,
	mergedPRs []gitobj.PullRequest,
	defaultBranch string,
	opts PlanOptions,
) (state PullState, reason Reason, newBase string, upstream string, err error) {
	if originalBase != nil {
		upstream, err := p.squashUpstreamForChild(ctx, pr, *originalBase)
		if err != nil {
			return PullStateClean, ReasonNone, "", "", err
		}
		reason := ReasonMergedAncestor
		if originalBase.HeadRefName == pr.BaseRefName {
			reason = ReasonMergedBase
		}
		return PullStateBroken, reason, originalBase.BaseRefName, upstream, nil
	}

	if _, isStackedPR := prByHead[pr.BaseRefName]; !isStackedPR {
		if mergedBasePR, isMerged := mergedByHead[pr.BaseRefName]; isMerged {
			upstream, err := p.squashUpstreamForChild(ctx, pr, mergedBasePR)
			if err != nil {
				return PullStateClean, ReasonNone, "", "", err
			}
			return PullStateBroken, ReasonMergedBase, mergedBasePR.BaseRefName, upstream, nil
		}
	}

	if _, ok := prByHead[pr.BaseRefName]; ok {
		baseShaOnOrigin, err := p.store.RefSHA(ctx, fmt.Sprintf("%s/%s", opts.Remote, pr.BaseRefName))
		if err != nil {
			return PullStateClean, ReasonNone, "", "", fmt.Errorf("get SHA for base %s: %w", pr.BaseRefName, err)
		}
		headSha := headSHAs[pr.HeadRefName]
		mergeBase, err := p.store.MergeBase(ctx, fmt.Sprintf("%s/%s", opts.Remote, pr.BaseRefName), headSha)
		if err != nil {
			return PullStateClean, ReasonNone, "", "", fmt.Errorf("get merge base for %s and %s: %w", pr.BaseRefName, pr.HeadRefName, err)
		}
		if mergeBase != baseShaOnOrigin {
			return PullStateBroken, ReasonParentDiverged, "", "", nil
		}
	}

	if pr.BaseRefName == defaultBranch {
		headSha := headSHAs[pr.HeadRefName]
		mergeBase, err := p.store.MergeBase(ctx, fmt.Sprintf("%s/%s", opts.Remote, defaultBranch), headSha)
		if err != nil {
			return PullStateClean, ReasonNone, "", "", fmt.Errorf("get merge base for %s: %w", pr.HeadRefName, err)
		}

		for _, mergedPR := range mergedPRs {
			if mergedPR.BaseRefName != pr.BaseRefName || !isStackableHead(mergedPR, defaultBranch) {
				continue
			}
			for _, commit := range mergedPR.Commits {
				if mergeBase == commit.Oid {
					return PullStateBroken, ReasonMergedAncestor, mergedPR.BaseRefName, "", nil
				}
			}
		}
	}

	if _, ok := prByHead[pr.BaseRefName]; !ok {
		state, reason, err := p.determineBaseBranchFreshness(ctx, pr, headSHAs, opts)
		if err != nil {
			return PullStateClean, ReasonNone, "", "", err
		}
		if state != PullStateClean {
			return state, reason, "", "", nil
		}
	}

	return PullStateClean, ReasonNone, "", "", nil
}

func (p Planner) determineBaseBranchFreshness(
	ctx context.Context,
	pr gitobj.PullRequest,
	headSHAs map[string]string,
	opts PlanOptions,
) (PullState, Reason, error) {
	baseRef := fmt.Sprintf("%s/%s", opts.Remote, pr.BaseRefName)
	baseSha, err := p.store.RefSHA(ctx, baseRef)
	if err != nil {
		return PullStateClean, ReasonNone, fmt.Errorf("get SHA for base %s: %w", pr.BaseRefName, err)
	}
	headSha := headSHAs[pr.HeadRefName]
	mergeBase, err := p.store.MergeBase(ctx, baseRef, headSha)
	if err != nil {
		return PullStateClean, ReasonNone, fmt.Errorf("get merge base for %s: %w", pr.HeadRefName, err)
	}
	if mergeBase != baseSha {
		return PullStateUpdateable, ReasonBaseStale, nil
	}
	return PullStateClean, ReasonNone, nil
}

func (p Planner) squashUpstream(ctx context.Context, pr gitobj.PullRequest) (string, error) {
	if pr.MergeCommit.Sha == "" || len(pr.Commits) == 0 {
		return "", nil
	}

	for _, commit := range pr.Commits {
		isAncestor, err := p.isAncestor(ctx, commit.Oid, pr.MergeCommit.Sha)
		if err != nil {
			return "", fmt.Errorf("check ancestry for commit %s: %w", commit.Oid, err)
		}
		if isAncestor {
			return "", nil
		}
	}

	return pr.Commits[len(pr.Commits)-1].Oid, nil
}

func (p Planner) squashUpstreamForChild(ctx context.Context, child, base gitobj.PullRequest) (string, error) {
	upstream, err := p.squashUpstream(ctx, base)
	if err != nil || upstream == "" {
		return upstream, err
	}

	end, ok := matchingCommitSequenceEnd(child.Commits, base.Commits)
	if !ok || end >= len(child.Commits)-1 {
		return upstream, nil
	}
	return child.Commits[end].Oid, nil
}

func (p Planner) buildDependencyTree(
	ctx context.Context,
	prs []gitobj.PullRequest,
	mergedPRs []gitobj.PullRequest,
	headSHAs map[string]string,
	defaultBranch string,
	opts PlanOptions,
) ([]*stackedpr.Node, error) {
	prMap := make(map[string]*stackedpr.Node, len(prs))
	stackParentMap := make(map[string]*stackedpr.Node, len(prs))
	isChild := make(map[string]bool, len(prs))
	mergedByHead := make(map[string]gitobj.PullRequest, len(mergedPRs))
	for _, pr := range mergedPRs {
		if !isStackableHead(pr, defaultBranch) {
			continue
		}
		mergedByHead[pr.HeadRefName] = pr
	}

	for _, pr := range prs {
		node := &stackedpr.Node{Value: pr}
		prMap[pr.HeadRefName] = node
		if isStackableHead(pr, defaultBranch) {
			stackParentMap[pr.HeadRefName] = node
		}
	}

	for _, pr := range prs {
		if pr.BaseRefName == pr.HeadRefName {
			continue
		}
		if parent, ok := stackParentMap[pr.BaseRefName]; ok {
			node := prMap[pr.HeadRefName]
			parent.Children = append(parent.Children, node)
			isChild[pr.HeadRefName] = true
		}
	}

	var roots []*stackedpr.Node
	for _, pr := range prs {
		if !isChild[pr.HeadRefName] {
			roots = append(roots, prMap[pr.HeadRefName])
		}
	}

	for _, node := range prMap {
		if _, ok := stackParentMap[node.Value.BaseRefName]; ok {
			continue
		}

		if mergedPR, ok := mergedByHead[node.Value.BaseRefName]; ok {
			node.OriginalBase = &mergedPR
			continue
		}

		if node.Value.BaseRefName != defaultBranch {
			continue
		}

		baseRef := fmt.Sprintf("%s/%s", opts.Remote, node.Value.BaseRefName)
		baseIsAncestor, err := p.isAncestor(ctx, baseRef, headSHAs[node.Value.HeadRefName])
		if err != nil {
			return nil, fmt.Errorf("check ancestry for %s: %w", baseRef, err)
		}
		if baseIsAncestor {
			continue
		}

		originalBase, err := p.bestMergedOriginalBase(ctx, node.Value, mergedPRs, headSHAs[node.Value.HeadRefName])
		if err != nil {
			return nil, err
		}
		if originalBase != nil {
			node.OriginalBase = originalBase
		}
	}

	return roots, nil
}

func (p Planner) bestMergedOriginalBase(
	ctx context.Context,
	pr gitobj.PullRequest,
	mergedPRs []gitobj.PullRequest,
	headSHA string,
) (*gitobj.PullRequest, error) {
	if len(pr.Commits) > 1 {
		bestIndex := -1
		bestEnd := -1
		for i := range mergedPRs {
			mergedPR := mergedPRs[i]
			if !sameDefaultBaseCandidate(pr, mergedPR) {
				continue
			}
			end, ok := matchingCommitSequenceEnd(pr.Commits, mergedPR.Commits)
			if !ok || end >= len(pr.Commits)-1 {
				continue
			}
			if bestIndex < 0 || end > bestEnd || end == bestEnd && newerPullNumber(mergedPR, mergedPRs[bestIndex]) {
				bestIndex = i
				bestEnd = end
			}
		}
		if bestIndex >= 0 {
			return &mergedPRs[bestIndex], nil
		}
	}

	bestIndex := -1
	for i := range mergedPRs {
		mergedPR := mergedPRs[i]
		if !sameDefaultBaseCandidate(pr, mergedPR) {
			continue
		}

		ancestorCommit := mergedPR.Commits[len(mergedPR.Commits)-1].Oid
		isAncestor, err := p.isAncestor(ctx, ancestorCommit, headSHA)
		if err != nil {
			return nil, fmt.Errorf("check ancestry for commit %s: %w", ancestorCommit, err)
		}
		if !isAncestor {
			continue
		}

		if bestIndex < 0 || newerPullNumber(mergedPR, mergedPRs[bestIndex]) {
			bestIndex = i
		}
	}
	if bestIndex >= 0 {
		return &mergedPRs[bestIndex], nil
	}
	return nil, nil
}

func sameDefaultBaseCandidate(pr, mergedPR gitobj.PullRequest) bool {
	if len(mergedPR.Commits) == 0 {
		return false
	}
	if mergedPR.BaseRefName != pr.BaseRefName {
		return false
	}
	if pr.Number > 0 && mergedPR.Number >= pr.Number {
		return false
	}
	return true
}

func newerPullNumber(a, b gitobj.PullRequest) bool {
	return a.Number > b.Number
}

func matchingCommitSequenceEnd(target, pattern []gitobj.PullRequestCommit) (int, bool) {
	if len(target) == 0 || len(pattern) == 0 || len(pattern) > len(target) {
		return -1, false
	}

	bestEnd := -1
	for start := 0; start <= len(target)-len(pattern); start++ {
		if commitSequencesMatch(target[start:start+len(pattern)], pattern, sameCommitOID) ||
			commitSequencesMatch(target[start:start+len(pattern)], pattern, sameCommitHeadline) {
			bestEnd = start + len(pattern) - 1
		}
	}

	if bestEnd < 0 {
		return -1, false
	}
	return bestEnd, true
}

func commitSequencesMatch(
	target []gitobj.PullRequestCommit,
	pattern []gitobj.PullRequestCommit,
	same func(gitobj.PullRequestCommit, gitobj.PullRequestCommit) bool,
) bool {
	for i := range pattern {
		if !same(target[i], pattern[i]) {
			return false
		}
	}
	return true
}

func sameCommitOID(a, b gitobj.PullRequestCommit) bool {
	return a.Oid != "" && a.Oid == b.Oid
}

func sameCommitHeadline(a, b gitobj.PullRequestCommit) bool {
	return a.MessageHeadline != "" && a.MessageHeadline == b.MessageHeadline
}

func isStackableHead(pr gitobj.PullRequest, defaultBranch string) bool {
	return pr.HeadRefName != "" && pr.HeadRefName != defaultBranch
}

func (p Planner) isAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	isAncestor, err := p.store.IsAncestor(ctx, ancestor, descendant)
	if err == nil {
		return isAncestor, nil
	}
	if isMissingCommitError(err) {
		return false, nil
	}
	return false, err
}

func isMissingCommitError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "Not a valid commit name") ||
		strings.Contains(message, "unknown revision") ||
		strings.Contains(message, "bad revision") ||
		strings.Contains(message, "ambiguous argument")
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

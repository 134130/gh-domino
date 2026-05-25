package app

import (
	"context"
	"fmt"

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
	if err := p.store.Fetch(ctx, opts.Remote); err != nil {
		return nil, fmt.Errorf("fetch %s: %w", opts.Remote, err)
	}

	openPRs, err := p.store.OpenPullRequests(ctx, opts.Author)
	if err != nil {
		return nil, fmt.Errorf("list pull requests: %w", err)
	}

	mergedPRs, err := p.store.MergedPullRequests(ctx, opts.Author, opts.MergedLimit)
	if err != nil {
		return nil, fmt.Errorf("list merged pull requests: %w", err)
	}

	headSHAs := make(map[string]string, len(openPRs))
	for _, pr := range openPRs {
		ref := fmt.Sprintf("%s/%s", opts.Remote, pr.HeadRefName)
		sha, err := p.store.RefSHA(ctx, ref)
		if err != nil {
			return nil, fmt.Errorf("get SHA for %s: %w", pr.HeadRefName, err)
		}
		headSHAs[pr.HeadRefName] = sha
	}

	return p.Build(ctx, Snapshot{
		OpenPullRequests:   openPRs,
		MergedPullRequests: mergedPRs,
		HeadSHAs:           headSHAs,
	}, opts)
}

func (p Planner) Build(ctx context.Context, snapshot Snapshot, opts PlanOptions) (*Plan, error) {
	opts = opts.normalized()

	headSHAs := cloneStringMap(snapshot.HeadSHAs)
	roots, err := p.buildDependencyTree(ctx, snapshot.OpenPullRequests, snapshot.MergedPullRequests, headSHAs, opts)
	if err != nil {
		return nil, err
	}

	prByHead := make(map[string]gitobj.PullRequest, len(snapshot.OpenPullRequests))
	for _, pr := range snapshot.OpenPullRequests {
		prByHead[pr.HeadRefName] = pr
	}

	mergedByHead := make(map[string]gitobj.PullRequest, len(snapshot.MergedPullRequests))
	for _, pr := range snapshot.MergedPullRequests {
		mergedByHead[pr.HeadRefName] = pr
	}

	plan := &Plan{
		Roots:    roots,
		HeadSHAs: headSHAs,
	}

	actionByHead := map[string]string{}
	processed := map[int]bool{}

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

		status, err := p.classify(ctx, pr, node.OriginalBase, prByHead, mergedByHead, headSHAs, snapshot.MergedPullRequests, opts)
		if err != nil {
			return err
		}
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
			action = &Action{
				ID:     fmt.Sprintf("update-branch-%d", pr.Number),
				Kind:   ActionUpdateBranch,
				PR:     pr,
				Reason: status.Reason,
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
			return nil, err
		}
	}

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
	opts PlanOptions,
) (PullStatus, error) {
	status := PullStatus{
		PR:           pr,
		OriginalBase: originalBase,
		State:        PullStateClean,
	}

	isBroken, reason, newBase, upstream, err := p.determinePRState(
		ctx,
		pr,
		originalBase,
		prByHead,
		mergedByHead,
		headSHAs,
		mergedPRs,
		opts,
	)
	if err != nil {
		return status, err
	}

	if isBroken {
		if newBase == "" {
			newBase = pr.BaseRefName
		}
		status.State = PullStateBroken
		status.Reason = reason
		status.NewBase = newBase
		status.Upstream = upstream
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
	opts PlanOptions,
) (isBroken bool, reason Reason, newBase string, upstream string, err error) {
	if originalBase != nil {
		upstream, err := p.squashUpstream(ctx, *originalBase)
		if err != nil {
			return false, ReasonNone, "", "", err
		}
		reason := ReasonMergedAncestor
		if originalBase.HeadRefName == pr.BaseRefName {
			reason = ReasonMergedBase
		}
		return true, reason, originalBase.BaseRefName, upstream, nil
	}

	if _, isStackedPR := prByHead[pr.BaseRefName]; !isStackedPR {
		if mergedBasePR, isMerged := mergedByHead[pr.BaseRefName]; isMerged {
			upstream, err := p.squashUpstream(ctx, mergedBasePR)
			if err != nil {
				return false, ReasonNone, "", "", err
			}
			return true, ReasonMergedBase, mergedBasePR.BaseRefName, upstream, nil
		}
	}

	if _, ok := prByHead[pr.BaseRefName]; ok {
		baseShaOnOrigin, err := p.store.RefSHA(ctx, fmt.Sprintf("%s/%s", opts.Remote, pr.BaseRefName))
		if err != nil {
			return false, ReasonNone, "", "", fmt.Errorf("get SHA for base %s: %w", pr.BaseRefName, err)
		}
		headSha := headSHAs[pr.HeadRefName]
		mergeBase, err := p.store.MergeBase(ctx, fmt.Sprintf("%s/%s", opts.Remote, pr.BaseRefName), headSha)
		if err != nil {
			return false, ReasonNone, "", "", fmt.Errorf("get merge base for %s and %s: %w", pr.BaseRefName, pr.HeadRefName, err)
		}
		if mergeBase != baseShaOnOrigin {
			return true, ReasonParentDiverged, "", "", nil
		}
	}

	defaultBranch, err := p.store.DefaultBranch(ctx, opts.Remote)
	if err != nil {
		return false, ReasonNone, "", "", fmt.Errorf("get default branch: %w", err)
	}
	if pr.BaseRefName == defaultBranch {
		headSha := headSHAs[pr.HeadRefName]
		mergeBase, err := p.store.MergeBase(ctx, fmt.Sprintf("%s/%s", opts.Remote, defaultBranch), headSha)
		if err != nil {
			return false, ReasonNone, "", "", fmt.Errorf("get merge base for %s: %w", pr.HeadRefName, err)
		}

		for _, mergedPR := range mergedPRs {
			for _, commit := range mergedPR.Commits {
				if mergeBase == commit.Oid {
					return true, ReasonMergedAncestor, mergedPR.BaseRefName, "", nil
				}
			}
		}
	}

	return false, ReasonNone, "", "", nil
}

func (p Planner) squashUpstream(ctx context.Context, pr gitobj.PullRequest) (string, error) {
	if pr.MergeCommit.Sha == "" || len(pr.Commits) == 0 {
		return "", nil
	}

	for _, commit := range pr.Commits {
		isAncestor, err := p.store.IsAncestor(ctx, commit.Oid, pr.MergeCommit.Sha)
		if err != nil {
			return "", fmt.Errorf("check ancestry for commit %s: %w", commit.Oid, err)
		}
		if isAncestor {
			return "", nil
		}
	}

	return pr.Commits[len(pr.Commits)-1].Oid, nil
}

func (p Planner) buildDependencyTree(
	ctx context.Context,
	prs []gitobj.PullRequest,
	mergedPRs []gitobj.PullRequest,
	headSHAs map[string]string,
	opts PlanOptions,
) ([]*stackedpr.Node, error) {
	prMap := make(map[string]*stackedpr.Node, len(prs))
	isChild := make(map[string]bool, len(prs))
	mergedByHead := make(map[string]gitobj.PullRequest, len(mergedPRs))
	for _, pr := range mergedPRs {
		mergedByHead[pr.HeadRefName] = pr
	}

	for _, pr := range prs {
		prMap[pr.HeadRefName] = &stackedpr.Node{Value: pr}
	}

	for _, pr := range prs {
		if pr.BaseRefName == pr.HeadRefName {
			continue
		}
		if parent, ok := prMap[pr.BaseRefName]; ok {
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

	defaultBranch := ""
	for _, node := range prMap {
		if _, ok := prMap[node.Value.BaseRefName]; ok {
			continue
		}

		if mergedPR, ok := mergedByHead[node.Value.BaseRefName]; ok {
			node.OriginalBase = &mergedPR
			continue
		}

		if defaultBranch == "" {
			var err error
			defaultBranch, err = p.store.DefaultBranch(ctx, opts.Remote)
			if err != nil {
				return nil, fmt.Errorf("get default branch: %w", err)
			}
		}

		if node.Value.BaseRefName != defaultBranch {
			continue
		}

		for i, mergedPR := range mergedPRs {
			if len(mergedPR.Commits) == 0 {
				continue
			}
			if mergedPR.BaseRefName != node.Value.BaseRefName {
				continue
			}

			ancestorCommit := mergedPR.Commits[0].Oid
			isAncestor, err := p.store.IsAncestor(ctx, ancestorCommit, headSHAs[node.Value.HeadRefName])
			if err == nil && isAncestor {
				node.OriginalBase = &mergedPRs[i]
				break
			}
			if err != nil {
				return nil, fmt.Errorf("check ancestry for commit %s: %w", ancestorCommit, err)
			}
		}
	}

	return roots, nil
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

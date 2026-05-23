package app

import (
	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/stackedpr"
)

type PullState string

const (
	PullStateClean      PullState = "clean"
	PullStateBroken     PullState = "broken"
	PullStateUpdateable PullState = "updateable"
)

type Reason string

const (
	ReasonNone           Reason = ""
	ReasonMergedBase     Reason = "merged_base"
	ReasonParentDiverged Reason = "parent_diverged"
	ReasonMergedAncestor Reason = "merged_ancestor"
	ReasonRebaseAll      Reason = "rebase_all"
)

type ActionKind string

const (
	ActionRepairPR     ActionKind = "repair_pr"
	ActionUpdateBranch ActionKind = "update_branch"
)

type Action struct {
	ID        string
	Kind      ActionKind
	PR        gitobj.PullRequest
	NewBase   string
	Upstream  string
	Reason    Reason
	DependsOn []string
}

type PullStatus struct {
	PR           gitobj.PullRequest
	OriginalBase *gitobj.PullRequest
	State        PullState
	Reason       Reason
	NewBase      string
	Upstream     string
}

type Plan struct {
	Roots    []*stackedpr.Node
	Pulls    []PullStatus
	Actions  []Action
	HeadSHAs map[string]string
}

type Snapshot struct {
	OpenPullRequests   []gitobj.PullRequest
	MergedPullRequests []gitobj.PullRequest
	HeadSHAs           map[string]string
}

type PlanOptions struct {
	Remote       string
	Author       string
	MergedLimit  int
	IncludeClean bool
}

func (o PlanOptions) normalized() PlanOptions {
	if o.Remote == "" {
		o.Remote = "origin"
	}
	if o.Author == "" {
		o.Author = "@me"
	}
	if o.MergedLimit == 0 {
		o.MergedLimit = 30
	}
	return o
}

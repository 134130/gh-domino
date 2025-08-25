package domino

import (
	"fmt"

	"github.com/134130/gh-domino/internal/color"
	"github.com/134130/gh-domino/internal/stackedpr"
)

type ErrRebaseConflict struct {
	BrokenPR stackedpr.RebaseInfo
}

var _ error = (*ErrRebaseConflict)(nil)

func (e *ErrRebaseConflict) Error() string {
	return fmt.Sprintf("failed to rebase %s onto origin/%s", e.BrokenPR.PR.HeadRefName, e.BrokenPR.NewBase)
}

func (e *ErrRebaseConflict) Command() string {
	newBase := color.Cyan(fmt.Sprintf("origin/%s", e.BrokenPR.NewBase))
	branch := color.Blue(e.BrokenPR.PR.HeadRefName)
	if e.BrokenPR.Upstream != "" {
		upstream := color.Yellow(e.BrokenPR.Upstream)
		return fmt.Sprintf("git rebase --onto %s %s %s", newBase, upstream, branch)
	}
	return fmt.Sprintf("git rebase %s %s", newBase, branch)
}

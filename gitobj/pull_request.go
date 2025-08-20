package gitobj

import (
	"fmt"

	"github.com/134130/gh-domino/internal/color"
)

type PullRequestState string

const (
	PullRequestStateOpen   PullRequestState = "OPEN"
	PullRequestStateClosed PullRequestState = "CLOSED"
	PullRequestStateMerged PullRequestState = "MERGED"
)

type Repository struct {
	Owner struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name string `json:"name"`
}

type PullRequest struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Url    string `json:"url"`
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	State       PullRequestState `json:"state"`
	IsDraft     bool             `json:"isDraft"`
	MergeCommit struct {
		Sha string `json:"oid"`
	} `json:"mergeCommit"`
	BaseRefName string `json:"baseRefName"`
	HeadRefName string `json:"headRefName"`
}

func (pr PullRequest) StateString() string {
	switch pr.State {
	case PullRequestStateOpen:
		return color.Green("open")
	case PullRequestStateClosed:
		return color.Red("closed")
	case PullRequestStateMerged:
		return color.Purple("merged")
	default:
		if pr.IsDraft {
			return color.Grey("draft")
		}
		return "UNKNOWN"
	}
}

func (pr PullRequest) PRNumberString() string {
	str := fmt.Sprintf("#%d", pr.Number)
	switch pr.State {
	case PullRequestStateOpen:
		return color.Green(str)
	case PullRequestStateClosed:
		return color.Red(str)
	case PullRequestStateMerged:
		return color.Purple(str)
	default:
		if pr.IsDraft {
			return color.Grey(str)
		}
		return "UNKNOWN"
	}
}

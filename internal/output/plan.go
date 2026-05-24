package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/stackedpr"
)

type Format string

const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
)

type ListOptions struct {
	Format Format
	State  string
	Flat   bool
}

func RenderList(w io.Writer, plan *app.Plan, opts ListOptions) error {
	if opts.Format == FormatJSON {
		return renderListJSON(w, plan)
	}
	if opts.Flat {
		return renderListFlat(w, plan, opts.State)
	}
	return renderListTree(w, plan, opts.State)
}

func RenderPlan(w io.Writer, plan *app.Plan, format Format) error {
	if format == FormatJSON {
		return renderPlanJSON(w, plan)
	}

	if _, err := fmt.Fprintln(w, "Actions"); err != nil {
		return err
	}
	if len(plan.Actions) == 0 {
		_, err := fmt.Fprintln(w, "  No actions.")
		return err
	}
	for _, action := range plan.Actions {
		if _, err := fmt.Fprintf(w, "  %s\n", actionLine(action)); err != nil {
			return err
		}
	}
	if len(plan.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, ""); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Warnings"); err != nil {
			return err
		}
		for _, warning := range plan.Warnings {
			if _, err := fmt.Fprintf(w, "  %s\n", warningLine(warning)); err != nil {
				return err
			}
		}
	}
	return nil
}

func RenderRunResult(w io.Writer, result *app.RunResult, format Format) error {
	if format == FormatJSON {
		return renderRunResultJSON(w, result)
	}

	if _, err := fmt.Fprintln(w, "Results"); err != nil {
		return err
	}
	if len(result.Actions) == 0 {
		if _, err := fmt.Fprintln(w, "  No actions."); err != nil {
			return err
		}
	}
	for _, action := range result.Actions {
		line := fmt.Sprintf("%s %s", action.Status, actionLine(action.Action))
		if action.Error != "" {
			line += ": " + action.Error
		}
		if _, err := fmt.Fprintf(w, "  %s\n", line); err != nil {
			return err
		}
	}
	if len(result.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, ""); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(w, "Warnings"); err != nil {
			return err
		}
		for _, warning := range result.Warnings {
			if _, err := fmt.Fprintf(w, "  %s\n", warningLine(warning)); err != nil {
				return err
			}
		}
	}
	return nil
}

func renderListTree(w io.Writer, plan *app.Plan, state string) error {
	statuses := statusesByPR(plan.Pulls)
	if _, err := fmt.Fprintln(w, "Pull Requests"); err != nil {
		return err
	}
	for i, root := range plan.Roots {
		if err := renderTreeNode(w, root, statuses, state, "", i == len(plan.Roots)-1); err != nil {
			return err
		}
	}
	return nil
}

func renderTreeNode(w io.Writer, node *stackedpr.Node, statuses map[int]app.PullStatus, state, prefix string, last bool) error {
	if !shouldRenderNode(node, statuses, state) {
		return nil
	}

	connector := "├─ "
	childPrefix := prefix + "│  "
	if last {
		connector = "└─ "
		childPrefix = prefix + "   "
	}
	if _, err := fmt.Fprintf(w, "%s%s%s\n", prefix, connector, pullLine(statuses[node.Value.Number])); err != nil {
		return err
	}

	children := visibleChildren(node.Children, statuses, state)
	for i, child := range children {
		if err := renderTreeNode(w, child, statuses, state, childPrefix, i == len(children)-1); err != nil {
			return err
		}
	}
	return nil
}

func renderListFlat(w io.Writer, plan *app.Plan, state string) error {
	if _, err := fmt.Fprintln(w, "Pull Requests"); err != nil {
		return err
	}
	for _, status := range plan.Pulls {
		if !matchesState(status, state) {
			continue
		}
		if _, err := fmt.Fprintf(w, "  %s\n", pullLine(status)); err != nil {
			return err
		}
	}
	return nil
}

func visibleChildren(nodes []*stackedpr.Node, statuses map[int]app.PullStatus, state string) []*stackedpr.Node {
	children := make([]*stackedpr.Node, 0, len(nodes))
	for _, node := range nodes {
		if shouldRenderNode(node, statuses, state) {
			children = append(children, node)
		}
	}
	return children
}

func shouldRenderNode(node *stackedpr.Node, statuses map[int]app.PullStatus, state string) bool {
	if matchesState(statuses[node.Value.Number], state) {
		return true
	}
	for _, child := range node.Children {
		if shouldRenderNode(child, statuses, state) {
			return true
		}
	}
	return false
}

func matchesState(status app.PullStatus, state string) bool {
	switch state {
	case "", "all":
		return true
	case "broken":
		return status.State == app.PullStateBroken
	case "clean":
		return status.State == app.PullStateClean
	case "updateable":
		return status.State == app.PullStateUpdateable
	default:
		return false
	}
}

func pullLine(status app.PullStatus) string {
	pr := status.PR
	suffix := ""
	switch status.State {
	case app.PullStateBroken:
		if status.NewBase != "" {
			suffix = fmt.Sprintf(" [broken: %s -> %s]", status.Reason, status.NewBase)
		} else {
			suffix = fmt.Sprintf(" [broken: %s]", status.Reason)
		}
	case app.PullStateUpdateable:
		suffix = " [updateable]"
	}
	if status.OriginalBase != nil {
		suffix += fmt.Sprintf(" [was on #%d]", status.OriginalBase.Number)
	}
	return fmt.Sprintf("#%d %s (%s <- %s)%s", pr.Number, pr.Title, pr.BaseRefName, pr.HeadRefName, suffix)
}

func actionLine(action app.Action) string {
	pr := action.PR
	switch action.Kind {
	case app.ActionRepairPR:
		parts := []string{fmt.Sprintf("repair #%d %s onto %s", pr.Number, pr.HeadRefName, action.NewBase)}
		if action.Upstream != "" {
			parts = append(parts, fmt.Sprintf("(upstream %s)", action.Upstream))
		}
		return strings.Join(parts, " ")
	case app.ActionUpdateBranch:
		return fmt.Sprintf("update-branch #%d %s", pr.Number, pr.HeadRefName)
	default:
		return fmt.Sprintf("%s #%d %s", action.Kind, pr.Number, pr.HeadRefName)
	}
}

func warningLine(warning app.SelectionWarning) string {
	switch warning.Kind {
	case app.SelectionWarningUnselectedDependency:
		if warning.DependencyPR != 0 {
			return fmt.Sprintf(
				"#%d has an unselected related action: %s. Use --chain or select it explicitly.",
				warning.PRNumber,
				dependencyActionLine(warning),
			)
		}
		return fmt.Sprintf(
			"#%d has an unselected related action %s. Use --chain or select it explicitly.",
			warning.PRNumber,
			warning.DependencyID,
		)
	default:
		return fmt.Sprintf("%s for #%d", warning.Kind, warning.PRNumber)
	}
}

func dependencyActionLine(warning app.SelectionWarning) string {
	switch warning.DependencyKind {
	case app.ActionRepairPR:
		if warning.DependencyBase != "" {
			return fmt.Sprintf("repair #%d %s onto %s", warning.DependencyPR, warning.DependencyHead, warning.DependencyBase)
		}
		return fmt.Sprintf("repair #%d %s", warning.DependencyPR, warning.DependencyHead)
	case app.ActionUpdateBranch:
		return fmt.Sprintf("update-branch #%d %s", warning.DependencyPR, warning.DependencyHead)
	default:
		return fmt.Sprintf("%s #%d %s", warning.DependencyKind, warning.DependencyPR, warning.DependencyHead)
	}
}

type pullJSON struct {
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Base         string `json:"base"`
	Head         string `json:"head"`
	State        string `json:"state"`
	Reason       string `json:"reason,omitempty"`
	NewBase      string `json:"newBase,omitempty"`
	Upstream     string `json:"upstream,omitempty"`
	OriginalBase *int   `json:"originalBase,omitempty"`
	URL          string `json:"url,omitempty"`
	Draft        bool   `json:"draft,omitempty"`
	Author       string `json:"author,omitempty"`
}

type actionJSON struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	PR        int      `json:"pr"`
	Head      string   `json:"head"`
	NewBase   string   `json:"newBase,omitempty"`
	Upstream  string   `json:"upstream,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	DependsOn []string `json:"dependsOn,omitempty"`
}

type actionResultJSON struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	PR       int    `json:"pr"`
	Head     string `json:"head"`
	NewBase  string `json:"newBase,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

type warningJSON struct {
	Kind             string `json:"kind"`
	PR               int    `json:"pr"`
	ActionID         string `json:"actionId"`
	DependencyID     string `json:"dependencyId"`
	DependencyPR     int    `json:"dependencyPr,omitempty"`
	DependencyKind   string `json:"dependencyKind,omitempty"`
	DependencyHead   string `json:"dependencyHead,omitempty"`
	DependencyBase   string `json:"dependencyBase,omitempty"`
	DependencyReason string `json:"dependencyReason,omitempty"`
}

func renderListJSON(w io.Writer, plan *app.Plan) error {
	return json.NewEncoder(w).Encode(struct {
		Pulls []pullJSON `json:"pulls"`
	}{
		Pulls: pullStatusesJSON(plan.Pulls),
	})
}

func renderPlanJSON(w io.Writer, plan *app.Plan) error {
	return json.NewEncoder(w).Encode(struct {
		Actions  []actionJSON  `json:"actions"`
		Warnings []warningJSON `json:"warnings,omitempty"`
	}{
		Actions:  actionsJSON(plan.Actions),
		Warnings: warningsJSON(plan.Warnings),
	})
}

func renderRunResultJSON(w io.Writer, result *app.RunResult) error {
	return json.NewEncoder(w).Encode(struct {
		Actions  []actionResultJSON `json:"actions"`
		Warnings []warningJSON      `json:"warnings,omitempty"`
	}{
		Actions:  actionResultsJSON(result.Actions),
		Warnings: warningsJSON(result.Warnings),
	})
}

func pullStatusesJSON(statuses []app.PullStatus) []pullJSON {
	pulls := make([]pullJSON, 0, len(statuses))
	for _, status := range statuses {
		pr := status.PR
		var originalBase *int
		if status.OriginalBase != nil {
			n := status.OriginalBase.Number
			originalBase = &n
		}
		pulls = append(pulls, pullJSON{
			Number:       pr.Number,
			Title:        pr.Title,
			Base:         pr.BaseRefName,
			Head:         pr.HeadRefName,
			State:        string(status.State),
			Reason:       string(status.Reason),
			NewBase:      status.NewBase,
			Upstream:     status.Upstream,
			OriginalBase: originalBase,
			URL:          pr.Url,
			Draft:        pr.IsDraft,
			Author:       pr.Author.Login,
		})
	}
	return pulls
}

func actionsJSON(actions []app.Action) []actionJSON {
	out := make([]actionJSON, 0, len(actions))
	for _, action := range actions {
		out = append(out, actionJSON{
			ID:        action.ID,
			Kind:      string(action.Kind),
			PR:        action.PR.Number,
			Head:      action.PR.HeadRefName,
			NewBase:   action.NewBase,
			Upstream:  action.Upstream,
			Reason:    string(action.Reason),
			DependsOn: action.DependsOn,
		})
	}
	return out
}

func actionResultsJSON(results []app.ActionResult) []actionResultJSON {
	out := make([]actionResultJSON, 0, len(results))
	for _, result := range results {
		action := result.Action
		out = append(out, actionResultJSON{
			ID:       action.ID,
			Kind:     string(action.Kind),
			PR:       action.PR.Number,
			Head:     action.PR.HeadRefName,
			NewBase:  action.NewBase,
			Upstream: action.Upstream,
			Reason:   string(action.Reason),
			Status:   string(result.Status),
			Error:    result.Error,
		})
	}
	return out
}

func warningsJSON(warnings []app.SelectionWarning) []warningJSON {
	out := make([]warningJSON, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, warningJSON{
			Kind:             string(warning.Kind),
			PR:               warning.PRNumber,
			ActionID:         warning.ActionID,
			DependencyID:     warning.DependencyID,
			DependencyPR:     warning.DependencyPR,
			DependencyKind:   string(warning.DependencyKind),
			DependencyHead:   warning.DependencyHead,
			DependencyBase:   warning.DependencyBase,
			DependencyReason: string(warning.DependencyReason),
		})
	}
	return out
}

func statusesByPR(statuses []app.PullStatus) map[int]app.PullStatus {
	out := make(map[int]app.PullStatus, len(statuses))
	for _, status := range statuses {
		out[status.PR.Number] = status
	}
	return out
}

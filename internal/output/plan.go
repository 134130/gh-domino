package output

import (
	"encoding/json"
	"fmt"
	"io"

	"charm.land/lipgloss/v2"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/termrender"
)

type Format string

const (
	FormatHuman Format = "human"
	FormatJSON  Format = "json"
)

type ListOptions struct {
	Format  Format
	State   string
	Flat    bool
	NoColor bool
	Width   int
}

type PlanOptions struct {
	Format  Format
	NoColor bool
	Width   int
}

func RenderList(w io.Writer, plan *app.Plan, opts ListOptions) error {
	if opts.Format == FormatJSON {
		return renderListJSON(w, plan)
	}
	_, err := io.WriteString(w, termrender.RenderList(plan, opts.State, opts.Flat, termrender.Options{
		NoColor: opts.NoColor,
		Width:   opts.Width,
	}))
	return err
}

func RenderPlan(w io.Writer, plan, selectedPlan *app.Plan, opts PlanOptions) error {
	if opts.Format == FormatJSON {
		return renderPlanJSON(w, selectedPlan)
	}

	_, err := io.WriteString(w, termrender.RenderPlanSnapshot(plan, selectedPlan, termrender.Options{
		NoColor: opts.NoColor,
		Width:   opts.Width,
	}))
	return err
}

func RenderRunResult(w io.Writer, result *app.RunResult, format Format, noColor ...bool) error {
	if format == FormatJSON {
		return renderRunResultJSON(w, result)
	}
	opts := termrender.Options{}
	if len(noColor) > 0 {
		opts.NoColor = noColor[0]
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
		line := fmt.Sprintf("%s %s", statusSummary(action.Status, opts.NoColor), termrender.ActionSummary(action.Action, opts))
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

func statusSummary(status app.ActionStatus, noColor bool) string {
	style := lipgloss.NewStyle()
	symbol := string(status)
	switch status {
	case app.ActionStatusSuccess:
		symbol = "✔"
		style = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(2))
	case app.ActionStatusFailed:
		symbol = "✘"
		style = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(1))
	case app.ActionStatusSkipped:
		symbol = "!"
		style = lipgloss.NewStyle().Foreground(lipgloss.ANSIColor(3))
	}
	if noColor {
		return symbol
	}
	return style.Render(symbol)
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

package app

import (
	"fmt"
	"slices"

	"github.com/134130/gh-domino/internal/stackedpr"
)

func (p *Plan) Select(selection Selection) (*Plan, error) {
	if p == nil {
		return nil, fmt.Errorf("plan is nil")
	}

	if len(selection.Items) == 0 {
		selected := p.clone()
		selected.Warnings = selectionWarnings(selected.Actions, p.Actions)
		return selected, nil
	}

	nodes := nodeIndex(p.Roots)
	selectedPRs := map[int]struct{}{}
	for _, item := range selection.Items {
		node, ok := nodes[item.PRNumber]
		if !ok {
			return nil, fmt.Errorf("PR #%d was not found in the dependency tree", item.PRNumber)
		}

		switch item.Mode {
		case SelectNode:
			selectedPRs[item.PRNumber] = struct{}{}
		case SelectSubtree:
			collectSubtreePRs(node, selectedPRs)
		case SelectStack:
			root := stackRoot(p.Roots, item.PRNumber)
			if root == nil {
				return nil, fmt.Errorf("stack containing PR #%d was not found", item.PRNumber)
			}
			collectSubtreePRs(root, selectedPRs)
		default:
			return nil, fmt.Errorf("unsupported selection mode: %s", item.Mode)
		}
	}

	selected := p.clone()
	selected.Actions = nil
	selectedIDs := map[string]struct{}{}
	for _, action := range p.Actions {
		if _, ok := selectedPRs[action.PR.Number]; !ok {
			continue
		}
		if _, ok := selectedIDs[action.ID]; ok {
			continue
		}
		selected.Actions = append(selected.Actions, action)
		selectedIDs[action.ID] = struct{}{}
	}
	selected.Warnings = selectionWarnings(selected.Actions, p.Actions)
	return selected, nil
}

func (p *Plan) SelectStacks(prNumbers []int) (*Plan, error) {
	if p == nil {
		return nil, fmt.Errorf("plan is nil")
	}
	if len(prNumbers) == 0 {
		return p.clone(), nil
	}

	nodes := nodeIndex(p.Roots)
	selectedPRs := map[int]struct{}{}
	selectedRootPRs := map[int]struct{}{}
	for _, prNumber := range prNumbers {
		if _, ok := nodes[prNumber]; !ok {
			return nil, fmt.Errorf("PR #%d was not found in the dependency tree", prNumber)
		}
		root := stackRoot(p.Roots, prNumber)
		if root == nil {
			return nil, fmt.Errorf("stack containing PR #%d was not found", prNumber)
		}
		selectedRootPRs[root.Value.Number] = struct{}{}
		collectSubtreePRs(root, selectedPRs)
	}

	selected := p.clone()
	selected.Roots = selected.Roots[:0]
	for _, root := range p.Roots {
		if _, ok := selectedRootPRs[root.Value.Number]; ok {
			selected.Roots = append(selected.Roots, root)
		}
	}
	selected.Pulls = filterPullStatusesByPR(p.Pulls, selectedPRs)
	selected.Actions = filterActionsByPR(p.Actions, selectedPRs)
	selected.Warnings = selectionWarnings(selected.Actions, p.Actions)
	return selected, nil
}

func (p *Plan) clone() *Plan {
	return &Plan{
		Roots:    slices.Clone(p.Roots),
		Pulls:    slices.Clone(p.Pulls),
		Actions:  slices.Clone(p.Actions),
		Warnings: slices.Clone(p.Warnings),
		HeadSHAs: cloneStringMap(p.HeadSHAs),
	}
}

func selectionWarnings(selectedActions, allActions []Action) []SelectionWarning {
	selectedByID := make(map[string]Action, len(selectedActions))
	for _, action := range selectedActions {
		selectedByID[action.ID] = action
	}

	allByID := actionsByIDFrom(allActions)
	var warnings []SelectionWarning
	for _, action := range selectedActions {
		for _, dependencyID := range action.DependsOn {
			if _, selected := selectedByID[dependencyID]; selected {
				continue
			}
			warning := SelectionWarning{
				Kind:         SelectionWarningUnselectedDependency,
				PRNumber:     action.PR.Number,
				ActionID:     action.ID,
				DependencyID: dependencyID,
			}
			if dependency, ok := allByID[dependencyID]; ok {
				warning.DependencyPR = dependency.PR.Number
				warning.DependencyKind = dependency.Kind
				warning.DependencyHead = dependency.PR.HeadRefName
				warning.DependencyBase = dependency.NewBase
				warning.DependencyReason = dependency.Reason
			}
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func filterPullStatusesByPR(statuses []PullStatus, prNumbers map[int]struct{}) []PullStatus {
	out := make([]PullStatus, 0, len(statuses))
	for _, status := range statuses {
		if _, ok := prNumbers[status.PR.Number]; ok {
			out = append(out, status)
		}
	}
	return out
}

func filterActionsByPR(actions []Action, prNumbers map[int]struct{}) []Action {
	out := make([]Action, 0, len(actions))
	for _, action := range actions {
		if _, ok := prNumbers[action.PR.Number]; ok {
			out = append(out, action)
		}
	}
	return out
}

func actionsByIDFrom(actions []Action) map[string]Action {
	out := make(map[string]Action, len(actions))
	for _, action := range actions {
		out[action.ID] = action
	}
	return out
}

func nodeIndex(roots []*stackedpr.Node) map[int]*stackedpr.Node {
	index := map[int]*stackedpr.Node{}
	var walk func(*stackedpr.Node)
	walk = func(node *stackedpr.Node) {
		if node == nil {
			return
		}
		index[node.Value.Number] = node
		for _, child := range node.Children {
			walk(child)
		}
	}
	for _, root := range roots {
		walk(root)
	}
	return index
}

func collectSubtreePRs(node *stackedpr.Node, out map[int]struct{}) {
	if node == nil {
		return
	}
	out[node.Value.Number] = struct{}{}
	for _, child := range node.Children {
		collectSubtreePRs(child, out)
	}
}

func stackRoot(roots []*stackedpr.Node, prNumber int) *stackedpr.Node {
	for _, root := range roots {
		if subtreeContains(root, prNumber) {
			return root
		}
	}
	return nil
}

func subtreeContains(node *stackedpr.Node, prNumber int) bool {
	if node == nil {
		return false
	}
	if node.Value.Number == prNumber {
		return true
	}
	for _, child := range node.Children {
		if subtreeContains(child, prNumber) {
			return true
		}
	}
	return false
}

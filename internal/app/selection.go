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

	tree := indexTree(p.Roots)
	selectedPRs := map[int]struct{}{}
	for _, item := range selection.Items {
		node, ok := tree.nodes[item.PRNumber]
		if !ok {
			return nil, fmt.Errorf("PR #%d was not found in the dependency tree", item.PRNumber)
		}

		switch item.Mode {
		case SelectNode:
			selectedPRs[item.PRNumber] = struct{}{}
		case SelectSubtree:
			collectSubtreePRs(node, selectedPRs)
		case SelectChain:
			collectChainPRs(item.PRNumber, tree.parents, selectedPRs)
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

func actionsByIDFrom(actions []Action) map[string]Action {
	out := make(map[string]Action, len(actions))
	for _, action := range actions {
		out[action.ID] = action
	}
	return out
}

type treeIndex struct {
	nodes   map[int]*stackedpr.Node
	parents map[int]int
}

func indexTree(roots []*stackedpr.Node) treeIndex {
	index := treeIndex{
		nodes:   map[int]*stackedpr.Node{},
		parents: map[int]int{},
	}
	var walk func(*stackedpr.Node, int)
	walk = func(node *stackedpr.Node, parentPR int) {
		if node == nil {
			return
		}
		index.nodes[node.Value.Number] = node
		if parentPR != 0 {
			index.parents[node.Value.Number] = parentPR
		}
		for _, child := range node.Children {
			walk(child, node.Value.Number)
		}
	}
	for _, root := range roots {
		walk(root, 0)
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

func collectChainPRs(prNumber int, parents map[int]int, out map[int]struct{}) {
	for current := prNumber; current != 0; current = parents[current] {
		out[current] = struct{}{}
	}
}

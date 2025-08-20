package stackedpr

import (
	"fmt"

	"github.com/charmbracelet/lipgloss/tree"

	"github.com/134130/gh-domino/internal/color"
)

func RenderDependencyTree(nodes []*Node) string {
	root := tree.Root(color.Bold("Pull Requests"))

	for _, node := range nodes {
		root.Child(asTreeNode(node))
	}

	root.Indenter(func(children tree.Children, index int) string {
		if children.Length()-1 == index {
			return "   "
		}
		return "│  "
	})
	root.Enumerator(func(children tree.Children, index int) string {
		if children.Length()-1 == index {
			return "└─ "
		}
		return "├─ "
	})

	return root.String()
}

func asTreeNode(node *Node) tree.Node {
	originalBaseStr := ""
	if node.OriginalBase != nil {
		originalBaseStr = fmt.Sprintf(" [was on %s]", node.OriginalBase.PRNumberString())
	}

	str := fmt.Sprintf("%s %s (%s ← %s)%s",
		node.Value.PRNumberString(),
		node.Value.Title,
		color.Cyan(node.Value.BaseRefName),
		color.Blue(node.Value.HeadRefName),
		originalBaseStr,
	)

	t := tree.New()
	t.Root(str)
	for _, child := range node.Children {
		t.Child(asTreeNode(child))
	}
	return t
}

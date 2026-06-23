package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRootHelpDescribesCLIWorkflow(t *testing.T) {
	out := helpOutput(t, "--help")

	assertContains(t, out, "gh-domino repairs existing stacked pull requests after a parent PR is merged or rebased.")
	assertContains(t, out, "Usage:")
	assertContains(t, out, "gh domino [flags]")
	assertContains(t, out, "gh domino [command]")
	assertContains(t, out, "list        Show stacked PRs and their repair status")
	assertContains(t, out, "plan        Preview repair actions without changing branches")
	assertContains(t, out, "merge       Execute selected repair actions")
	assertContains(t, out, "Use \"gh domino [command] --help\" for more information about a command.")
}

func TestPlanHelpDescribesChainOrdering(t *testing.T) {
	out := helpOutput(t, "plan", "--help")

	assertContains(t, out, "Usage:")
	assertContains(t, out, "gh domino plan [flags]")
	assertContains(t, out, "Preview the repair actions gh-domino would run without changing branches.")
	assertContains(t, out, "With --chain, gh-domino selects the path from the upper/root PR down to the selected lower/child PR")
	assertContains(t, out, "--chain int")
	assertContains(t, out, "ordered parent before child")
}

func TestMergeHelpDescribesDependencyAwareParallelism(t *testing.T) {
	out := helpOutput(t, "merge", "--help")

	assertContains(t, out, "Usage:")
	assertContains(t, out, "gh domino merge [flags]")
	assertContains(t, out, "parent actions run before child actions")
	assertContains(t, out, "--parallel runs independent stacks concurrently when possible")
	assertContains(t, out, "dependent PRs in the same chain are still serialized")
	assertContains(t, out, "--parallel int")
	assertContains(t, out, "when dependencies allow")
	assertContains(t, out, "push with --force-with-lease")
}

func helpOutput(t *testing.T, args ...string) string {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := RunWithIO(context.Background(), args, strings.NewReader(""), &stdout, &stderr)
	if err != nil {
		t.Fatalf("RunWithIO returned error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected no stderr, got %q", stderr.String())
	}
	return stdout.String()
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected output to contain %q, got:\n%s", want, got)
	}
}

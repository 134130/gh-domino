package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/internal/domino"
)

func TestIntegration(t *testing.T) {
	testcases := []struct {
		name     string
		expected string
	}{{
		name: "test-merge-commit-1",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #56 bar (stack-1 ← stack-2) [was on #55]
    └─  #57 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #56 bar (stack-1 ← stack-2) (update base branch to main)
  #57 baz (stack-2 ← stack-3)
`,
	}, {
		name: "test-merge-commit-2",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #56 bar (stack-1 ← stack-2) [was on #55]
    └─  #57 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #56 bar (stack-1 ← stack-2) (update base branch to main)
  #57 baz (stack-2 ← stack-3)
`,
	}, {
		name: "test-merge-commit-hard-1",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #62 bar (stack-1 ← stack-2) [was on #61]
    └─  #63 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #62 bar (stack-1 ← stack-2) (update base branch to main)
  #63 baz (stack-2 ← stack-3)
`}, {
		name: "test-merge-commit-hard-2",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #73 bar (stack-1 ← stack-2) [was on #72]
    └─  #74 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #73 bar (stack-1 ← stack-2) (update base branch to main)
  #74 baz (stack-2 ← stack-3)
`,
	}, {
		name: "test-multiple",
		expected: `✔ Fetching pull requests... done
Pull Requests
├─  #78 foo (main ← stack-1)
│   └─  #79 bar (stack-1 ← stack-2)
│       └─  #80 baz (stack-2 ← stack-3)
└─  #82 bbb (feature-a ← feature-b) [was on #81]
    └─  #83 ccc (feature-b ← feature-c)
Dry run mode enabled. The following PRs would be rebased:
  #82 bbb (feature-a ← feature-b) (update base branch to main)
  #83 ccc (feature-b ← feature-c)
`,
	}, {
		name: "test-rebase-1",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #59 bar (stack-1 ← stack-2) [was on #58]
    └─  #60 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #59 bar (stack-1 ← stack-2) (update base branch to main)
  #60 baz (stack-2 ← stack-3)
`,
	}, {
		name: "test-rebase-hard-1",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #65 bar (stack-1 ← stack-2) [was on #64]
    └─  #66 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #65 bar (stack-1 ← stack-2) (update base branch to main)
  #66 baz (stack-2 ← stack-3)
`}, {
		name: "test-rebase-hard-2",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #76 bar (stack-1 ← stack-2) [was on #75]
    └─  #77 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #76 bar (stack-1 ← stack-2) (update base branch to main)
  #77 baz (stack-2 ← stack-3)
`,
	}, {
		name: "test-squash-1",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #51 foo (main ← stack-1)
    └─  #52 bar (stack-1 ← stack-2)
        └─  #53 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
No broken PRs found.
`,
	}, {
		name: "test-squash-2",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #52 bar (stack-1 ← stack-2) [was on #51]
    └─  #53 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #52 bar (stack-1 ← stack-2) (update base branch to main)
  #53 baz (stack-2 ← stack-3)
`,
	}, {
		name: "test-squash-3",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #52 bar (main ← stack-2) [was on #51]
    └─  #53 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #52 bar (main ← stack-2)
  #53 baz (stack-2 ← stack-3)
`,
	}, {
		name: "test-squash-hard-1",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #68 bar (stack-1 ← stack-2) [was on #67]
    └─  #69 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #68 bar (stack-1 ← stack-2) (update base branch to main)
  #69 baz (stack-2 ← stack-3)
`,
	}, {
		name: "test-squash-hard-2",
		expected: `✔ Fetching pull requests... done
Pull Requests
└─  #68 bar (main ← stack-2) [was on #67]
    └─  #69 baz (stack-2 ← stack-3)
Dry run mode enabled. The following PRs would be rebased:
  #68 bar (main ← stack-2)
  #69 baz (stack-2 ← stack-3)
`,
	}}

	for _, tc := range testcases {
		t.Run(tc.name, func(tt *testing.T) {
			cr, err := NewYAMLRunner(tt.Context(), fmt.Sprintf("testdata/%s.yaml", tc.name))
			if err != nil {
				tt.Fatalf("failed to create YAML runner: %v", err)
			}
			git.CommandRunner = cr

			out := &strings.Builder{}
			if err = domino.Run(tt.Context(), domino.Config{DryRun: ptr(true), Writer: out}); err != nil {
				tt.Fatalf("Integration test failed: %v", err)
			}

			assert.Equal(tt, tc.expected, out.String())
		})
	}
}

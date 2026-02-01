package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/134130/gh-domino/git"
	"github.com/134130/gh-domino/internal/domino"
)

func TestDryRun(t *testing.T) {
	testcases := []struct {
		name     string
		expected string
	}{{
		name: "test-dry-run-merge-commit-1",
		expected: `✔ Fetching pull requests...

Pull Requests
└─ #56 bar (stack-1 ← stack-2) [was on #55]
   └─ #57 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #56 bar (stack-1 ← stack-2) (update base branch to main)
  #57 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`,
	}, {
		name: "test-dry-run-merge-commit-2",
		expected: `✔ Fetching pull requests...

Pull Requests
└─ #56 bar (stack-1 ← stack-2) [was on #55]
   └─ #57 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #56 bar (stack-1 ← stack-2) (update base branch to main)
  #57 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`,
	}, {
		name: "test-dry-run-merge-commit-hard-1",
		expected: `✔ Fetching pull requests...

Pull Requests
└─ #62 bar (stack-1 ← stack-2) [was on #61]
   └─ #63 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #62 bar (stack-1 ← stack-2) (update base branch to main)
  #63 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`},
		{
			name: "test-dry-run-merge-commit-hard-2",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #73 bar (stack-1 ← stack-2) [was on #72]
   └─ #74 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #73 bar (stack-1 ← stack-2) (update base branch to main)
  #74 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`,
		}, {
			name: "test-dry-run-multiple",
			expected: `✔ Fetching pull requests...

Pull Requests
├─ #78 foo (main ← stack-1)
│  └─ #79 bar (stack-1 ← stack-2)
│     └─ #80 baz (stack-2 ← stack-3)
└─ #82 bbb (feature-a ← feature-b) [was on #81]
   └─ #83 ccc (feature-b ← feature-c)

Dry run mode enabled. The following PRs would be rebased:
  #82 bbb (feature-a ← feature-b) (update base branch to main)
  #83 ccc (feature-b ← feature-c)
✘ Failed to remove worktree: context canceled
`,
		}, {
			name: "test-dry-run-rebase-1",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #59 bar (stack-1 ← stack-2) [was on #58]
   └─ #60 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #59 bar (stack-1 ← stack-2) (update base branch to main)
  #60 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`,
		}, {
			name: "test-dry-run-rebase-hard-1",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #65 bar (stack-1 ← stack-2) [was on #64]
   └─ #66 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #65 bar (stack-1 ← stack-2) (update base branch to main)
  #66 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`},
		{
			name: "test-dry-run-rebase-hard-2",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #76 bar (stack-1 ← stack-2) [was on #75]
   └─ #77 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #76 bar (stack-1 ← stack-2) (update base branch to main)
  #77 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`,
		}, {
			name: "test-dry-run-squash-1",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #51 foo (main ← stack-1)
   └─ #52 bar (stack-1 ← stack-2)
      └─ #53 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
✔ No broken PRs found.
✘ Failed to remove worktree: context canceled
`,
		}, {
			name: "test-dry-run-squash-2",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #52 bar (stack-1 ← stack-2) [was on #51]
   └─ #53 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #52 bar (stack-1 ← stack-2) (update base branch to main)
  #53 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`,
		}, {
			name: "test-dry-run-squash-3",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #52 bar (main ← stack-2) [was on #51]
   └─ #53 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #52 bar (main ← stack-2)
  #53 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`,
		}, {
			name: "test-dry-run-squash-hard-1",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #68 bar (stack-1 ← stack-2) [was on #67]
   └─ #69 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #68 bar (stack-1 ← stack-2) (update base branch to main)
  #69 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`,
		}, {
			name: "test-dry-run-squash-hard-2",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #68 bar (main ← stack-2) [was on #67]
   └─ #69 baz (stack-2 ← stack-3)

Dry run mode enabled. The following PRs would be rebased:
  #68 bar (main ← stack-2)
  #69 baz (stack-2 ← stack-3)
✘ Failed to remove worktree: context canceled
`,
		}, {
			name: "test-dry-run-squash-hard-3",
			expected: `✔ Fetching pull requests...

Pull Requests
└─ #3527 refactor: Rename Schedule recurrence type from MINUTELY to CUSTOM (main ← cooper/schedule/minutely-custom) [was on #3510]

Dry run mode enabled. The following PRs would be rebased:
  #3527 refactor: Rename Schedule recurrence type from MINUTELY to CUSTOM (main ← cooper/schedule/minutely-custom)
✘ Failed to remove worktree: context canceled
`,
		}}

	for _, tc := range testcases {
		t.Run(tc.name, func(tt *testing.T) {
			cr, err := NewYAMLRunner(tt.Context(), fmt.Sprintf("testdata/%s.yaml", tc.name))
			if err != nil {
				t.Fatalf("failed to create YAML runner: %v", err)
			}
			git.CommandRunner = cr

			out := &strings.Builder{}
			if err = domino.Run(tt.Context(), domino.Config{DryRun: true, Writer: out, Headless: true}); err != nil {
				t.Fatalf("Integration test failed: %v", err)
			}

			assert.Equal(tt, tc.expected, out.String())
		})
	}
}

func TestAuto(t *testing.T) {
	testcases := []struct {
		name     string
		expected string
	}{{
		name: "test-auto-merge-1",
		expected: `✔ Fetching pull requests...

Pull Requests
└─ #91 bar (stack-1 ← stack-2) [was on #90]
   └─ #92 baz (stack-2 ← stack-3)

✔ Rebasing #91 bar (stack-1 ← stack-2) onto main...
✔ Pushing #91...
✔ Updating base branch of #91 to main...
✔ Rebasing #92 baz (stack-2 ← stack-3) onto stack-2...
✔ Pushing #92...
✘ Failed to remove worktree: context canceled
`,
	}, {
		name: "test-auto-merge-2",
		expected: `✔ Fetching pull requests...

Pull Requests
└─ #94 bar (stack-1 ← stack-2) [was on #93]
   └─ #95 baz (stack-2 ← stack-3)

✔ Rebasing #94 bar (stack-1 ← stack-2) onto main...
✔ Pushing #94...
✔ Updating base branch of #94 to main...
✔ Rebasing #95 baz (stack-2 ← stack-3) onto stack-2...
✔ Pushing #95...
✘ Failed to remove worktree: context canceled
`,
	}, {
		name: "test-auto-merge-conflict",
		expected: `✔ Fetching pull requests...

Pull Requests
├─ #112 bar (main ← stack-2)
│  └─ #113 baz (stack-2 ← stack-3)
└─ #115 bbb (feature-a ← feature-b) [was on #114]

✘ Failed to handle broken PR #115 due to rebase conflicts.
  Please resolve the conflicts manually and re-run the tool if needed.
  You can use the following command to rebase manually:
      git rebase --onto origin/main 2e6584b4cf5357c768400670d1a7ca89b862e0b7 feature-b
✘ Failed to remove worktree: context canceled
`,
	}, {
		name: "test-auto-merge-trunk",
		expected: `✔ Fetching pull requests...

Pull Requests
└─ #109 bbb (feature/trunk-a ← feature/trunk-b) [was on #108]
   └─ #110 ccc (feature/trunk-b ← feature/trunk-c)

✔ Rebasing #109 bbb (feature/trunk-a ← feature/trunk-b) onto trunk...
✔ Pushing #109...
✔ Updating base branch of #109 to trunk...
✔ Rebasing #110 ccc (feature/trunk-b ← feature/trunk-c) onto feature/trunk-b...
✔ Pushing #110...
✘ Failed to remove worktree: context canceled
`,
	}}

	for _, tc := range testcases {
		t.Run(tc.name, func(tt *testing.T) {
			cr, err := NewYAMLRunner(tt.Context(), fmt.Sprintf("testdata/%s.yaml", tc.name))
			if err != nil {
				t.Fatalf("failed to create YAML runner: %v", err)
			}
			git.CommandRunner = cr

			out := &strings.Builder{}
			if err := domino.Run(tt.Context(), domino.Config{
				Auto:     true,
				DryRun:   false,
				Headless: true,
				Writer:   out,
			}); err != nil {
				t.Fatalf("Integration test failed: %v", err)
			}

			assert.Equal(tt, tc.expected, out.String())
		})
	}
}

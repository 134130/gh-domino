package harness

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/134130/gh-domino/gitobj"
	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/app/gitkitexec"
	"github.com/134130/gitkit/gitcmd"
	"github.com/134130/gitkit/gitrepo"
)

const (
	defaultBranch = "main"
	defaultRemote = "origin"
)

type Harness struct {
	t         testing.TB
	ctx       context.Context
	state     *State
	runner    *Runner
	RootDir   string
	RemoteDir string
	WorkDir   string
}

func New(t testing.TB) *Harness {
	t.Helper()

	ctx := context.Background()
	if tb, ok := t.(interface{ Context() context.Context }); ok {
		ctx = tb.Context()
	}

	root := t.TempDir()
	h := &Harness{
		t:         t,
		ctx:       ctx,
		state:     NewState(defaultBranch),
		RootDir:   root,
		RemoteDir: filepath.Join(root, "origin.git"),
		WorkDir:   filepath.Join(root, "work"),
	}

	h.git("", "init", "--bare", "--initial-branch="+defaultBranch, h.RemoteDir)
	h.git("", "clone", h.RemoteDir, h.WorkDir)
	h.git(h.WorkDir, "config", "user.name", "gh-domino test")
	h.git(h.WorkDir, "config", "user.email", "gh-domino@example.invalid")
	h.writeFile("README.md", "# test repository\n")
	h.git(h.WorkDir, "add", "README.md")
	h.git(h.WorkDir, "commit", "-m", "Initial commit")
	h.git(h.WorkDir, "push", "--set-upstream", defaultRemote, defaultBranch)
	h.Fetch()

	return h
}

func (h *Harness) Runner() *Runner {
	h.t.Helper()
	if h.runner == nil {
		h.runner = NewRunner(h.WorkDir, h.state)
	}
	return h.runner
}

func (h *Harness) Store() Store {
	return Store{
		state:   h.state,
		workDir: h.WorkDir,
		git:     gitrepo.New(h.Runner(), gitrepo.WithDir(h.WorkDir)),
	}
}

func (h *Harness) Plan(opts app.PlanOptions) *app.Plan {
	h.t.Helper()
	plan, err := app.NewPlanner(h.Store()).BuildPlan(h.ctx, opts)
	if err != nil {
		h.t.Fatalf("build plan: %v", err)
	}
	return plan
}

func (h *Harness) Execute(plan *app.Plan, opts app.ExecuteOptions) *app.RunResult {
	h.t.Helper()
	if opts.Remote == "" {
		opts.Remote = defaultRemote
	}
	result, err := gitkitexec.New(h.Runner()).Execute(h.ctx, plan, opts)
	if err != nil {
		h.t.Fatalf("execute plan: %v", err)
	}
	h.Fetch()
	return result
}

func (h *Harness) Branch(name, startPoint string) *Harness {
	h.t.Helper()
	h.Fetch()
	h.git(h.WorkDir, "switch", "-C", name, startPoint)
	return h
}

func (h *Harness) Commit(branch, path, content string) string {
	h.t.Helper()
	h.git(h.WorkDir, "switch", branch)
	h.writeFile(path, content)
	h.git(h.WorkDir, "add", path)
	h.git(h.WorkDir, "commit", "-m", fmt.Sprintf("%s: %s", branch, path))
	return h.Ref("HEAD")
}

func (h *Harness) Push(branch string) *Harness {
	h.t.Helper()
	h.git(h.WorkDir, "push", "--set-upstream", defaultRemote, branch)
	h.Fetch()
	return h
}

func (h *Harness) Fetch() *Harness {
	h.t.Helper()
	h.git(h.WorkDir, "fetch", defaultRemote)
	return h
}

func (h *Harness) OpenPR(number int, title, base, head string) *Harness {
	h.t.Helper()
	h.Fetch()
	h.state.Upsert(PR{
		Number:  number,
		Title:   title,
		State:   gitobj.PullRequestStateOpen,
		Base:    base,
		Head:    head,
		Commits: h.commitsBetween(base, head),
	})
	return h
}

func (h *Harness) MergeCommit(number int) *Harness {
	h.t.Helper()
	pr := h.state.Must(number)
	h.checkoutRemote(pr.Base)
	h.git(h.WorkDir, "merge", "--no-ff", "--no-edit", remoteRef(pr.Head))
	mergeSHA := h.Ref("HEAD")
	h.git(h.WorkDir, "push", defaultRemote, pr.Base)
	h.Fetch()
	h.state.MarkMerged(number, mergeSHA)
	return h
}

func (h *Harness) SquashMerge(number int) *Harness {
	h.t.Helper()
	pr := h.state.Must(number)
	h.checkoutRemote(pr.Base)
	h.git(h.WorkDir, "merge", "--squash", remoteRef(pr.Head))
	h.git(h.WorkDir, "commit", "-m", fmt.Sprintf("Squash PR #%d", number))
	mergeSHA := h.Ref("HEAD")
	h.git(h.WorkDir, "push", defaultRemote, pr.Base)
	h.Fetch()
	h.state.MarkMerged(number, mergeSHA)
	return h
}

func (h *Harness) RebaseMerge(number int) *Harness {
	h.t.Helper()
	pr := h.state.Must(number)
	h.checkoutRemote(pr.Base)
	for _, commit := range pr.Commits {
		h.git(h.WorkDir, "cherry-pick", commit)
	}
	mergeSHA := h.Ref("HEAD")
	h.git(h.WorkDir, "push", defaultRemote, pr.Base)
	h.Fetch()
	h.state.MarkMerged(number, mergeSHA)
	return h
}

func (h *Harness) DeleteLocalBranch(branch string) *Harness {
	h.t.Helper()
	if h.CurrentBranch() == branch {
		h.git(h.WorkDir, "switch", defaultBranch)
	}
	h.git(h.WorkDir, "branch", "-D", branch)
	return h
}

func (h *Harness) CurrentBranch() string {
	h.t.Helper()
	return h.gitOutput(h.WorkDir, "branch", "--show-current")
}

func (h *Harness) PRBase(number int) string {
	h.t.Helper()
	return h.state.Must(number).Base
}

func (h *Harness) Ref(ref string) string {
	h.t.Helper()
	return h.gitOutput(h.WorkDir, "rev-parse", ref)
}

func (h *Harness) MergeBase(a, b string) string {
	h.t.Helper()
	return h.gitOutput(h.WorkDir, "merge-base", a, b)
}

func (h *Harness) RevList(revRange string) []string {
	h.t.Helper()
	out := h.gitOutput(h.WorkDir, "rev-list", "--reverse", revRange)
	if out == "" {
		return nil
	}
	return strings.Fields(out)
}

func (h *Harness) StatusPorcelain() string {
	h.t.Helper()
	return h.gitOutput(h.WorkDir, "status", "--porcelain")
}

func (h *Harness) checkoutRemote(branch string) {
	h.t.Helper()
	h.Fetch()
	h.git(h.WorkDir, "switch", "-C", branch, remoteRef(branch))
}

func (h *Harness) commitsBetween(base, head string) []string {
	h.t.Helper()
	out := h.gitOutput(h.WorkDir, "rev-list", "--reverse", remoteRef(base)+".."+remoteRef(head))
	if out == "" {
		return nil
	}
	return strings.Fields(out)
}

func (h *Harness) writeFile(path, content string) {
	h.t.Helper()
	fullPath := filepath.Join(h.WorkDir, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		h.t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		h.t.Fatalf("write %s: %v", path, err)
	}
}

func (h *Harness) gitOutput(dir string, args ...string) string {
	h.t.Helper()
	stdout, stderr, err := runGit(h.ctx, dir, args...)
	if err != nil {
		h.t.Fatalf("git %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
	}
	return strings.TrimSpace(stdout)
}

func (h *Harness) git(dir string, args ...string) {
	h.t.Helper()
	stdout, stderr, err := runGit(h.ctx, dir, args...)
	if err != nil {
		h.t.Fatalf("git %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
	}
}

func runGit(ctx context.Context, dir string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = sanitizedEnv()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func remoteRef(branch string) string {
	if strings.HasPrefix(branch, defaultRemote+"/") {
		return branch
	}
	return defaultRemote + "/" + branch
}

type Store struct {
	state   *State
	workDir string
	git     gitrepo.Client
}

var _ app.RepositoryStore = Store{}

func (s Store) Fetch(ctx context.Context, remote string) error {
	return s.git.Fetch(ctx, remote)
}

func (s Store) OpenPullRequests(ctx context.Context, _ string) ([]gitobj.PullRequest, error) {
	prs := s.state.Open()
	out := make([]gitobj.PullRequest, 0, len(prs))
	for _, pr := range prs {
		commits, err := revList(ctx, s.workDir, remoteRef(pr.Base)+".."+remoteRef(pr.Head))
		if err != nil {
			return nil, fmt.Errorf("list commits for #%d: %w", pr.Number, err)
		}
		pr.Commits = commits
		s.state.Upsert(pr)
		out = append(out, pr.GitObject())
	}
	return out, nil
}

func (s Store) MergedPullRequests(context.Context, string, int) ([]gitobj.PullRequest, error) {
	prs := s.state.Merged()
	out := make([]gitobj.PullRequest, 0, len(prs))
	for _, pr := range prs {
		out = append(out, pr.GitObject())
	}
	return out, nil
}

func (s Store) RefSHA(ctx context.Context, ref string) (string, error) {
	return s.git.RevParse(ctx, ref)
}

func (s Store) DefaultBranch(context.Context, string) (string, error) {
	return s.state.DefaultBranch(), nil
}

func (s Store) MergeBase(ctx context.Context, a, b string) (string, error) {
	return s.git.MergeBase(ctx, a, b)
}

func (s Store) IsAncestor(ctx context.Context, ancestor, descendant string) (bool, error) {
	return s.git.IsAncestor(ctx, ancestor, descendant)
}

func revList(ctx context.Context, dir, revRange string) ([]string, error) {
	stdout, stderr, err := runGit(ctx, dir, "rev-list", "--reverse", revRange)
	if err != nil {
		return nil, fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr))
	}
	out := strings.TrimSpace(stdout)
	if out == "" {
		return nil, nil
	}
	return strings.Fields(out), nil
}

type State struct {
	mu            sync.Mutex
	defaultBranch string
	prs           map[int]PR
}

func NewState(defaultBranch string) *State {
	return &State{
		defaultBranch: defaultBranch,
		prs:           map[int]PR{},
	}
}

func (s *State) DefaultBranch() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.defaultBranch
}

func (s *State) Upsert(pr PR) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prs[pr.Number] = pr.Clone()
}

func (s *State) MarkMerged(number int, mergeCommit string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pr, ok := s.prs[number]
	if !ok {
		panic(fmt.Sprintf("missing PR #%d", number))
	}
	pr.State = gitobj.PullRequestStateMerged
	pr.MergeCommit = mergeCommit
	s.prs[number] = pr
}

func (s *State) SetBase(number int, base string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pr, ok := s.prs[number]
	if !ok {
		return fmt.Errorf("missing PR #%d", number)
	}
	pr.Base = base
	s.prs[number] = pr
	return nil
}

func (s *State) Must(number int) PR {
	s.mu.Lock()
	defer s.mu.Unlock()
	pr, ok := s.prs[number]
	if !ok {
		panic(fmt.Sprintf("missing PR #%d", number))
	}
	return pr.Clone()
}

func (s *State) Open() []PR {
	return s.byState(gitobj.PullRequestStateOpen)
}

func (s *State) Merged() []PR {
	return s.byState(gitobj.PullRequestStateMerged)
}

func (s *State) byState(state gitobj.PullRequestState) []PR {
	s.mu.Lock()
	defer s.mu.Unlock()
	prs := make([]PR, 0, len(s.prs))
	for _, pr := range s.prs {
		if pr.State == state {
			prs = append(prs, pr.Clone())
		}
	}
	sort.Slice(prs, func(i, j int) bool {
		return prs[i].Number < prs[j].Number
	})
	return prs
}

type PR struct {
	Number      int
	Title       string
	State       gitobj.PullRequestState
	Base        string
	Head        string
	Commits     []string
	MergeCommit string
}

func (pr PR) Clone() PR {
	pr.Commits = append([]string(nil), pr.Commits...)
	return pr
}

func (pr PR) GitObject() gitobj.PullRequest {
	out := gitobj.PullRequest{
		Number:      pr.Number,
		Title:       pr.Title,
		Url:         fmt.Sprintf("https://example.invalid/pull/%d", pr.Number),
		State:       pr.State,
		BaseRefName: pr.Base,
		HeadRefName: pr.Head,
	}
	out.Author.Login = "test-user"
	out.MergeCommit.Sha = pr.MergeCommit
	for _, commit := range pr.Commits {
		out.Commits = append(out.Commits, struct {
			Oid string `json:"oid"`
		}{Oid: commit})
	}
	return out
}

type Runner struct {
	workDir string
	state   *State

	mu       sync.Mutex
	commands []gitcmd.Command
}

func NewRunner(workDir string, state *State) *Runner {
	return &Runner{
		workDir: workDir,
		state:   state,
	}
}

var _ gitcmd.Runner = (*Runner)(nil)

func (r *Runner) Run(ctx context.Context, cmd gitcmd.Command) (gitcmd.Result, error) {
	r.record(cmd)
	cmd = r.withDefaultDir(cmd)
	if cmd.Program == gitcmd.ProgramGH {
		return r.runGH(ctx, cmd)
	}
	return runGitCommand(ctx, cmd)
}

func (r *Runner) Start(ctx context.Context, cmd gitcmd.Command) (gitcmd.Process, error) {
	r.record(cmd)
	cmd = r.withDefaultDir(cmd)
	if cmd.Program == gitcmd.ProgramGH {
		result, err := r.runGH(ctx, cmd)
		return completedProcess{result: result, err: err}, nil
	}
	result, err := runGitCommand(ctx, cmd)
	return completedProcess{result: result, err: err}, nil
}

func (r *Runner) Commands() []gitcmd.Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]gitcmd.Command(nil), r.commands...)
}

func (r *Runner) CommandStrings() []string {
	commands := r.Commands()
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		out = append(out, command.String())
	}
	return out
}

func (r *Runner) record(cmd gitcmd.Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, cmd)
}

func (r *Runner) withDefaultDir(cmd gitcmd.Command) gitcmd.Command {
	if cmd.Dir == "" {
		cmd.Dir = r.workDir
	}
	return cmd
}

func (r *Runner) runGH(ctx context.Context, cmd gitcmd.Command) (gitcmd.Result, error) {
	args := cmd.Args
	if len(args) == 5 && args[0] == "pr" && args[1] == "edit" && args[3] == "--base" {
		number, err := strconv.Atoi(args[2])
		if err != nil {
			return exitResult(cmd, fmt.Errorf("parse PR number %q: %w", args[2], err))
		}
		if err := r.state.SetBase(number, args[4]); err != nil {
			return exitResult(cmd, err)
		}
		return gitcmd.Result{Command: cmd}, nil
	}

	if len(args) == 4 && args[0] == "pr" && args[1] == "update-branch" && args[2] == "--rebase" {
		number, err := strconv.Atoi(args[3])
		if err != nil {
			return exitResult(cmd, fmt.Errorf("parse PR number %q: %w", args[3], err))
		}
		if err := r.updateBranch(ctx, cmd.Dir, number); err != nil {
			return exitResult(cmd, err)
		}
		return gitcmd.Result{Command: cmd}, nil
	}

	return exitResult(cmd, fmt.Errorf("unsupported gh command: gh %s", strings.Join(args, " ")))
}

func (r *Runner) updateBranch(ctx context.Context, dir string, number int) error {
	pr := r.state.Must(number)
	for _, args := range [][]string{
		{"fetch", defaultRemote},
		{"switch", "-C", pr.Head, remoteRef(pr.Head)},
		{"rebase", remoteRef(pr.Base)},
		{"push", "--force-with-lease", defaultRemote, pr.Head},
		{"fetch", defaultRemote},
	} {
		if stdout, stderr, err := runGit(ctx, dir, args...); err != nil {
			return fmt.Errorf("git %s failed: %v\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), err, stdout, stderr)
		}
	}
	return nil
}

func runGitCommand(ctx context.Context, cmd gitcmd.Command) (gitcmd.Result, error) {
	execCmd := exec.CommandContext(ctx, "git", cmd.Args...)
	execCmd.Dir = cmd.Dir
	execCmd.Env = sanitizedEnv(cmd.Env...)
	execCmd.Stdin = cmd.Stdin

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	execCmd.Stdout = stdout
	execCmd.Stderr = stderr
	if cmd.Stdout != nil {
		execCmd.Stdout = io.MultiWriter(stdout, cmd.Stdout)
	}
	if cmd.Stderr != nil {
		execCmd.Stderr = io.MultiWriter(stderr, cmd.Stderr)
	}

	err := execCmd.Run()
	result := gitcmd.Result{
		Command: cmd,
		Stdout:  stdout.Bytes(),
		Stderr:  stderr.Bytes(),
	}
	if err == nil {
		return result, nil
	}
	result.ExitCode = 1
	if exitError, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitError.ExitCode()
	}
	return result, &gitcmd.ExitError{Result: result, Err: err}
}

func sanitizedEnv(extra ...string) []string {
	blocked := map[string]struct{}{
		"GIT_DIR":        {},
		"GIT_INDEX_FILE": {},
		"GIT_WORK_TREE":  {},
	}
	env := make([]string, 0, len(os.Environ())+len(extra))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, ok := blocked[key]; ok {
			continue
		}
		env = append(env, entry)
	}
	return append(env, extra...)
}

func exitResult(cmd gitcmd.Command, err error) (gitcmd.Result, error) {
	result := gitcmd.Result{
		Command:  cmd,
		Stderr:   []byte(err.Error()),
		ExitCode: 1,
	}
	return result, &gitcmd.ExitError{Result: result, Err: err}
}

type completedProcess struct {
	result gitcmd.Result
	err    error
}

func (p completedProcess) Wait() (gitcmd.Result, error) {
	return p.result, p.err
}

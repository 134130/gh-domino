package cli

import (
	"reflect"
	"testing"

	"github.com/134130/gh-domino/internal/app"
	"github.com/134130/gh-domino/internal/output"
)

func TestParseDefaultCommandIsTUI(t *testing.T) {
	cfg, err := Parse([]string{"--format", "json", "--remote", "upstream"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	want := Config{
		Command:     CommandTUI,
		Remote:      "upstream",
		Author:      "@me",
		MergedLimit: 30,
		Format:      output.FormatJSON,
		Parallel:    1,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("config mismatch\nwant: %#v\n got: %#v", want, cfg)
	}
}

func TestParsePlanWithGlobalFlagsBeforeCommand(t *testing.T) {
	cfg, err := Parse([]string{"--format", "json", "--remote", "upstream", "plan", "--include-clean"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	want := Config{
		Command:      CommandPlan,
		Remote:       "upstream",
		Author:       "@me",
		MergedLimit:  30,
		Format:       output.FormatJSON,
		IncludeClean: true,
		Parallel:     1,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("config mismatch\nwant: %#v\n got: %#v", want, cfg)
	}
}

func TestParseListJSONAlias(t *testing.T) {
	cfg, err := Parse([]string{"list", "--json", "--state", "broken", "--flat"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Command != CommandList {
		t.Fatalf("command mismatch: want %q, got %q", CommandList, cfg.Command)
	}
	if cfg.Format != output.FormatJSON {
		t.Fatalf("format mismatch: want %q, got %q", output.FormatJSON, cfg.Format)
	}
	if cfg.State != "broken" || !cfg.Flat {
		t.Fatalf("list flags were not parsed: %#v", cfg)
	}
}

func TestParseVerboseShorthand(t *testing.T) {
	cfg, err := Parse([]string{"plan", "-v"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if !cfg.Verbose {
		t.Fatalf("verbose shorthand was not parsed")
	}
}

func TestParsePersistentFlagsAfterCommand(t *testing.T) {
	cfg, err := Parse([]string{"plan", "--remote", "upstream", "--json", "--include-clean"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Remote != "upstream" {
		t.Fatalf("remote mismatch: %q", cfg.Remote)
	}
	if cfg.Format != output.FormatJSON {
		t.Fatalf("format mismatch: %q", cfg.Format)
	}
	if !cfg.IncludeClean {
		t.Fatalf("include-clean was not parsed")
	}
}

func TestParsePlanRepeatableSelectors(t *testing.T) {
	cfg, err := Parse([]string{"plan", "--pr", "52", "--pr", "57", "--subtree", "60", "--chain", "80", "--chain", "81"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !reflect.DeepEqual(cfg.PRNumbers, []int{52, 57}) {
		t.Fatalf("PRNumbers mismatch: %#v", cfg.PRNumbers)
	}
	if !reflect.DeepEqual(cfg.SubtreeNums, []int{60}) {
		t.Fatalf("SubtreeNums mismatch: %#v", cfg.SubtreeNums)
	}
	if !reflect.DeepEqual(cfg.ChainNumbers, []int{80, 81}) {
		t.Fatalf("ChainNumbers mismatch: %#v", cfg.ChainNumbers)
	}

	selection := cfg.Selection()
	got := selection.Items
	want := []app.SelectionItem{
		{PRNumber: 52, Mode: app.SelectNode},
		{PRNumber: 57, Mode: app.SelectNode},
		{PRNumber: 60, Mode: app.SelectSubtree},
		{PRNumber: 80, Mode: app.SelectChain},
		{PRNumber: 81, Mode: app.SelectChain},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selection mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestParseListRejectsSelectors(t *testing.T) {
	_, err := Parse([]string{"list", "--chain", "52"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseMergeRejectsWorktreeDir(t *testing.T) {
	_, err := Parse([]string{"merge", "--worktree-dir", "/tmp/worktrees"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParseMergeRequiresPositiveParallel(t *testing.T) {
	_, err := Parse([]string{"merge", "--parallel", "0"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

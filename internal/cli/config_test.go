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

func TestParsePlanRepeatableSelectors(t *testing.T) {
	cfg, err := Parse([]string{"plan", "--pr", "52", "--pr", "57", "--subtree", "60", "--stack", "80", "--stack", "81"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !reflect.DeepEqual(cfg.PRNumbers, []int{52, 57}) {
		t.Fatalf("PRNumbers mismatch: %#v", cfg.PRNumbers)
	}
	if !reflect.DeepEqual(cfg.SubtreeNums, []int{60}) {
		t.Fatalf("SubtreeNums mismatch: %#v", cfg.SubtreeNums)
	}
	if !reflect.DeepEqual(cfg.StackNumbers, []int{80, 81}) {
		t.Fatalf("StackNumbers mismatch: %#v", cfg.StackNumbers)
	}

	selection := cfg.Selection()
	got := selection.Items
	want := []app.SelectionItem{
		{PRNumber: 52, Mode: app.SelectNode},
		{PRNumber: 57, Mode: app.SelectNode},
		{PRNumber: 60, Mode: app.SelectSubtree},
		{PRNumber: 80, Mode: app.SelectStack},
		{PRNumber: 81, Mode: app.SelectStack},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("selection mismatch\nwant: %#v\n got: %#v", want, got)
	}
}

func TestParseListRepeatableStackSelector(t *testing.T) {
	cfg, err := Parse([]string{"list", "--stack", "52", "--stack", "80"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if !reflect.DeepEqual(cfg.StackNumbers, []int{52, 80}) {
		t.Fatalf("StackNumbers mismatch: %#v", cfg.StackNumbers)
	}
}

func TestParseMergeRequiresPositiveParallel(t *testing.T) {
	_, err := Parse([]string{"merge", "--parallel", "0"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

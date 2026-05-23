package cli

import (
	"reflect"
	"testing"

	"github.com/134130/gh-domino/internal/output"
)

func TestParseLegacyFlags(t *testing.T) {
	cfg, err := Parse([]string{"--dry-run", "--headless"})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if cfg.Command != CommandLegacy {
		t.Fatalf("command mismatch: want %q, got %q", CommandLegacy, cfg.Command)
	}
	if !cfg.DryRun || !cfg.Headless {
		t.Fatalf("legacy flags were not parsed: %#v", cfg)
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

func TestParseMergeRequiresPositiveParallel(t *testing.T) {
	_, err := Parse([]string{"merge", "--parallel", "0"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

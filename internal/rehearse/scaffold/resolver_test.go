package scaffold

import (
	"fmt"
	"strings"
	"testing"
)

// featureREADME is a minimal fixture with two ACs.
const featureREADME = `---
format: https://specscore.md/feature-specification
---
# Feature: CLI Studio Index

## Overview

Some overview text.

### AC: index-two-repos

Given a workspace with two repos
When the user runs the index command
Then both repos are listed

### AC: other-ac

Given another scenario
When something happens
Then something else occurs

## Other Section

Not an AC.
`

func readFileOk(path string) (string, error) {
	return featureREADME, nil
}

func readFileNotFound(path string) (string, error) {
	return "", fmt.Errorf("open %s: no such file or directory", path)
}

func TestResolve_ReadFileOk(t *testing.T) {
	result, err := Resolve("cli/studio/index#ac:index-two-repos", readFileOk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.FeatureSlug != "cli/studio/index" {
		t.Errorf("FeatureSlug = %q, want %q", result.FeatureSlug, "cli/studio/index")
	}
	if result.ACSlug != "index-two-repos" {
		t.Errorf("ACSlug = %q, want %q", result.ACSlug, "index-two-repos")
	}
	if len(result.RawBody) == 0 {
		t.Error("RawBody is empty, want AC body lines")
	}
	for _, line := range result.RawBody {
		if strings.HasPrefix(line, "### AC:") {
			t.Errorf("RawBody contains heading line: %q", line)
		}
	}
}

func TestResolve_ReadFileNotFound(t *testing.T) {
	_, err := Resolve("cli/studio/index#ac:index-two-repos", readFileNotFound)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "spec/features/cli/studio/index/README.md") {
		t.Errorf("error does not name the path: %v", err)
	}
}

func TestResolve_ResolveRefInvalid(t *testing.T) {
	cases := []string{
		"cli/studio",           // no #ac: separator
		"#ac:bar",              // empty feature slug
		"cli/studio/index#ac:", // empty AC slug
	}
	for _, acRef := range cases {
		_, err := Resolve(acRef, readFileOk)
		if err == nil {
			t.Fatalf("acRef %q: expected error, got nil", acRef)
		}
		if !strings.Contains(err.Error(), "AC reference must be in the form <feature-slug>#ac:<ac-slug>") {
			t.Errorf("acRef %q: error missing guidance: %v", acRef, err)
		}
	}
}

func TestResolve_ReadFileOkLastAC(t *testing.T) {
	// Resolves the last AC, which is terminated by a ## heading (not ### AC:).
	result, err := Resolve("cli/studio/index#ac:other-ac", readFileOk)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ACSlug != "other-ac" {
		t.Errorf("ACSlug = %q, want %q", result.ACSlug, "other-ac")
	}
}

func TestResolve_ResolveRefMissingAC(t *testing.T) {
	_, err := Resolve("cli/studio/index#ac:nonexistent", readFileOk)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error does not mention the missing AC slug: %v", err)
	}
	// Error should list available ACs
	if !strings.Contains(err.Error(), "index-two-repos") {
		t.Errorf("error does not list available ACs: %v", err)
	}
}

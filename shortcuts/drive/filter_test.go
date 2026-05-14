// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"strings"
	"testing"
)

func TestParseDriveIgnore(t *testing.T) {
	rules, err := parseDriveIgnore(strings.NewReader("# comment\n\nnode_modules/\n*.log\n docs/** \n"))
	if err != nil {
		t.Fatalf("parseDriveIgnore: %v", err)
	}
	want := []string{"node_modules/**", "*.log", "docs/**"}
	if len(rules) != len(want) {
		t.Fatalf("len(rules) = %d, want %d (%v)", len(rules), len(want), rules)
	}
	for i := range want {
		if rules[i] != want[i] {
			t.Fatalf("rules[%d] = %q, want %q", i, rules[i], want[i])
		}
	}
}

func TestDriveSyncFilterPrecedence(t *testing.T) {
	filter := &driveSyncFilter{
		exts:            map[string]struct{}{"md": {}},
		includes:        []string{"docs/**"},
		excludes:        []string{"docs/private/**"},
		ignoreRules:     []string{"docs/**", "tmp/**"},
		builtinExcludes: []string{".git/**"},
	}

	tests := []struct {
		name string
		rel  string
		want bool
	}{
		{name: "include overrides ignore", rel: "docs/readme.md", want: true},
		{name: "exclude beats include", rel: "docs/private/secret.md", want: false},
		{name: "ext filters non-matching file", rel: "docs/readme.txt", want: false},
		{name: "builtin excluded when not included", rel: ".git/config.md", want: false},
		{name: "include miss excluded", rel: "other/readme.md", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filter.MatchFile(tt.rel).Included
			if got != tt.want {
				t.Fatalf("MatchFile(%q) = %v, want %v", tt.rel, got, tt.want)
			}
		})
	}
}

func TestDriveSyncFilterMatchDirIncludeAncestor(t *testing.T) {
	filter := &driveSyncFilter{includes: []string{"docs/**/*.md"}}
	if !filter.MatchDir("docs").Included {
		t.Fatal("docs dir should be included as ancestor of include pattern")
	}
	if filter.MatchDir("vendor").Included {
		t.Fatal("vendor dir should not be included when include patterns only target docs")
	}
}

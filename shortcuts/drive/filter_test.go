// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"strings"
	"testing"
)

func TestParseDriveIgnore(t *testing.T) {
	t.Run("valid rules", func(t *testing.T) {
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
	})
	t.Run("invalid pattern returns error", func(t *testing.T) {
		_, err := parseDriveIgnore(strings.NewReader("[invalid\n"))
		if err == nil {
			t.Fatal("expected error for invalid glob pattern")
		}
	})
	t.Run("empty input returns nil", func(t *testing.T) {
		rules, err := parseDriveIgnore(strings.NewReader(""))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(rules) != 0 {
			t.Fatalf("expected nil rules, got %v", rules)
		}
	})
}

func TestNormalizeDrivePattern(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"*.log", "*.log"},
		{"docs/**", "docs/**"},
		{"./src/", "src/**"},
		{"/root.txt", "root.txt"},
		{"trailing/", "trailing/**"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizeDrivePattern(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("normalizeDrivePattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
	t.Run("invalid glob returns error", func(t *testing.T) {
		_, err := normalizeDrivePattern("[invalid")
		if err == nil {
			t.Fatal("expected error for invalid glob")
		}
	})
}

func TestNormalizeDrivePatterns(t *testing.T) {
	t.Run("invalid pattern returns validation error", func(t *testing.T) {
		_, err := normalizeDrivePatterns([]string{"[bad"}, "--include")
		if err == nil {
			t.Fatal("expected error for invalid pattern")
		}
		if !strings.Contains(err.Error(), "--include") {
			t.Fatalf("error should reference --include, got: %v", err)
		}
	})
	t.Run("empty patterns returns empty slice", func(t *testing.T) {
		out, err := normalizeDrivePatterns([]string{}, "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 0 {
			t.Fatalf("expected empty, got %v", out)
		}
	})
	t.Run("whitespace-only entries are skipped", func(t *testing.T) {
		out, err := normalizeDrivePatterns([]string{"  ", "*.log"}, "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 || out[0] != "*.log" {
			t.Fatalf("expected [*.log], got %v", out)
		}
	})
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

func TestDriveSyncFilterMatchFileIgnoreAndBuiltin(t *testing.T) {
	t.Run("ignore file excludes when no include", func(t *testing.T) {
		filter := &driveSyncFilter{
			ignoreRules: []string{"tmp/**"},
		}
		if filter.MatchFile("tmp/cache.dat").Included {
			t.Fatal("file matching .larkignore should be excluded")
		}
	})
	t.Run("builtin excludes when no include", func(t *testing.T) {
		filter := &driveSyncFilter{
			builtinExcludes: []string{".git/**"},
		}
		if filter.MatchFile(".git/config").Included {
			t.Fatal("file matching builtin excludes should be excluded")
		}
	})
	t.Run("default allow when no rules match", func(t *testing.T) {
		filter := &driveSyncFilter{}
		if !filter.MatchFile("readme.md").Included {
			t.Fatal("file with no matching rules should be included by default")
		}
	})
	t.Run("empty rel excluded", func(t *testing.T) {
		filter := &driveSyncFilter{}
		if filter.MatchFile("").Included {
			t.Fatal("empty rel should be excluded")
		}
		if filter.MatchFile(".").Included {
			t.Fatal("dot rel should be excluded")
		}
	})
}

func TestDriveSyncFilterMatchDir(t *testing.T) {
	t.Run("root dir always included", func(t *testing.T) {
		filter := &driveSyncFilter{includes: []string{"docs/**"}}
		if !filter.MatchDir("").Included {
			t.Fatal("empty dir should be included")
		}
		if !filter.MatchDir(".").Included {
			t.Fatal("dot dir should be included")
		}
	})
	t.Run("exclude overrides all", func(t *testing.T) {
		filter := &driveSyncFilter{
			excludes: []string{"vendor/**"},
			includes: []string{"vendor/**"},
		}
		if filter.MatchDir("vendor").Included {
			t.Fatal("excluded dir should not be included even with matching include")
		}
	})
	t.Run("ignore file excludes dir", func(t *testing.T) {
		filter := &driveSyncFilter{
			ignoreRules: []string{"tmp/**"},
		}
		if filter.MatchDir("tmp").Included {
			t.Fatal("dir matching .larkignore should be excluded")
		}
	})
	t.Run("builtin excludes dir", func(t *testing.T) {
		filter := &driveSyncFilter{
			builtinExcludes: []string{".git/**"},
		}
		if filter.MatchDir(".git").Included {
			t.Fatal("dir matching builtin excludes should be excluded")
		}
	})
	t.Run("default allow when no rules", func(t *testing.T) {
		filter := &driveSyncFilter{}
		if !filter.MatchDir("anydir").Included {
			t.Fatal("dir with no rules should be included by default")
		}
	})
}

func TestDriveSyncFilterMatchDirIncludeAncestor(t *testing.T) {
	filter := &driveSyncFilter{includes: []string{"docs/**/*.md"}}
	if !filter.MatchDir("docs").Included {
		t.Fatal("docs dir should be included as ancestor of include pattern")
	}
	if !filter.MatchDir("docs/sub").Included {
		t.Fatal("docs/sub dir should be included as ancestor of include pattern docs/**/*.md")
	}
	if filter.MatchDir("vendor").Included {
		t.Fatal("vendor dir should not be included when include patterns only target docs")
	}
}

func TestDriveConcreteDirPrefix(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"docs/**/*.md", "docs/"},
		{"src/lib/*.go", "src/lib/"},
		{"docs/**", "docs/"},
		{"*.log", ""},
		{"**/*.tmp", ""},
		{"a/b/c/*.txt", "a/b/c/"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			got := driveConcreteDirPrefix(tt.pattern)
			if got != tt.want {
				t.Fatalf("driveConcreteDirPrefix(%q) = %q, want %q", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestDriveMatchPattern(t *testing.T) {
	t.Run("empty inputs return false", func(t *testing.T) {
		if driveMatchPattern("", "file.txt") {
			t.Fatal("empty pattern should not match")
		}
		if driveMatchPattern("*.txt", "") {
			t.Fatal("empty rel should not match")
		}
	})
	t.Run("glob star suffix exact match", func(t *testing.T) {
		if !driveMatchPattern("docs/**", "docs") {
			t.Fatal("docs/** should match docs exactly")
		}
	})
	t.Run("bare name matches at any depth", func(t *testing.T) {
		if !driveMatchPattern("*.log", "error.log") {
			t.Fatal("*.log should match error.log at root")
		}
		if !driveMatchPattern("*.log", "sub/error.log") {
			t.Fatal("*.log should match error.log in subdirectory")
		}
	})
	t.Run("full glob match", func(t *testing.T) {
		if !driveMatchPattern("docs/**/*.md", "docs/sub/readme.md") {
			t.Fatal("docs/**/*.md should match docs/sub/readme.md")
		}
	})
}

func TestFilterDriveRemoteEntries(t *testing.T) {
	t.Run("nil filter returns all", func(t *testing.T) {
		entries := []driveRemoteEntry{
			{RelPath: "a.txt", Type: driveTypeFile},
			{RelPath: "b.md", Type: driveTypeFile},
		}
		got := filterDriveRemoteEntries(entries, nil)
		if len(got) != 2 {
			t.Fatalf("nil filter should return all entries, got %d", len(got))
		}
	})
	t.Run("filters files and dirs", func(t *testing.T) {
		filter := &driveSyncFilter{exts: map[string]struct{}{"md": {}}, includes: []string{"docs/**"}}
		entries := []driveRemoteEntry{
			{RelPath: "a.txt", Type: driveTypeFile},
			{RelPath: "docs/b.md", Type: driveTypeFile},
			{RelPath: "other", Type: driveTypeFolder},
		}
		got := filterDriveRemoteEntries(entries, filter)
		if len(got) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(got))
		}
		if got[0].RelPath != "docs/b.md" {
			t.Fatalf("expected docs/b.md, got %s", got[0].RelPath)
		}
	})
}

func TestFilterDrivePushLocalView(t *testing.T) {
	t.Run("nil filter returns all with parent dirs", func(t *testing.T) {
		files := map[string]drivePushLocalFile{
			"sub/a.txt": {},
			"b.txt":     {},
		}
		dirs := []string{"other"}
		filtered, filteredDirs := filterDrivePushLocalView(files, dirs, nil)
		if len(filtered) != 2 {
			t.Fatalf("expected 2 files, got %d", len(filtered))
		}
		// "sub" should appear as parent dir of "sub/a.txt", plus "other"
		foundSub := false
		foundOther := false
		for _, d := range filteredDirs {
			if d == "sub" {
				foundSub = true
			}
			if d == "other" {
				foundOther = true
			}
		}
		if !foundSub {
			t.Fatal("expected 'sub' in dirs")
		}
		if !foundOther {
			t.Fatal("expected 'other' in dirs")
		}
	})
	t.Run("filter removes non-matching files and dirs", func(t *testing.T) {
		files := map[string]drivePushLocalFile{
			"a.txt": {},
			"b.md":  {},
		}
		dirs := []string{"sub"}
		filter := &driveSyncFilter{exts: map[string]struct{}{"md": {}}, includes: []string{"docs/**"}}
		filtered, filteredDirs := filterDrivePushLocalView(files, dirs, filter)
		if len(filtered) != 0 {
			t.Fatalf("expected 0 files (no docs/ prefix), got %d", len(filtered))
		}
		// "sub" dir should be excluded (include only targets docs/)
		for _, d := range filteredDirs {
			if d == "sub" {
				t.Fatal("sub dir should be filtered out")
			}
		}
	})
}

func TestSortedDriveDirs(t *testing.T) {
	dirs := sortedDriveDirs(map[string]struct{}{
		"a/b":  {},
		"a":    {},
		"x/y/z": {},
		"x":    {},
	})
	// Should be sorted by depth then name
	expected := []string{"a", "x", "a/b", "x/y/z"}
	if len(dirs) != len(expected) {
		t.Fatalf("expected %d dirs, got %d: %v", len(expected), len(dirs), dirs)
	}
	for i, e := range expected {
		if dirs[i] != e {
			t.Fatalf("dirs[%d] = %q, want %q", i, dirs[i], e)
		}
	}
}

func TestFilterDriveStatusLocalFiles(t *testing.T) {
	t.Run("nil filter returns all", func(t *testing.T) {
		files := map[string]driveStatusLocalFile{
			"a.txt": {},
			"b.md":  {},
		}
		got := filterDriveStatusLocalFiles(files, nil)
		if len(got) != 2 {
			t.Fatalf("nil filter should return all, got %d", len(got))
		}
	})
	t.Run("filter by ext", func(t *testing.T) {
		files := map[string]driveStatusLocalFile{
			"a.txt": {},
			"b.md":  {},
		}
		filter := &driveSyncFilter{exts: map[string]struct{}{"md": {}}}
		got := filterDriveStatusLocalFiles(files, filter)
		if len(got) != 1 {
			t.Fatalf("expected 1 file, got %d", len(got))
		}
		if _, ok := got["b.md"]; !ok {
			t.Fatal("expected b.md in filtered files")
		}
	})
}

func TestFilterDrivePullLocalAbsPaths(t *testing.T) {
	t.Run("nil filter returns all", func(t *testing.T) {
		paths := []string{"/root/a.txt", "/root/b.md"}
		got, err := filterDrivePullLocalAbsPaths("/root", paths, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("nil filter should return all, got %d", len(got))
		}
	})
	t.Run("filter by ext", func(t *testing.T) {
		paths := []string{"/root/a.txt", "/root/b.md"}
		filter := &driveSyncFilter{exts: map[string]struct{}{"md": {}}}
		got, err := filterDrivePullLocalAbsPaths("/root", paths, filter)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0] != "/root/b.md" {
			t.Fatalf("expected [/root/b.md], got %v", got)
		}
	})
}

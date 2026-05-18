// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// driveIgnoreFileName is the name of the ignore file read from the sync root.
const driveIgnoreFileName = ".larkignore"

// driveSyncBuiltinExcludePatterns lists glob patterns that are always excluded
// from drive sync unless overridden by --include.
var driveSyncBuiltinExcludePatterns = []string{
	".lark-sync/**",
	".git/**",
	".DS_Store",
	"Thumbs.db",
	"**/*~",
	"**/*.swp",
	"**/*.tmp",
	"**/*.temp",
	"node_modules/**",
	"dist/**",
	"build/**",
	"coverage/**",
}

// driveSyncFilter applies filtering rules to drive sync file paths.
// Precedence: CLI flags (--exclude > --include > --ext) > .larkignore > built-in excludes.
type driveSyncFilter struct {
	exts            map[string]struct{}
	includes        []string
	excludes        []string
	ignoreRules     []string
	builtinExcludes []string
}

// driveFilterDecision reports whether a path is included and why.
type driveFilterDecision struct {
	Included bool
	Reason   string
}

// buildDriveSyncFilter constructs a driveSyncFilter from CLI flags and the
// .larkignore file in localDir. Returns a validation error for invalid patterns.
func buildDriveSyncFilter(runtime *common.RuntimeContext, localDir string) (*driveSyncFilter, error) {
	ignoreRules, err := readDriveIgnoreRules(runtime, localDir)
	if err != nil {
		return nil, err
	}
	includes, err := normalizeDrivePatterns(runtime.StrSlice("include"), "--include")
	if err != nil {
		return nil, err
	}
	excludes, err := normalizeDrivePatterns(runtime.StrSlice("exclude"), "--exclude")
	if err != nil {
		return nil, err
	}
	builtinExcludes, err := normalizeDrivePatterns(driveSyncBuiltinExcludePatterns, "built-in excludes")
	if err != nil {
		return nil, err
	}
	exts := make(map[string]struct{})
	for _, ext := range runtime.StrSlice("ext") {
		ext = strings.TrimSpace(strings.ToLower(ext))
		if ext == "" {
			continue
		}
		ext = strings.TrimPrefix(ext, ".")
		if ext == "" {
			return nil, output.ErrValidation("--ext contains an empty extension")
		}
		exts[ext] = struct{}{}
	}
	return &driveSyncFilter{
		exts:            exts,
		includes:        includes,
		excludes:        excludes,
		ignoreRules:     ignoreRules,
		builtinExcludes: builtinExcludes,
	}, nil
}

// readDriveIgnoreRules reads and parses .larkignore from localDir.
// Returns nil rules (no error) when the file does not exist.
func readDriveIgnoreRules(runtime *common.RuntimeContext, localDir string) ([]string, error) {
	ignorePath := filepath.Join(localDir, driveIgnoreFileName)
	if _, err := runtime.FileIO().Stat(ignorePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, common.WrapInputStatError(err)
	}
	f, err := runtime.FileIO().Open(ignorePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, common.WrapInputStatError(err)
	}
	defer f.Close()
	rules, err := parseDriveIgnore(f)
	if err != nil {
		return nil, output.ErrValidation("%s: %s", ignorePath, err)
	}
	return rules, nil
}

// parseDriveIgnore parses ignore rules from r. Blank lines and lines
// starting with "#" are skipped. Each rule is normalized via normalizeDrivePattern.
func parseDriveIgnore(r io.Reader) ([]string, error) {
	var rules []string
	scanner := bufio.NewScanner(r)
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		normalized, err := normalizeDrivePattern(line)
		if err != nil {
			return nil, fmt.Errorf("line %d invalid pattern %q: %w", lineNo, line, err)
		}
		rules = append(rules, normalized)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// normalizeDrivePatterns validates and normalizes a slice of glob patterns.
// source is used in error messages to identify the origin of a bad pattern.
func normalizeDrivePatterns(patterns []string, source string) ([]string, error) {
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		normalized, err := normalizeDrivePattern(pattern)
		if err != nil {
			return nil, output.ErrValidation("%s contains invalid glob %q: %s", source, pattern, err)
		}
		if normalized == "" {
			continue
		}
		out = append(out, normalized)
	}
	return out, nil
}

// normalizeDrivePattern normalizes a single glob pattern: trims whitespace,
// converts to forward slashes, strips leading "./" and "/", appends "/**" to
// trailing slashes, and validates the pattern with doublestar.
func normalizeDrivePattern(pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "", nil
	}
	pattern = filepath.ToSlash(pattern)
	pattern = strings.TrimPrefix(pattern, "./")
	pattern = strings.TrimPrefix(pattern, "/")
	if strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimSuffix(pattern, "/") + "/**"
	}
	if _, err := doublestar.Match(pattern, ""); err != nil {
		return "", err
	}
	return pattern, nil
}

// MatchFile returns the filter decision for a file at the given relative path.
// The precedence order is: --exclude > --include miss > --ext miss > --include > .larkignore > built-in excludes > default allow.
func (f *driveSyncFilter) MatchFile(rel string) driveFilterDecision {
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." {
		return driveFilterDecision{Included: false, Reason: "default"}
	}
	if matchedAny(rel, f.excludes) {
		return driveFilterDecision{Included: false, Reason: "flag_exclude"}
	}
	if len(f.includes) > 0 && !matchedAny(rel, f.includes) {
		return driveFilterDecision{Included: false, Reason: "flag_include_miss"}
	}
	if len(f.exts) > 0 {
		ext := strings.TrimPrefix(strings.ToLower(path.Ext(rel)), ".")
		if _, ok := f.exts[ext]; !ok {
			return driveFilterDecision{Included: false, Reason: "flag_ext_miss"}
		}
	}
	if matchedAny(rel, f.includes) {
		return driveFilterDecision{Included: true, Reason: "flag_include"}
	}
	if matchedAny(rel, f.ignoreRules) {
		return driveFilterDecision{Included: false, Reason: "ignore_file"}
	}
	if matchedAny(rel, f.builtinExcludes) {
		return driveFilterDecision{Included: false, Reason: "builtin"}
	}
	return driveFilterDecision{Included: true, Reason: "default"}
}

// MatchDir returns the filter decision for a directory at the given relative path.
// Directories are included if they are ancestors of an include pattern so that
// directory traversal can reach the matching files beneath them.
func (f *driveSyncFilter) MatchDir(rel string) driveFilterDecision {
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." {
		return driveFilterDecision{Included: true, Reason: "default"}
	}
	if matchedAny(rel, f.excludes) {
		return driveFilterDecision{Included: false, Reason: "flag_exclude"}
	}
	if len(f.includes) > 0 && matchedAny(rel, f.includes) {
		return driveFilterDecision{Included: true, Reason: "flag_include"}
	}
	if matchedAny(rel, f.ignoreRules) {
		return driveFilterDecision{Included: false, Reason: "ignore_file"}
	}
	if matchedAny(rel, f.builtinExcludes) {
		return driveFilterDecision{Included: false, Reason: "builtin"}
	}
	if len(f.includes) > 0 {
		prefix := rel + "/"
		for _, pattern := range f.includes {
			// Direct prefix match: pattern starts with "docs/sub/".
			if strings.HasPrefix(pattern, prefix) {
				return driveFilterDecision{Included: true, Reason: "flag_include_ancestor"}
			}
			// Extract the leading non-wildcard directory prefix from the
			// pattern. For "docs/**/*.md" the concrete prefix is "docs/",
			// so "docs" and "docs/sub" are ancestors. For "src/lib/*.go"
			// the concrete prefix is "src/lib/".
			concretePrefix := driveConcreteDirPrefix(pattern)
			if concretePrefix == "" {
				continue
			}
			// rel is an ancestor if its path falls within the concrete
			// prefix (e.g. "docs" is a prefix of "docs/") or the
			// concrete prefix falls within rel (e.g. "docs/" is a
			// prefix of "docs/sub/").
			if strings.HasPrefix(prefix, concretePrefix) || strings.HasPrefix(concretePrefix, prefix) {
				return driveFilterDecision{Included: true, Reason: "flag_include_ancestor"}
			}
		}
		return driveFilterDecision{Included: false, Reason: "flag_include_miss"}
	}
	return driveFilterDecision{Included: true, Reason: "default"}
}

// driveConcreteDirPrefix returns the leading non-wildcard directory portion of
// a glob pattern, with a trailing "/". For example:
//   - "docs/**/*.md" → "docs/"
//   - "src/lib/*.go" → "src/lib/"
//   - "docs/**"      → "docs/"
//   - "*.log"        → "" (no concrete directory prefix)
func driveConcreteDirPrefix(pattern string) string {
	segments := strings.Split(pattern, "/")
	var concrete []string
	for _, seg := range segments {
		if strings.ContainsAny(seg, "*?[") {
			break
		}
		concrete = append(concrete, seg)
	}
	if len(concrete) == 0 {
		return ""
	}
	return strings.Join(concrete, "/") + "/"
}

// matchedAny reports whether rel matches any of the given glob patterns.
func matchedAny(rel string, patterns []string) bool {
	for _, pattern := range patterns {
		if driveMatchPattern(pattern, rel) {
			return true
		}
	}
	return false
}

// driveMatchPattern matches a single glob pattern against rel.
// It handles "/**" suffix matching, doublestar glob matching, and
// bare-name patterns (no "/") which are matched at any depth via "**/" prefix.
func driveMatchPattern(pattern, rel string) bool {
	if pattern == "" || rel == "" {
		return false
	}
	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		if rel == base {
			return true
		}
	}
	if ok, err := doublestar.Match(pattern, rel); err == nil && ok {
		return true
	}
	if !strings.Contains(pattern, "/") {
		if ok, err := doublestar.Match("**/"+pattern, rel); err == nil && ok {
			return true
		}
	}
	return false
}

// filterDriveRemoteEntries filters remote Drive entries using the filter.
// Folders are matched with MatchDir; files are matched with MatchFile.
func filterDriveRemoteEntries(entries []driveRemoteEntry, filter *driveSyncFilter) []driveRemoteEntry {
	if filter == nil {
		return entries
	}
	out := make([]driveRemoteEntry, 0, len(entries))
	for _, entry := range entries {
		decision := filter.MatchFile(entry.RelPath)
		if entry.Type == driveTypeFolder {
			decision = filter.MatchDir(entry.RelPath)
		}
		if decision.Included {
			out = append(out, entry)
		}
	}
	return out
}

// filterDrivePushLocalView filters local files and directories for the push
// flow. It returns the filtered file map and a sorted list of directories
// (both parent dirs of kept files and explicitly included dirs).
func filterDrivePushLocalView(files map[string]drivePushLocalFile, dirs []string, filter *driveSyncFilter) (map[string]drivePushLocalFile, []string) {
	if filter == nil {
		dirsSet := make(map[string]struct{})
		for rel := range files {
			for parent := drivePushParentRel(rel); parent != ""; parent = drivePushParentRel(parent) {
				dirsSet[parent] = struct{}{}
			}
		}
		for _, dir := range dirs {
			dirsSet[dir] = struct{}{}
		}
		return files, sortedDriveDirs(dirsSet)
	}
	filtered := make(map[string]drivePushLocalFile, len(files))
	dirsSet := make(map[string]struct{})
	for rel, file := range files {
		if !filter.MatchFile(rel).Included {
			continue
		}
		filtered[rel] = file
		for parent := drivePushParentRel(rel); parent != ""; parent = drivePushParentRel(parent) {
			dirsSet[parent] = struct{}{}
		}
	}
	for _, dir := range dirs {
		if filter.MatchDir(dir).Included {
			dirsSet[dir] = struct{}{}
		}
	}
	return filtered, sortedDriveDirs(dirsSet)
}

// sortedDriveDirs returns the directory set sorted by depth then name.
func sortedDriveDirs(dirsSet map[string]struct{}) []string {
	dirs := make([]string, 0, len(dirsSet))
	for d := range dirsSet {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool {
		di, dj := strings.Count(dirs[i], "/"), strings.Count(dirs[j], "/")
		if di != dj {
			return di < dj
		}
		return dirs[i] < dirs[j]
	})
	return dirs
}

// filterDriveStatusLocalFiles filters the local file info map by the filter.
func filterDriveStatusLocalFiles(files map[string]driveStatusLocalFile, filter *driveSyncFilter) map[string]driveStatusLocalFile {
	if filter == nil {
		return files
	}
	filtered := make(map[string]driveStatusLocalFile, len(files))
	for rel, file := range files {
		if filter.MatchFile(rel).Included {
			filtered[rel] = file
		}
	}
	return filtered
}

// filterDrivePullLocalAbsPaths filters absolute local paths for the pull
// --delete-local flow, keeping only paths whose relative form passes the filter.
func filterDrivePullLocalAbsPaths(root string, absPaths []string, filter *driveSyncFilter) ([]string, error) {
	if filter == nil {
		return absPaths, nil
	}
	filtered := make([]string, 0, len(absPaths))
	for _, absPath := range absPaths {
		rel, err := filepath.Rel(root, absPath)
		if err != nil {
			return nil, output.Errorf(output.ExitInternal, "io", "rel %s: %s", absPath, err)
		}
		if filter.MatchFile(filepath.ToSlash(rel)).Included {
			filtered = append(filtered, absPath)
		}
	}
	return filtered, nil
}

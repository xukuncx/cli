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

const driveIgnoreFileName = ".larkignore"

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

type driveSyncFilter struct {
	exts            map[string]struct{}
	includes        []string
	excludes        []string
	ignoreRules     []string
	builtinExcludes []string
}

type driveFilterDecision struct {
	Included bool
	Reason   string
}

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
			base := strings.TrimSuffix(pattern, "/**")
			if strings.HasPrefix(pattern, prefix) || strings.HasPrefix(base, prefix) {
				return driveFilterDecision{Included: true, Reason: "flag_include_ancestor"}
			}
		}
		return driveFilterDecision{Included: false, Reason: "flag_include_miss"}
	}
	return driveFilterDecision{Included: true, Reason: "default"}
}

func matchedAny(rel string, patterns []string) bool {
	for _, pattern := range patterns {
		if driveMatchPattern(pattern, rel) {
			return true
		}
	}
	return false
}

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

func filterDriveStatusLocalHashes(files map[string]string, filter *driveSyncFilter) map[string]string {
	if filter == nil {
		return files
	}
	filtered := make(map[string]string, len(files))
	for rel, hash := range files {
		if filter.MatchFile(rel).Included {
			filtered[rel] = hash
		}
	}
	return filtered
}

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

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy

import (
	"errors"
	"fmt"
	"os"

	"github.com/larksuite/cli/extension/platform"
	pyaml "github.com/larksuite/cli/internal/cmdpolicy/yaml"
	"github.com/larksuite/cli/internal/vfs"
)

type SourceKind string

const (
	SourcePlugin SourceKind = "plugin"
	SourceYAML   SourceKind = "yaml"
	SourceNone   SourceKind = "none"
)

type ResolveSource struct {
	Kind SourceKind
	Name string
}

type PluginRule struct {
	PluginName string
	Rule       *platform.Rule
}

type Sources struct {
	PluginRules []PluginRule
	YAMLRule    *platform.Rule
	YAMLPath    string
}

var ErrMultipleRestricts = errors.New("multiple plugins called Restrict; only one is permitted")

// Resolve picks by precedence: plugin > yaml > none. Pure function; load
// yaml via LoadYAMLPolicy first. Winner is validated.
func Resolve(s Sources) (*platform.Rule, ResolveSource, error) {
	if len(s.PluginRules) > 1 {
		names := make([]string, len(s.PluginRules))
		for i, pr := range s.PluginRules {
			names[i] = pr.PluginName
		}
		return nil, ResolveSource{}, fmt.Errorf("%w: %v", ErrMultipleRestricts, names)
	}

	if len(s.PluginRules) == 1 {
		pr := s.PluginRules[0]
		if err := ValidateRule(pr.Rule); err != nil {
			return nil, ResolveSource{}, fmt.Errorf("plugin %q rule invalid: %w", pr.PluginName, err)
		}
		return pr.Rule, ResolveSource{Kind: SourcePlugin, Name: pr.PluginName}, nil
	}

	if s.YAMLRule != nil {
		if err := ValidateRule(s.YAMLRule); err != nil {
			return nil, ResolveSource{}, fmt.Errorf("policy yaml %q: %w", s.YAMLPath, err)
		}
		return s.YAMLRule, ResolveSource{Kind: SourceYAML, Name: s.YAMLPath}, nil
	}

	return nil, ResolveSource{Kind: SourceNone}, nil
}

// LoadYAMLPolicy returns (nil, nil) when path is empty or file is absent,
// so callers can pass the result straight into Sources.YAMLRule.
func LoadYAMLPolicy(path string) (*platform.Rule, error) {
	if path == "" {
		return nil, nil
	}
	if _, err := vfs.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat policy yaml %q: %w", path, err)
	}
	data, err := vfs.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy yaml %q: %w", path, err)
	}
	rule, err := pyaml.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("policy yaml %q: %w", path, err)
	}
	return rule, nil
}

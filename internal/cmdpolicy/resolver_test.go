// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdpolicy_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
)

func TestResolve_singlePluginWins(t *testing.T) {
	rule := &platform.Rule{Name: "secaudit"}
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		PluginRules: []cmdpolicy.PluginRule{{PluginName: "secaudit", Rule: rule}},
	})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if got != rule || src.Kind != cmdpolicy.SourcePlugin || src.Name != "secaudit" {
		t.Fatalf("Resolve = (%v, %+v)", got, src)
	}
}

func TestResolve_pluginShadowsYaml(t *testing.T) {
	pluginRule := &platform.Rule{Name: "from-plugin"}
	yamlRule := &platform.Rule{Name: "from-yaml"}
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		PluginRules: []cmdpolicy.PluginRule{{PluginName: "secaudit", Rule: pluginRule}},
		YAMLRule:    yamlRule,
		YAMLPath:    "/some/policy.yml",
	})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if got.Name != "from-plugin" || src.Kind != cmdpolicy.SourcePlugin {
		t.Fatalf("plugin should shadow yaml, got %+v / %+v", got, src)
	}
}

func TestResolve_yamlWhenNoPlugin(t *testing.T) {
	yamlRule := &platform.Rule{Name: "from-yaml", MaxRisk: "read"}
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		YAMLRule: yamlRule,
		YAMLPath: "/some/policy.yml",
	})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if got.Name != "from-yaml" || src.Kind != cmdpolicy.SourceYAML {
		t.Fatalf("yaml should win when no plugin, got %+v / %+v", got, src)
	}
	if src.Name != "/some/policy.yml" {
		t.Errorf("yaml source Name should carry path, got %q", src.Name)
	}
}

func TestResolve_emptyEverythingIsNone(t *testing.T) {
	got, src, err := cmdpolicy.Resolve(cmdpolicy.Sources{})
	if err != nil {
		t.Fatalf("Resolve err: %v", err)
	}
	if got != nil || src.Kind != cmdpolicy.SourceNone {
		t.Fatalf("expected (nil, SourceNone), got (%v, %+v)", got, src)
	}
}

// Two plugins both contributing a Rule must produce the typed error so
// the bootstrap pipeline aborts (hard-constraint #7).
func TestResolve_multipleRestrictIsError(t *testing.T) {
	_, _, err := cmdpolicy.Resolve(cmdpolicy.Sources{
		PluginRules: []cmdpolicy.PluginRule{
			{PluginName: "a", Rule: &platform.Rule{Name: "a"}},
			{PluginName: "b", Rule: &platform.Rule{Name: "b"}},
		},
	})
	if !errors.Is(err, cmdpolicy.ErrMultipleRestricts) {
		t.Fatalf("err = %v, want ErrMultipleRestricts", err)
	}
}

// LoadYAMLPolicy: missing file returns (nil, nil) silently so callers
// can pass the result straight into Sources.YAMLRule without special-
// casing not-exist.
func TestLoadYAMLPolicy_missingIsSilent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent-policy.yml")
	rule, err := cmdpolicy.LoadYAMLPolicy(missing)
	if err != nil {
		t.Fatalf("missing yaml should not error, got %v", err)
	}
	if rule != nil {
		t.Fatalf("missing yaml should return nil rule, got %+v", rule)
	}
}

func TestLoadYAMLPolicy_emptyPathIsNoop(t *testing.T) {
	rule, err := cmdpolicy.LoadYAMLPolicy("")
	if err != nil {
		t.Fatalf("empty path should not error, got %v", err)
	}
	if rule != nil {
		t.Fatalf("empty path should return nil rule, got %+v", rule)
	}
}

func TestLoadYAMLPolicy_parsesValid(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "policy.yml")
	if err := os.WriteFile(yamlPath, []byte("name: from-yaml\nmax_risk: read\n"), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	rule, err := cmdpolicy.LoadYAMLPolicy(yamlPath)
	if err != nil {
		t.Fatalf("LoadYAMLPolicy err: %v", err)
	}
	if rule == nil || rule.Name != "from-yaml" {
		t.Fatalf("expected rule with name=from-yaml, got %+v", rule)
	}
}

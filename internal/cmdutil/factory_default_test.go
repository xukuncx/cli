// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	_ "github.com/larksuite/cli/extension/credential/env"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/envvars"
	"github.com/larksuite/cli/internal/lockfile"
	"github.com/larksuite/cli/internal/tracking"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

type countingFileIOProvider struct {
	resolveCalls int
}

func (p *countingFileIOProvider) Name() string { return "counting" }

func (p *countingFileIOProvider) ResolveFileIO(context.Context) fileio.FileIO {
	p.resolveCalls++
	return &localfileio.LocalFileIO{}
}

func TestNewDefault_InvocationProfileUsedByStrictModeAndConfig(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")

	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	bot := core.StrictModeBot
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "app-default",
				AppSecret: core.PlainSecret("secret-default"),
				Brand:     core.BrandFeishu,
			},
			{
				Name:       "target",
				AppId:      "app-target",
				AppSecret:  core.PlainSecret("secret-target"),
				Brand:      core.BrandFeishu,
				StrictMode: &bot,
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f := NewDefault(nil, InvocationContext{Profile: "target"})
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeBot {
		t.Fatalf("ResolveStrictMode() = %q, want %q", got, core.StrictModeBot)
	}
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if cfg.ProfileName != "target" {
		t.Fatalf("Config() profile = %q, want %q", cfg.ProfileName, "target")
	}
	if cfg.AppID != "app-target" {
		t.Fatalf("Config() appID = %q, want %q", cfg.AppID, "app-target")
	}
}

func TestNewDefault_InvocationProfileMissingSticksAcrossEarlyStrictMode(t *testing.T) {
	t.Setenv(envvars.CliAppID, "")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")

	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{
			{
				Name:      "default",
				AppId:     "app-default",
				AppSecret: core.PlainSecret("secret-default"),
				Brand:     core.BrandFeishu,
			},
		},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f := NewDefault(nil, InvocationContext{Profile: "missing"})
	if got := f.ResolveStrictMode(context.Background()); got != core.StrictModeOff {
		t.Fatalf("ResolveStrictMode() = %q, want %q", got, core.StrictModeOff)
	}
	_, err := f.Config()
	if err == nil {
		t.Fatal("Config() error = nil, want non-nil")
	}
	var cfgErr *core.ConfigError
	if !errors.As(err, &cfgErr) {
		t.Fatalf("Config() error type = %T, want *core.ConfigError", err)
	}
	if cfgErr.Message != `profile "missing" not found` {
		t.Fatalf("Config() error message = %q, want %q", cfgErr.Message, `profile "missing" not found`)
	}
}

func TestNewDefault_ResolveAs_UsesDefaultAsFromEnvAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "env-secret")
	t.Setenv(envvars.CliDefaultAs, "user")
	t.Setenv(envvars.CliUserAccessToken, "")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(nil, InvocationContext{})
	cmd := newCmdWithAsFlag("auto", false)

	got := f.ResolveAs(context.Background(), cmd, "auto")
	if got != core.AsUser {
		t.Fatalf("ResolveAs() = %q, want %q", got, core.AsUser)
	}
	if f.IdentityAutoDetected {
		t.Fatal("IdentityAutoDetected = true, want false")
	}
}

func TestNewDefault_ConfigReturnsCliConfigCopyOfCredentialAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "env-secret")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliUserAccessToken, "uat-token")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(nil, InvocationContext{})

	acct, err := f.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}

	cfg.AppID = "mutated-cli-config"
	if acct.AppID != "env-app" {
		t.Fatalf("credential account mutated via Config(): got %q, want %q", acct.AppID, "env-app")
	}
}

func TestNewDefault_ConfigUsesRuntimePlaceholderForTokenOnlyEnvAccount(t *testing.T) {
	t.Setenv(envvars.CliAppID, "env-app")
	t.Setenv(envvars.CliAppSecret, "")
	t.Setenv(envvars.CliDefaultAs, "")
	t.Setenv(envvars.CliUserAccessToken, "uat-token")
	t.Setenv(envvars.CliTenantAccessToken, "")
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	f := NewDefault(nil, InvocationContext{})

	acct, err := f.Credential.ResolveAccount(context.Background())
	if err != nil {
		t.Fatalf("ResolveAccount() error = %v", err)
	}
	if acct.AppSecret != "" {
		t.Fatalf("credential account AppSecret = %q, want empty string", acct.AppSecret)
	}

	cfg, err := f.Config()
	if err != nil {
		t.Fatalf("Config() error = %v", err)
	}
	if cfg.AppSecret != "" {
		t.Fatalf("Config().AppSecret = %q, want empty string for token-only account", cfg.AppSecret)
	}
	if credential.HasRealAppSecret(cfg.AppSecret) {
		t.Fatalf("Config().AppSecret = %q, want token-only no-secret marker", cfg.AppSecret)
	}
}

func TestNewDefault_FileIOProviderDoesNotResolveDuringInitialization(t *testing.T) {
	prev := fileio.GetProvider()
	provider := &countingFileIOProvider{}
	fileio.Register(provider)
	t.Cleanup(func() { fileio.Register(prev) })

	f := NewDefault(nil, InvocationContext{})
	if f.FileIOProvider != provider {
		t.Fatalf("NewDefault() provider = %T, want %T", f.FileIOProvider, provider)
	}
	if provider.resolveCalls != 0 {
		t.Fatalf("ResolveFileIO() calls after NewDefault() = %d, want 0", provider.resolveCalls)
	}

	if got := f.ResolveFileIO(context.Background()); got == nil {
		t.Fatal("ResolveFileIO() = nil, want non-nil")
	}
	if provider.resolveCalls != 1 {
		t.Fatalf("ResolveFileIO() calls after explicit resolve = %d, want 1", provider.resolveCalls)
	}
}

func TestLoadOrCreateAuthLogUserUniqueID_PersistsInConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:     "cli_test",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	id, err := loadOrCreateAuthLogUserUniqueID()
	if err != nil {
		t.Fatalf("loadOrCreateAuthLogUserUniqueID() error = %v", err)
	}
	if id == "" {
		t.Fatal("loadOrCreateAuthLogUserUniqueID() returned empty id")
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("uuid.Parse(id) error = %v", err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("uuid version = %d, want 7", parsed.Version())
	}

	reloaded, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if reloaded.UserUniqueID != id {
		t.Fatalf("config userUniqueId = %q, want %q", reloaded.UserUniqueID, id)
	}

	id2, err := loadOrCreateAuthLogUserUniqueID()
	if err != nil {
		t.Fatalf("second loadOrCreateAuthLogUserUniqueID() error = %v", err)
	}
	if id2 != id {
		t.Fatalf("second id = %q, want %q", id2, id)
	}
}

func TestLoadOrCreateAuthLogUserUniqueID_UpgradesConfigWithoutField(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	raw := []byte(`{
	  "currentApp": "default",
	  "apps": [
	    {
	      "name": "default",
	      "appId": "cli_test",
	      "appSecret": "secret",
	      "brand": "feishu"
	    }
	  ]
	}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), raw, 0600); err != nil {
		t.Fatalf("WriteFile(config.json) error = %v", err)
	}

	id, err := loadOrCreateAuthLogUserUniqueID()
	if err != nil {
		t.Fatalf("loadOrCreateAuthLogUserUniqueID() error = %v", err)
	}
	parsed, err := uuid.Parse(id)
	if err != nil {
		t.Fatalf("uuid.Parse(id) error = %v", err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("uuid version = %d, want 7", parsed.Version())
	}

	reloaded, err := core.LoadMultiAppConfig()
	if err != nil {
		t.Fatalf("LoadMultiAppConfig() error = %v", err)
	}
	if reloaded.UserUniqueID != id {
		t.Fatalf("config userUniqueId = %q, want %q", reloaded.UserUniqueID, id)
	}
	if len(reloaded.Apps) != 1 || reloaded.Apps[0].AppId != "cli_test" {
		t.Fatalf("apps changed during upgrade: %+v", reloaded.Apps)
	}
}

func TestLoadOrCreateAuthLogUserUniqueID_FailsWhenLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	multi := &core.MultiAppConfig{
		Apps: []core.AppConfig{{
			AppId:     "cli_test",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	if err := os.MkdirAll(filepath.Join(core.GetConfigDir(), "locks"), 0700); err != nil {
		t.Fatalf("MkdirAll(locks) error = %v", err)
	}

	lockPath := filepath.Join(core.GetConfigDir(), "locks", "config.json.lock")
	lock := lockfile.New(lockPath)
	if err := lock.TryLock(); err != nil {
		t.Fatalf("TryLock() error = %v", err)
	}
	defer lock.Unlock()

	_, err := loadOrCreateAuthLogUserUniqueID()
	if err == nil {
		t.Fatal("loadOrCreateAuthLogUserUniqueID() error = nil, want non-nil")
	}
}

func TestCachedAuthLogRemoteEndpointProvider_FeishuEnabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "cli_test",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.BrandFeishu,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	endpoint, enabled := cachedAuthLogRemoteEndpointProvider("")()
	if !enabled {
		t.Fatal("enabled = false, want true")
	}
	if endpoint != tracking.ResolveTelemetryEndpoint(string(core.BrandFeishu)) {
		t.Fatalf("endpoint = %q, want %q", endpoint, tracking.ResolveTelemetryEndpoint(string(core.BrandFeishu)))
	}
}

func TestCachedAuthLogRemoteEndpointProvider_LarkDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	core.SetCurrentWorkspace(core.WorkspaceLocal)

	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:      "default",
			AppId:     "cli_test",
			AppSecret: core.PlainSecret("secret"),
			Brand:     core.BrandLark,
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	endpoint, enabled := cachedAuthLogRemoteEndpointProvider("")()
	if enabled {
		t.Fatal("enabled = true, want false")
	}
	if endpoint != "" {
		t.Fatalf("endpoint = %q, want empty", endpoint)
	}
}

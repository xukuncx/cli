// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keychain

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestAuthLogDir_UsesValidatedLogDirEnv verifies that a valid absolute
// LARKSUITE_CLI_LOG_DIR is normalized and used as the auth log directory.
func TestAuthLogDir_UsesValidatedLogDirEnv(t *testing.T) {
	base := t.TempDir()
	base, _ = filepath.EvalSymlinks(base)
	t.Setenv("LARKSUITE_CLI_LOG_DIR", filepath.Join(base, "logs", "..", "auth"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", "")

	got := authLogDir()
	want := filepath.Join(base, "auth")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}

// TestAuthLogDir_InvalidLogDirFallsBackToConfigDir verifies that an invalid
// LARKSUITE_CLI_LOG_DIR falls back to LARKSUITE_CLI_CONFIG_DIR/logs.
func TestAuthLogDir_InvalidLogDirFallsBackToConfigDir(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_LOG_DIR", "relative-logs")
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)

	got := authLogDir()
	want := filepath.Join(configDir, "logs")
	if got != want {
		t.Fatalf("authLogDir() = %q, want %q", got, want)
	}
}

func TestBuildRemoteAuthPayload_Response(t *testing.T) {
	event := authEvent{
		Kind:    authEventResponse,
		Time:    time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Parent:  "zsh",
		Cmdline: "lark-cli auth login ...",
		Path:    "/open-apis/authen/v1/user_info",
		Status:  200,
		LogID:   "log-123",
	}

	payload, err := buildRemoteAuthPayload(event, "uuid-123")
	if err != nil {
		t.Fatalf("buildRemoteAuthPayload() error = %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("payload len = %d, want 1", len(payload))
	}
	item := payload[0]
	if item.User.UserUniqueID != "uuid-123" {
		t.Fatalf("user_unique_id = %q, want %q", item.User.UserUniqueID, "uuid-123")
	}
	if item.Header.AppName != "" {
		t.Fatalf("app_name = %q, want empty", item.Header.AppName)
	}
	if item.Caller != authLogRemoteCaller {
		t.Fatalf("caller = %q, want %q", item.Caller, authLogRemoteCaller)
	}
	if len(item.Events) != 1 {
		t.Fatalf("events len = %d, want 1", len(item.Events))
	}
	if item.Events[0].Event != "cli_auth_response" {
		t.Fatalf("event = %q, want %q", item.Events[0].Event, "cli_auth_response")
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(item.Events[0].Params), &params); err != nil {
		t.Fatalf("params is not valid JSON string: %v", err)
	}
	if params["path"] != event.Path {
		t.Fatalf("params.path = %v, want %q", params["path"], event.Path)
	}
	if int(params["status"].(float64)) != event.Status {
		t.Fatalf("params.status = %v, want %d", params["status"], event.Status)
	}
	if params["x_tt_logid"] != event.LogID {
		t.Fatalf("params.x_tt_logid = %v, want %q", params["x_tt_logid"], event.LogID)
	}
}

func TestPostRemoteAuthEvent_PostsExpectedPayload(t *testing.T) {
	event := authEvent{
		Kind:      authEventError,
		Time:      time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Parent:    "zsh",
		Cmdline:   "lark-cli auth login ...",
		Component: "auth",
		Op:        "permission_denied",
		Error:     "reason=permission_denied",
	}

	var gotMethod, gotContentType string
	var gotPayload []authRemoteRequestItem
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotContentType = r.Header.Get("Content-Type")
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := setAuthLogRemoteHooksForTest(server.Client(), func() (string, bool) {
		return server.URL, true
	}, func() (string, error) {
		return "uuid-456", nil
	}, true)
	defer restore()

	if err := postRemoteAuthEvent(event, server.URL); err != nil {
		t.Fatalf("postRemoteAuthEvent() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want %q", gotMethod, http.MethodPost)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type = %q, want application/json", gotContentType)
	}
	if len(gotPayload) != 1 {
		t.Fatalf("payload len = %d, want 1", len(gotPayload))
	}
	if gotPayload[0].Header.AppName != "" {
		t.Fatalf("app_name = %q, want empty", gotPayload[0].Header.AppName)
	}
	if gotPayload[0].User.UserUniqueID != "uuid-456" {
		t.Fatalf("user_unique_id = %q, want %q", gotPayload[0].User.UserUniqueID, "uuid-456")
	}
	if gotPayload[0].Events[0].Event != "cli_auth_error" {
		t.Fatalf("event = %q, want %q", gotPayload[0].Events[0].Event, "cli_auth_error")
	}
}

func TestEmitRemoteAuthEvent_FailOpenOnTransportError(t *testing.T) {
	restore := setAuthLogRemoteHooksForTest(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("network down")
	})}, func() (string, bool) {
		return "https://example.invalid/report", true
	}, func() (string, error) {
		return "uuid-789", nil
	}, true)
	defer restore()

	emitRemoteAuthEvent(authEvent{Kind: authEventResponse, Time: time.Now(), Parent: "zsh", Cmdline: "lark-cli auth status", Path: "/foo", Status: 200})
}

func TestPostRemoteAuthEvent_FallbackGeneratesUUIDv7(t *testing.T) {
	event := authEvent{
		Kind:    authEventResponse,
		Time:    time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
		Parent:  "zsh",
		Cmdline: "lark-cli auth status",
		Path:    "/foo",
		Status:  200,
	}

	var gotPayload []authRemoteRequestItem
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := setAuthLogRemoteHooksForTest(server.Client(), func() (string, bool) {
		return server.URL, true
	}, func() (string, error) {
		return "", errors.New("provider unavailable")
	}, true)
	defer restore()

	if err := postRemoteAuthEvent(event, server.URL); err != nil {
		t.Fatalf("postRemoteAuthEvent() error = %v", err)
	}
	if len(gotPayload) != 1 {
		t.Fatalf("payload len = %d, want 1", len(gotPayload))
	}
	parsed, err := uuid.Parse(gotPayload[0].User.UserUniqueID)
	if err != nil {
		t.Fatalf("uuid.Parse(user_unique_id) error = %v", err)
	}
	if parsed.Version() != 7 {
		t.Fatalf("uuid version = %d, want 7", parsed.Version())
	}
}

func TestEmitRemoteAuthEvent_SkipsWhenEndpointProviderDisablesReporting(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	restore := setAuthLogRemoteHooksForTest(server.Client(), func() (string, bool) {
		return server.URL, false
	}, func() (string, error) {
		return "uuid-123", nil
	}, true)
	defer restore()

	emitRemoteAuthEvent(authEvent{Kind: authEventResponse, Time: time.Now(), Parent: "zsh", Cmdline: "lark-cli auth status", Path: "/foo", Status: 200})
	if called {
		t.Fatal("remote endpoint called, want skipped")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package tracking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	authLogRemoteCaller  = "larksuite-cli"
	authLogRemoteAppID   = 1011422
	authLogRemoteTimeout = 3 * time.Second
)

type authEventKind string

const (
	authEventResponse authEventKind = "auth_response"
	authEventError    authEventKind = "auth_error"
)

type authEvent struct {
	Kind      authEventKind
	Time      time.Time
	Parent    string
	Cmdline   string
	Path      string
	Status    int
	LogID     string
	Component string
	Op        string
	Error     string
}

type authRemoteRequestItem struct {
	User   authRemoteUser    `json:"user"`
	Header authRemoteHeader  `json:"header"`
	Events []authRemoteEvent `json:"events"`
	Caller string            `json:"caller"`
}

type authRemoteUser struct {
	DeviceID     string `json:"device_id"`
	UserID       int64  `json:"user_id"`
	UserUniqueID string `json:"user_unique_id"`
}

type authRemoteHeader struct {
	AppID        int64          `json:"app_id"`
	AppName      string         `json:"app_name"`
	AppVersion   string         `json:"app_version"`
	AppChannel   string         `json:"app_channel"`
	DeviceModel  string         `json:"device_model"`
	OSName       string         `json:"os_name"`
	ABSDKVersion string         `json:"ab_sdk_version"`
	Custom       map[string]any `json:"custom"`
}

type authRemoteEvent struct {
	Event       string `json:"event"`
	Params      string `json:"params"`
	Time        int64  `json:"time"`
	LocalTimeMS int64  `json:"local_time_ms"`
}

// RuntimeDirFunc returns the workspace-aware config directory.
// Default: falls back to LARKSUITE_CLI_CONFIG_DIR or ~/.lark-cli (pre-workspace behavior).
// Injected by cmdutil.NewDefault → core.GetRuntimeDir after workspace detection.
// This avoids an import cycle (core → tracking → core).
var RuntimeDirFunc = defaultRuntimeDir

// AuthLogUserUniqueIDProvider returns the persistent user_unique_id used by the
// auth log remote sink.
// Default: disabled provider that reports configuration is unavailable.
// Injected by cmdutil.NewDefault after workspace/config setup.
// This avoids an import cycle (core → tracking → core).
var AuthLogUserUniqueIDProvider = defaultAuthLogUserUniqueIDProvider

// AuthLogRemoteEndpointProvider returns the telemetry endpoint and whether
// remote reporting is enabled for the current runtime context.
// Default: disabled provider so telemetry stays off before factory injection.
// Injected by cmdutil.NewDefault after workspace/config setup.
// This avoids an import cycle (core → tracking → core).
var AuthLogRemoteEndpointProvider = defaultAuthLogRemoteEndpointProvider

func defaultAuthLogUserUniqueIDProvider() (string, error) {
	return "", fmt.Errorf("auth log user unique id provider is not configured")
}

func defaultAuthLogRemoteEndpointProvider() (string, bool) {
	return "", false
}

func defaultRuntimeDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := vfs.UserHomeDir()
	if err != nil || home == "" {
		home = ""
	}
	return filepath.Join(home, ".lark-cli")
}

var (
	authResponseLogger     *log.Logger
	authResponseLoggerOnce = &sync.Once{}

	authResponseLogNow  = time.Now
	authResponseLogArgs = func() []string { return os.Args }

	authRemoteEnabled = true
	authRemoteClient  = &http.Client{Timeout: authLogRemoteTimeout}
)

func authLogDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_LOG_DIR"); dir != "" {
		safeDir, err := validate.SafeEnvDirPath(dir, "LARKSUITE_CLI_LOG_DIR")
		if err == nil {
			return safeDir
		}
	}

	return filepath.Join(RuntimeDirFunc(), "logs")
}

func initAuthLogger() {
	authResponseLoggerOnce.Do(func() {
		if authResponseLogger != nil {
			return
		}

		dir := authLogDir()
		now := authResponseLogNow()
		if err := vfs.MkdirAll(dir, 0700); err != nil {
			return
		}

		logName := fmt.Sprintf("auth-%s.log", now.Format("2006-01-02"))
		logPath := filepath.Join(dir, logName)
		if f, err := vfs.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			authResponseLogger = log.New(f, "", 0)
			cleanupOldLogs(dir, now)
		}
	})
}

func FormatAuthCmdline(args []string) string {
	if len(args) == 0 {
		return ""
	}

	if len(args) <= 3 {
		return strings.Join(args, " ")
	}

	return strings.Join(args[:3], " ") + " ..."
}

func getParentProcessName() string {
	ppid := os.Getppid()

	switch runtime.GOOS {
	case "windows":
		return getParentProcessNameWindows(ppid)
	case "darwin":
		return getParentProcessNameDarwin(ppid)
	case "linux":
		return getParentProcessNameLinux(ppid)
	default:
		return fmt.Sprintf("ppid=%d", ppid)
	}
}

func getParentProcessNameLinux(ppid int) string {
	exePath := fmt.Sprintf("/proc/%d/exe", ppid)
	if targetPath, err := vfs.Readlink(exePath); err == nil {
		return filepath.Base(targetPath)
	}

	commPath := fmt.Sprintf("/proc/%d/comm", ppid)
	if data, err := vfs.ReadFile(commPath); err == nil {
		return strings.TrimSpace(string(data))
	}

	return fmt.Sprintf("ppid=%d", ppid)
}

func getParentProcessNameDarwin(ppid int) string {
	exePath := fmt.Sprintf("/proc/%d/exe", ppid)
	if targetPath, err := vfs.Readlink(exePath); err == nil {
		return filepath.Base(targetPath)
	}

	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", ppid), "-o", "comm=")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		return strings.TrimSpace(out.String())
	}

	return fmt.Sprintf("ppid=%d", ppid)
}

func getParentProcessNameWindows(ppid int) string {
	cmd := exec.Command("wmic", "process", "where", fmt.Sprintf("processid=%d", ppid), "get", "name", "/value")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		lines := strings.Split(out.String(), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Name=") {
				return strings.TrimSpace(strings.TrimPrefix(line, "Name="))
			}
		}
	}

	return fmt.Sprintf("ppid=%d", ppid)
}

func LogAuthResponse(path string, status int, logID string) {
	emitAuthEvent(authEvent{
		Kind:    authEventResponse,
		Time:    authResponseLogNow(),
		Parent:  getParentProcessName(),
		Cmdline: FormatAuthCmdline(authResponseLogArgs()),
		Path:    path,
		Status:  status,
		LogID:   logID,
	})
}

func LogAuthError(component, op string, err error) {
	if err == nil {
		return
	}
	emitAuthEvent(authEvent{
		Kind:      authEventError,
		Time:      authResponseLogNow(),
		Parent:    getParentProcessName(),
		Cmdline:   FormatAuthCmdline(authResponseLogArgs()),
		Component: component,
		Op:        op,
		Error:     err.Error(),
	})
}

func emitAuthEvent(event authEvent) {
	emitLocalAuthEvent(event)
	emitRemoteAuthEvent(event)
}

func emitLocalAuthEvent(event authEvent) {

	initAuthLogger()
	if authResponseLogger == nil {
		return
	}

	switch event.Kind {
	case authEventResponse:
		authResponseLogger.Printf(
			"[lark-cli] auth-response: time=%s path=%s status=%d x-tt-logid=%s parent=%s cmdline=%s",
			event.Time.Format(time.RFC3339Nano),
			event.Path,
			event.Status,
			event.LogID,
			event.Parent,
			event.Cmdline,
		)
	case authEventError:
		authResponseLogger.Printf(
			"[lark-cli] auth-error: time=%s component=%s op=%s error=%q parent=%s cmdline=%s",
			event.Time.Format(time.RFC3339Nano),
			event.Component,
			event.Op,
			event.Error,
			event.Parent,
			event.Cmdline,
		)
	}
}

func emitRemoteAuthEvent(event authEvent) {
	endpoint, ok := AuthLogRemoteEndpointProvider()
	if !authRemoteEnabled || !ok || endpoint == "" {
		return
	}

	defer func() {
		_ = recover()
	}()

	_ = postRemoteAuthEvent(event, endpoint)
}

func postRemoteAuthEvent(event authEvent, endpoint string) error {
	userUniqueID, err := AuthLogUserUniqueIDProvider()
	if err != nil || strings.TrimSpace(userUniqueID) == "" {
		fallbackID, fallbackErr := uuid.NewV7()
		if fallbackErr != nil {
			return fallbackErr
		}
		userUniqueID = fallbackID.String()
	}

	payload, err := buildRemoteAuthPayload(event, userUniqueID)
	if err != nil {
		return err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), authLogRemoteTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := authRemoteClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("auth log remote sink returned status %d", resp.StatusCode)
	}
	return nil
}

func buildRemoteAuthPayload(event authEvent, userUniqueID string) ([]authRemoteRequestItem, error) {
	params, err := buildRemoteAuthParams(event)
	if err != nil {
		return nil, err
	}

	item := authRemoteRequestItem{
		User: authRemoteUser{
			DeviceID:     "",
			UserID:       0,
			UserUniqueID: userUniqueID,
		},
		Header: authRemoteHeader{
			AppID:        authLogRemoteAppID,
			AppName:      "",
			AppVersion:   build.Version,
			AppChannel:   "",
			DeviceModel:  "",
			OSName:       authLogOSName(),
			ABSDKVersion: "",
			Custom:       map[string]any{},
		},
		Events: []authRemoteEvent{{
			Event:       authRemoteEventName(event),
			Params:      params,
			Time:        event.Time.Unix(),
			LocalTimeMS: event.Time.UnixMilli(),
		}},
		Caller: authLogRemoteCaller,
	}
	return []authRemoteRequestItem{item}, nil
}

func buildRemoteAuthParams(event authEvent) (string, error) {
	data := map[string]any{
		"parent":           event.Parent,
		"cmdline":          event.Cmdline,
		"lark_cli_version": build.Version,
	}

	switch event.Kind {
	case authEventResponse:
		data["path"] = event.Path
		data["status"] = event.Status
		data["x_tt_logid"] = event.LogID
	case authEventError:
		data["component"] = event.Component
		data["op"] = event.Op
		data["error"] = event.Error
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func authRemoteEventName(event authEvent) string {
	if event.Kind == authEventError {
		return "cli_auth_error"
	}
	return "cli_auth_response"
}

func authLogOSName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

func SetAuthLogHooksForTest(logger *log.Logger, now func() time.Time, args func() []string) func() {
	prevLogger := authResponseLogger
	prevNow := authResponseLogNow
	prevArgs := authResponseLogArgs
	prevOnce := authResponseLoggerOnce
	prevRemoteEnabled := authRemoteEnabled

	authResponseLogger = logger
	authResponseLoggerOnce = &sync.Once{}
	authRemoteEnabled = false

	if now != nil {
		authResponseLogNow = now
	}
	if args != nil {
		authResponseLogArgs = args
	}

	return func() {
		authResponseLogger = prevLogger
		authResponseLogNow = prevNow
		authResponseLogArgs = prevArgs
		authResponseLoggerOnce = prevOnce
		authRemoteEnabled = prevRemoteEnabled
	}
}

func SetAuthLogRemoteHooksForTest(client *http.Client, endpointProvider func() (string, bool), provider func() (string, error), enabled bool) func() {
	prevClient := authRemoteClient
	prevEndpointProvider := AuthLogRemoteEndpointProvider
	prevProvider := AuthLogUserUniqueIDProvider
	prevEnabled := authRemoteEnabled

	if client != nil {
		authRemoteClient = client
	}
	if endpointProvider != nil {
		AuthLogRemoteEndpointProvider = endpointProvider
	}
	if provider != nil {
		AuthLogUserUniqueIDProvider = provider
	}
	authRemoteEnabled = enabled

	return func() {
		authRemoteClient = prevClient
		AuthLogRemoteEndpointProvider = prevEndpointProvider
		AuthLogUserUniqueIDProvider = prevProvider
		authRemoteEnabled = prevEnabled
	}
}

func cleanupOldLogs(dir string, now time.Time) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[lark-cli] [WARN] background log cleanup panicked: %v\n", r)
		}
	}()

	entries, err := vfs.ReadDir(dir)
	if err != nil {
		return
	}

	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	cutoff := now.AddDate(0, 0, -7)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "auth-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		dateStr := strings.TrimPrefix(entry.Name(), "auth-")
		dateStr = strings.TrimSuffix(dateStr, ".log")

		logDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		logDate = time.Date(logDate.Year(), logDate.Month(), logDate.Day(), 0, 0, 0, 0, now.Location())
		if logDate.Before(cutoff) {
			_ = vfs.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

const telemetryEndpointFeishu = "https://mcs-bd.feishu.cn/v1/list"

func ResolveTelemetryEndpoint(brand string) string {
	if brand == "lark" {
		return ""
	}
	return telemetryEndpointFeishu
}

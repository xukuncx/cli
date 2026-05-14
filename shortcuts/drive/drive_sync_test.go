// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

// TestDriveSyncRemoteWinsPullsNewRemoteAndPushesNewLocal verifies the basic
// two-way sync flow: new_remote files are pulled, new_local files are pushed,
// and modified files use --on-conflict=remote-wins (the default) to pull the
// remote version.
func TestDriveSyncRemoteWinsPullsNewRemoteAndPushesNewLocal(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-remote-wins", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	// Local layout:
	//   local/b.txt  — only local → push
	//   local/a.txt  — both sides, different content → conflict (remote-wins → pull)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local-a"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}
	if err := os.WriteFile("local/b.txt", []byte("local-b"), 0o644); err != nil {
		t.Fatalf("WriteFile b.txt: %v", err)
	}

	// Remote listing: a.txt (modified), d.txt (new_remote)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
					map[string]interface{}{"token": "tok_d", "name": "d.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	// Download a.txt for hash comparison (exact mode)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("remote-a"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	// Download d.txt (new_remote → pull)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_d/download",
		Status:  200,
		Body:    []byte("remote-d"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	// Download a.txt again (conflict: remote-wins → pull remote over local)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("remote-a"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	// Upload b.txt (new_local → push)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "tok_b_uploaded",
			},
		},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-conflict", "remote-wins",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"action": "downloaded"`) {
		t.Errorf("output missing downloaded action\noutput: %s", out)
	}
	if !strings.Contains(out, `"action": "uploaded"`) {
		t.Errorf("output missing uploaded action\noutput: %s", out)
	}
	if !strings.Contains(out, `"direction": "pull"`) {
		t.Errorf("output missing pull direction\noutput: %s", out)
	}
	if !strings.Contains(out, `"direction": "push"`) {
		t.Errorf("output missing push direction\noutput: %s", out)
	}

	// Verify local file was overwritten with remote content
	data, err := os.ReadFile("local/a.txt")
	if err != nil {
		t.Fatalf("ReadFile a.txt: %v", err)
	}
	if string(data) != "remote-a" {
		t.Errorf("a.txt content = %q, want %q", string(data), "remote-a")
	}

	// Verify d.txt was downloaded
	data, err = os.ReadFile("local/d.txt")
	if err != nil {
		t.Fatalf("ReadFile d.txt: %v", err)
	}
	if string(data) != "remote-d" {
		t.Errorf("d.txt content = %q, want %q", string(data), "remote-d")
	}
}

// TestDriveSyncLocalWinsPushesOverRemote verifies that --on-conflict=local-wins
// pushes the local version over the remote file.
func TestDriveSyncLocalWinsPushesOverRemote(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-local-wins", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local-a"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	// Download a.txt for hash comparison (exact mode)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("remote-a"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	// Upload a.txt with overwrite (local-wins → push over remote)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "tok_a",
				"version":    "v2",
			},
		},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-conflict", "local-wins",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"action": "overwritten"`) {
		t.Errorf("output missing overwritten action\noutput: %s", out)
	}
	if !strings.Contains(out, `"direction": "push"`) {
		t.Errorf("output missing push direction\noutput: %s", out)
	}
}

// TestDriveSyncKeepBothRenamesLocalAndPullsRemote verifies that
// --on-conflict=keep-both renames the local file with a hash suffix
// and then downloads the remote version to the original path.
func TestDriveSyncKeepBothRenamesLocalAndPullsRemote(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-keep-both", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local-a"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	// Download a.txt for hash comparison
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("remote-a"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	// Download a.txt again (keep-both: pull remote to original path after rename)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("remote-a"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-conflict", "keep-both",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"action": "renamed_local"`) {
		t.Errorf("output missing renamed_local action\noutput: %s", out)
	}
	if !strings.Contains(out, `"action": "downloaded"`) {
		t.Errorf("output missing downloaded action\noutput: %s", out)
	}

	// Original path should now have remote content
	data, err := os.ReadFile("local/a.txt")
	if err != nil {
		t.Fatalf("ReadFile a.txt: %v", err)
	}
	if string(data) != "remote-a" {
		t.Errorf("a.txt content = %q, want %q", string(data), "remote-a")
	}

	// There should be a renamed file with __lark_ suffix
	entries, err := os.ReadDir("local")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "__lark_") && strings.HasSuffix(e.Name(), ".txt") {
			found = true
			renamedData, err := os.ReadFile("local/" + e.Name())
			if err != nil {
				t.Fatalf("ReadFile renamed: %v", err)
			}
			if string(renamedData) != "local-a" {
				t.Errorf("renamed file content = %q, want %q", string(renamedData), "local-a")
			}
		}
	}
	if !found {
		t.Errorf("expected a file with __lark_ suffix in local/, got entries: %v", entries)
	}
}

// TestDriveSyncKeepBothRollsBackRenameOnPullFailure verifies that keep-both
// restores the original local path if the remote download fails after the
// local file has been renamed.
func TestDriveSyncKeepBothRollsBackRenameOnPullFailure(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-keep-both-rollback", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local-a"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	// Download a.txt for the exact diff phase.
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("remote-a"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-conflict", "keep-both",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatalf("expected +sync keep-both to fail when the post-rename pull has no stub\nstdout: %s", stdout.String())
	}

	data, readErr := os.ReadFile("local/a.txt")
	if readErr != nil {
		t.Fatalf("ReadFile a.txt after rollback: %v", readErr)
	}
	if string(data) != "local-a" {
		t.Fatalf("a.txt content after rollback = %q, want %q", string(data), "local-a")
	}

	entries, readDirErr := os.ReadDir("local")
	if readDirErr != nil {
		t.Fatalf("ReadDir local: %v", readDirErr)
	}
	if len(entries) != 1 || entries[0].Name() != "a.txt" {
		t.Fatalf("expected rollback to restore only local/a.txt, got entries: %v", entries)
	}
}

// TestDriveSyncAskConflictFailsOnEOF verifies that --on-conflict=ask does not
// silently fall back to remote-wins when stdin has no answer.
func TestDriveSyncAskConflictFailsOnEOF(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-ask-eof", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local-a"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	// Download a.txt for the exact diff phase.
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("remote-a"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-conflict", "ask",
		"--as", "bot",
	}, f, stdout)
	if err == nil {
		t.Fatalf("expected +sync --on-conflict=ask to fail on EOF\nstdout: %s", stdout.String())
	}
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		t.Fatalf("expected structured ExitError, got: %v", err)
	}
	detailMap, _ := exitErr.Detail.Detail.(map[string]interface{})
	items, _ := detailMap["items"].([]driveSyncItem)
	if len(items) == 0 || !strings.Contains(items[0].Error, "stdin") {
		t.Fatalf("expected item-level stdin-related error, got detail: %#v", exitErr.Detail.Detail)
	}

	data, readErr := os.ReadFile("local/a.txt")
	if readErr != nil {
		t.Fatalf("ReadFile a.txt after ask failure: %v", readErr)
	}
	if string(data) != "local-a" {
		t.Fatalf("a.txt content after ask failure = %q, want %q", string(data), "local-a")
	}
}

// TestDriveSyncUsesResolvedDuplicateTargetForDiff verifies that +sync computes
// the diff against the same duplicate-remote selection used during execution.
func TestDriveSyncUsesResolvedDuplicateTargetForDiff(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-duplicate-resolution", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("same-as-oldest"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_old", "name": "a.txt", "type": "file", "created_time": "100", "modified_time": "100"},
					map[string]interface{}{"token": "tok_new", "name": "a.txt", "type": "file", "created_time": "200", "modified_time": "200"},
				},
				"has_more": false,
			},
		},
	})

	// The chosen --on-duplicate-remote=oldest target is tok_old. The test omits
	// any tok_new download stub so a stale last-seen overwrite bug would fail.
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_old/download",
		Status:  200,
		Body:    []byte("same-as-oldest"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-duplicate-remote", "oldest",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"pushed": 0`) || !strings.Contains(out, `"pulled": 0`) {
		t.Fatalf("expected unchanged duplicate target to produce no sync actions\noutput: %s", out)
	}
	if !strings.Contains(out, `"file_token": "tok_old"`) {
		t.Fatalf("expected diff to reference the oldest duplicate target token\noutput: %s", out)
	}
}

// TestDriveSyncLocalWinsNestedFileUsesParentFolderToken verifies that local-wins
// overwrites on nested files keep parent_node aligned with the file's parent.
func TestDriveSyncLocalWinsNestedFileUsesParentFolderToken(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-local-wins-nested", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local/sub", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/sub/a.txt", []byte("local-a"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "fld_sub", "name": "sub", "type": "folder"},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=fld_sub",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	// Diff phase exact hash download.
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("remote-a"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "tok_a",
				"version":    "v2",
			},
		},
	}
	reg.Register(uploadStub)

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--on-conflict", "local-wins",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	body := decodeDriveMultipartBody(t, uploadStub)
	if got := body.Fields["file_token"]; got != "tok_a" {
		t.Fatalf("upload_all file_token = %q, want tok_a", got)
	}
	if got := body.Fields["parent_node"]; got != "fld_sub" {
		t.Fatalf("upload_all parent_node = %q, want fld_sub", got)
	}
}

// TestDriveSyncNewLocalDisappearanceIsReported verifies that files discovered
// during diff but removed before the push phase are surfaced as skipped items
// instead of being silently dropped.
func TestDriveSyncNewLocalDisappearanceIsReported(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-new-local-disappeared", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/ephemeral.txt", []byte("temp"), 0o644); err != nil {
		t.Fatalf("WriteFile ephemeral.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		OnMatch: func(_ *http.Request) {
			if err := os.Remove("local/ephemeral.txt"); err != nil && !os.IsNotExist(err) {
				t.Fatalf("Remove ephemeral.txt in OnMatch: %v", err)
			}
		},
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files":    []interface{}{},
				"has_more": false,
			},
		},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"skipped": 1`) {
		t.Fatalf("expected skipped=1 when new_local disappears during execution\noutput: %s", out)
	}
	if !strings.Contains(out, `"rel_path": "ephemeral.txt"`) || !strings.Contains(out, `"local file disappeared during sync"`) {
		t.Fatalf("expected vanished new_local file to be reported in items\noutput: %s", out)
	}
}

// TestDriveSyncQuickModeUsesModifiedTime verifies that --quick mode
// classifies files by modified_time instead of SHA-256 hash.
func TestDriveSyncQuickModeUsesModifiedTime(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-quick", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("local-a"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}
	if err := os.WriteFile("local/b.txt", []byte("local-b"), 0o644); err != nil {
		t.Fatalf("WriteFile b.txt: %v", err)
	}

	// Set a.txt mtime to match remote → unchanged in quick mode
	matchTime := time.Unix(1715594880, 0)
	if err := os.Chtimes("local/a.txt", matchTime, matchTime); err != nil {
		t.Fatalf("Chtimes a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file", "modified_time": "1715594880"},
					map[string]interface{}{"token": "tok_d", "name": "d.txt", "type": "file", "modified_time": "1715595000"},
				},
				"has_more": false,
			},
		},
	})

	// Download d.txt (new_remote → pull)
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_d/download",
		Status:  200,
		Body:    []byte("remote-d"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	// Upload b.txt (new_local → push)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/files/upload_all",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"file_token": "tok_b_uploaded",
			},
		},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--quick",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"detection": "quick"`) {
		t.Errorf("output missing detection=quick\noutput: %s", out)
	}
	// a.txt should be unchanged (mtime matches), not downloaded or uploaded
	// It should appear in diff.unchanged but NOT in items[] with a pull/push action
	for _, item := range []string{`"downloaded"`, `"uploaded"`, `"overwritten"`} {
		if strings.Count(out, item) > 0 {
			// Check that a.txt is not the one being downloaded/uploaded
			// by verifying it only appears in the diff section, not items
			break
		}
	}
	// More precise: a.txt should not appear in items[] at all since it's unchanged
	itemsSection := out[strings.Index(out, `"items"`):]
	if strings.Contains(itemsSection, `"rel_path": "a.txt"`) {
		t.Errorf("a.txt should not appear in items[] (mtime matches remote, should be unchanged)\noutput: %s", out)
	}
}

// TestDriveSyncQuickModeMTimeMismatchStillTriggersWrites verifies the best-effort
// nature of --quick: a timestamp mismatch alone is enough to drive a real sync
// action even when the file bytes are already identical.
func TestDriveSyncQuickModeMTimeMismatchStillTriggersWrites(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-quick-mismatch", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)
	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("same-content"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}
	localTime := time.Unix(1715594880, 0)
	if err := os.Chtimes("local/a.txt", localTime, localTime); err != nil {
		t.Fatalf("Chtimes a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file", "modified_time": "1715594999"},
				},
				"has_more": false,
			},
		},
	})

	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("same-content"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--quick",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"detection": "quick"`) {
		t.Fatalf("expected detection=quick\noutput: %s", out)
	}
	if !strings.Contains(out, `"modified":`) || !strings.Contains(out, `"action": "downloaded"`) {
		t.Fatalf("expected quick mtime mismatch to trigger a real pull action\noutput: %s", out)
	}
}

// TestDriveSyncNoChangesReportsEmptyItems verifies that when local and remote
// are identical, +sync reports zero pulled/pushed items.
func TestDriveSyncNoChangesReportsEmptyItems(t *testing.T) {
	syncTestConfig := &core.CliConfig{
		AppID: "drive-sync-no-changes", AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, stdout, _, reg := cmdutil.TestFactory(t, syncTestConfig)

	tmpDir := t.TempDir()
	withDriveWorkingDir(t, tmpDir)

	if err := os.MkdirAll("local", 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile("local/a.txt", []byte("same"), 0o644); err != nil {
		t.Fatalf("WriteFile a.txt: %v", err)
	}

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "folder_token=folder_root",
		Body: map[string]interface{}{
			"code": 0, "msg": "ok",
			"data": map[string]interface{}{
				"files": []interface{}{
					map[string]interface{}{"token": "tok_a", "name": "a.txt", "type": "file"},
				},
				"has_more": false,
			},
		},
	})

	// Download a.txt for hash comparison → same content → unchanged
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/drive/v1/files/tok_a/download",
		Status:  200,
		Body:    []byte("same"),
		Headers: http.Header{"Content-Type": []string{"application/octet-stream"}},
	})

	err := mountAndRunDrive(t, DriveSync, []string{
		"+sync",
		"--local-dir", "local",
		"--folder-token", "folder_root",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout: %s", err, stdout.String())
	}

	out := stdout.String()
	if !strings.Contains(out, `"pulled": 0`) {
		t.Errorf("expected pulled=0\noutput: %s", out)
	}
	if !strings.Contains(out, `"pushed": 0`) {
		t.Errorf("expected pushed=0\noutput: %s", out)
	}
	if !strings.Contains(out, `"failed": 0`) {
		t.Errorf("expected failed=0\noutput: %s", out)
	}
}

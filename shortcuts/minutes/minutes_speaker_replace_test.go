// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

func TestMinutesSpeakerReplace_Validate(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing minute token",
			args:    []string{"+speaker-replace", "--from-user-id", "ou_a", "--to-user-id", "ou_b", "--as", "user"},
			wantErr: "required flag(s) \"minute-token\" not set",
		},
		{
			name:    "missing from",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--to-user-id", "ou_b", "--as", "user"},
			wantErr: "required flag(s) \"from-user-id\" not set",
		},
		{
			name:    "missing to",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--from-user-id", "ou_a", "--as", "user"},
			wantErr: "required flag(s) \"to-user-id\" not set",
		},
		{
			name:    "invalid from prefix",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--from-user-id", "u_a", "--to-user-id", "ou_b", "--as", "user"},
			wantErr: "--from-user-id",
		},
		{
			name:    "invalid to prefix",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--from-user-id", "ou_a", "--to-user-id", "u_b", "--as", "user"},
			wantErr: "--to-user-id",
		},
		{
			name:    "from equals to",
			args:    []string{"+speaker-replace", "--minute-token", "obcn123456", "--from-user-id", "ou_same", "--to-user-id", "ou_same", "--as", "user"},
			wantErr: "must be different",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "minutes"}
			MinutesSpeakerReplace.Mount(parent, f)
			parent.SetArgs(tt.args)
			parent.SilenceErrors = true
			parent.SilenceUsage = true
			err := parent.Execute()
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error should contain %q, got: %s", tt.wantErr, err.Error())
			}
		})
	}
}

func TestMinutesSpeakerReplace_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesSpeakerReplace, []string{
		"+speaker-replace",
		"--minute-token", "obcnq3b9jl72l83w4f149w9c",
		"--from-user-id", "ou_old_speaker",
		"--to-user-id", "ou_new_speaker",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "PUT") {
		t.Errorf("expected PUT method, got:\n%s", out)
	}
	if !strings.Contains(out, "/open-apis/minutes/v1/minutes/obcnq3b9jl72l83w4f149w9c/transcript/speaker") {
		t.Errorf("expected speaker endpoint, got:\n%s", out)
	}
	if !strings.Contains(out, "ou_old_speaker") {
		t.Errorf("expected from_user_id in body, got:\n%s", out)
	}
	if !strings.Contains(out, "ou_new_speaker") {
		t.Errorf("expected to_user_id in body, got:\n%s", out)
	}
}

func TestMinutesSpeakerReplace_Execute(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: http.MethodPut,
		URL:    "/open-apis/minutes/v1/minutes/obcnq3b9jl72l83w4f149w9c/transcript/speaker",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, MinutesSpeakerReplace, []string{
		"+speaker-replace",
		"--minute-token", "obcnq3b9jl72l83w4f149w9c",
		"--from-user-id", "ou_old_speaker",
		"--to-user-id", "ou_new_speaker",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

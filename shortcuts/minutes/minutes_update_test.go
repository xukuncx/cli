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

func TestMinutesUpdate_Validate(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name:    "missing minute token",
			args:    []string{"+update", "--topic", "new title", "--as", "user"},
			wantErr: "required flag(s) \"minute-token\" not set",
		},
		{
			name:    "missing topic",
			args:    []string{"+update", "--minute-token", "obcn123456", "--as", "user"},
			wantErr: "required flag(s) \"topic\" not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := &cobra.Command{Use: "minutes"}
			MinutesUpdate.Mount(parent, f)
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

func TestMinutesUpdate_DryRun(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	err := mountAndRun(t, MinutesUpdate, []string{
		"+update",
		"--minute-token", "obcnq3b9jl72l83w4f149w9c",
		"--topic", "周会纪要",
		"--dry-run", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "PATCH") {
		t.Errorf("expected PATCH method, got:\n%s", out)
	}
	if !strings.Contains(out, "/open-apis/minutes/v1/minutes/obcnq3b9jl72l83w4f149w9c") {
		t.Errorf("expected PATCH /open-apis/minutes/v1/minutes/<token>, got:\n%s", out)
	}
	if !strings.Contains(out, "周会纪要") {
		t.Errorf("expected topic in body, got:\n%s", out)
	}
}

func TestMinutesUpdate_Execute(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	warmTokenCache(t)

	reg.Register(&httpmock.Stub{
		Method: http.MethodPatch,
		URL:    "/open-apis/minutes/v1/minutes/obcnq3b9jl72l83w4f149w9c",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	})

	err := mountAndRun(t, MinutesUpdate, []string{
		"+update",
		"--minute-token", "obcnq3b9jl72l83w4f149w9c",
		"--topic", "新标题",
		"--format", "json", "--as", "user",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

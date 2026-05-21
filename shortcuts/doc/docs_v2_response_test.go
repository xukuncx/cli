// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestWarnDocsUpdateV2Response(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       map[string]interface{}
		wantWarn   bool
		wantSubstr string
	}{
		{
			name: "failed result emits warning",
			data: map[string]interface{}{
				"result": "failed",
			},
			wantWarn:   true,
			wantSubstr: "failed entirely",
		},
		{
			name: "partial_success emits warning with count",
			data: map[string]interface{}{
				"result":               "partial_success",
				"updated_blocks_count": float64(3),
			},
			wantWarn:   true,
			wantSubstr: "partial_success",
		},
		{
			name: "success with zero blocks emits warning",
			data: map[string]interface{}{
				"result":               "success",
				"updated_blocks_count": float64(0),
			},
			wantWarn:   true,
			wantSubstr: "updated_blocks_count=0",
		},
		{
			name: "success with blocks is silent",
			data: map[string]interface{}{
				"result":               "success",
				"updated_blocks_count": float64(5),
			},
			wantWarn: false,
		},
		{
			name:       "empty result is silent",
			data:       map[string]interface{}{},
			wantWarn:   false,
			wantSubstr: "",
		},
		{
			name: "unknown result emits warning",
			data: map[string]interface{}{
				"result":               "unknown_value",
				"updated_blocks_count": float64(1),
			},
			wantWarn:   true,
			wantSubstr: "unexpected result",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			runtime := &common.RuntimeContext{
				Factory: &cmdutil.Factory{
					IOStreams: &cmdutil.IOStreams{ErrOut: &stderr},
				},
			}

			warnDocsUpdateV2Response(runtime, tt.data)

			got := stderr.String()
			hasWarn := got != ""
			if hasWarn != tt.wantWarn {
				t.Fatalf("warnDocsUpdateV2Response() stderr=%q, wantWarn=%v", got, tt.wantWarn)
			}
			if tt.wantWarn && !strings.Contains(got, tt.wantSubstr) {
				t.Fatalf("expected warning to contain %q, got: %s", tt.wantSubstr, got)
			}
		})
	}
}

func TestWarnDocsCreateV2Response(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		data       map[string]interface{}
		wantWarn   bool
		wantSubstr string
	}{
		{
			name: "failed result emits warning",
			data: map[string]interface{}{
				"result": "failed",
			},
			wantWarn:   true,
			wantSubstr: "creation failed",
		},
		{
			name: "success is silent",
			data: map[string]interface{}{
				"result": "success",
			},
			wantWarn: false,
		},
		{
			name:       "empty result is silent",
			data:       map[string]interface{}{},
			wantWarn:   false,
			wantSubstr: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			runtime := &common.RuntimeContext{
				Factory: &cmdutil.Factory{
					IOStreams: &cmdutil.IOStreams{ErrOut: &stderr},
				},
			}

			warnDocsCreateV2Response(runtime, tt.data)

			got := stderr.String()
			hasWarn := got != ""
			if hasWarn != tt.wantWarn {
				t.Fatalf("warnDocsCreateV2Response() stderr=%q, wantWarn=%v", got, tt.wantWarn)
			}
			if tt.wantWarn && !strings.Contains(got, tt.wantSubstr) {
				t.Fatalf("expected warning to contain %q, got: %s", tt.wantSubstr, got)
			}
		})
	}
}

func TestValidateUpdateV2MarkdownShorthand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errSub  string
	}{
		{
			name:    "markdown and content are mutually exclusive",
			args:    []string{"+update", "--api-version", "v2", "--doc", "doxcn_test", "--command", "append", "--markdown", "## Hello", "--content", "<p>text</p>"},
			wantErr: true,
			errSub:  "mutually exclusive",
		},
		{
			name:    "markdown with conflicting doc-format",
			args:    []string{"+update", "--api-version", "v2", "--doc", "doxcn_test", "--command", "append", "--markdown", "## Hello", "--doc-format", "xml"},
			wantErr: true,
			errSub:  "implies --doc-format markdown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &core.CliConfig{
				AppID:     "test-markdown-validate",
				AppSecret: "secret",
				Brand:     core.BrandFeishu,
			}
			f, _, _, _ := cmdutil.TestFactory(t, cfg)

			err := mountAndRunDocs(t, DocsUpdate, tt.args, f, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error: %v, wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
				t.Fatalf("expected error to contain %q, got: %v", tt.errSub, err)
			}
		})
	}
}

func TestValidateCreateV2MarkdownShorthand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantErr bool
		errSub  string
	}{
		{
			name:    "markdown and content are mutually exclusive",
			args:    []string{"+create", "--api-version", "v2", "--markdown", "## Hello", "--content", "<p>text</p>"},
			wantErr: true,
			errSub:  "mutually exclusive",
		},
		{
			name:    "markdown with conflicting doc-format",
			args:    []string{"+create", "--api-version", "v2", "--markdown", "## Hello", "--doc-format", "xml"},
			wantErr: true,
			errSub:  "implies --doc-format markdown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := &core.CliConfig{
				AppID:     "test-markdown-validate-create",
				AppSecret: "secret",
				Brand:     core.BrandFeishu,
			}
			f, _, _, _ := cmdutil.TestFactory(t, cfg)

			err := mountAndRunDocs(t, DocsCreate, tt.args, f, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error: %v, wantErr=%v", err, tt.wantErr)
			}
			if tt.wantErr && tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
				t.Fatalf("expected error to contain %q, got: %v", tt.errSub, err)
			}
		})
	}
}

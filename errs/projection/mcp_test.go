// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package projection

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestMCPCodeFor_ValidationAndConfirmation(t *testing.T) {
	if got := MCPCodeFor(errs.CategoryValidation, ""); got != -32602 {
		t.Errorf("validation: got %d, want -32602", got)
	}
	if got := MCPCodeFor(errs.CategoryConfirmation, ""); got != -32602 {
		t.Errorf("confirmation: got %d, want -32602", got)
	}
}

func TestMCPCodeFor_InternalFamily(t *testing.T) {
	cases := []struct {
		cat  errs.Category
		sub  errs.Subtype
		name string
	}{
		{errs.CategoryAuthentication, errs.SubtypeTokenMissing, "authentication"},
		{errs.CategoryAuthorization, errs.SubtypeMissingScope, "authorization"},
		{errs.CategoryConfig, "", "config"},
		{errs.CategoryNetwork, "", "network"},
		{errs.CategoryPolicy, "", "policy"},
		{errs.CategoryInternal, "", "internal"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MCPCodeFor(tc.cat, tc.sub); got != -32603 {
				t.Errorf("got %d, want -32603", got)
			}
		})
	}
}

func TestMCPCodeFor_APIDefault(t *testing.T) {
	// CategoryAPI with no prefix override → -32000 default.
	if got := MCPCodeFor(errs.CategoryAPI, "rate_limit"); got != -32000 {
		t.Errorf("got %d, want -32000", got)
	}
}

func TestMCPCodeFor_APITaskOverride(t *testing.T) {
	if got := MCPCodeFor(errs.CategoryAPI, "task_invalid_params"); got != -32010 {
		t.Errorf("got %d, want -32010", got)
	}
	if got := MCPCodeFor(errs.CategoryAPI, "task_conflict"); got != -32010 {
		t.Errorf("got %d, want -32010", got)
	}
}

func TestMCPCodeFor_APIPrefixDeterministic(t *testing.T) {
	// Run lookup 100 times to surface any nondeterminism that map iteration
	// would have introduced. With the sorted-slice form, every iteration must
	// return the same code for the same input.
	want := MCPCodeFor(errs.CategoryAPI, errs.Subtype("task_invalid_params"))
	for i := 0; i < 100; i++ {
		if got := MCPCodeFor(errs.CategoryAPI, errs.Subtype("task_invalid_params")); got != want {
			t.Fatalf("nondeterministic lookup on iteration %d: got %d want %d", i, got, want)
		}
	}
}

func TestMCPCodeFor_APIPrefixSortInvariant(t *testing.T) {
	// Guard against future maintainers reordering the slice. Each entry's
	// prefix must not be a prefix of any later entry's prefix (longest-first).
	for i := 0; i < len(apiSubtypePrefixCodes); i++ {
		for j := i + 1; j < len(apiSubtypePrefixCodes); j++ {
			if strings.HasPrefix(apiSubtypePrefixCodes[j].prefix, apiSubtypePrefixCodes[i].prefix) {
				t.Errorf("apiSubtypePrefixCodes order violates longest-first: %q (idx %d) is a prefix of %q (idx %d) — swap order",
					apiSubtypePrefixCodes[i].prefix, i, apiSubtypePrefixCodes[j].prefix, j)
			}
		}
	}
}

func TestMCPCodeFor_UnknownCategoryFallsBackToInternal(t *testing.T) {
	if got := MCPCodeFor(errs.Category("nonexistent"), ""); got != -32603 {
		t.Errorf("got %d, want -32603", got)
	}
}

func TestBuildMCPError_Nil(t *testing.T) {
	if got := BuildMCPError(nil); got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestBuildMCPError_UntypedFallback(t *testing.T) {
	got := BuildMCPError(fmt.Errorf("untyped"))
	if got.Code != -32603 {
		t.Errorf("code = %v, want -32603", got.Code)
	}
	if got.Message != "untyped" {
		t.Errorf("message = %v, want %q", got.Message, "untyped")
	}
	if got.Data == nil {
		t.Fatal("data is nil")
	}
	if got.Data.Type != errs.CategoryInternal {
		t.Errorf("data.type = %v, want %q", got.Data.Type, errs.CategoryInternal)
	}
	if got.Data.Subtype != errs.SubtypeWrapped {
		t.Errorf("data.subtype = %v, want %q", got.Data.Subtype, errs.SubtypeWrapped)
	}
	if got.Data.Code != 0 {
		t.Errorf("untyped fallback should not synthesize data.code, got %d", got.Data.Code)
	}
}

func TestBuildMCPError_PermissionError(t *testing.T) {
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			Code:     99991679,
			Message:  "x",
		},
		MissingScopes: []string{"docx:document"},
		ConsoleURL:    "https://example",
	}
	got := BuildMCPError(pe)
	if got.Code != -32603 {
		t.Errorf("outer code = %v, want -32603", got.Code)
	}
	if got.Message != "x" {
		t.Errorf("message = %v, want %q", got.Message, "x")
	}
	if got.Data.Code != 99991679 {
		t.Errorf("data.code = %v, want 99991679", got.Data.Code)
	}
	if got.Data.Type != errs.CategoryAuthorization {
		t.Errorf("data.type = %v, want %q", got.Data.Type, errs.CategoryAuthorization)
	}
	if got.Data.Subtype != errs.SubtypeMissingScope {
		t.Errorf("data.subtype = %v, want %q", got.Data.Subtype, errs.SubtypeMissingScope)
	}
	wantScopes := []string{"docx:document"}
	if !reflect.DeepEqual(got.Data.RequiredScopes, wantScopes) {
		t.Errorf("data.required_scopes = %v, want %v", got.Data.RequiredScopes, wantScopes)
	}
	if got.Data.ConsoleURL != "https://example" {
		t.Errorf("data.console_url = %v, want %q", got.Data.ConsoleURL, "https://example")
	}
}

func TestBuildMCPError_APIErrorTaskOverrideAndRetryable(t *testing.T) {
	ae := &errs.APIError{
		Problem: errs.Problem{
			Category:  errs.CategoryAPI,
			Subtype:   errs.Subtype("task_conflict"),
			Code:      1470422,
			Message:   "x",
			Retryable: true,
		},
	}
	got := BuildMCPError(ae)
	if got.Code != -32010 {
		t.Errorf("outer code = %v, want -32010", got.Code)
	}
	if !got.Data.Retryable {
		t.Errorf("data.retryable = false, want true")
	}
}

func TestBuildMCPError_SecurityPolicyError(t *testing.T) {
	spe := &errs.SecurityPolicyError{
		Problem: errs.Problem{
			Category: errs.CategoryPolicy,
			Subtype:  errs.Subtype("challenge_required"),
		},
		ChallengeURL: "https://challenge",
	}
	got := BuildMCPError(spe)
	if got.Data.ChallengeURL != "https://challenge" {
		t.Errorf("data.challenge_url = %v, want %q", got.Data.ChallengeURL, "https://challenge")
	}
}

func TestBuildMCPError_InternalErrorOmitsRetryableWhenFalse(t *testing.T) {
	ie := &errs.InternalError{
		Problem: errs.Problem{
			Category: errs.CategoryInternal,
			Subtype:  errs.SubtypeWrapped,
			Message:  "boom",
		},
	}
	got := BuildMCPError(ie)
	if got.Code != -32603 {
		t.Errorf("outer code = %v, want -32603", got.Code)
	}
	if got.Data.Retryable {
		t.Errorf("data.retryable should be false when source Retryable=false, got true")
	}
}

func TestBuildMCPError_ContentSafetyError(t *testing.T) {
	cse := &errs.ContentSafetyError{
		Problem: errs.Problem{
			Category: errs.CategoryPolicy,
			Subtype:  errs.Subtype("content_blocked"),
			Message:  "blocked",
		},
		Rules: []string{"pii", "violence"},
	}
	got := BuildMCPError(cse)
	if !reflect.DeepEqual(got.Data.Rules, []string{"pii", "violence"}) {
		t.Errorf("data.rules = %v, want [pii violence]", got.Data.Rules)
	}
}

func TestBuildMCPError_ValidationError(t *testing.T) {
	ve := &errs.ValidationError{
		Problem: errs.Problem{
			Category: errs.CategoryValidation,
			Subtype:  errs.SubtypeInvalidParams,
			Message:  "bad",
		},
		Param: "title",
	}
	got := BuildMCPError(ve)
	if got.Code != -32602 {
		t.Errorf("outer code = %v, want -32602", got.Code)
	}
	if got.Data.Param != "title" {
		t.Errorf("data.param = %v, want %q", got.Data.Param, "title")
	}
}

func TestBuildMCPError_NetworkError(t *testing.T) {
	ne := &errs.NetworkError{
		Problem: errs.Problem{
			Category: errs.CategoryNetwork,
			Message:  "boom",
		},
		CauseKind: "timeout",
	}
	got := BuildMCPError(ne)
	if got.Code != -32603 {
		t.Errorf("outer code = %v, want -32603", got.Code)
	}
	if got.Data.Cause != "timeout" {
		t.Errorf("data.cause = %v, want %q", got.Data.Cause, "timeout")
	}
}

// TestBuildMCPError_TypedExtensionsThroughWrap pins that typed-extension
// fields survive an fmt.Errorf("%w", ...) wrap chain. Both
// internal/output's envelope writer and addTypedExtensions here use
// errors.As; without it the extensions silently disappear whenever a caller
// adds context before reaching the dispatcher.
func TestBuildMCPError_TypedExtensionsThroughWrap(t *testing.T) {
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			Message:  "x",
		},
		MissingScopes: []string{"docx:document"},
	}
	wrapped := fmt.Errorf("outer: %w", pe)
	out := BuildMCPError(wrapped)
	if out.Data == nil || len(out.Data.RequiredScopes) == 0 {
		t.Errorf("required_scopes missing — typed extensions should survive errors.As unwrap chain; data = %#v", out.Data)
	}
}

// TestBuildMCPError_OuterCodeNeverLarkNumeric asserts across a representative
// sample that the outer JSON-RPC `code` is always one of the reserved values
// and never equals the Lark numeric (which always lives in `data.code`).
func TestBuildMCPError_OuterCodeNeverLarkNumeric(t *testing.T) {
	samples := []error{
		&errs.PermissionError{Problem: errs.Problem{
			Category: errs.CategoryAuthorization, Subtype: errs.SubtypeMissingScope,
			Code: 99991679, Message: "x",
		}},
		&errs.APIError{Problem: errs.Problem{
			Category: errs.CategoryAPI, Subtype: errs.Subtype("task_conflict"),
			Code: 1470422, Message: "x",
		}},
		&errs.APIError{Problem: errs.Problem{
			Category: errs.CategoryAPI, Subtype: errs.SubtypeRateLimit,
			Code: 99991400, Message: "x",
		}},
		&errs.ValidationError{Problem: errs.Problem{
			Category: errs.CategoryValidation, Subtype: errs.SubtypeInvalidParams,
			Code: 1, Message: "x",
		}},
		&errs.InternalError{Problem: errs.Problem{
			Category: errs.CategoryInternal, Subtype: errs.SubtypeWrapped,
			Code: 500, Message: "x",
		}},
	}
	allowed := map[int]bool{-32602: true, -32603: true}
	for i := -32099; i <= -32000; i++ {
		allowed[i] = true
	}
	for _, err := range samples {
		got := BuildMCPError(err)
		if !allowed[got.Code] {
			t.Errorf("outer code %d is not in JSON-RPC reserved range", got.Code)
		}
		p, _ := errs.ProblemOf(err)
		if got.Code == p.Code {
			t.Errorf("outer code %d equals Lark numeric (must live in data.code)", got.Code)
		}
		if p.Code != 0 && got.Data.Code != p.Code {
			t.Errorf("data.code = %v, want %d", got.Data.Code, p.Code)
		}
	}
}

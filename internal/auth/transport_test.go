// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
)

// TestTryHandleMCPResponse_RecognisesDataCode pins the canonical MCP shape
// produced by errs/projection/mcp.go: the outer `error.code` is a JSON-RPC
// status (e.g. -32603) and the Lark numeric code lives in `error.data.code`.
// The transport must read `data.code` to look up the codeMeta and convert the
// response into *errs.SecurityPolicyError.
func TestTryHandleMCPResponse_RecognisesDataCode(t *testing.T) {
	t.Parallel()
	transport := &SecurityPolicyTransport{}

	result := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"error": map[string]interface{}{
			"code":    -32603, // JSON-RPC internal error
			"message": "challenge required",
			"data": map[string]interface{}{
				"code":          21000, // Lark code for challenge_required
				"type":          "policy",
				"subtype":       "challenge_required",
				"challenge_url": "https://example.com/challenge",
				"hint":          "please complete the challenge in your browser",
			},
		},
	}

	got := transport.tryHandleMCPResponse(result)
	var spErr *errs.SecurityPolicyError
	if !errors.As(got, &spErr) {
		t.Fatalf("expected *errs.SecurityPolicyError, got %T (err = %v)", got, got)
	}
	if spErr.Code != 21000 {
		t.Errorf("Code = %d, want 21000", spErr.Code)
	}
	if spErr.Subtype != errs.SubtypeChallengeRequired {
		t.Errorf("Subtype = %q, want %q", spErr.Subtype, errs.SubtypeChallengeRequired)
	}
	if spErr.ChallengeURL != "https://example.com/challenge" {
		t.Errorf("ChallengeURL = %q", spErr.ChallengeURL)
	}
	if spErr.Hint != "please complete the challenge in your browser" {
		t.Errorf("Hint = %q", spErr.Hint)
	}
}

// TestTryHandleMCPResponse_FallsBackToOuterCode verifies the transport stays
// compatible with legacy MCP producers that placed the Lark code in the outer
// `error.code` slot (data.code missing or zero).
func TestTryHandleMCPResponse_FallsBackToOuterCode(t *testing.T) {
	t.Parallel()
	transport := &SecurityPolicyTransport{}

	result := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    21001, // legacy shape: outer code carries the Lark code
			"message": "access denied",
			"data": map[string]interface{}{
				"challenge_url": "https://example.com/c",
				"cli_hint":      "contact admin",
			},
		},
	}

	got := transport.tryHandleMCPResponse(result)
	var spErr *errs.SecurityPolicyError
	if !errors.As(got, &spErr) {
		t.Fatalf("expected *errs.SecurityPolicyError, got %T (err = %v)", got, got)
	}
	if spErr.Subtype != errs.SubtypeAccessDenied {
		t.Errorf("Subtype = %q, want %q", spErr.Subtype, errs.SubtypeAccessDenied)
	}
	// Legacy `cli_hint` key must still surface when canonical `hint` is absent.
	if spErr.Hint != "contact admin" {
		t.Errorf("Hint = %q, want fallback from cli_hint", spErr.Hint)
	}
}

// TestTryHandleMCPResponse_NonPolicyCodeIgnored verifies the transport returns
// nil (passes through) when the Lark code does not classify as
// CategoryPolicy — keeps regular API errors out of the security-policy path.
func TestTryHandleMCPResponse_NonPolicyCodeIgnored(t *testing.T) {
	t.Parallel()
	transport := &SecurityPolicyTransport{}

	result := map[string]interface{}{
		"error": map[string]interface{}{
			"code":    -32603,
			"message": "permission denied",
			"data": map[string]interface{}{
				"code": 99991672, // app_scope_not_enabled — Authorization, not Policy
				"type": "authorization",
			},
		},
	}

	if err := transport.tryHandleMCPResponse(result); err != nil {
		t.Fatalf("expected nil (non-policy code), got %v", err)
	}
}

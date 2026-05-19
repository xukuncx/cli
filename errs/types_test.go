// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestPermissionErrorJSONShape(t *testing.T) {
	perm := &PermissionError{
		Problem: Problem{
			Category: CategoryAuthorization,
			Subtype:  SubtypeMissingScope,
			Message:  "x",
		},
		MissingScopes: []string{"docx:document"},
	}
	b, err := json.Marshal(perm)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	got := string(b)

	mustContain := []string{
		`"type":"authorization"`,
		`"subtype":"missing_scope"`,
		`"missing_scopes":["docx:document"]`,
	}
	for _, want := range mustContain {
		if !strings.Contains(got, want) {
			t.Errorf("json output missing %q\nfull output: %s", want, got)
		}
	}

	mustNotContain := []string{
		`"component"`,
		`"doc_url"`,
		`"retryable":false`,
	}
	for _, bad := range mustNotContain {
		if strings.Contains(got, bad) {
			t.Errorf("json output unexpectedly contains %q\nfull output: %s", bad, got)
		}
	}
}

// TestEmbedSemanticChasm proves the documented Go embed limitation:
// errors.As(*PermissionError, &p *Problem) returns false even though
// PermissionError embeds Problem. ProblemOf works around this by routing
// via the unexported problemCarrier interface.
func TestEmbedSemanticChasm(t *testing.T) {
	perm := &PermissionError{
		Problem: Problem{
			Category: CategoryAuthorization,
			Subtype:  SubtypeMissingScope,
			Message:  "missing",
		},
	}

	var p *Problem
	if errors.As(perm, &p) {
		t.Errorf("errors.As(*PermissionError, &*Problem) unexpectedly succeeded; Go embed semantic changed")
	}

	got, ok := ProblemOf(perm)
	if !ok {
		t.Fatalf("ProblemOf(*PermissionError) returned ok=false; expected to extract embedded Problem")
	}
	if got != &perm.Problem {
		t.Errorf("ProblemOf returned %p, want &perm.Problem = %p", got, &perm.Problem)
	}
	if got.Category != CategoryAuthorization {
		t.Errorf("extracted Problem.Category = %q, want %q", got.Category, CategoryAuthorization)
	}
}

func TestSecurityPolicyErrorUnwrap(t *testing.T) {
	orig := errors.New("transport stalled")
	spe := &SecurityPolicyError{
		Problem: Problem{Category: CategoryPolicy, Subtype: Subtype("challenge_required"), Message: "blocked"},
		Cause:   orig,
	}
	if got := errors.Unwrap(spe); got != orig {
		t.Fatalf("errors.Unwrap(spe) = %v, want %v", got, orig)
	}
	if !errors.Is(spe, orig) {
		t.Fatal("errors.Is(spe, orig) = false, want true")
	}
}

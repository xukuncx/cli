// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderFetchPretty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		result         map[string]interface{}
		omitTitle      bool
		wantHas        []string
		wantLacks      []string
		wantFirstIsH1  bool
		wantFirstIsNot string
	}{
		{
			name: "default includes title as H1",
			result: map[string]interface{}{
				"title":    "My Doc",
				"markdown": "body paragraph",
			},
			wantHas:       []string{"# My Doc", "body paragraph"},
			wantFirstIsH1: true,
		},
		{
			name: "omit-title strips the leading H1",
			result: map[string]interface{}{
				"title":    "My Doc",
				"markdown": "body paragraph",
			},
			omitTitle:      true,
			wantHas:        []string{"body paragraph"},
			wantLacks:      []string{"# My Doc"},
			wantFirstIsNot: "#",
		},
		{
			name: "empty title does not emit an empty H1 even with omit-title=false",
			result: map[string]interface{}{
				"title":    "",
				"markdown": "plain body",
			},
			wantHas:   []string{"plain body"},
			wantLacks: []string{"# "},
		},
		{
			name: "missing title field is not emitted",
			result: map[string]interface{}{
				"markdown": "only body",
			},
			wantHas:   []string{"only body"},
			wantLacks: []string{"# "},
		},
		{
			name: "has_more appends pagination hint",
			result: map[string]interface{}{
				"title":    "My Doc",
				"markdown": "partial body",
				"has_more": true,
			},
			wantHas: []string{"# My Doc", "partial body", "more content available"},
		},
		{
			name: "has_more false omits pagination hint",
			result: map[string]interface{}{
				"markdown": "whole body",
				"has_more": false,
			},
			wantLacks: []string{"more content available"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			renderFetchPretty(&buf, tt.result, tt.omitTitle)
			got := buf.String()
			for _, needle := range tt.wantHas {
				if !strings.Contains(got, needle) {
					t.Errorf("expected output to contain %q, got:\n%s", needle, got)
				}
			}
			for _, forbidden := range tt.wantLacks {
				if strings.Contains(got, forbidden) {
					t.Errorf("expected output NOT to contain %q, got:\n%s", forbidden, got)
				}
			}
			if tt.wantFirstIsH1 {
				if !strings.HasPrefix(got, "# ") {
					t.Errorf("expected output to start with '# ', got prefix %q", firstNChars(got, 20))
				}
			}
			if tt.wantFirstIsNot != "" {
				if strings.HasPrefix(got, tt.wantFirstIsNot) {
					t.Errorf("expected output NOT to start with %q, got prefix %q", tt.wantFirstIsNot, firstNChars(got, 20))
				}
			}
		})
	}
}

// TestRenderFetchPrettyRoundTripSafety verifies that omit-title output, when
// fed back as markdown body, does not carry a title line that would
// accumulate on re-import — the core Case 13 invariant.
func TestRenderFetchPrettyRoundTripSafety(t *testing.T) {
	t.Parallel()

	result := map[string]interface{}{
		"title":    "My Doc",
		"markdown": "## Section One\n\ncontent.\n",
	}

	var buf bytes.Buffer
	renderFetchPretty(&buf, result, true)
	body := buf.String()

	// The exported body must not start with the title as an H1. Section
	// headings deeper than H1 are fine and expected.
	if strings.HasPrefix(body, "# My Doc") {
		t.Fatalf("omit-title output still starts with the title H1; round-trip will accumulate duplicates:\n%s", body)
	}
	if !strings.Contains(body, "## Section One") {
		t.Fatalf("section heading was dropped, got:\n%s", body)
	}
}

func firstNChars(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

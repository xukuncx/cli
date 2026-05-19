// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package projection

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestOAuthErrorFor_AuthorizationAllowlist(t *testing.T) {
	cases := []struct {
		name    string
		subtype errs.Subtype
	}{
		{"missing_scope", errs.SubtypeMissingScope},
		{"app_scope_not_enabled", errs.SubtypeAppScopeNotEnabled},
		{"token_no_permission", errs.SubtypeTokenNoPermission},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := OAuthErrorFor(errs.CategoryAuthorization, tc.subtype)
			if !ok {
				t.Fatalf("ok = false, want true for %q", tc.subtype)
			}
			if got != "insufficient_scope" {
				t.Errorf("oauthError = %q, want %q", got, "insufficient_scope")
			}
		})
	}
}

func TestOAuthErrorFor_AuthenticationAllowlist(t *testing.T) {
	cases := []struct {
		name    string
		subtype errs.Subtype
	}{
		{"token_missing", errs.SubtypeTokenMissing},
		{"token_invalid", errs.SubtypeTokenInvalid},
		{"token_expired", errs.SubtypeTokenExpired},
		{"refresh_failed", errs.SubtypeRefreshFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := OAuthErrorFor(errs.CategoryAuthentication, tc.subtype)
			if !ok {
				t.Fatalf("ok = false, want true for %q", tc.subtype)
			}
			if got != "invalid_token" {
				t.Errorf("oauthError = %q, want %q", got, "invalid_token")
			}
		})
	}
}

func TestOAuthErrorFor_AppStatusNotAllowlisted(t *testing.T) {
	// app_status is intentionally NOT in the OAuth allowlist — re-auth
	// won't fix an app-disabled error, so we don't mislead OAuth consumers.
	got, ok := OAuthErrorFor(errs.CategoryAuthorization, errs.SubtypeAppStatus)
	if ok {
		t.Fatalf("ok = true, want false for app_status (got %q)", got)
	}
	if got != "" {
		t.Errorf("oauthError = %q, want empty", got)
	}
}

func TestOAuthErrorFor_ValidationNotMapped(t *testing.T) {
	got, ok := OAuthErrorFor(errs.CategoryValidation, errs.SubtypeInvalidParams)
	if ok {
		t.Fatalf("ok = true, want false for validation category")
	}
	if got != "" {
		t.Errorf("oauthError = %q, want empty", got)
	}
}

func TestOAuthErrorFor_OtherCategoriesNotMapped(t *testing.T) {
	for _, cat := range []errs.Category{
		errs.CategoryConfig,
		errs.CategoryNetwork,
		errs.CategoryAPI,
		errs.CategoryPolicy,
		errs.CategoryInternal,
		errs.CategoryConfirmation,
	} {
		t.Run(string(cat), func(t *testing.T) {
			got, ok := OAuthErrorFor(cat, errs.Subtype("anything"))
			if ok {
				t.Errorf("ok = true, want false for %q (got %q)", cat, got)
			}
		})
	}
}

func TestBuildBearerHeader_Nil(t *testing.T) {
	got, ok := BuildBearerHeader(nil)
	if ok {
		t.Fatalf("ok = true, want false")
	}
	if got != "" {
		t.Errorf("header = %q, want empty", got)
	}
}

func TestBuildBearerHeader_SubtypeNotInAllowlist(t *testing.T) {
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeAppStatus, // not allowlisted
			Message:  "app disabled",
		},
	}
	got, ok := BuildBearerHeader(pe)
	if ok {
		t.Fatalf("ok = true, want false (got %q)", got)
	}
}

func TestBuildBearerHeader_AllFields(t *testing.T) {
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			Message:  "need scope",
		},
		MissingScopes: []string{"docx:document", "drive:drive"},
		ConsoleURL:    "https://example/app",
	}
	got, ok := BuildBearerHeader(pe)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if !strings.HasPrefix(got, "Bearer ") {
		t.Errorf("header does not start with %q: %q", "Bearer ", got)
	}
	wantFragments := []string{
		`error="insufficient_scope"`,
		`scope="docx:document drive:drive"`,
		`error_description="need scope"`,
		`error_uri="https://example/app"`,
	}
	for _, frag := range wantFragments {
		if !strings.Contains(got, frag) {
			t.Errorf("header = %q, missing fragment %q", got, frag)
		}
	}
}

func TestBuildBearerHeader_EscapesQuotesAndBackslashes(t *testing.T) {
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			Message:  `he said "hi\there"`,
		},
	}
	got, ok := BuildBearerHeader(pe)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	// Each `"` becomes `\"`, each `\` becomes `\\`.
	wantDesc := `error_description="he said \"hi\\there\""`
	if !strings.Contains(got, wantDesc) {
		t.Errorf("header = %q\nwant to contain %q", got, wantDesc)
	}
}

func TestBuildBearerHeader_StripsControlChars(t *testing.T) {
	// Control chars and bare CR/LF are not legal qdtext (RFC 7230 §3.2.6) and
	// would corrupt the WWW-Authenticate header framing. They must be replaced
	// with a single space. HTAB is preserved.
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			Message:  "bad\nrequest\twith\rctrl",
		},
	}
	got, ok := BuildBearerHeader(pe)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("header contains raw CR/LF: %q", got)
	}
	for _, r := range got {
		if r < 0x20 && r != '\t' {
			t.Errorf("header contains raw control char %#x: %q", r, got)
		}
		if r == 0x7f {
			t.Errorf("header contains DEL (0x7f): %q", got)
		}
	}
	// HTAB must survive.
	wantDesc := "error_description=\"bad request\twith ctrl\""
	if !strings.Contains(got, wantDesc) {
		t.Errorf("header = %q\nwant to contain %q", got, wantDesc)
	}
}

// TestBuildBearerHeader_EscapesScopeValues pins that MissingScopes values are
// passed through escapeRFC6750 so a hostile or malformed scope string cannot
// break out of its quoted-string and corrupt the WWW-Authenticate header.
func TestBuildBearerHeader_EscapesScopeValues(t *testing.T) {
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
		},
		MissingScopes: []string{`docx:doc"hostile`, `drive:drive`},
	}
	got, ok := BuildBearerHeader(pe)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	// The embedded quote must be escaped to \" so the scope value remains
	// inside its quoted-string framing.
	wantFrag := `scope="docx:doc\"hostile drive:drive"`
	if !strings.Contains(got, wantFrag) {
		t.Errorf("header = %q\nwant to contain %q", got, wantFrag)
	}
}

// TestBuildBearerHeader_EscapesErrorURI pins that ConsoleURL is escaped before
// emission as error_uri so a backslash or quote in the URL cannot break header
// framing. Control characters (here \n) must also be neutralised.
func TestBuildBearerHeader_EscapesErrorURI(t *testing.T) {
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
		},
		ConsoleURL: "https://example\\path\nbroken",
	}
	got, ok := BuildBearerHeader(pe)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	// Backslash is escaped via quoted-pair; \n is replaced with a single space.
	wantFrag := `error_uri="https://example\\path broken"`
	if !strings.Contains(got, wantFrag) {
		t.Errorf("header = %q\nwant to contain %q", got, wantFrag)
	}
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("header contains raw CR/LF: %q", got)
	}
}

func TestBuildBearerHeader_OmitsEmptyOptionals(t *testing.T) {
	pe := &errs.PermissionError{
		Problem: errs.Problem{
			Category: errs.CategoryAuthorization,
			Subtype:  errs.SubtypeMissingScope,
			// Message empty, no scopes, no ConsoleURL
		},
	}
	got, ok := BuildBearerHeader(pe)
	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if got != `Bearer error="insufficient_scope"` {
		t.Errorf("header = %q, want %q", got, `Bearer error="insufficient_scope"`)
	}
}

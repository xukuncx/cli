// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package projection translates canonical lark-cli typed errors into wire formats
// expected by external consumers (OAuth Bearer header, MCP JSON-RPC).
package projection

import (
	"strings"

	"github.com/larksuite/cli/errs"
)

// OAuthErrorFor maps a (Category, Subtype) pair to the RFC 6750 Bearer `error` value.
// Returns ok=false when no mapping exists (adapter must NOT emit a Bearer header in that case).
//
// Allowlist only. Unlisted pairs return ok=false to avoid misleading downstream
// OAuth consumers (e.g. routing app_status as invalid_token would push consumers
// to re-auth, which won't fix an app-disabled error).
func OAuthErrorFor(cat errs.Category, subtype errs.Subtype) (oauthError string, ok bool) {
	switch cat {
	case errs.CategoryAuthentication:
		switch subtype {
		case errs.SubtypeTokenMissing,
			errs.SubtypeTokenInvalid,
			errs.SubtypeTokenExpired,
			errs.SubtypeRefreshFailed:
			return "invalid_token", true
		}
	case errs.CategoryAuthorization:
		switch subtype {
		case errs.SubtypeMissingScope,
			errs.SubtypeAppScopeNotEnabled,
			errs.SubtypeTokenNoPermission:
			return "insufficient_scope", true
		}
	}
	return "", false
}

// BuildBearerHeader composes the value of a `WWW-Authenticate: Bearer ...` header
// from a PermissionError. Returns ok=false when no header should be emitted
// (e.g. the error's subtype is not in the OAuth allowlist).
//
// Output format per RFC 6750 §3:
//
//	Bearer error="<oauth>", scope="<sp-joined missing scopes>", error_description="<msg>", error_uri="<console>"
func BuildBearerHeader(pe *errs.PermissionError) (string, bool) {
	if pe == nil {
		return "", false
	}
	oauthErr, ok := OAuthErrorFor(pe.Category, pe.Subtype)
	if !ok {
		return "", false
	}
	var parts []string
	parts = append(parts, `error="`+oauthErr+`"`)
	if len(pe.MissingScopes) > 0 {
		// Scope tokens defensively escaped: RFC 6750 scope-token grammar
		// forbids whitespace and quote characters, but missing-scope strings
		// come from upstream Lark API responses and we cannot assume they
		// conform. Escaping each individually preserves the space-joined
		// scope1 scope2 wire format without letting a stray `"` or `\`
		// corrupt the WWW-Authenticate header.
		escapedScopes := make([]string, len(pe.MissingScopes))
		for i, s := range pe.MissingScopes {
			escapedScopes[i] = escapeRFC6750(s)
		}
		parts = append(parts, `scope="`+strings.Join(escapedScopes, " ")+`"`)
	}
	if pe.Message != "" {
		parts = append(parts, `error_description="`+escapeRFC6750(pe.Message)+`"`)
	}
	if pe.ConsoleURL != "" {
		parts = append(parts, `error_uri="`+escapeRFC6750(pe.ConsoleURL)+`"`)
	}
	return "Bearer " + strings.Join(parts, ", "), true
}

// escapeRFC6750 prepares a string for inclusion as a quoted-string value per
// RFC 6750 / RFC 7230 §3.2.6. Control characters (other than HTAB) and bare
// CR/LF are replaced with a single space — they are not legal qdtext and
// would corrupt the WWW-Authenticate header framing. Backslash and double-quote
// are escaped with a leading backslash (quoted-pair).
func escapeRFC6750(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for _, r := range s {
		switch {
		case r == '\t', r == ' ':
			b.WriteRune(r)
		case r < 0x20, r == 0x7f:
			b.WriteByte(' ')
		case r == '"', r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

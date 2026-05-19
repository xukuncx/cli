// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package projection

import (
	"errors"
	"strings"

	"github.com/larksuite/cli/errs"
)

// JSON-RPC 2.0 error code values used by MCP. The Lark numeric code is NEVER
// placed on `error.code` — it always lives in `error.data.code`.
const (
	mcpInvalidParams = -32602
	mcpInternalError = -32603
	mcpAPIDefault    = -32000 // application-defined; -32000..-32099 reserved
)

// apiSubtypePrefixCodes assigns sub-ranges of the JSON-RPC application-error
// space to specific Lark API services. Walked longest-prefix-first so
// overlapping entries (e.g. "task_attachment_" vs "task_") have deterministic
// resolution. Same-package static slice; service packages MUST NOT register
// here (enforced by CI lint CheckNoRegistrar).
//
// Maintenance: keep entries sorted by prefix length DESC, then alphabetically
// for stability across edits.
var apiSubtypePrefixCodes = []struct {
	prefix string
	code   int
}{
	{prefix: "task_", code: -32010},
}

// MCPCodeFor maps (Category, Subtype) to the JSON-RPC outer `error.code`.
// Lark numeric codes never appear here — they live in `error.data.code`.
func MCPCodeFor(cat errs.Category, subtype errs.Subtype) int {
	switch cat {
	case errs.CategoryValidation, errs.CategoryConfirmation:
		return mcpInvalidParams
	case errs.CategoryAuthentication,
		errs.CategoryAuthorization,
		errs.CategoryConfig,
		errs.CategoryNetwork,
		errs.CategoryPolicy,
		errs.CategoryInternal:
		return mcpInternalError
	case errs.CategoryAPI:
		for _, entry := range apiSubtypePrefixCodes {
			if strings.HasPrefix(string(subtype), entry.prefix) {
				return entry.code
			}
		}
		return mcpAPIDefault
	}
	return mcpInternalError
}

// MCPErrorObject is the outer JSON-RPC 2.0 error object that goes into the
// `error` field of a Response (per JSON-RPC 2.0 §5.1). It is a DTO, not a
// Go `error` — the type name avoids shadowing the `error` interface in the
// reader's mental model.
//
// BuildMCPError returns *MCPErrorObject ready for json.Marshal.
type MCPErrorObject struct {
	Code    int           `json:"code"`
	Message string        `json:"message"`
	Data    *MCPErrorData `json:"data,omitempty"`
}

// MCPErrorData carries the typed Problem fields and per-category extensions
// on the JSON-RPC outer error's `data` field.
type MCPErrorData struct {
	Type      errs.Category `json:"type"`
	Subtype   errs.Subtype  `json:"subtype,omitempty"`
	Code      int           `json:"code,omitempty"`
	Hint      string        `json:"hint,omitempty"`
	LogID     string        `json:"log_id,omitempty"`
	Retryable bool          `json:"retryable,omitempty"`

	// Typed extensions (only set when matching error type).
	RequiredScopes []string `json:"required_scopes,omitempty"`
	Identity       string   `json:"identity,omitempty"`
	ConsoleURL     string   `json:"console_url,omitempty"`
	ChallengeURL   string   `json:"challenge_url,omitempty"`
	Rules          []string `json:"rules,omitempty"`
	Param          string   `json:"param,omitempty"`
	Cause          string   `json:"cause,omitempty"`
}

// BuildMCPError composes a JSON-RPC 2.0 error envelope from any typed error.
// Returns a *MCPErrorObject suitable for use as the `error` field of a
// JSON-RPC response. The outer `code` is from MCPCodeFor; the Lark numeric
// `code`, if present, goes in `data.code`. Typed extension fields are
// placed in `data` under their wire names (required_scopes, console_url,
// challenge_url, etc.).
func BuildMCPError(err error) *MCPErrorObject {
	if err == nil {
		return nil
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		// Fall back to a generic internal envelope for untyped errors.
		return &MCPErrorObject{
			Code:    mcpInternalError,
			Message: err.Error(),
			Data: &MCPErrorData{
				Type:    errs.CategoryInternal,
				Subtype: errs.SubtypeWrapped,
			},
		}
	}
	data := &MCPErrorData{
		Type:      p.Category,
		Subtype:   p.Subtype,
		Code:      p.Code,
		Hint:      p.Hint,
		LogID:     p.LogID,
		Retryable: p.Retryable,
	}
	addTypedExtensions(err, data)
	return &MCPErrorObject{
		Code:    MCPCodeFor(p.Category, p.Subtype),
		Message: p.Message,
		Data:    data,
	}
}

// addTypedExtensions mirrors the typed-error extension fields onto the
// JSON-RPC data envelope using their wire names. PermissionError extensions
// go through the lark-adapter convention (required_scopes, console_url,
// identity); SecurityPolicyError exposes challenge_url; ContentSafetyError
// exposes rules; ValidationError exposes param; NetworkError exposes cause.
//
// Uses errors.As so the extension lookup survives fmt.Errorf("%w", ...) wrap
// chains — important when callers wrap typed errors with additional context
// before they reach BuildMCPError.
func addTypedExtensions(err error, data *MCPErrorData) {
	var pe *errs.PermissionError
	if errors.As(err, &pe) {
		if len(pe.MissingScopes) > 0 {
			data.RequiredScopes = pe.MissingScopes
		}
		data.Identity = pe.Identity
		data.ConsoleURL = pe.ConsoleURL
		return
	}
	var spe *errs.SecurityPolicyError
	if errors.As(err, &spe) {
		data.ChallengeURL = spe.ChallengeURL
		return
	}
	var cse *errs.ContentSafetyError
	if errors.As(err, &cse) {
		data.Rules = cse.Rules
		return
	}
	var ve *errs.ValidationError
	if errors.As(err, &ve) {
		data.Param = ve.Param
		return
	}
	var ne *errs.NetworkError
	if errors.As(err, &ne) {
		data.Cause = ne.CauseKind
	}
}

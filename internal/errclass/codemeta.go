// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errclass

import (
	"fmt"

	"github.com/larksuite/cli/errs"
)

// CodeMeta is the classification metadata attached to a Lark numeric code.
// It does NOT carry Message or Hint — those are derived at the dispatcher
// (see BuildAPIError).
type CodeMeta struct {
	Category  errs.Category
	Subtype   errs.Subtype
	Retryable bool
}

// codeMeta is the central registry. Top-level entries (auth/authorization/api/
// policy/config codes shared across services) live here; service-specific
// sub-tables (e.g. task) live in dedicated files like codemeta_task.go and
// merge into this map via init().
//
// Go language guarantees package-level vars initialize before init() functions,
// so sub-tables registering via init() can always assume codeMeta is non-nil.
var codeMeta = map[int]CodeMeta{
	// CategoryAuthentication
	99991661: {errs.CategoryAuthentication, errs.SubtypeTokenMissing, false},
	99991671: {errs.CategoryAuthentication, errs.SubtypeTokenMissing, false},
	99991668: {errs.CategoryAuthentication, errs.SubtypeTokenInvalid, false},
	99991663: {errs.CategoryAuthentication, errs.SubtypeTokenInvalid, false},
	99991677: {errs.CategoryAuthentication, errs.SubtypeTokenExpired, false},
	20026:    {errs.CategoryAuthentication, errs.SubtypeRefreshFailed, false},
	20037:    {errs.CategoryAuthentication, errs.SubtypeRefreshFailed, false},
	20064:    {errs.CategoryAuthentication, errs.SubtypeRefreshFailed, false},
	20073:    {errs.CategoryAuthentication, errs.SubtypeRefreshFailed, false},
	20050:    {errs.CategoryAuthentication, errs.SubtypeRefreshFailed, true}, // sole retryable refresh code

	// CategoryAuthorization
	99991672: {errs.CategoryAuthorization, errs.SubtypeAppScopeNotEnabled, false},
	99991676: {errs.CategoryAuthorization, errs.SubtypeTokenNoPermission, false},
	99991679: {errs.CategoryAuthorization, errs.SubtypeMissingScope, false},
	230027:   {errs.CategoryAuthorization, errs.SubtypeMissingScope, false},
	99991673: {errs.CategoryAuthorization, errs.SubtypeAppStatus, false},
	99991662: {errs.CategoryAuthorization, errs.SubtypeAppStatus, false},

	// CategoryAPI
	99991400: {errs.CategoryAPI, errs.SubtypeRateLimit, true},
	1061045:  {errs.CategoryAPI, errs.SubtypeConflict, true},
	1064510:  {errs.CategoryAPI, errs.SubtypeCrossTenantUnit, false},
	1064511:  {errs.CategoryAPI, errs.SubtypeCrossBrand, false},
	1310246:  {errs.CategoryAPI, errs.SubtypeInvalidParams, false},
	1063006:  {errs.CategoryAPI, errs.SubtypeRateLimit, false}, // drive perm-apply quota; 5/day, not short-term retryable
	1063007:  {errs.CategoryAPI, errs.SubtypeInvalidParams, false},
	231205:   {errs.CategoryAPI, errs.SubtypeOwnershipMismatch, false},

	// CategoryConfig
	99991543: {errs.CategoryConfig, errs.SubtypeAppCredInvalid, false},

	// CategoryPolicy
	21000: {errs.CategoryPolicy, errs.SubtypeChallengeRequired, false},
	21001: {errs.CategoryPolicy, errs.SubtypeAccessDenied, false},
}

// LookupCodeMeta is the single lookup entry. Returns ok=false for unknown codes —
// the caller (BuildAPIError) is responsible for falling back to
// CategoryAPI/SubtypeAPIGeneric.
func LookupCodeMeta(code int) (CodeMeta, bool) {
	m, ok := codeMeta[code]
	return m, ok
}

// mergeCodeMeta is invoked by sub-table init() functions to merge service-specific
// codes into the central registry. Panics on duplicate code so a misregistration
// fails fast at startup rather than producing silently-inconsistent classification.
func mergeCodeMeta(src map[int]CodeMeta, owner string) {
	for code, meta := range src {
		if existing, dup := codeMeta[code]; dup {
			panic(fmt.Sprintf("codeMeta dup: code %d already mapped %+v; %s wants %+v",
				code, existing, owner, meta))
		}
		codeMeta[code] = meta
	}
}

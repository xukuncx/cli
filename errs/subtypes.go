// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package errs

// Subtype is the second-level taxonomy axis. Wire JSON: "subtype".
type Subtype string

// CategoryValidation subtypes
const (
	SubtypeInvalidArg Subtype = "invalid_arg"
)

// CategoryAuthentication subtypes
const (
	SubtypeTokenMissing  Subtype = "token_missing"
	SubtypeTokenExpired  Subtype = "token_expired"
	SubtypeTokenInvalid  Subtype = "token_invalid"
	SubtypeRefreshFailed Subtype = "refresh_failed"
	SubtypeAuthGeneric   Subtype = "auth_generic"
)

// CategoryAuthorization subtypes
const (
	SubtypeMissingScope       Subtype = "missing_scope"
	SubtypeAppScopeNotEnabled Subtype = "app_scope_not_enabled"
	SubtypeTokenNoPermission  Subtype = "token_no_permission"
	SubtypeAppStatus          Subtype = "app_status"
)

// CategoryConfig subtypes
const (
	SubtypeAppCredInvalid Subtype = "app_cred_invalid"
)

// CategoryNetwork subtypes
const (
	SubtypeNetworkTransport Subtype = "transport"
)

// CategoryAPI subtypes
const (
	SubtypeRateLimit         Subtype = "rate_limit"
	SubtypeConflict          Subtype = "conflict"
	SubtypeCrossTenantUnit   Subtype = "cross_tenant_unit"
	SubtypeCrossBrand        Subtype = "cross_brand"
	SubtypeInvalidParams     Subtype = "invalid_params"
	SubtypeOwnershipMismatch Subtype = "ownership_mismatch"
	SubtypeAPIGeneric        Subtype = "api_generic"
)

// CategoryPolicy subtypes (security-policy envelope shape)
const (
	SubtypeChallengeRequired Subtype = "challenge_required"
	SubtypeAccessDenied      Subtype = "access_denied"
)

// CategoryInternal subtypes
const (
	SubtypeWrapped    Subtype = "wrapped"
	SubtypeSDKFailure Subtype = "sdk_failure"
	SubtypeJSONParse  Subtype = "json_parse"
)

// CategoryConfirmation subtypes intentionally have no declarations yet.

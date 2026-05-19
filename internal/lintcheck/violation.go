// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package lintcheck

// Action enumerates the response modes for a violation.
type Action string

const (
	// ActionReject hard-fails CI. Only REJECT contributes to a nonzero
	// lintcheck exit code.
	ActionReject Action = "REJECT"
	// ActionLabel emits a diagnostic so CI can label the PR but does not fail.
	ActionLabel Action = "LABEL"
	// ActionWarning surfaces a reviewer-attention note without failing CI.
	// CI does NOT exit nonzero on warnings; they are reviewer signal only.
	ActionWarning Action = "WARNING"
)

// Violation describes a single lint hit.
type Violation struct {
	Rule       string // "problem_embed" | "no_registrar" | "adhoc_subtype" | "declared_subtype"
	Action     Action
	File       string
	Line       int
	Message    string
	Suggestion string
}

// subtypeClassification is the package-internal verdict produced by the
// CheckDeclaredSubtype classifier for a single Subtype: expression. Empty action means
// "accept silently".
type subtypeClassification struct {
	rule, message, suggestion string
	action                    Action
}

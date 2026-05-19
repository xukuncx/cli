// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package lintcheck implements the five source-level AST checks that guard
// the errs/ contract. golangci-lint's forbidigo (.golangci.yml) contributes
// one additional rule (business path must use typed errors); the five
// checks here are AST-level guards golangci-lint cannot express directly.
//
//	CheckProblemEmbed             (REJECT)  errs/ typed errors must embed Problem,
//	                                        have a matching IsXxx predicate, and be
//	                                        exercised by a test in errs/.
//	CheckNoRegistrar              (REJECT)  shortcuts/* and internal/* (except
//	                                        internal/output) must not register
//	                                        code-meta via mergeCodeMeta or an init()
//	                                        registrar.
//	CheckAdHocSubtype             (LABEL)   Subtype: "ad_hoc_*" literals emit a
//	                                        label-trigger diagnostic
//	                                        ([needs-taxonomy-decision]) — NOT a hard
//	                                        failure.
//	CheckDeclaredSubtype          (REJECT)  Subtype: literals must resolve to a
//	                                        declared errs.Subtype* constant or match
//	                                        ad_hoc_*. Dynamic values emit WARNING.
//	CheckTypedErrorCompleteness   (REJECT)  every *errs.<X>Error literal must set
//	                                        Problem.Category, Problem.Subtype, and
//	                                        Problem.Message — incomplete typed
//	                                        errors emit empty wire fields.
//
// The exported entrypoints operate on either source strings (cheap
// unit-test fixtures) or directory trees (the production cmd/lintcheck CLI
// driver). They return a Violation list; callers decide how to surface (CLI
// exit-code, golangci-lint custom plugin, etc.).
package lintcheck

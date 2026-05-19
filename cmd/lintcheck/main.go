// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command lintcheck runs the source-level errs/ contract guards (all four checks).
// The fifth contract rule (business path must use typed errors) lives in
// .golangci.yml as a forbidigo entry; the four checks here are AST-level
// guards that golangci-lint cannot express.
//
// Usage (from repo root):
//
//	go run ./cmd/lintcheck            # scan current directory
//	go run ./cmd/lintcheck ./...      # same; argument is informational
//	go run ./cmd/lintcheck /path/to/repo
//
// Exit codes:
//
//	0  no REJECT violations (LABEL and WARNING diagnostics are advisory)
//	1  one or more REJECT violations
//
// WARNING and LABEL diagnostics are still printed so a CI workflow can grep
// for the prefixes — LABEL emits `[needs-taxonomy-decision]` for an
// auto-labeler — but neither severity fails CI. Only REJECT does.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/larksuite/cli/internal/lintcheck"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr,
			"Usage: lintcheck [repo-root]\n"+
				"Runs errs/ contract guards (all four checks). Default root is the current directory.\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
		// `./...` is a common Go-toolchain idiom; map it to the working dir.
		if root == "./..." {
			root = "."
		}
	}

	violations, err := lintcheck.ScanRepo(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lintcheck: %v\n", err)
		os.Exit(2)
	}

	exitCode := 0
	for _, v := range violations {
		fmt.Fprintf(os.Stderr, "%s:%d: [%s/%s] %s\n", v.File, v.Line, v.Action, v.Rule, v.Message)
		if v.Suggestion != "" {
			fmt.Fprintf(os.Stderr, "    hint: %s\n", v.Suggestion)
		}
		if v.Action == lintcheck.ActionReject {
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

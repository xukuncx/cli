// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Dev-only tool: inject scope strings into the local stored UAT.
// Workaround for BOE not yet registering miaoda:app:* scopes so that
// local mock testing can pass framework scope-check.
//
// NOTE: this directory starts with "_" so `go build ./...` / `go test ./...` ignore it.
// Run explicitly:
//
//	go run ./cmd/_apps_dev_tools/inject_scopes <appId> <userOpenId> <scope1> [scope2 ...]
//
// Caveat: every time lark-cli refreshes the UAT (server-side refresh),
// the server-provided scope list overwrites this injection. Re-run after refresh.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/larksuite/cli/internal/auth"
)

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: inject-scopes <appId> <userOpenId> <scope1> [scope2 ...]")
		os.Exit(2)
	}
	appID, userID := os.Args[1], os.Args[2]
	extra := os.Args[3:]

	t := auth.GetStoredToken(appID, userID)
	if t == nil {
		fmt.Fprintln(os.Stderr, "no stored token for", appID, userID)
		os.Exit(1)
	}

	have := map[string]bool{}
	for _, s := range strings.Fields(t.Scope) {
		have[s] = true
	}
	out := strings.Fields(t.Scope)
	added := 0
	for _, s := range extra {
		if !have[s] {
			out = append(out, s)
			have[s] = true
			added++
		}
	}
	t.Scope = strings.Join(out, " ")

	if err := auth.SetStoredToken(t); err != nil {
		fmt.Fprintln(os.Stderr, "save err:", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "OK: injected %d new scope(s); total %d\n", added, len(out))
	for _, s := range extra {
		fmt.Fprintf(os.Stderr, "  %s\n", s)
	}
}

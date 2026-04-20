// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

// v1FetchFlags returns the flag definitions for the v1 (MCP) fetch path.
func v1FetchFlags() []common.Flag {
	return []common.Flag{
		{Name: "offset", Desc: "pagination offset", Hidden: true},
		{Name: "limit", Desc: "pagination limit", Hidden: true},
	}
}

var docsFetchFlagVersions = buildFlagVersionMap(v1FetchFlags(), v2FetchFlags())

// useV2Fetch returns true when the v2 (OpenAPI) fetch path should be used.
// Explicit --api-version v2 takes priority; otherwise auto-detect by v2-only
// flags with non-default values (bare "--doc xxx" stays on v1).
func useV2Fetch(runtime *common.RuntimeContext) bool {
	if runtime.Str("api-version") == "v2" {
		return true
	}
	// --doc-format default is "xml", --detail default is "simple", --revision-id default is -1.
	// Only trigger auto-detect when a non-default value is present.
	if d := runtime.Str("detail"); d != "" && d != "simple" {
		return true
	}
	if f := runtime.Str("doc-format"); f != "" && f != "xml" {
		return true
	}
	if runtime.Int("revision-id") != -1 {
		return true
	}
	if m := runtime.Str("scope"); m != "" && m != "full" {
		return true
	}
	return false
}

var DocsFetch = common.Shortcut{
	Service:     "docs",
	Command:     "+fetch",
	Description: "Fetch Lark document content",
	Risk:        "read",
	Scopes:      []string{"docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: concatFlags(
		[]common.Flag{
			{Name: "api-version", Desc: "API version", Default: "v1", Enum: []string{"v1", "v2"}},
			{Name: "doc", Desc: "document URL or token", Required: true},
		},
		v1FetchFlags(),
		v2FetchFlags(),
	),
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if useV2Fetch(runtime) {
			return dryRunFetchV2(ctx, runtime)
		}
		return dryRunFetchV1(ctx, runtime)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if useV2Fetch(runtime) {
			return executeFetchV2(ctx, runtime)
		}
		return executeFetchV1(ctx, runtime)
	},
	PostMount: func(cmd *cobra.Command) {
		installVersionedHelp(cmd, "v1", docsFetchFlagVersions)
	},
}

// ── V1 (MCP) implementation ──

func dryRunFetchV1(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	args := buildFetchArgsV1(runtime)
	return common.NewDryRunAPI().
		POST(common.MCPEndpoint(runtime.Config.Brand)).
		Desc("MCP tool: fetch-doc").
		Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "fetch-doc", "arguments": args}}).
		Set("mcp_tool", "fetch-doc").Set("args", args)
}

func executeFetchV1(_ context.Context, runtime *common.RuntimeContext) error {
	warnDeprecatedV1(runtime, "+fetch")
	args := buildFetchArgsV1(runtime)

	result, err := common.CallMCPTool(runtime, "fetch-doc", args)
	if err != nil {
		return err
	}

	if md, ok := result["markdown"].(string); ok {
		result["markdown"] = fixExportedMarkdown(md)
	}

	runtime.OutFormat(result, nil, func(w io.Writer) {
		if title, ok := result["title"].(string); ok && title != "" {
			fmt.Fprintf(w, "# %s\n\n", title)
		}
		if md, ok := result["markdown"].(string); ok {
			fmt.Fprintln(w, md)
		}
		if hasMore, ok := result["has_more"].(bool); ok && hasMore {
			fmt.Fprintln(w, "\n--- more content available, use --offset and --limit to paginate ---")
		}
	})
	return nil
}

func buildFetchArgsV1(runtime *common.RuntimeContext) map[string]interface{} {
	args := map[string]interface{}{
		"doc_id": runtime.Str("doc"),
		"skip_task_detail": true,
	}
	if v := runtime.Str("offset"); v != "" {
		n, _ := strconv.Atoi(v)
		args["offset"] = n
	}
	if v := runtime.Str("limit"); v != "" {
		n, _ := strconv.Atoi(v)
		args["limit"] = n
	}
	return args
}

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/shortcuts/common"
)

var DocsFetch = common.Shortcut{
	Service:     "docs",
	Command:     "+fetch",
	Description: "Fetch Lark document content",
	Risk:        "read",
	Scopes:      []string{"docx:document:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or token", Required: true},
		{Name: "offset", Desc: "pagination offset"},
		{Name: "limit", Desc: "pagination limit"},
		{Name: "omit-title", Type: "bool", Desc: "in --format=pretty output, skip the leading '# <title>' line. Lark stores document title as an independent field, so including it when piping fetch output into `docs +update --mode=overwrite` accumulates duplicate H1 blocks on every round-trip."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		args := map[string]interface{}{
			"doc_id": runtime.Str("doc"),
			// Default to skipping embedded task detail expansion for faster +fetch output.
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
		return common.NewDryRunAPI().
			POST(common.MCPEndpoint(runtime.Config.Brand)).
			Desc("MCP tool: fetch-doc").
			Body(map[string]interface{}{"method": "tools/call", "params": map[string]interface{}{"name": "fetch-doc", "arguments": args}}).
			Set("mcp_tool", "fetch-doc").Set("args", args)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		args := map[string]interface{}{
			"doc_id": runtime.Str("doc"),
			// Default to skipping embedded task detail expansion for faster +fetch output.
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

		result, err := common.CallMCPTool(runtime, "fetch-doc", args)
		if err != nil {
			return err
		}

		if md, ok := result["markdown"].(string); ok {
			result["markdown"] = fixExportedMarkdown(md)
		}

		omitTitle := runtime.Bool("omit-title")
		runtime.OutFormat(result, nil, func(w io.Writer) {
			renderFetchPretty(w, result, omitTitle)
		})
		return nil
	},
}

// renderFetchPretty writes the human-readable (pretty) form of a fetch-doc
// result to w. Split out from Execute so tests can verify the --omit-title
// behavior directly without spinning up a runtime/MCP mock.
//
// When omitTitle is false (default), the leading "# <title>\n\n" is
// preserved for reader orientation. When true, the title line is skipped so
// the output is safe to pipe into `docs +update --mode=overwrite` without
// accumulating duplicate H1 blocks on every round-trip (see Case 13 in the
// pitfall list).
func renderFetchPretty(w io.Writer, result map[string]interface{}, omitTitle bool) {
	if !omitTitle {
		if title, ok := result["title"].(string); ok && title != "" {
			fmt.Fprintf(w, "# %s\n\n", title)
		}
	}
	if md, ok := result["markdown"].(string); ok {
		fmt.Fprintln(w, md)
	}
	if hasMore, ok := result["has_more"].(bool); ok && hasMore {
		fmt.Fprintln(w, "\n--- more content available, use --offset and --limit to paginate ---")
	}
}

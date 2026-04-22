// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/larksuite/cli/shortcuts/common"
)

// ── Table command routing ──

var validTableCommands = map[string]bool{
	"table_insert_rows":     true,
	"table_insert_cols":     true,
	"table_delete_rows":     true,
	"table_delete_cols":     true,
	"table_merge_cells":     true,
	"table_unmerge_cells":   true,
	"table_update_property": true,
}

func isTableCommand(cmd string) bool {
	return validTableCommands[cmd]
}

// tableUpdateFlags returns the flag definitions for table operations.
// All flags are Hidden: true following the v2 pattern — visible in versioned help only.
func tableUpdateFlags() []common.Flag {
	return []common.Flag{
		{Name: "table-block-id", Desc: "table block ID (from +fetch --detail with-ids)", Hidden: true},
		{Name: "cell", Desc: "cell coordinate in A1 notation (e.g. A1, B3)", Hidden: true},
		{Name: "range", Desc: "cell range in A1 notation, inclusive (e.g. A1:C3)", Hidden: true},
		{Name: "col", Desc: "column letter (A, B, ...) or 0=before-first, -1=append", Hidden: true},
		{Name: "row-index", Type: "int", Desc: "row position (1-based, aligned with A1 row numbers; 0=append fallback, 1=insert as row 1, -1=end)", Hidden: true},
		{Name: "row-start", Type: "int", Desc: "row start index (1-based, inclusive; e.g. 1=first row)", Hidden: true},
		{Name: "row-end", Type: "int", Desc: "row end index (1-based, exclusive; e.g. 2=up to but not including second row)", Hidden: true},
		{Name: "col-start", Desc: "column range start letter (inclusive; range is half-open [col-start, col-end))", Hidden: true},
		{Name: "col-end", Desc: "column range end letter (exclusive; range is half-open [col-start, col-end))", Hidden: true},
		{Name: "col-width", Type: "int", Desc: "column width in px", Hidden: true},
		{Name: "header-row", Type: "bool", Desc: "set first row as header", Hidden: true},
		{Name: "header-column", Type: "bool", Desc: "set first column as header", Hidden: true},
		{Name: "background-color", Desc: "cell background color (named or rgb/rgba)", Hidden: true},
		{Name: "vertical-align", Desc: "cell vertical alignment: top|middle|bottom", Hidden: true, Enum: []string{"top", "middle", "bottom"}},
	}
}

// ── A1 notation parsing ──

// parseColLetter converts a column letter (A=0, B=1, ..., Z=25, AA=26) to 0-based index.
// Special values: "0" = 0 (before-first), "-1" = -1 (append).
func parseColLetter(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty column")
	}
	if s == "0" {
		return 0, nil
	}
	if s == "-1" {
		return -1, nil
	}
	s = strings.ToUpper(s)
	col := 0
	for _, c := range s {
		if c < 'A' || c > 'Z' {
			return 0, fmt.Errorf("invalid column letter %q", s)
		}
		col = col*26 + int(c-'A'+1)
	}
	return col - 1, nil // convert to 0-based
}

// parseA1Cell parses "B3" → (row=2, col=1). Row is 1-based in input, 0-based in output.
func parseA1Cell(s string) (row, col int, err error) {
	if s == "" {
		return 0, 0, fmt.Errorf("empty cell reference")
	}
	s = strings.ToUpper(s)
	// Find boundary between letters and digits
	i := 0
	for i < len(s) && unicode.IsLetter(rune(s[i])) {
		i++
	}
	if i == 0 || i == len(s) {
		return 0, 0, fmt.Errorf("invalid cell %q: expected format like A1, B3", s)
	}
	colPart := s[:i]
	rowPart := s[i:]
	col, err = parseColLetter(colPart)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid cell %q: %w", s, err)
	}
	rowNum, err := strconv.Atoi(rowPart)
	if err != nil || rowNum < 1 {
		return 0, 0, fmt.Errorf("invalid cell %q: row must be >= 1", s)
	}
	return rowNum - 1, col, nil // convert to 0-based
}

// parseA1Range parses "A1:C3" (inclusive) → half-open (rowStart, rowEnd, colStart, colEnd).
func parseA1Range(s string) (rowStart, rowEnd, colStart, colEnd int, err error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, 0, 0, 0, fmt.Errorf("invalid range %q: expected format like A1:C3", s)
	}
	r1, c1, err := parseA1Cell(parts[0])
	if err != nil {
		return 0, 0, 0, 0, err
	}
	r2, c2, err := parseA1Cell(parts[1])
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if r2 < r1 || c2 < c1 {
		return 0, 0, 0, 0, fmt.Errorf("invalid range %q: start must be before end", s)
	}
	// Convert inclusive → half-open
	return r1, r2 + 1, c1, c2 + 1, nil
}

// ── Validation helpers ──

// backgroundColorPattern accepts the formats docx_engine recognizes: named colors
// (e.g. `red`, `light-blue`), `rgb(r,g,b)`, `rgba(r,g,b,a)`, and `#RRGGBB` / `#RGB`.
// This is a shape check only — specific color names / component ranges are validated
// downstream in docx_engine. Keep the pattern permissive enough that new named
// colors added server-side don't require a CLI re-release.
var backgroundColorPattern = regexp.MustCompile(`^(?:#[0-9a-fA-F]{3,8}|rgba?\([^)]*\)|[a-zA-Z][a-zA-Z0-9_-]{0,31})$`)

func isValidBackgroundColor(s string) bool {
	return backgroundColorPattern.MatchString(strings.TrimSpace(s))
}

// validateStyleFlags is shared by the four cell-targeting modes of
// table_update_property. Enforces: at least one style flag is set, and each set
// flag has a plausible value. Returns a FlagError with an example when invalid.
func validateStyleFlags(runtime *common.RuntimeContext, mode string) error {
	bg := runtime.Str("background-color")
	valign := runtime.Str("vertical-align")
	if bg == "" && valign == "" {
		return common.FlagErrorf("%s update requires --background-color or --vertical-align (example: --background-color light-blue)", mode)
	}
	if bg != "" && !isValidBackgroundColor(bg) {
		return common.FlagErrorf("--background-color %q is not a recognized format. Use a named color (light-blue, red, ...), rgb(r,g,b), rgba(r,g,b,a), or #RRGGBB.", bg)
	}
	// --vertical-align is constrained to the enum {top, middle, bottom} at the flag
	// definition layer (see tableUpdateFlags), so no extra check here.
	return nil
}

// ── Validation ──

func validateTableUpdate(_ context.Context, runtime *common.RuntimeContext) error {
	cmd := runtime.Str("command")
	if !validTableCommands[cmd] {
		return common.FlagErrorf("invalid table command %q", cmd)
	}

	if runtime.Str("table-block-id") == "" {
		return common.FlagErrorf("--table-block-id is required for %s", cmd)
	}

	switch cmd {
	case "table_insert_rows":
		if runtime.Int("row-index") < -1 {
			return common.FlagErrorf("--row-index must be >= -1")
		}
	case "table_insert_cols":
		col := runtime.Str("col")
		if col == "" {
			return common.FlagErrorf("--col is required for table_insert_cols")
		}
		if _, err := parseColLetter(col); err != nil {
			return common.FlagErrorf("--col: %v", err)
		}
	case "table_delete_rows":
		if runtime.Int("row-start") < 0 {
			return common.FlagErrorf("--row-start must be >= 0")
		}
		if runtime.Int("row-end") <= runtime.Int("row-start") {
			return common.FlagErrorf("--row-end must be > --row-start")
		}
	case "table_delete_cols":
		colStart := runtime.Str("col-start")
		colEnd := runtime.Str("col-end")
		if colStart == "" || colEnd == "" {
			return common.FlagErrorf("--col-start and --col-end are required for table_delete_cols")
		}
		s, err := parseColLetter(colStart)
		if err != nil {
			return common.FlagErrorf("--col-start: %v", err)
		}
		e, err := parseColLetter(colEnd)
		if err != nil {
			return common.FlagErrorf("--col-end: %v", err)
		}
		if e <= s {
			return common.FlagErrorf("--col-end must be > --col-start (ranges are half-open; got --col-start %s --col-end %s)", colStart, colEnd)
		}
	case "table_merge_cells":
		rangeStr := runtime.Str("range")
		if rangeStr == "" {
			return common.FlagErrorf("--range is required for table_merge_cells")
		}
		if _, _, _, _, err := parseA1Range(rangeStr); err != nil {
			return common.FlagErrorf("--range: %v", err)
		}
	case "table_unmerge_cells":
		cellStr := runtime.Str("cell")
		if cellStr == "" {
			return common.FlagErrorf("--cell is required for table_unmerge_cells")
		}
		if _, _, err := parseA1Cell(cellStr); err != nil {
			return common.FlagErrorf("--cell: %v", err)
		}
	case "table_update_property":
		// Four mutually exclusive cell-styling modes (any may coexist with orthogonal
		// table-level props: --col/--col-width/--header-*):
		//   cellMode   : --cell B3
		//   rangeMode  : --range A1:C3
		//   rowMode    : --row-start + --row-end (1-based, half-open)
		//   colMode    : --col-start + --col-end (letters, half-open [start, end))
		cellStr := runtime.Str("cell")
		rangeStr := runtime.Str("range")
		rowStart := runtime.Int("row-start")
		rowEnd := runtime.Int("row-end")
		colStart := runtime.Str("col-start")
		colEnd := runtime.Str("col-end")

		cellMode := cellStr != ""
		rangeMode := rangeStr != ""
		rowMode := rowStart != 0 || rowEnd != 0
		colMode := colStart != "" || colEnd != ""

		modes := 0
		for _, m := range []bool{cellMode, rangeMode, rowMode, colMode} {
			if m {
				modes++
			}
		}
		if modes > 1 {
			return common.FlagErrorf("--cell, --range, --row-start/--row-end, --col-start/--col-end are mutually exclusive for table_update_property. Pick one targeting mode — example: --cell B3, or --range A1:C3, or --row-start 1 --row-end 3, or --col-start A --col-end D.")
		}

		switch {
		case cellMode:
			if _, _, err := parseA1Cell(cellStr); err != nil {
				return common.FlagErrorf("--cell: %v", err)
			}
			if err := validateStyleFlags(runtime, "cell-level"); err != nil {
				return err
			}
		case rangeMode:
			if _, _, _, _, err := parseA1Range(rangeStr); err != nil {
				return common.FlagErrorf("--range: %v", err)
			}
			if err := validateStyleFlags(runtime, "range"); err != nil {
				return err
			}
		case rowMode:
			if rowStart <= 0 || rowEnd <= 0 {
				return common.FlagErrorf("--row-start and --row-end are both required (1-based half-open; e.g. --row-start 1 --row-end 3 selects rows 1 and 2)")
			}
			if rowEnd <= rowStart {
				return common.FlagErrorf("--row-end must be > --row-start (ranges are half-open; got --row-start %d --row-end %d)", rowStart, rowEnd)
			}
			if err := validateStyleFlags(runtime, "row"); err != nil {
				return err
			}
		case colMode:
			if colStart == "" || colEnd == "" {
				return common.FlagErrorf("--col-start and --col-end are both required for column range (half-open [start, end); e.g. --col-start A --col-end D selects columns A, B, C)")
			}
			s, err := parseColLetter(colStart)
			if err != nil {
				return common.FlagErrorf("--col-start: %v", err)
			}
			e, err := parseColLetter(colEnd)
			if err != nil {
				return common.FlagErrorf("--col-end: %v", err)
			}
			if e <= s {
				return common.FlagErrorf("--col-end must be > --col-start (ranges are half-open; got --col-start %s --col-end %s)", colStart, colEnd)
			}
			if err := validateStyleFlags(runtime, "column"); err != nil {
				return err
			}
		default:
			// Table-level mode: --col-width / --header-row / --header-column
			colWidth := runtime.Int("col-width")
			hasHeaderRow := runtime.Cmd.Flags().Changed("header-row")
			hasHeaderCol := runtime.Cmd.Flags().Changed("header-column")
			if colWidth == 0 && !hasHeaderRow && !hasHeaderCol {
				return common.FlagErrorf("table_update_property requires at least one property when no cell targeting is set. Use --col + --col-width, --header-row, --header-column, or one of the cell targeting modes (--cell, --range, --row-start/--row-end, --col-start/--col-end) with --background-color or --vertical-align.")
			}
			if colWidth != 0 && runtime.Str("col") == "" {
				return common.FlagErrorf("--col is required when --col-width is set (e.g. --col B --col-width 240)")
			}
			if runtime.Str("col") != "" {
				if _, err := parseColLetter(runtime.Str("col")); err != nil {
					return common.FlagErrorf("--col: %v", err)
				}
			}
		}
	}
	return nil
}

// ── Request body construction ──

func buildTableRequestBody(runtime *common.RuntimeContext, cmd string) map[string]interface{} {
	return buildTableSingleBody(runtime, cmd)
}

// buildTableSingleBody packs the table-op request.
//
// The OpenAPI gateway maps top-level JSON keys 1:1 onto OpenDocsAIUpdateDocumentRequest's
// typed Thrift fields (command, format, revision_id, block_id, …). Table-specific
// parameters without dedicated Thrift fields travel as a JSON-encoded string under
// "extra_param" (field 10, *string), which ai_edit's handler decodes back into a
// struct. Keep the "extra_param" key names snake_case and in sync with the
// updateExtraParam struct in ai_edit/biz/handler/open_docs_ai.go.
//
// The table target block id is sent as the top-level "block_id" — same field used by
// block_* commands. ai_edit validates it in buildOpenDocsAIUpdateInput for table_*
// commands. The --table-block-id CLI flag name is kept for backwards compatibility.
func buildTableSingleBody(runtime *common.RuntimeContext, cmd string) map[string]interface{} {
	extra := map[string]interface{}{}

	// Everything cli produces here is a pure passthrough of what the user typed ——
	// no A1 splitting, no case-fold, no letter→index math. The SDK owns all of those
	// transforms (pkg/util.ParseA1Cell / ParseA1Range / ParseColLetter); cli forwarding
	// the same conversion would risk doubling it. "cell" and "range" travel as raw
	// A1 strings; ai_edit uses the SDK helpers to expand them before hitting GMFCommand.
	switch cmd {
	case "table_insert_rows":
		// Only include row_index when the caller explicitly set it. Omitting the key
		// lets the SDK's *int tri-state default to -1 (append at end), which matches
		// the --row-index help text ("0-indexed, -1=end"). The residual limitation is
		// that --row-index=0 (insert at the very top) is indistinguishable from
		// omitting the flag — acceptable, since the documented sentinel for "no row
		// specified" is -1 and an empty table makes top-vs-end equivalent anyway.
		if v := runtime.Int("row-index"); v != 0 {
			extra["row_index"] = v
		}
	case "table_insert_cols":
		extra["column_index"] = runtime.Str("col")
	case "table_delete_rows":
		// row_start / row_end are 1-based A1-style indices; 0 is never a valid
		// value, so treat it as "unset" and let ai_edit's validator reject the
		// request rather than silently forwarding row_start_index=0 downstream.
		if v := runtime.Int("row-start"); v != 0 {
			extra["row_start_index"] = v
		}
		if v := runtime.Int("row-end"); v != 0 {
			extra["row_end_index"] = v
		}
	case "table_delete_cols":
		extra["column_start_index"] = runtime.Str("col-start")
		extra["column_end_index"] = runtime.Str("col-end")
	case "table_merge_cells":
		extra["range"] = runtime.Str("range")
	case "table_unmerge_cells":
		extra["cell"] = runtime.Str("cell")
	case "table_update_property":
		// Cell-styling modes (cellMode / rangeMode / rowMode / colMode). Validation
		// already enforced mutual exclusion, so the first matching branch wins.
		if v := runtime.Str("cell"); v != "" {
			extra["cell"] = v
		} else if v := runtime.Str("range"); v != "" {
			extra["range"] = v
		} else if rs, re := runtime.Int("row-start"), runtime.Int("row-end"); rs != 0 && re != 0 {
			extra["row_start_index"] = rs
			extra["row_end_index"] = re
		} else if cs, ce := runtime.Str("col-start"), runtime.Str("col-end"); cs != "" && ce != "" {
			extra["column_start_index"] = cs
			extra["column_end_index"] = ce
		}
		// Cell-styling props layer onto whichever targeting mode is active.
		if bg := runtime.Str("background-color"); bg != "" {
			extra["background_color"] = bg
		}
		if va := runtime.Str("vertical-align"); va != "" {
			extra["vertical_align"] = va
		}
		// Table-level props remain orthogonal.
		if v := runtime.Str("col"); v != "" {
			extra["column_index"] = v
		}
		if v := runtime.Int("col-width"); v != 0 {
			extra["column_width"] = v
		}
		if runtime.Cmd.Flags().Changed("header-row") {
			extra["header_row"] = runtime.Bool("header-row")
		}
		if runtime.Cmd.Flags().Changed("header-column") {
			extra["header_column"] = runtime.Bool("header-column")
		}
	}

	// json.Marshal over a map[string]interface{} is deterministic only across Go versions
	// that sort map keys during marshal (all modern releases). We don't care about the
	// exact byte order; ai_edit decodes by key name, not position.
	extraJSON, _ := json.Marshal(extra)

	body := map[string]interface{}{
		"command":     cmd,
		"format":      "xml",
		"extra_param": string(extraJSON),
	}
	if v := runtime.Str("table-block-id"); v != "" {
		body["block_id"] = v
	}
	if v := runtime.Int("revision-id"); v != 0 {
		body["revision_id"] = v
	}
	return body
}

// ── Execution ──

func executeTableUpdate(_ context.Context, runtime *common.RuntimeContext) error {
	ref, err := parseDocumentRef(runtime.Str("doc"))
	if err != nil {
		return err
	}

	cmd := runtime.Str("command")
	body := buildTableRequestBody(runtime, cmd)

	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", ref.Token)
	data, err := doDocAPI(runtime, "PUT", apiPath, body)
	if err != nil {
		return err
	}
	runtime.OutRaw(data, nil)
	return nil
}

func dryRunTableUpdate(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
	ref, err := parseDocumentRef(runtime.Str("doc"))
	if err != nil {
		return common.NewDryRunAPI().Desc(fmt.Sprintf("error: %v", err))
	}
	cmd := runtime.Str("command")
	body := buildTableRequestBody(runtime, cmd)
	apiPath := fmt.Sprintf("/open-apis/docs_ai/v1/documents/%s", ref.Token)
	return common.NewDryRunAPI().
		PUT(apiPath).
		Desc(fmt.Sprintf("OpenAPI: table operation %s", cmd)).
		Body(body).
		Set("document_id", ref.Token)
}

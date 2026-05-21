// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// TestBatchOp_BodyMatchesStandalone is the core contract: for every batchable
// shortcut, the MCP body produced inside +batch-update must be byte-for-byte
// identical to the body the same shortcut produces when invoked standalone
// (both observed via --dry-run, comparing tool_name + decoded input). This is
// what guarantees "a sub-op behaves exactly like the standalone command", and
// it is the regression guard for the whole flag→body translator reuse.
//
// Each case provides the standalone CLI args and the equivalent sub-op input
// object (same CLI flag names, minus the spreadsheet locator which the batch
// supplies at the top level).
func TestBatchOp_BodyMatchesStandalone(t *testing.T) {
	t.Parallel()

	cases := []struct {
		shortcut string
		sc       common.Shortcut
		// standalone args (excluding --url, which every case shares)
		args []string
		// sub-op input object as JSON (CLI flag names; no excel_id/url)
		subInput string
	}{
		{
			shortcut: "+cells-set",
			sc:       CellsSet,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:B1", "--cells", `[[{"value":"x"},{"value":"y"}]]`},
			subInput: `{"sheet-id":"sh1","range":"A1:B1","cells":[[{"value":"x"},{"value":"y"}]]}`,
		},
		{
			shortcut: "+cells-clear",
			sc:       CellsClear,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:C3", "--scope", "formats"},
			subInput: `{"sheet-id":"sh1","range":"A1:C3","scope":"formats"}`,
		},
		{
			shortcut: "+cells-replace",
			sc:       CellsReplace,
			args:     []string{"--sheet-id", "sh1", "--find", "foo", "--replacement", "bar", "--match-case"},
			subInput: `{"sheet-id":"sh1","find":"foo","replacement":"bar","match-case":true}`,
		},
		{
			shortcut: "+csv-put",
			sc:       CsvPut,
			args:     []string{"--sheet-id", "sh1", "--csv", "a,b\n1,2", "--start-cell", "B2"},
			subInput: `{"sheet-id":"sh1","csv":"a,b\n1,2","start-cell":"B2"}`,
		},
		{
			shortcut: "+cells-merge",
			sc:       CellsMerge,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:C1", "--merge-type", "rows"},
			subInput: `{"sheet-id":"sh1","range":"A1:C1","merge-type":"rows"}`,
		},
		{
			shortcut: "+cells-unmerge",
			sc:       CellsUnmerge,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:C1"},
			subInput: `{"sheet-id":"sh1","range":"A1:C1"}`,
		},
		{
			shortcut: "+dim-insert",
			sc:       DimInsert,
			args:     []string{"--sheet-id", "sh1", "--dimension", "row", "--start", "10", "--end", "12", "--inherit-style", "before"},
			subInput: `{"sheet-id":"sh1","dimension":"row","start":10,"end":12,"inherit-style":"before"}`,
		},
		{
			shortcut: "+dim-delete",
			sc:       DimDelete,
			args:     []string{"--sheet-id", "sh1", "--dimension", "column", "--start", "2", "--end", "4"},
			subInput: `{"sheet-id":"sh1","dimension":"column","start":2,"end":4}`,
		},
		{
			shortcut: "+dim-hide",
			sc:       DimHide,
			args:     []string{"--sheet-id", "sh1", "--dimension", "row", "--start", "1", "--end", "3"},
			subInput: `{"sheet-id":"sh1","dimension":"row","start":1,"end":3}`,
		},
		{
			shortcut: "+dim-freeze",
			sc:       DimFreeze,
			args:     []string{"--sheet-id", "sh1", "--dimension", "row", "--count", "2"},
			subInput: `{"sheet-id":"sh1","dimension":"row","count":2}`,
		},
		{
			shortcut: "+dim-group",
			sc:       DimGroup,
			args:     []string{"--sheet-id", "sh1", "--dimension", "row", "--start", "1", "--end", "5", "--group-state", "fold"},
			subInput: `{"sheet-id":"sh1","dimension":"row","start":1,"end":5,"group-state":"fold"}`,
		},
		{
			shortcut: "+rows-resize",
			sc:       RowsResize,
			args:     []string{"--sheet-id", "sh1", "--start", "0", "--end", "0", "--type", "pixel", "--size", "30"},
			subInput: `{"sheet-id":"sh1","start":0,"end":0,"type":"pixel","size":30}`,
		},
		{
			shortcut: "+cols-resize",
			sc:       ColsResize,
			args:     []string{"--sheet-id", "sh1", "--start", "1", "--end", "3", "--type", "standard"},
			subInput: `{"sheet-id":"sh1","start":1,"end":3,"type":"standard"}`,
		},
		{
			shortcut: "+range-move",
			sc:       RangeMove,
			args:     []string{"--sheet-id", "sh1", "--source-range", "A1:C5", "--target-range", "D1"},
			subInput: `{"sheet-id":"sh1","source-range":"A1:C5","target-range":"D1"}`,
		},
		{
			shortcut: "+range-copy",
			sc:       RangeCopy,
			args:     []string{"--sheet-id", "sh1", "--source-range", "A1:B2", "--target-range", "A10", "--paste-type", "values"},
			subInput: `{"sheet-id":"sh1","source-range":"A1:B2","target-range":"A10","paste-type":"values"}`,
		},
		{
			shortcut: "+range-fill",
			sc:       RangeFill,
			args:     []string{"--sheet-id", "sh1", "--source-range", "A1:A2", "--target-range", "A1:A10", "--series-type", "linear"},
			subInput: `{"sheet-id":"sh1","source-range":"A1:A2","target-range":"A1:A10","series-type":"linear"}`,
		},
		{
			shortcut: "+range-sort",
			sc:       RangeSort,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:D10", "--sort-keys", `[{"col":"B","order":"asc"}]`, "--has-header"},
			subInput: `{"sheet-id":"sh1","range":"A1:D10","sort-keys":[{"col":"B","order":"asc"}],"has-header":true}`,
		},
		{
			shortcut: "+sheet-create",
			sc:       SheetCreate,
			args:     []string{"--title", "New", "--index", "2"},
			subInput: `{"title":"New","index":2}`,
		},
		{
			shortcut: "+sheet-delete",
			sc:       SheetDelete,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+sheet-rename",
			sc:       SheetRename,
			args:     []string{"--sheet-id", "sh1", "--title", "Renamed"},
			subInput: `{"sheet-id":"sh1","title":"Renamed"}`,
		},
		{
			shortcut: "+sheet-copy",
			sc:       SheetCopy,
			args:     []string{"--sheet-id", "sh1", "--title", "Copy"},
			subInput: `{"sheet-id":"sh1","title":"Copy"}`,
		},
		{
			shortcut: "+sheet-hide",
			sc:       SheetHide,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+sheet-unhide",
			sc:       SheetUnhide,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+sheet-set-tab-color",
			sc:       SheetSetTabColor,
			args:     []string{"--sheet-id", "sh1", "--color", "#FF0000"},
			subInput: `{"sheet-id":"sh1","color":"#FF0000"}`,
		},
		{
			shortcut: "+dropdown-set",
			sc:       DropdownSet,
			args:     []string{"--sheet-id", "sh1", "--range", "A2:A4", "--options", `["x","y"]`, "--multiple"},
			subInput: `{"sheet-id":"sh1","range":"A2:A4","options":["x","y"],"multiple":true}`,
		},
		{
			shortcut: "+chart-create",
			sc:       ChartCreate,
			args:     []string{"--sheet-id", "sh1", "--properties", `{"position":{"start":"A1"}}`},
			subInput: `{"sheet-id":"sh1","properties":{"position":{"start":"A1"}}}`,
		},
		{
			shortcut: "+chart-update",
			sc:       ChartUpdate,
			args:     []string{"--sheet-id", "sh1", "--chart-id", "c1", "--properties", `{"title":"T"}`},
			subInput: `{"sheet-id":"sh1","chart-id":"c1","properties":{"title":"T"}}`,
		},
		{
			shortcut: "+chart-delete",
			sc:       ChartDelete,
			args:     []string{"--sheet-id", "sh1", "--chart-id", "c1"},
			subInput: `{"sheet-id":"sh1","chart-id":"c1"}`,
		},
		{
			shortcut: "+pivot-create",
			sc:       PivotCreate,
			args:     []string{"--sheet-id", "sh1", "--properties", `{"rows":[]}`, "--source", "Sheet1!A1:D100"},
			subInput: `{"sheet-id":"sh1","properties":{"rows":[]},"source":"Sheet1!A1:D100"}`,
		},
		{
			shortcut: "+cond-format-create",
			sc:       CondFormatCreate,
			args:     []string{"--sheet-id", "sh1", "--properties", `{"style":{}}`, "--rule-type", "duplicate", "--ranges", `["A1:A100"]`},
			subInput: `{"sheet-id":"sh1","properties":{"style":{}},"rule-type":"duplicate","ranges":["A1:A100"]}`,
		},
		{
			shortcut: "+filter-create",
			sc:       FilterCreate,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:F1000", "--properties", `{"rules":[]}`},
			subInput: `{"sheet-id":"sh1","range":"A1:F1000","properties":{"rules":[]}}`,
		},
		{
			shortcut: "+filter-update",
			sc:       FilterUpdate,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:F1000", "--properties", `{"rules":[]}`},
			subInput: `{"sheet-id":"sh1","range":"A1:F1000","properties":{"rules":[]}}`,
		},
		{
			shortcut: "+filter-delete",
			sc:       FilterDelete,
			args:     []string{"--sheet-id", "sh1"},
			subInput: `{"sheet-id":"sh1"}`,
		},
		{
			shortcut: "+filter-view-create",
			sc:       FilterViewCreate,
			args:     []string{"--sheet-id", "sh1", "--range", "A1:Z100", "--view-name", "v1", "--properties", `{"rules":[]}`},
			subInput: `{"sheet-id":"sh1","range":"A1:Z100","view-name":"v1","properties":{"rules":[]}}`,
		},
		{
			shortcut: "+sparkline-create",
			sc:       SparklineCreate,
			args:     []string{"--sheet-id", "sh1", "--properties", `{"type":"line","data_range":"A2:F2","target_range":"G2"}`},
			subInput: `{"sheet-id":"sh1","properties":{"type":"line","data_range":"A2:F2","target_range":"G2"}}`,
		},
		{
			shortcut: "+sparkline-delete",
			sc:       SparklineDelete,
			args:     []string{"--sheet-id", "sh1", "--group-id", "g1"},
			subInput: `{"sheet-id":"sh1","group-id":"g1"}`,
		},
		{
			shortcut: "+float-image-create",
			sc:       FloatImageCreate,
			args:     []string{"--sheet-id", "sh1", "--image-name", "logo.png", "--image-token", "tok", "--position-row", "0", "--position-col", "A", "--size-width", "100", "--size-height", "50"},
			subInput: `{"sheet-id":"sh1","image-name":"logo.png","image-token":"tok","position-row":0,"position-col":"A","size-width":100,"size-height":50}`,
		},
		{
			shortcut: "+float-image-delete",
			sc:       FloatImageDelete,
			args:     []string{"--sheet-id", "sh1", "--float-image-id", "fi1"},
			subInput: `{"sheet-id":"sh1","float-image-id":"fi1"}`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.shortcut, func(t *testing.T) {
			t.Parallel()

			mapping, ok := batchOpDispatch[tc.shortcut]
			if !ok {
				t.Fatalf("%s not in batchOpDispatch", tc.shortcut)
			}

			// Standalone body via the shortcut's own dry-run.
			standaloneBody := decodeToolInput(t, parseDryRunBody(t, tc.sc, append([]string{"--url", testURL}, tc.args...)), mapping.mcpToolName)

			// Batch body via the +batch-update translator.
			var subInput map[string]interface{}
			if err := json.Unmarshal([]byte(tc.subInput), &subInput); err != nil {
				t.Fatalf("bad subInput JSON: %v", err)
			}
			fv := newMapFlagViewForCommand(tc.shortcut, subInput)
			sid := subInput["sheet-id"]
			sname := subInput["sheet-name"]
			sidStr, _ := sid.(string)
			snameStr, _ := sname.(string)
			batchBody, err := mapping.translate(fv, testToken, sidStr, snameStr)
			if err != nil {
				t.Fatalf("batch translate failed: %v", err)
			}

			// Round-trip the batch body through JSON so number types match the
			// standalone path (which is decoded from a JSON string).
			batchBody = jsonRoundTrip(t, batchBody)

			if !reflect.DeepEqual(standaloneBody, batchBody) {
				t.Errorf("%s: batch body != standalone body\n standalone=%#v\n batch     =%#v", tc.shortcut, standaloneBody, batchBody)
			}
		})
	}
}

func jsonRoundTrip(t *testing.T, m map[string]interface{}) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return out
}

// TestBatchOp_DispatchCoversReportedBugs is a focused guard for the two
// originally reported failures: +range-copy and +rows-resize sub-ops must
// translate to the correct MCP body (not a near-passthrough that drops
// required fields).
func TestBatchOp_DispatchCoversReportedBugs(t *testing.T) {
	t.Parallel()

	// +range-copy → transform_range with range / destination_range (not the
	// raw source_range / target_range that used to leak through).
	body := parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+range-copy","input":{"sheet-id":"sh1","source-range":"A1:B2","target-range":"A10","paste-type":"all"}}]`,
		"--yes",
	})
	ops := decodeToolInput(t, body, "batch_update")["operations"].([]interface{})
	copyIn := ops[0].(map[string]interface{})["input"].(map[string]interface{})
	if copyIn["range"] != "A1:B2" || copyIn["destination_range"] != "A10" {
		t.Errorf("+range-copy sub-op body wrong: %#v", copyIn)
	}
	if copyIn["operation"] != "copy" {
		t.Errorf("+range-copy operation = %v, want copy", copyIn["operation"])
	}

	// +rows-resize → resize_range with range + resize_height (not raw start/end).
	body = parseDryRunBody(t, BatchUpdate, []string{
		"--url", testURL,
		"--operations", `[{"shortcut":"+rows-resize","input":{"sheet-id":"sh1","start":22,"end":22,"type":"pixel","size":40}}]`,
		"--yes",
	})
	ops = decodeToolInput(t, body, "batch_update")["operations"].([]interface{})
	resizeIn := ops[0].(map[string]interface{})["input"].(map[string]interface{})
	if resizeIn["range"] != "23:23" {
		t.Errorf("+rows-resize single-row range = %v, want 23:23", resizeIn["range"])
	}
	rh, _ := resizeIn["resize_height"].(map[string]interface{})
	if rh == nil || rh["type"] != "pixel" {
		t.Errorf("+rows-resize resize_height wrong: %#v", resizeIn)
	}
}

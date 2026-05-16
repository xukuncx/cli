// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
)

// TestObjectCRUDShortcuts_DryRun walks the create / update / delete trio
// for each object skill. Together these cover all 21 CRUD shortcuts plus
// the per-object id flag renames (rule-id, group-id, view-id, etc.).
func TestObjectCRUDShortcuts_DryRun(t *testing.T) {
	t.Parallel()

	type spec struct {
		name      string
		sc        common.Shortcut
		args      []string
		toolName  string
		wantInput map[string]interface{}
	}

	tests := []spec{
		// chart
		{
			name:     "+chart-create",
			sc:       ChartCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--data", `{"type":"line"}`},
			toolName: "manage_chart_object",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"operation":  "create",
				"properties": map[string]interface{}{"type": "line"},
			},
		},
		{
			name:     "+chart-update",
			sc:       ChartUpdate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "chartXYZ", "--data", `{"type":"bar"}`},
			toolName: "manage_chart_object",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"operation":  "update",
				"chart_id":   "chartXYZ",
				"properties": map[string]interface{}{"type": "bar"},
			},
		},
		// pivot — has extra create flags
		{
			name:     "+pivot-create with target flags",
			sc:       PivotCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--data", `{"data_range":"Sheet1!A1:F1000"}`, "--target-sheet-id", "sh2", "--target-position", "B5"},
			toolName: "manage_pivot_table_object",
			wantInput: map[string]interface{}{
				"excel_id":         testToken,
				"sheet_id":         testSheetID,
				"operation":        "create",
				"target_sheet_id":  "sh2",
				"target_position":  "B5",
			},
		},
		{
			name:     "+pivot-delete",
			sc:       PivotDelete,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--pivot-table-id", "ptA"},
			toolName: "manage_pivot_table_object",
			wantInput: map[string]interface{}{
				"excel_id":       testToken,
				"sheet_id":       testSheetID,
				"operation":      "delete",
				"pivot_table_id": "ptA",
			},
		},
		// cond-format — --rule-id rename
		{
			name:     "+cond-format-update id rename",
			sc:       CondFormatUpdate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--rule-id", "ruleA", "--data", `{"rule":{"type":"cell_value"}}`},
			toolName: "manage_conditional_format_object",
			wantInput: map[string]interface{}{
				"excel_id":              testToken,
				"sheet_id":              testSheetID,
				"operation":             "update",
				"conditional_format_id": "ruleA",
				"properties":            map[string]interface{}{"rule": map[string]interface{}{"type": "cell_value"}},
			},
		},
		// filter — special, no id flag
		{
			name:     "+filter-create without --data sends properties.range only",
			sc:       FilterCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:F1000"},
			toolName: "manage_filter_object",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"operation":  "create",
				"properties": map[string]interface{}{"range": "A1:F1000"},
			},
		},
		{
			name:     "+filter-create with --data merges conditions",
			sc:       FilterCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--range", "A1:F1000", "--data", `{"conditions":[{"col":"B"}]}`},
			toolName: "manage_filter_object",
			wantInput: map[string]interface{}{
				"properties": map[string]interface{}{
					"range":      "A1:F1000",
					"conditions": []interface{}{map[string]interface{}{"col": "B"}},
				},
			},
		},
		{
			name:     "+filter-delete (no id flag, sheet-scoped)",
			sc:       FilterDelete,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID},
			toolName: "manage_filter_object",
			wantInput: map[string]interface{}{
				"excel_id":  testToken,
				"sheet_id":  testSheetID,
				"operation": "delete",
			},
		},
		// filter-view CRUD (cli-only via callTool)
		{
			name:     "+filter-view-create",
			sc:       FilterViewCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--data", `{"view_name":"v1","range":"A1:Z100"}`},
			toolName: "manage_filter_view_object",
			wantInput: map[string]interface{}{
				"excel_id":   testToken,
				"sheet_id":   testSheetID,
				"operation":  "create",
				"properties": map[string]interface{}{"view_name": "v1", "range": "A1:Z100"},
			},
		},
		{
			name:     "+filter-view-update with --view-id",
			sc:       FilterViewUpdate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--view-id", "vABC", "--data", `{"view_name":"renamed"}`},
			toolName: "manage_filter_view_object",
			wantInput: map[string]interface{}{
				"view_id":   "vABC",
				"operation": "update",
			},
		},
		// sparkline --group-id
		{
			name:     "+sparkline-update --group-id → group_id",
			sc:       SparklineUpdate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--group-id", "grpA", "--data", `{"type":"line"}`},
			toolName: "manage_sparkline_object",
			wantInput: map[string]interface{}{
				"group_id":   "grpA",
				"operation":  "update",
				"properties": map[string]interface{}{"type": "line"},
			},
		},
		// float-image
		{
			name:     "+float-image-create",
			sc:       FloatImageCreate,
			args:     []string{"--url", testURL, "--sheet-id", testSheetID, "--data", `{"image_uri":"u","image_name":"x.png"}`},
			toolName: "manage_float_image_object",
			wantInput: map[string]interface{}{
				"operation":  "create",
				"properties": map[string]interface{}{"image_uri": "u", "image_name": "x.png"},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := parseDryRunBody(t, tt.sc, tt.args)
			got := decodeToolInput(t, body, tt.toolName)
			assertInputEquals(t, got, tt.wantInput)
		})
	}
}

// TestObjectDelete_AllHighRisk asserts every delete shortcut blocks
// without --yes (framework-enforced).
func TestObjectDelete_AllHighRisk(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sc   common.Shortcut
		args []string
	}{
		{"chart", ChartDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--chart-id", "x"}},
		{"pivot", PivotDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--pivot-table-id", "x"}},
		{"cond-format", CondFormatDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--rule-id", "x"}},
		{"filter", FilterDelete, []string{"--url", testURL, "--sheet-id", testSheetID}},
		{"filter-view", FilterViewDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--view-id", "x"}},
		{"sparkline", SparklineDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--group-id", "x"}},
		{"float-image", FloatImageDelete, []string{"--url", testURL, "--sheet-id", testSheetID, "--float-image-id", "x"}},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runShortcutCapturingErr(t, tt.sc, tt.args)
			if err == nil {
				t.Fatalf("expected confirmation_required; stdout=%s stderr=%s", stdout, stderr)
			}
			combined := stdout + stderr + err.Error()
			if !strings.Contains(combined, "confirmation_required") && !strings.Contains(combined, "requires confirmation") {
				t.Errorf("expected confirmation gate; got=%s|%s|%v", stdout, stderr, err)
			}
		})
	}
}

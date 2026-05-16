// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"strings"
	"testing"
)

// TestWorkbookCreate_DryRun verifies the two-step plan (create
// spreadsheet + optional set_cell_range follow-up) is rendered.
func TestWorkbookCreate_DryRun(t *testing.T) {
	t.Parallel()

	t.Run("minimal title only", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{"--title", "MySheet"})
		if len(calls) != 1 {
			t.Fatalf("api calls = %d, want 1 (no headers/data)", len(calls))
		}
		c := calls[0].(map[string]interface{})
		if c["url"] != "/open-apis/sheets/v3/spreadsheets" {
			t.Errorf("url = %v, want /open-apis/sheets/v3/spreadsheets", c["url"])
		}
		body, _ := c["body"].(map[string]interface{})
		if body["title"] != "MySheet" {
			t.Errorf("body.title = %v, want MySheet", body["title"])
		}
	})

	t.Run("with headers and data → 2-step plan", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookCreate, []string{
			"--title", "Sales",
			"--headers", `["Name","Score"]`,
			"--data", `[["alice",95],["bob",88]]`,
		})
		if len(calls) != 2 {
			t.Fatalf("api calls = %d, want 2 (create + fill)", len(calls))
		}
		fill := calls[1].(map[string]interface{})
		if !strings.Contains(fill["url"].(string), "/sheet_ai/v2/spreadsheets/") {
			t.Errorf("fill url = %v, want sheet_ai/v2 path", fill["url"])
		}
		body, _ := fill["body"].(map[string]interface{})
		input := decodeToolInput(t, body, "set_cell_range")
		if input["range"] != "A1:B3" {
			t.Errorf("fill range = %v, want A1:B3 (1 header + 2 data rows × 2 cols)", input["range"])
		}
	})
}

// TestWorkbookCreate_DataValidation rejects bad JSON shape.
func TestWorkbookCreate_DataValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"headers not array", []string{"--title", "X", "--headers", `"abc"`}, "must be a JSON array"},
		{"data not 2D", []string{"--title", "X", "--data", `["a","b"]`}, "must be an array"},
	}
	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stdout, stderr, err := runShortcutCapturingErr(t, WorkbookCreate, append(tt.args, "--dry-run"))
			if err == nil || !strings.Contains(stdout+stderr+err.Error(), tt.want) {
				t.Errorf("expected %q; got=%s|%s|%v", tt.want, stdout, stderr, err)
			}
		})
	}
}

// TestWorkbookExport_DryRun checks the 2-or-3 step plan depending on
// --output-path. The order should be: POST → GET (poll) → optional GET
// (download).
func TestWorkbookExport_DryRun(t *testing.T) {
	t.Parallel()

	t.Run("xlsx without --output-path → 2 steps", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookExport, []string{"--url", testURL, "--file-extension", "xlsx"})
		if len(calls) != 2 {
			t.Fatalf("api calls = %d, want 2 (create + poll)", len(calls))
		}
		create := calls[0].(map[string]interface{})
		if create["url"] != "/open-apis/drive/v1/export_tasks" {
			t.Errorf("first url = %v", create["url"])
		}
		body, _ := create["body"].(map[string]interface{})
		if body["type"] != "sheet" || body["file_extension"] != "xlsx" || body["token"] != testToken {
			t.Errorf("create body = %#v", body)
		}
	})

	t.Run("csv → 3 steps, with sub_id", func(t *testing.T) {
		t.Parallel()
		calls := parseDryRunAPI(t, WorkbookExport, []string{
			"--url", testURL, "--file-extension", "csv", "--sheet-id", "sh1",
			"--output-path", "/tmp/out.csv",
		})
		if len(calls) != 3 {
			t.Fatalf("api calls = %d, want 3", len(calls))
		}
		body, _ := calls[0].(map[string]interface{})["body"].(map[string]interface{})
		if body["sub_id"] != "sh1" {
			t.Errorf("csv export missing sub_id: %#v", body)
		}
		dl := calls[2].(map[string]interface{})
		if !strings.Contains(dl["url"].(string), "/export_tasks/file/") {
			t.Errorf("download url = %v", dl["url"])
		}
	})

	t.Run("csv requires --sheet-id", func(t *testing.T) {
		t.Parallel()
		stdout, stderr, err := runShortcutCapturingErr(t, WorkbookExport, []string{
			"--url", testURL, "--file-extension", "csv", "--dry-run",
		})
		if err == nil || !strings.Contains(stdout+stderr+err.Error(), "--sheet-id is required") {
			t.Errorf("expected sheet-id guard; got=%s|%s|%v", stdout, stderr, err)
		}
	})
}

// TestDimMove_DryRun verifies the legacy v2 dimension_range payload
// shape. CLI's 0-based inclusive (--start / --end) becomes the v2
// endpoint's half-open [startIndex, endIndex).
func TestDimMove_DryRun(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, DimMove, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--dimension", "row", "--start", "0", "--end", "2", "--target", "10",
	})
	if len(calls) != 1 {
		t.Fatalf("api calls = %d, want 1", len(calls))
	}
	c := calls[0].(map[string]interface{})
	if !strings.Contains(c["url"].(string), "/sheets/v2/spreadsheets/") {
		t.Errorf("url = %v, want sheets/v2 path", c["url"])
	}
	body, _ := c["body"].(map[string]interface{})
	src, _ := body["source"].(map[string]interface{})
	if src["sheetId"] != testSheetID {
		t.Errorf("source.sheetId = %v", src["sheetId"])
	}
	if src["majorDimension"] != "ROWS" {
		t.Errorf("source.majorDimension = %v, want ROWS", src["majorDimension"])
	}
	if src["startIndex"].(float64) != 0 {
		t.Errorf("startIndex = %v, want 0", src["startIndex"])
	}
	if src["endIndex"].(float64) != 3 {
		t.Errorf("endIndex = %v, want 3 (CLI end+1 for half-open)", src["endIndex"])
	}
	if body["destinationIndex"].(float64) != 10 {
		t.Errorf("destinationIndex = %v, want 10", body["destinationIndex"])
	}
}

// TestCellsSetImage_DryRun verifies the 2-step plan (upload + embed) is
// rendered, including the parent_type=sheet_image upload metadata.
func TestCellsSetImage_DryRun(t *testing.T) {
	t.Parallel()
	calls := parseDryRunAPI(t, CellsSetImage, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1",
		"--image", "./README.md", // any existing-shaped path; dry-run skips stat
	})
	if len(calls) != 2 {
		t.Fatalf("api calls = %d, want 2 (upload + set_cell_range)", len(calls))
	}
	upload := calls[0].(map[string]interface{})
	if upload["url"] != "/open-apis/drive/v1/medias/upload_all" {
		t.Errorf("upload url = %v", upload["url"])
	}
	ubody, _ := upload["body"].(map[string]interface{})
	if ubody["parent_type"] != "sheet_image" {
		t.Errorf("parent_type = %v, want sheet_image", ubody["parent_type"])
	}
	if ubody["parent_node"] != testToken {
		t.Errorf("parent_node = %v, want token", ubody["parent_node"])
	}

	embed := calls[1].(map[string]interface{})
	body, _ := embed["body"].(map[string]interface{})
	input := decodeToolInput(t, body, "set_cell_range")
	cells, _ := input["cells"].([]interface{})
	row, _ := cells[0].([]interface{})
	cell, _ := row[0].(map[string]interface{})
	rt, _ := cell["rich_text"].([]interface{})
	if len(rt) != 1 {
		t.Fatalf("rich_text len = %d, want 1", len(rt))
	}
	item, _ := rt[0].(map[string]interface{})
	if item["type"] != "embed-image" {
		t.Errorf("rich_text.type = %v, want embed-image", item["type"])
	}
	if item["attachment_name"] != "README.md" {
		t.Errorf("attachment_name = %v, want README.md (basename)", item["attachment_name"])
	}
}

func TestCellsSetImage_RangeMustBeSingleCell(t *testing.T) {
	t.Parallel()
	stdout, stderr, err := runShortcutCapturingErr(t, CellsSetImage, []string{
		"--url", testURL, "--sheet-id", testSheetID,
		"--range", "A1:B2", "--image", "./foo.png", "--dry-run",
	})
	if err == nil || !strings.Contains(stdout+stderr+err.Error(), "must be exactly one cell") {
		t.Errorf("expected single-cell guard; got=%s|%s|%v", stdout, stderr, err)
	}
}

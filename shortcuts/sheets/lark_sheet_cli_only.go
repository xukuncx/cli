// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// ─── cli-only shortcuts (legacy OAPI direct calls) ────────────────────
//
// Four shortcuts that don't fit the One-OpenAPI dispatcher because their
// backing capability isn't in the MCP tool catalog:
//
//   - +workbook-create   POST /open-apis/sheets/v3/spreadsheets
//                        + optional set_cell_range follow-up (headers / data)
//
//   - +workbook-export   POST /open-apis/drive/v1/export_tasks
//                        → poll /export_tasks/:ticket
//                        → optional GET /export_tasks/file/:file_token/download
//
//   - +dim-move          POST /open-apis/sheets/v2/spreadsheets/:token
//                                              /dimension_range
//
//   - +cells-set-image   POST /open-apis/drive/v1/medias/upload_all
//                        (parent_type=sheet_image) → callTool set_cell_range
//                        with rich_text embed-image
//
// These do NOT go through the One-OpenAPI; CLI talks directly to the
// classic Feishu open APIs via runtime.CallAPI / DoAPI.

// ─── +workbook-create ─────────────────────────────────────────────────

// WorkbookCreate creates a brand-new spreadsheet in the user's drive
// (optionally inside --folder-token) and can pre-fill the first row of
// headers and an initial data block.
var WorkbookCreate = common.Shortcut{
	Service:     "sheets",
	Command:     "+workbook-create",
	Description: "Create a new spreadsheet (optionally pre-filled with --headers and --data).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:create", "sheets:spreadsheet:write_only"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "title", Required: true, Desc: "spreadsheet title"},
		{Name: "folder-token", Desc: "destination folder token; omit to land at the drive root"},
		{Name: "headers", Input: []string{common.File, common.Stdin}, Desc: "header row JSON array, e.g. [\"列A\",\"列B\"]"},
		{Name: "data", Input: []string{common.File, common.Stdin}, Desc: "initial data JSON 2D array, e.g. [[\"alice\",95]]"},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("title")) == "" {
			return common.FlagErrorf("--title is required")
		}
		if runtime.Str("headers") != "" {
			v, err := parseJSONFlag(runtime, "headers")
			if err != nil {
				return err
			}
			if _, ok := v.([]interface{}); !ok {
				return common.FlagErrorf("--headers must be a JSON array")
			}
		}
		if runtime.Str("data") != "" {
			v, err := parseJSONFlag(runtime, "data")
			if err != nil {
				return err
			}
			rows, ok := v.([]interface{})
			if !ok {
				return common.FlagErrorf("--data must be a JSON 2D array")
			}
			for i, r := range rows {
				if _, ok := r.([]interface{}); !ok {
					return common.FlagErrorf("--data[%d] must be an array", i)
				}
			}
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		body := map[string]interface{}{"title": strings.TrimSpace(runtime.Str("title"))}
		if v := strings.TrimSpace(runtime.Str("folder-token")); v != "" {
			body["folder_token"] = v
		}
		dry := common.NewDryRunAPI().
			POST("/open-apis/sheets/v3/spreadsheets").
			Desc("create spreadsheet").
			Body(body)
		if runtime.Str("headers") != "" || runtime.Str("data") != "" {
			fill, _ := buildInitialFillInput(runtime)
			wireBody, _ := buildToolBody("set_cell_range", fill)
			dry.POST("/open-apis/sheet_ai/v2/spreadsheets/<new-token>/tools/invoke_write").
				Desc("fill headers + data via set_cell_range").
				Body(wireBody)
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		body := map[string]interface{}{"title": strings.TrimSpace(runtime.Str("title"))}
		if v := strings.TrimSpace(runtime.Str("folder-token")); v != "" {
			body["folder_token"] = v
		}
		data, err := runtime.CallAPI("POST", "/open-apis/sheets/v3/spreadsheets", nil, body)
		if err != nil {
			return err
		}
		ss := common.GetMap(data, "spreadsheet")
		token := common.GetString(ss, "spreadsheet_token")
		if token == "" {
			token = common.GetString(ss, "token")
		}
		if token == "" {
			return output.Errorf(output.ExitAPI, "api_error", "spreadsheet created but token missing in response")
		}

		result := map[string]interface{}{"spreadsheet": ss}

		if runtime.Str("headers") != "" || runtime.Str("data") != "" {
			fill, err := buildInitialFillInput(runtime)
			if err != nil {
				return err
			}
			fill["excel_id"] = token
			fillOut, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", fill)
			if err != nil {
				// Spreadsheet exists; surface the fill failure but keep the new
				// token in the envelope so the caller can recover or retry.
				return fmt.Errorf("spreadsheet %s created but initial fill failed: %w", token, err)
			}
			result["initial_fill"] = fillOut
		}
		runtime.Out(result, nil)
		return nil
	},
	Tips: []string{
		"--headers and --data are optional follow-up writes. They use the same set_cell_range tool as +cells-set; partial failure leaves the spreadsheet created but empty.",
	},
}

// buildInitialFillInput zips --headers + --data into a single set_cell_range
// payload writing to the first sheet starting at A1.
func buildInitialFillInput(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	var rows [][]interface{}
	if runtime.Str("headers") != "" {
		v, _ := parseJSONFlag(runtime, "headers")
		headerArr, _ := v.([]interface{})
		row := make([]interface{}, 0, len(headerArr))
		for _, h := range headerArr {
			row = append(row, map[string]interface{}{"value": h})
		}
		rows = append(rows, row)
	}
	if runtime.Str("data") != "" {
		v, _ := parseJSONFlag(runtime, "data")
		dataArr, _ := v.([]interface{})
		for _, r := range dataArr {
			cells, _ := r.([]interface{})
			row := make([]interface{}, 0, len(cells))
			for _, c := range cells {
				row = append(row, map[string]interface{}{"value": c})
			}
			rows = append(rows, row)
		}
	}
	if len(rows) == 0 {
		return nil, nil
	}
	maxCols := 0
	for _, r := range rows {
		if len(r) > maxCols {
			maxCols = len(r)
		}
	}
	// Normalize rows to the same length so cells matrix is rectangular.
	for i := range rows {
		for len(rows[i]) < maxCols {
			rows[i] = append(rows[i], map[string]interface{}{})
		}
	}
	endCol := columnIndexToLetter(maxCols - 1)
	rangeStr := fmt.Sprintf("A1:%s%d", endCol, len(rows))
	return map[string]interface{}{
		"range":  rangeStr,
		"cells":  rows,
		"sheet_id": "", // filled in by caller if sheet_id known; otherwise server picks first sheet
	}, nil
}

// ─── +workbook-export ─────────────────────────────────────────────────

// WorkbookExport drives the three-step export flow: create task → poll →
// optionally download. CSV mode requires --sheet-id (the API exports one
// sheet at a time as csv).
var WorkbookExport = common.Shortcut{
	Service:     "sheets",
	Command:     "+workbook-export",
	Description: "Export a spreadsheet to xlsx or a single sheet to csv (async + poll + optional download).",
	Risk:        "read",
	Scopes:      []string{"sheets:spreadsheet:read", "docs:document:export", "drive:drive.metadata:readonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicTokenFlags(),
		common.Flag{Name: "file-extension", Enum: []string{"xlsx", "csv"}, Default: "xlsx", Desc: "xlsx (whole workbook) or csv (one sheet via --sheet-id)"},
		common.Flag{Name: "sheet-id", Desc: "csv mode only: target sheet reference_id to export"},
		common.Flag{Name: "output-path", Desc: "local file path to save into; omit to just trigger and report the file_token"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		ext := runtime.Str("file-extension")
		if ext == "" {
			ext = "xlsx"
		}
		if ext == "csv" && strings.TrimSpace(runtime.Str("sheet-id")) == "" {
			return common.FlagErrorf("--sheet-id is required when --file-extension=csv")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		ext := runtime.Str("file-extension")
		if ext == "" {
			ext = "xlsx"
		}
		body := map[string]interface{}{
			"token":          token,
			"type":           "sheet",
			"file_extension": ext,
		}
		if sid := strings.TrimSpace(runtime.Str("sheet-id")); sid != "" {
			body["sub_id"] = sid
		}
		dry := common.NewDryRunAPI().
			POST("/open-apis/drive/v1/export_tasks").
			Desc("create export task").
			Body(body).
			GET("/open-apis/drive/v1/export_tasks/<ticket>").
			Desc("poll task status").
			Params(map[string]interface{}{"token": token})
		if strings.TrimSpace(runtime.Str("output-path")) != "" {
			dry.GET("/open-apis/drive/v1/export_tasks/file/<file_token>/download").
				Desc("download exported file")
		}
		return dry
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		ext := runtime.Str("file-extension")
		if ext == "" {
			ext = "xlsx"
		}
		body := map[string]interface{}{
			"token":          token,
			"type":           "sheet",
			"file_extension": ext,
		}
		if sid := strings.TrimSpace(runtime.Str("sheet-id")); sid != "" {
			body["sub_id"] = sid
		}
		taskData, err := runtime.CallAPI("POST", "/open-apis/drive/v1/export_tasks", nil, body)
		if err != nil {
			return err
		}
		ticket := common.GetString(taskData, "ticket")
		if ticket == "" {
			return output.Errorf(output.ExitAPI, "api_error", "export task created but ticket missing")
		}

		result := map[string]interface{}{
			"ticket":         ticket,
			"file_extension": ext,
		}

		// Poll up to ~30s for completion.
		var fileToken, fileName string
		for attempt := 0; attempt < 15; attempt++ {
			status, err := pollExportTask(runtime, token, ticket)
			if err != nil {
				return err
			}
			switch status.JobStatus {
			case 0: // success
				fileToken = status.FileToken
				fileName = status.FileName
				result["file_token"] = fileToken
				result["file_name"] = fileName
				result["file_size"] = status.FileSize
				attempt = 999 // break outer loop
			case 1, 2: // pending / in progress
				time.Sleep(2 * time.Second)
				continue
			default: // any non-zero status outside the in-progress window is a failure
				if status.JobErrorMsg != "" {
					return output.Errorf(output.ExitAPI, "api_error", "export task %s failed: %s", ticket, status.JobErrorMsg)
				}
				return output.Errorf(output.ExitAPI, "api_error", "export task %s failed with job_status=%d", ticket, status.JobStatus)
			}
		}
		if fileToken == "" {
			result["status"] = "polling_timeout"
			runtime.Out(result, nil)
			return nil
		}

		outPath := strings.TrimSpace(runtime.Str("output-path"))
		if outPath == "" {
			runtime.Out(result, nil)
			return nil
		}

		saved, err := downloadExportFile(ctx, runtime, fileToken, outPath, fileName)
		if err != nil {
			return err
		}
		result["saved_path"] = saved
		runtime.Out(result, nil)
		return nil
	},
	Tips: []string{
		"Polls up to ~30s (15 × 2s). For very large workbooks rerun and pass --output-path to capture the file once status flips to success.",
	},
}

type exportTaskStatus struct {
	JobStatus     int
	JobErrorMsg   string
	FileToken     string
	FileName      string
	FileSize      int64
	FileExtension string
}

func pollExportTask(runtime *common.RuntimeContext, token, ticket string) (exportTaskStatus, error) {
	data, err := runtime.CallAPI(
		"GET",
		fmt.Sprintf("/open-apis/drive/v1/export_tasks/%s", validate.EncodePathSegment(ticket)),
		map[string]interface{}{"token": token},
		nil,
	)
	if err != nil {
		return exportTaskStatus{}, err
	}
	result := common.GetMap(data, "result")
	if result == nil {
		return exportTaskStatus{}, output.Errorf(output.ExitAPI, "api_error", "export task %s: empty result", ticket)
	}
	js, _ := util.ToFloat64(result["job_status"])
	fs, _ := util.ToFloat64(result["file_size"])
	return exportTaskStatus{
		JobStatus:     int(js),
		JobErrorMsg:   common.GetString(result, "job_error_msg"),
		FileToken:     common.GetString(result, "file_token"),
		FileName:      common.GetString(result, "file_name"),
		FileSize:      int64(fs),
		FileExtension: common.GetString(result, "file_extension"),
	}, nil
}

func downloadExportFile(ctx context.Context, runtime *common.RuntimeContext, fileToken, outPath, preferredName string) (string, error) {
	apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/drive/v1/export_tasks/file/%s/download", validate.EncodePathSegment(fileToken)),
	}, larkcore.WithFileDownload())
	if err != nil {
		return "", output.ErrNetwork("download failed: %s", err)
	}
	if apiResp.StatusCode >= 400 {
		return "", output.ErrNetwork("download failed: HTTP %d: %s", apiResp.StatusCode, string(apiResp.RawBody))
	}
	target := outPath
	if info, statErr := runtime.FileIO().Stat(outPath); statErr == nil && info.IsDir() {
		name := strings.TrimSpace(preferredName)
		if name == "" {
			name = client.ResolveFilename(apiResp)
		}
		target = filepath.Join(outPath, name)
	}
	if _, err := runtime.FileIO().Save(target, fileio.SaveOptions{
		ContentType:   apiResp.Header.Get("Content-Type"),
		ContentLength: int64(len(apiResp.RawBody)),
	}, strings.NewReader(string(apiResp.RawBody))); err != nil {
		return "", common.WrapSaveErrorByCategory(err, "io")
	}
	resolved, _ := runtime.FileIO().ResolvePath(target)
	if resolved == "" {
		resolved = target
	}
	return resolved, nil
}

// ─── +dim-move ────────────────────────────────────────────────────────

// DimMove moves a contiguous block of rows or columns to a new index in the
// same sheet. The CLI flag semantic is 0-based inclusive (--start / --end);
// the legacy v2 endpoint expects half-open [startIndex, endIndex).
var DimMove = common.Shortcut{
	Service:     "sheets",
	Command:     "+dim-move",
	Description: "Move a contiguous block of rows or columns to a new position (re-numbers neighbors).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "sheets:spreadsheet:read"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "dimension", Required: true, Enum: dimEnum, Desc: "`row` or `column`"},
		common.Flag{Name: "start", Type: "int", Required: true, Desc: "source start (0-indexed, inclusive)"},
		common.Flag{Name: "end", Type: "int", Required: true, Desc: "source end (0-indexed, inclusive)"},
		common.Flag{Name: "target", Type: "int", Required: true, Desc: "destination index (0-indexed); rows/cols move to land BEFORE this index"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		if !runtime.Changed("dimension") || !runtime.Changed("start") || !runtime.Changed("end") || !runtime.Changed("target") {
			return common.FlagErrorf("--dimension / --start / --end / --target are all required")
		}
		if runtime.Int("start") < 0 || runtime.Int("end") < runtime.Int("start") {
			return common.FlagErrorf("--end (%d) must be >= --start (%d) (both 0-indexed, inclusive)", runtime.Int("end"), runtime.Int("start"))
		}
		if runtime.Int("target") < 0 {
			return common.FlagErrorf("--target must be >= 0")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		body := dimMoveBody(runtime, sheetSelectorPlaceholder(sheetID, sheetName))
		return common.NewDryRunAPI().
			POST(fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dimension_range", token)).
			Body(body).
			Set("spreadsheet_token", token)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		// Legacy v2 endpoint needs sheet_id. Resolve sheet_name client-side
		// when needed (reuses lookupSheetIndex which fetches workbook structure).
		if sheetID == "" {
			lookedID, _, err := lookupSheetIndex(ctx, runtime, token, "", sheetName)
			if err != nil {
				return err
			}
			sheetID = lookedID
		}
		body := dimMoveBody(runtime, sheetID)
		data, err := runtime.CallAPI(
			"POST",
			fmt.Sprintf("/open-apis/sheets/v2/spreadsheets/%s/dimension_range", validate.EncodePathSegment(token)),
			nil, body,
		)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

func dimMoveBody(runtime *common.RuntimeContext, sheetID string) map[string]interface{} {
	dim := "ROWS"
	if runtime.Str("dimension") == "column" {
		dim = "COLUMNS"
	}
	return map[string]interface{}{
		"source": map[string]interface{}{
			"sheetId":        sheetID,
			"majorDimension": dim,
			"startIndex":     runtime.Int("start"),
			"endIndex":       runtime.Int("end") + 1, // CLI inclusive → API exclusive
		},
		"destinationIndex": runtime.Int("target"),
	}
}

// ─── +cells-set-image ─────────────────────────────────────────────────

// CellsSetImage uploads a local image to drive (parent_type=sheet_image,
// parent_node=spreadsheet token) and then writes a rich_text embed-image
// into the target single-cell range via the set_cell_range tool.
var CellsSetImage = common.Shortcut{
	Service:     "sheets",
	Command:     "+cells-set-image",
	Description: "Embed a local image into a single cell (uploads via drive, then set_cell_range with rich_text embed-image).",
	Risk:        "write",
	Scopes:      []string{"sheets:spreadsheet:write_only", "drive:file:upload"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: append(publicSheetFlags(),
		common.Flag{Name: "range", Required: true, Desc: "single target cell (e.g. A1; start/end must equal)"},
		common.Flag{Name: "image", Required: true, Desc: "local image path (PNG/JPEG/JPG/GIF/BMP/JFIF/EXIF/TIFF/BPG/HEIC)"},
		common.Flag{Name: "name", Desc: "uploaded file name (with extension); defaults to basename(--image)"},
	),
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if _, err := resolveSpreadsheetToken(runtime); err != nil {
			return err
		}
		if _, _, err := resolveSheetSelector(runtime); err != nil {
			return err
		}
		r := strings.TrimSpace(runtime.Str("range"))
		if r == "" {
			return common.FlagErrorf("--range is required")
		}
		rows, cols, err := rangeDimensions(r)
		if err != nil {
			return common.FlagErrorf("--range %q: %v", r, err)
		}
		if rows != 1 || cols != 1 {
			return common.FlagErrorf("--range %q must be exactly one cell (got %d×%d)", r, rows, cols)
		}
		if strings.TrimSpace(runtime.Str("image")) == "" {
			return common.FlagErrorf("--image is required")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		token, _ := resolveSpreadsheetToken(runtime)
		sheetID, sheetName, _ := resolveSheetSelector(runtime)
		imgPath := strings.TrimSpace(runtime.Str("image"))
		fileName := strings.TrimSpace(runtime.Str("name"))
		if fileName == "" {
			fileName = filepath.Base(imgPath)
		}
		setCellBody, _ := buildToolBody("set_cell_range", map[string]interface{}{
			"excel_id": token,
			"range":    strings.TrimSpace(runtime.Str("range")),
			"sheet_id": sheetSelectorPlaceholder(sheetID, sheetName),
			"cells": [][]interface{}{{map[string]interface{}{
				"rich_text": []map[string]interface{}{{
					"type":             "embed-image",
					"attachment_token": "<file_token>",
					"attachment_name":  fileName,
				}},
			}}},
		})
		return common.NewDryRunAPI().
			POST("/open-apis/drive/v1/medias/upload_all").
			Desc("upload local image to drive (parent_type=sheet_image)").
			Body(map[string]interface{}{
				"file_name":   fileName,
				"parent_type": "sheet_image",
				"parent_node": token,
				"size":        "<file_size>",
				"file":        "@" + imgPath,
			}).
			POST(toolInvokePath(token, ToolKindWrite)).
			Desc("embed file_token into the cell via set_cell_range").
			Body(setCellBody)
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		token, err := resolveSpreadsheetToken(runtime)
		if err != nil {
			return err
		}
		sheetID, sheetName, err := resolveSheetSelector(runtime)
		if err != nil {
			return err
		}
		imgPath := strings.TrimSpace(runtime.Str("image"))
		fileName := strings.TrimSpace(runtime.Str("name"))
		if fileName == "" {
			fileName = filepath.Base(imgPath)
		}
		info, err := runtime.FileIO().Stat(imgPath)
		if err != nil {
			return common.WrapInputStatError(err)
		}
		fileToken, err := common.UploadDriveMediaAll(runtime, common.DriveMediaUploadAllConfig{
			FilePath:   imgPath,
			FileName:   fileName,
			FileSize:   info.Size(),
			ParentType: "sheet_image",
			ParentNode: &token,
		})
		if err != nil {
			return err
		}

		setCellInput := map[string]interface{}{
			"excel_id": token,
			"range":    strings.TrimSpace(runtime.Str("range")),
			"cells": [][]interface{}{{map[string]interface{}{
				"rich_text": []map[string]interface{}{{
					"type":             "embed-image",
					"attachment_token": fileToken,
					"attachment_name":  fileName,
				}},
			}}},
		}
		sheetSelectorForToolInput(setCellInput, sheetID, sheetName)
		setCellOut, err := callTool(ctx, runtime, token, ToolKindWrite, "set_cell_range", setCellInput)
		if err != nil {
			return fmt.Errorf("image uploaded (file_token=%s) but cell write failed: %w", fileToken, err)
		}
		runtime.Out(map[string]interface{}{
			"file_token":     fileToken,
			"file_name":      fileName,
			"set_cell_range": setCellOut,
		}, nil)
		return nil
	},
	Tips: []string{
		"--range must be a single cell. The uploaded image becomes a cell-internal embed; use +float-image-create for floating images.",
	},
}

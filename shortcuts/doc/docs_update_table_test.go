// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package doc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newTableUpdateRuntime builds a RuntimeContext populated with the full
// table_update_property flag set so tests can exercise validate / body
// construction without mounting the real Shortcut. Typed flags mirror the
// declarations in tableUpdateFlags().
func newTableUpdateRuntime(stringFlags map[string]string, intFlags map[string]int, boolFlags map[string]*bool) *common.RuntimeContext {
	cmd := &cobra.Command{Use: "test"}
	for _, name := range []string{"command", "table-block-id", "cell", "range", "col", "col-start", "col-end", "background-color", "vertical-align"} {
		cmd.Flags().String(name, "", "")
	}
	for _, name := range []string{"row-index", "row-start", "row-end", "col-width", "revision-id"} {
		cmd.Flags().Int(name, 0, "")
	}
	for _, name := range []string{"header-row", "header-column"} {
		cmd.Flags().Bool(name, false, "")
	}
	_ = cmd.ParseFlags(nil)
	for name, val := range stringFlags {
		_ = cmd.Flags().Set(name, val)
	}
	for name, val := range intFlags {
		_ = cmd.Flags().Set(name, intStr(val))
	}
	for name, val := range boolFlags {
		if val == nil {
			continue
		}
		if *val {
			_ = cmd.Flags().Set(name, "true")
		} else {
			_ = cmd.Flags().Set(name, "false")
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func intStr(v int) string {
	if v < 0 {
		return "-" + intStr(-v)
	}
	if v < 10 {
		return string(rune('0' + v))
	}
	return intStr(v/10) + string(rune('0'+v%10))
}

func boolPtr(b bool) *bool { return &b }

// ── A1 notation parsing tests ──

func TestParseColLetter(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"A", 0, false},
		{"B", 1, false},
		{"Z", 25, false},
		{"AA", 26, false},
		{"AB", 27, false},
		{"a", 0, false},   // case-insensitive
		{"0", 0, false},   // special: before-first
		{"-1", -1, false}, // special: append
		{"", 0, true},
		{"2", 0, true},  // numeric (not 0/-1) is invalid
		{"A1", 0, true}, // contains digit
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseColLetter(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseColLetter(%q) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Errorf("parseColLetter(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseA1Cell(t *testing.T) {
	tests := []struct {
		input   string
		wantRow int
		wantCol int
		wantErr bool
	}{
		{"A1", 0, 0, false},
		{"B3", 2, 1, false},
		{"AA1", 0, 26, false},
		{"Z26", 25, 25, false},
		{"c4", 3, 2, false}, // case-insensitive
		{"1A", 0, 0, true},  // wrong order
		{"A0", 0, 0, true},  // row must be >= 1
		{"A", 0, 0, true},   // missing row
		{"1", 0, 0, true},   // missing col
		{"", 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			row, col, err := parseA1Cell(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseA1Cell(%q) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
			if err == nil {
				if row != tt.wantRow || col != tt.wantCol {
					t.Errorf("parseA1Cell(%q) = (%d,%d), want (%d,%d)",
						tt.input, row, col, tt.wantRow, tt.wantCol)
				}
			}
		})
	}
}

func TestParseA1Range(t *testing.T) {
	tests := []struct {
		input                    string
		wantRowStart, wantRowEnd int // half-open
		wantColStart, wantColEnd int // half-open
		wantErr                  bool
	}{
		{"A1:C3", 0, 3, 0, 3, false}, // inclusive A1:C3 → half-open [0,3)×[0,3)
		{"B2:D5", 1, 5, 1, 4, false},
		{"A1:A1", 0, 1, 0, 1, false}, // single cell
		{"C3:A1", 0, 0, 0, 0, true},  // start after end
		{"A1:", 0, 0, 0, 0, true},
		{"A1", 0, 0, 0, 0, true}, // missing colon
		{"", 0, 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			rs, re, cs, ce, err := parseA1Range(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseA1Range(%q) err=%v, wantErr=%v", tt.input, err, tt.wantErr)
			}
			if err == nil {
				if rs != tt.wantRowStart || re != tt.wantRowEnd || cs != tt.wantColStart || ce != tt.wantColEnd {
					t.Errorf("parseA1Range(%q) = (%d,%d,%d,%d), want (%d,%d,%d,%d)",
						tt.input, rs, re, cs, ce,
						tt.wantRowStart, tt.wantRowEnd, tt.wantColStart, tt.wantColEnd)
				}
			}
		})
	}
}

// ── table_update_property validation tests (mode-aware) ──

func TestValidateTableUpdate_TableUpdateProperty_Modes(t *testing.T) {
	tests := []struct {
		name         string
		stringFlags  map[string]string
		intFlags     map[string]int
		boolFlags    map[string]*bool
		wantErrMatch string // substring match; "" expects success
	}{
		{
			name: "cell mode valid",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"cell": "B3", "background-color": "red",
			},
		},
		{
			name: "range mode valid",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"range": "A1:C3", "background-color": "rgb(1,2,3)",
			},
		},
		{
			name: "row mode valid",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"vertical-align": "top",
			},
			intFlags: map[string]int{"row-start": 1, "row-end": 3},
		},
		{
			name: "col mode valid",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"col-start": "A", "col-end": "C", "background-color": "#ffeecc",
			},
		},
		{
			name: "cell and range exclusive",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"cell": "B3", "range": "A1:C3", "background-color": "red",
			},
			wantErrMatch: "mutually exclusive",
		},
		{
			name: "cell and row-range exclusive",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"cell": "B3", "background-color": "red",
			},
			intFlags:     map[string]int{"row-start": 1, "row-end": 2},
			wantErrMatch: "mutually exclusive",
		},
		{
			name: "row-start without row-end",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"vertical-align": "top",
			},
			intFlags:     map[string]int{"row-start": 1},
			wantErrMatch: "both required",
		},
		{
			name: "row-end <= row-start",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"vertical-align": "top",
			},
			intFlags:     map[string]int{"row-start": 3, "row-end": 2},
			wantErrMatch: "--row-end must be > --row-start",
		},
		{
			name: "col-end before col-start",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"col-start": "C", "col-end": "A", "background-color": "red",
			},
			wantErrMatch: "--col-end must be > --col-start",
		},
		{
			name: "col-end equal to col-start (half-open empty range)",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"col-start": "B", "col-end": "B", "background-color": "red",
			},
			wantErrMatch: "--col-end must be > --col-start",
		},
		{
			name: "range without styling",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"range": "A1:C3",
			},
			wantErrMatch: "range update requires --background-color or --vertical-align",
		},
		{
			name: "table-level col-width without col",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
			},
			intFlags:     map[string]int{"col-width": 100},
			wantErrMatch: "--col is required when --col-width is set",
		},
		{
			// 非法颜色格式必须在 cli 就被拦下 —— 早一点失败，agent 拿到更精确的错误信息，
			// 也避免无谓的网络往返。覆盖 rgb(...) / 命名色 / hex 之外的典型错误输入。
			name: "invalid background color",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"cell": "B3", "background-color": "not a color",
			},
			wantErrMatch: "--background-color",
		},
		{
			// 完全空更新：没有 targeting，也没有表级属性 —— 老逻辑会静默走
			// default 分支生成只含 block_id 的 extra_param；新逻辑必须显式报错。
			name: "empty update - no targeting no table-level props",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
			},
			wantErrMatch: "requires at least one property",
		},
		{
			// 单独设置 --header-row 必须合法：它与其他 cell 属性 / 列宽 / targeting 正交，
			// 任何一个存在就算 "有事可做"。
			name: "header-row only is valid",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
			},
			boolFlags: map[string]*bool{"header-row": boolPtr(true)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTableUpdateRuntime(tt.stringFlags, tt.intFlags, tt.boolFlags)
			err := validateTableUpdate(context.Background(), rt)
			if tt.wantErrMatch == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErrMatch)
			}
			if !strings.Contains(err.Error(), tt.wantErrMatch) {
				t.Errorf("expected error containing %q, got %q", tt.wantErrMatch, err.Error())
			}
		})
	}
}

// TestBuildTableSingleBody_TableUpdateProperty_Modes verifies the exact
// extra_param key set emitted for each targeting mode. Keys must stay in sync
// with the JSON struct ai_edit decodes in buildUpdateCommandSet.
func TestBuildTableSingleBody_TableUpdateProperty_Modes(t *testing.T) {
	tests := []struct {
		name         string
		stringFlags  map[string]string
		intFlags     map[string]int
		wantKeys     map[string]interface{}
		wantKeysOnly []string // keys that MUST NOT appear
	}{
		{
			name: "cell mode",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"cell": "B3", "background-color": "red",
			},
			wantKeys:     map[string]interface{}{"cell": "B3", "background_color": "red"},
			wantKeysOnly: []string{"range", "row_start_index", "column_start_index"},
		},
		{
			name: "range mode",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"range": "A1:C3", "background-color": "rgb(1,2,3)",
			},
			wantKeys:     map[string]interface{}{"range": "A1:C3", "background_color": "rgb(1,2,3)"},
			wantKeysOnly: []string{"cell", "row_start_index", "column_start_index"},
		},
		{
			name: "row mode",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"vertical-align": "top",
			},
			intFlags:     map[string]int{"row-start": 1, "row-end": 3},
			wantKeys:     map[string]interface{}{"row_start_index": float64(1), "row_end_index": float64(3), "vertical_align": "top"},
			wantKeysOnly: []string{"cell", "range", "column_start_index"},
		},
		{
			name: "col mode",
			stringFlags: map[string]string{
				"command": "table_update_property", "table-block-id": "tbl",
				"col-start": "A", "col-end": "C", "background-color": "#fff",
			},
			wantKeys:     map[string]interface{}{"column_start_index": "A", "column_end_index": "C", "background_color": "#fff"},
			wantKeysOnly: []string{"cell", "range", "row_start_index"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newTableUpdateRuntime(tt.stringFlags, tt.intFlags, nil)
			body := buildTableSingleBody(rt, "table_update_property")
			raw, ok := body["extra_param"].(string)
			if !ok {
				t.Fatalf("expected string extra_param, got %T", body["extra_param"])
			}
			var extra map[string]interface{}
			if err := json.Unmarshal([]byte(raw), &extra); err != nil {
				t.Fatalf("extra_param not valid JSON: %v", err)
			}
			for k, v := range tt.wantKeys {
				got, present := extra[k]
				if !present {
					t.Errorf("extra missing key %q; full=%v", k, extra)
					continue
				}
				if got != v {
					t.Errorf("extra[%q] = %v (%T), want %v (%T)", k, got, got, v, v)
				}
			}
			for _, k := range tt.wantKeysOnly {
				if _, present := extra[k]; present {
					t.Errorf("extra unexpectedly contains key %q; full=%v", k, extra)
				}
			}
		})
	}
}

// ── Command routing tests ──

func TestIsTableCommand(t *testing.T) {
	tableCommands := []string{
		"table_insert_rows", "table_insert_cols",
		"table_delete_rows", "table_delete_cols",
		"table_merge_cells", "table_unmerge_cells",
		"table_update_property",
	}
	for _, cmd := range tableCommands {
		if !isTableCommand(cmd) {
			t.Errorf("isTableCommand(%q) = false, want true", cmd)
		}
	}
	nonTableCommands := []string{"str_replace", "block_replace", "overwrite", "append", "table_batch", ""}
	for _, cmd := range nonTableCommands {
		if isTableCommand(cmd) {
			t.Errorf("isTableCommand(%q) = true, want false", cmd)
		}
	}
}

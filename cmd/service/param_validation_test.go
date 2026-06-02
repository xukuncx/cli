// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
)

const testSchemaPath = "mail.user_mailbox.messages.list"

func pageSizeParamSpec(min, max string) map[string]interface{} {
	return map[string]interface{}{
		"type":     "integer",
		"location": "query",
		"min":      min,
		"max":      max,
	}
}

func TestValidateQueryParamRange_AllowlistNonTargetSkipped(t *testing.T) {
	if err := validateQueryParamRange("page_size", float64(50), pageSizeParamSpec("1", "20"), "calendar.events.list"); err != nil {
		t.Errorf("expected nil for non-target schemaPath, got: %v", err)
	}
}

func TestValidateQueryParamRange_NonTargetParamSkipped(t *testing.T) {
	if err := validateQueryParamRange("other_param", float64(9999), pageSizeParamSpec("1", "20"), testSchemaPath); err != nil {
		t.Errorf("expected nil for non-allowlisted param, got: %v", err)
	}
}

func TestValidateQueryParamRange_NilSpec(t *testing.T) {
	if err := validateQueryParamRange("page_size", float64(50), nil, testSchemaPath); err != nil {
		t.Errorf("expected nil when allowlisted param has no schema metadata, got: %v", err)
	}
}

func TestValidateQueryParamRange_WithinRange(t *testing.T) {
	if err := validateQueryParamRange("page_size", float64(10), pageSizeParamSpec("1", "20"), testSchemaPath); err != nil {
		t.Errorf("expected nil for value within range, got: %v", err)
	}
}

func TestValidateQueryParamRange_ExactlyAtMax(t *testing.T) {
	if err := validateQueryParamRange("page_size", float64(20), pageSizeParamSpec("1", "20"), testSchemaPath); err != nil {
		t.Errorf("expected nil for value at max boundary, got: %v", err)
	}
}

func TestValidateQueryParamRange_ExactlyAtMin(t *testing.T) {
	if err := validateQueryParamRange("page_size", float64(1), pageSizeParamSpec("1", "20"), testSchemaPath); err != nil {
		t.Errorf("expected nil for value at min boundary, got: %v", err)
	}
}

func TestValidateQueryParamRange_ExceedsMax(t *testing.T) {
	err := validateQueryParamRange("page_size", float64(21), pageSizeParamSpec("1", "20"), testSchemaPath)
	if err == nil {
		t.Fatal("expected error for page_size exceeding max")
	}
	if !strings.Contains(err.Error(), "exceeds the maximum") {
		t.Errorf("expected 'exceeds the maximum' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "20") {
		t.Errorf("expected error to mention max value 20, got: %v", err)
	}
}

func TestValidateQueryParamRange_BelowMin(t *testing.T) {
	err := validateQueryParamRange("page_size", float64(0), pageSizeParamSpec("1", "20"), testSchemaPath)
	if err == nil {
		t.Fatal("expected error for page_size below min")
	}
	if !strings.Contains(err.Error(), "less than the minimum") {
		t.Errorf("expected 'less than the minimum' error, got: %v", err)
	}
}

func TestValidateQueryParamRange_StringIntegerPasses(t *testing.T) {
	if err := validateQueryParamRange("page_size", "20", pageSizeParamSpec("1", "20"), testSchemaPath); err != nil {
		t.Errorf("expected nil for string integer \"20\", got: %v", err)
	}
}

func TestValidateQueryParamRange_StringIntegerExceedsMax(t *testing.T) {
	err := validateQueryParamRange("page_size", "21", pageSizeParamSpec("1", "20"), testSchemaPath)
	if err == nil {
		t.Fatal("expected error for string page_size exceeding max")
	}
	if !strings.Contains(err.Error(), "exceeds the maximum") {
		t.Errorf("expected 'exceeds the maximum' error, got: %v", err)
	}
}

func TestValidateQueryParamRange_FloatRejected(t *testing.T) {
	err := validateQueryParamRange("page_size", 1.5, pageSizeParamSpec("1", "20"), testSchemaPath)
	if err == nil {
		t.Fatal("expected error for float page_size")
	}
	if !strings.Contains(err.Error(), "not a valid integer") {
		t.Errorf("expected 'not a valid integer' error, got: %v", err)
	}
}

func TestValidateQueryParamRange_StringFloatRejected(t *testing.T) {
	err := validateQueryParamRange("page_size", "1.5", pageSizeParamSpec("1", "20"), testSchemaPath)
	if err == nil {
		t.Fatal("expected error for string float page_size")
	}
	if !strings.Contains(err.Error(), "not a valid integer") {
		t.Errorf("expected 'not a valid integer' error, got: %v", err)
	}
}

func TestValidateQueryParamRange_StringNonNumericRejected(t *testing.T) {
	err := validateQueryParamRange("page_size", "abc", pageSizeParamSpec("1", "20"), testSchemaPath)
	if err == nil {
		t.Fatal("expected error for non-numeric string page_size")
	}
	if !strings.Contains(err.Error(), "not a valid integer") {
		t.Errorf("expected 'not a valid integer' error, got: %v", err)
	}
}

func TestValidateQueryParamRange_Float64WithinRange(t *testing.T) {
	if err := validateQueryParamRange("page_size", float64(10), pageSizeParamSpec("1", "20"), testSchemaPath); err != nil {
		t.Errorf("expected nil for float64 within range, got: %v", err)
	}
}

func TestValidateQueryParamRange_JSONNumberPasses(t *testing.T) {
	if err := validateQueryParamRange("page_size", json.Number("10"), pageSizeParamSpec("1", "20"), testSchemaPath); err != nil {
		t.Errorf("expected nil for json.Number within range, got: %v", err)
	}
}

func TestValidateQueryParamRange_UnsupportedTypesRejected(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{"bool", true},
		{"object", map[string]interface{}{"value": 20}},
		{"array", []interface{}{20}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateQueryParamRange("page_size", tt.value, pageSizeParamSpec("1", "20"), testSchemaPath)
			if err == nil {
				t.Fatal("expected error for unsupported page_size type")
			}
			if !strings.Contains(err.Error(), "not a valid integer") {
				t.Errorf("expected 'not a valid integer' error, got: %v", err)
			}
		})
	}
}

func TestValidateQueryParamRange_UsesParamSpecMax(t *testing.T) {
	if err := validateQueryParamRange("page_size", float64(25), pageSizeParamSpec("1", "30"), testSchemaPath); err != nil {
		t.Errorf("expected nil when schema max is 30, got: %v", err)
	}
}

func TestValidateQueryParamRange_NonTargetServiceNotBlocked(t *testing.T) {
	if err := validateQueryParamRange("limit", float64(200), pageSizeParamSpec("1", "100"), "calendar.events.list"); err != nil {
		t.Errorf("expected nil for non-target service command, got: %v", err)
	}
}

func TestServiceMethod_PageSizeExceedsMax(t *testing.T) {
	spec := map[string]interface{}{
		"name": "mail", "servicePath": "/open-apis/mail/v1",
	}
	method := map[string]interface{}{
		"path":       "user_mailboxes/{user_mailbox_id}/messages",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
			"page_size": map[string]interface{}{
				"type": "integer", "location": "query", "required": true,
				"min": "1", "max": "20",
			},
		},
	}
	f, _, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, spec, method, "list", "user_mailbox.messages", nil)
	cmd.SetArgs([]string{"--params", `{"user_mailbox_id":"me","page_size":21}`, "--dry-run"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for page_size exceeding max")
	}
	if !strings.Contains(err.Error(), "exceeds the maximum") {
		t.Errorf("expected 'exceeds the maximum' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "20") {
		t.Errorf("expected error to mention max value 20, got: %v", err)
	}
}

func TestServiceMethod_PageSizeWithinMax(t *testing.T) {
	spec := map[string]interface{}{
		"name": "mail", "servicePath": "/open-apis/mail/v1",
	}
	method := map[string]interface{}{
		"path":       "user_mailboxes/{user_mailbox_id}/messages",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
			"page_size": map[string]interface{}{
				"type": "integer", "location": "query", "required": false,
				"min": "1", "max": "20",
			},
		},
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, spec, method, "list", "user_mailbox.messages", nil)
	cmd.SetArgs([]string{"--params", `{"user_mailbox_id":"me","page_size":20}`, "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected no error for valid page_size, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Dry Run") {
		t.Error("expected dry-run output")
	}
}

func TestServiceMethod_PageAllSkipsRequiredCheck(t *testing.T) {
	spec := map[string]interface{}{
		"name": "mail", "servicePath": "/open-apis/mail/v1",
	}
	method := map[string]interface{}{
		"path":       "user_mailboxes/{user_mailbox_id}/messages",
		"httpMethod": "GET",
		"parameters": map[string]interface{}{
			"user_mailbox_id": map[string]interface{}{
				"type": "string", "location": "path", "required": true,
			},
			"page_size": map[string]interface{}{
				"type": "integer", "location": "query", "required": true,
				"min": "1", "max": "20",
			},
			"page_token": map[string]interface{}{
				"type": "string", "location": "query", "required": true,
			},
		},
	}
	f, stdout, _, _ := cmdutil.TestFactory(t, testConfig)
	cmd := NewCmdServiceMethod(f, spec, method, "list", "user_mailbox.messages", nil)
	cmd.SetArgs([]string{"--params", `{"user_mailbox_id":"me"}`, "--page-all", "--page-limit", "1", "--dry-run"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("expected no error for --page-all skipping required params, got: %v", err)
	}
	if !strings.Contains(stdout.String(), "Dry Run") {
		t.Error("expected dry-run output")
	}
}

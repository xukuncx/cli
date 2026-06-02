// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/registry"
)

// serviceParamValidationAllowlist defines per-(schemaPath, paramName) local
// validation entry points. Constraint details still come from metadata.
var serviceParamValidationAllowlist = map[string]map[string]struct{}{
	"mail.user_mailbox.messages.list": {
		"page_size": {},
	},
}

// validateQueryParamRange validates an allowlisted query parameter against the
// type and range declared in the method metadata. Parameters not in the
// allowlist are intentionally skipped to keep service command compatibility.
func validateQueryParamRange(name string, value interface{}, paramSpec map[string]interface{}, schemaPath string) error {
	if !shouldValidateServiceParam(schemaPath, name) || paramSpec == nil {
		return nil
	}

	switch registry.GetStrFromMap(paramSpec, "type") {
	case "integer":
		return validateIntegerQueryParam(name, value, paramSpec, schemaPath)
	default:
		return nil
	}
}

func shouldValidateServiceParam(schemaPath, name string) bool {
	paramRules, ok := serviceParamValidationAllowlist[schemaPath]
	if !ok {
		return false
	}
	_, ok = paramRules[name]
	return ok
}

func validateIntegerQueryParam(name string, value interface{}, paramSpec map[string]interface{}, schemaPath string) error {
	intVal, rawStr, err := parseIntegerParamValue(value)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid --params %s: %s is not a valid integer", name, rawStr).
			WithHint("The parameter %s must be an integer. Run: lark-cli schema %s", name, schemaPath).
			WithParam(name)
	}

	if minStr := registry.GetStrFromMap(paramSpec, "min"); minStr != "" {
		if minVal, err := strconv.ParseInt(minStr, 10, 64); err == nil && intVal < minVal {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"invalid --params %s: %d is less than the minimum allowed value %s", name, intVal, minStr).
				WithHint("The parameter %s must be at least %s. Run: lark-cli schema %s", name, minStr, schemaPath).
				WithParam(name)
		}
	}
	if maxStr := registry.GetStrFromMap(paramSpec, "max"); maxStr != "" {
		if maxVal, err := strconv.ParseInt(maxStr, 10, 64); err == nil && intVal > maxVal {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"invalid --params %s: %d exceeds the maximum allowed value %s", name, intVal, maxStr).
				WithHint("The parameter %s must be at most %s. Run: lark-cli schema %s", name, maxStr, schemaPath).
				WithParam(name)
		}
	}
	return nil
}

func parseIntegerParamValue(value interface{}) (int64, string, error) {
	switch v := value.(type) {
	case json.Number:
		rawStr := v.String()
		intVal, err := strconv.ParseInt(rawStr, 10, 64)
		return intVal, rawStr, err
	case string:
		intVal, err := strconv.ParseInt(v, 10, 64)
		return intVal, v, err
	case float64:
		rawStr := strconv.FormatFloat(v, 'f', -1, 64)
		intVal, err := strconv.ParseInt(rawStr, 10, 64)
		return intVal, rawStr, err
	default:
		rawStr := fmt.Sprintf("%v", value)
		return 0, rawStr, fmt.Errorf("unsupported integer parameter type %T", value)
	}
}

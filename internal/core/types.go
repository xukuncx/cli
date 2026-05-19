// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import (
	"os"
	"strings"
)

// LarkBrand represents the Lark platform brand.
// "feishu" targets China-mainland, "lark" targets international.
// Any other string is treated as a custom base URL.
type LarkBrand string

const (
	BrandFeishu LarkBrand = "feishu"
	BrandLark   LarkBrand = "lark"
)

// ParseBrand normalizes a brand string to a LarkBrand constant.
// Unrecognized values default to BrandFeishu.
func ParseBrand(value string) LarkBrand {
	if value == "lark" {
		return BrandLark
	}
	return BrandFeishu
}

// Endpoints holds resolved endpoint URLs for different Lark services.
type Endpoints struct {
	Open     string // e.g. "https://open.feishu.cn"
	Accounts string // e.g. "https://accounts.feishu.cn"
	MCP      string // e.g. "https://mcp.feishu.cn"
	AppLink  string // e.g. "https://applink.feishu.cn"
}

// ResolveEndpoints resolves endpoint URLs based on brand.
func ResolveEndpoints(brand LarkBrand) Endpoints {
	switch brand {
	case BrandLark:
		return Endpoints{
			Open:     "https://open.larksuite.com",
			Accounts: "https://accounts.larksuite.com",
			MCP:      "https://mcp.larksuite.com",
			AppLink:  "https://applink.larksuite.com",
		}
	default:
		return Endpoints{
			Open:     "https://open.feishu.cn",
			Accounts: "https://accounts.feishu.cn",
			MCP:      "https://mcp.feishu.cn",
			AppLink:  "https://applink.feishu.cn",
		}
	}
}

// ResolveOpenBaseURL returns the Open API base URL for the given brand.
// If LARK_CLI_OPEN_API_BASE env var is set (non-empty after trim), it overrides
// the brand default. This is the supported way to redirect requests at a local
// mock server during development; see the apps domain local-test plan §4.
func ResolveOpenBaseURL(brand LarkBrand) string {
	if v := strings.TrimSpace(os.Getenv("LARK_CLI_OPEN_API_BASE")); v != "" {
		return v
	}
	return ResolveEndpoints(brand).Open
}

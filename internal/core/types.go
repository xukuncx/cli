// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

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
	Open      string // e.g. "https://open.feishu.cn"
	Accounts  string // e.g. "https://accounts.feishu.cn"
	MCP       string // e.g. "https://mcp.feishu.cn"
	AppLink   string // e.g. "https://applink.feishu.cn"
	Telemetry string // e.g. "https://mcs-bd.feishu.cn/v1/list"
}

// ResolveEndpoints resolves endpoint URLs based on brand.
func ResolveEndpoints(brand LarkBrand) Endpoints {
	switch brand {
	case BrandLark:
		return Endpoints{
			Open:      "https://open.larksuite.com",
			Accounts:  "https://accounts.larksuite.com",
			MCP:       "https://mcp.larksuite.com",
			AppLink:   "https://applink.larksuite.com",
			Telemetry: "",
		}
	default:
		return Endpoints{
			Open:      "https://open.feishu.cn",
			Accounts:  "https://accounts.feishu.cn",
			MCP:       "https://mcp.feishu.cn",
			AppLink:   "https://applink.feishu.cn",
			Telemetry: "https://mcs-bd.feishu.cn/v1/list",
		}
	}
}

// ResolveOpenBaseURL returns the Open API base URL for the given brand.
func ResolveOpenBaseURL(brand LarkBrand) string {
	return ResolveEndpoints(brand).Open
}

// ResolveTelemetryEndpoint returns the telemetry endpoint for the given brand.
// Empty string means telemetry is disabled for that brand.
func ResolveTelemetryEndpoint(brand LarkBrand) string {
	return ResolveEndpoints(brand).Telemetry
}

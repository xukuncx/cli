// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package core

import "testing"

func TestResolveEndpoints_Feishu(t *testing.T) {
	ep := ResolveEndpoints(BrandFeishu)
	if ep.Open != "https://open.feishu.cn" {
		t.Errorf("Open = %q, want feishu.cn", ep.Open)
	}
	if ep.Accounts != "https://accounts.feishu.cn" {
		t.Errorf("Accounts = %q, want feishu.cn", ep.Accounts)
	}
	if ep.MCP != "https://mcp.feishu.cn" {
		t.Errorf("MCP = %q, want feishu.cn", ep.MCP)
	}
	if ep.AppLink != "https://applink.feishu.cn" {
		t.Errorf("AppLink = %q, want feishu.cn", ep.AppLink)
	}
	if ep.Telemetry != "https://mcs-bd.feishu.cn/v1/list" {
		t.Errorf("Telemetry = %q, want mcs-bd.feishu.cn", ep.Telemetry)
	}
}

func TestResolveEndpoints_Lark(t *testing.T) {
	ep := ResolveEndpoints(BrandLark)
	if ep.Open != "https://open.larksuite.com" {
		t.Errorf("Open = %q, want larksuite.com", ep.Open)
	}
	if ep.Accounts != "https://accounts.larksuite.com" {
		t.Errorf("Accounts = %q, want larksuite.com", ep.Accounts)
	}
	if ep.MCP != "https://mcp.larksuite.com" {
		t.Errorf("MCP = %q, want larksuite.com", ep.MCP)
	}
	if ep.AppLink != "https://applink.larksuite.com" {
		t.Errorf("AppLink = %q, want larksuite.com", ep.AppLink)
	}
	if ep.Telemetry != "" {
		t.Errorf("Telemetry = %q, want empty", ep.Telemetry)
	}
}

func TestResolveEndpoints_EmptyDefaultsToFeishu(t *testing.T) {
	ep := ResolveEndpoints("")
	if ep.Open != "https://open.feishu.cn" {
		t.Errorf("Open = %q, want feishu.cn for empty brand", ep.Open)
	}
}

func TestResolveOpenBaseURL(t *testing.T) {
	if got := ResolveOpenBaseURL(BrandFeishu); got != "https://open.feishu.cn" {
		t.Errorf("ResolveOpenBaseURL(feishu) = %q", got)
	}
	if got := ResolveOpenBaseURL(BrandLark); got != "https://open.larksuite.com" {
		t.Errorf("ResolveOpenBaseURL(lark) = %q", got)
	}
}

func TestResolveTelemetryEndpoint(t *testing.T) {
	if got := ResolveTelemetryEndpoint(BrandFeishu); got != "https://mcs-bd.feishu.cn/v1/list" {
		t.Errorf("ResolveTelemetryEndpoint(feishu) = %q", got)
	}
	if got := ResolveTelemetryEndpoint(BrandLark); got != "" {
		t.Errorf("ResolveTelemetryEndpoint(lark) = %q", got)
	}
}

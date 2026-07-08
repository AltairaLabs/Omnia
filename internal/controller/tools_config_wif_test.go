/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func wifHTTPHandler() omniav1alpha1.HandlerDefinition {
	return omniav1alpha1.HandlerDefinition{
		Name: "wif-tool", Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{},
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeWorkloadIdentity,
			WorkloadIdentity: &omniav1alpha1.ToolAuthWorkloadIdentity{Cloud: "azure", Audience: "api://tool"}},
	}
}

func TestValidateHandlerAuth_AllowsAzureHTTPWIF(t *testing.T) {
	h := wifHTTPHandler()
	if err := validateHandlerAuth(&h); err != nil {
		t.Fatalf("azure HTTP WIF should be allowed: %v", err)
	}
}

func TestValidateHandlerAuth_RejectsOpenAPIWIF(t *testing.T) {
	h := wifHTTPHandler()
	h.Type = omniav1alpha1.HandlerTypeOpenAPI
	h.HTTPConfig, h.OpenAPIConfig = nil, &omniav1alpha1.OpenAPIConfig{SpecURL: "http://x/spec"}
	if err := validateHandlerAuth(&h); err == nil {
		t.Fatal("OpenAPI WIF should be rejected in this milestone (http only)")
	}
}

func TestValidateHandlerAuth_AllowsGRPCWIF(t *testing.T) {
	h := wifHTTPHandler()
	h.Type = omniav1alpha1.HandlerTypeGRPC
	h.HTTPConfig, h.GRPCConfig = nil, &omniav1alpha1.GRPCConfig{Endpoint: "x:1"}
	if err := validateHandlerAuth(&h); err != nil {
		t.Fatalf("gRPC WIF should now be allowed: %v", err)
	}
}

func TestValidateHandlerAuth_AllowsMCPWIFDespiteStrayHTTPConfig(t *testing.T) {
	// A handler declared type: mcp that also carries a stray non-nil HTTPConfig
	// must still validate on h.Type (mcp): buildHandlerEntry dispatches on
	// h.Type, so the MCP builder runs and applies workloadIdentityFieldsFor to
	// ToolMCP; the stray HTTPConfig is simply unused. Now that MCP is a
	// wifSupportedHandlerType, this is allowed — confirms the guard keys off
	// h.Type, not config presence.
	h := wifHTTPHandler()
	h.Type = omniav1alpha1.HandlerTypeMCP
	h.MCPConfig = &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportStreamableHTTP}
	// HTTPConfig deliberately left non-nil (the stray block).
	if err := validateHandlerAuth(&h); err != nil {
		t.Fatalf("MCP WIF should be allowed now that MCP is a supported WIF handler type: %v", err)
	}
}

func TestValidateHandlerAuth_AllowsMCPWIF(t *testing.T) {
	h := wifHTTPHandler()
	h.Type = omniav1alpha1.HandlerTypeMCP
	h.HTTPConfig, h.MCPConfig = nil, &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportStreamableHTTP}
	if err := validateHandlerAuth(&h); err != nil {
		t.Fatalf("streamable-http MCP WIF should be allowed: %v", err)
	}

	h.MCPConfig = &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportSSE}
	if err := validateHandlerAuth(&h); err != nil {
		t.Fatalf("sse MCP WIF should be allowed: %v", err)
	}
}

func TestValidateHandlerAuth_RejectsStdioMCPWIF(t *testing.T) {
	// Stdio MCP speaks JSON-RPC over stdin/stdout with no header channel, so
	// no auth (including workloadIdentity) can be applied. This guard must
	// keep rejecting stdio even though MCP is now a wifSupportedHandlerType.
	h := wifHTTPHandler()
	h.Type = omniav1alpha1.HandlerTypeMCP
	h.HTTPConfig, h.MCPConfig = nil, &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportStdio}
	if err := validateHandlerAuth(&h); err == nil {
		t.Fatal("stdio MCP WIF should still be rejected (no header channel)")
	}
}

func TestWorkloadIdentityFieldsFor(t *testing.T) {
	h := wifHTTPHandler()
	cloud, aud, _, ok := workloadIdentityFieldsFor(&h)
	if !ok || cloud != "azure" || aud != "api://tool" {
		t.Fatalf("got %q %q %v", cloud, aud, ok)
	}
}

func TestBuildHTTPConfig_SetsWIFFields(t *testing.T) {
	h := wifHTTPHandler()
	cfg, err := buildHTTPConfig(&h, "http://tool")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthType != string(omniav1alpha1.ToolAuthTypeWorkloadIdentity) || cfg.AuthCloud != "azure" || cfg.AuthAudience != "api://tool" {
		t.Fatalf("WIF fields not set: %+v", cfg)
	}
}

func wifGRPCHandler() omniav1alpha1.HandlerDefinition {
	h := wifHTTPHandler()
	h.Type = omniav1alpha1.HandlerTypeGRPC
	h.HTTPConfig, h.GRPCConfig = nil, &omniav1alpha1.GRPCConfig{Endpoint: "x:1"}
	return h
}

func TestBuildGRPCConfig_SetsWIFFields(t *testing.T) {
	h := wifGRPCHandler()
	cfg, err := buildGRPCConfig(&h, "tool:1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthType != string(omniav1alpha1.ToolAuthTypeWorkloadIdentity) || cfg.AuthCloud != "azure" || cfg.AuthAudience != "api://tool" {
		t.Fatalf("WIF fields not set: %+v", cfg)
	}
}

func wifMCPHandler() omniav1alpha1.HandlerDefinition {
	h := wifHTTPHandler()
	h.Type = omniav1alpha1.HandlerTypeMCP
	h.HTTPConfig, h.MCPConfig = nil, &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportStreamableHTTP}
	return h
}

func TestBuildMCPConfig_SetsWIFFields(t *testing.T) {
	h := wifMCPHandler()
	cfg, err := buildMCPConfig(&h)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthType != string(omniav1alpha1.ToolAuthTypeWorkloadIdentity) || cfg.AuthCloud != "azure" || cfg.AuthAudience != "api://tool" {
		t.Fatalf("WIF fields not set: %+v", cfg)
	}
}

func TestWifSupportedHandlerType(t *testing.T) {
	cases := []struct {
		typ  omniav1alpha1.HandlerType
		want bool
	}{
		{omniav1alpha1.HandlerTypeHTTP, true},
		{omniav1alpha1.HandlerTypeGRPC, true},
		{omniav1alpha1.HandlerTypeOpenAPI, false},
		{omniav1alpha1.HandlerTypeMCP, true},
	}
	for _, tc := range cases {
		if got := wifSupportedHandlerType(tc.typ); got != tc.want {
			t.Errorf("wifSupportedHandlerType(%v) = %v, want %v", tc.typ, got, tc.want)
		}
	}
}

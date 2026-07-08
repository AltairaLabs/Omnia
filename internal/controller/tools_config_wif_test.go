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

func TestValidateHandlerAuth_RejectsGRPCWIF(t *testing.T) {
	h := wifHTTPHandler()
	h.Type = omniav1alpha1.HandlerTypeGRPC
	h.HTTPConfig, h.GRPCConfig = nil, &omniav1alpha1.GRPCConfig{Endpoint: "x:1"}
	if err := validateHandlerAuth(&h); err == nil {
		t.Fatal("gRPC WIF should be rejected in this milestone (http only)")
	}
}

func TestValidateHandlerAuth_RejectsMCPWIFWithStrayHTTPConfig(t *testing.T) {
	// A handler declared type: mcp that also carries a stray non-nil HTTPConfig
	// must still be rejected: buildHandlerEntry dispatches on h.Type (mcp), so the
	// MCP builder runs and never applies workloadIdentityFieldsFor — allowing it
	// would silently drop the auth header (fail-open). The guard keys off h.Type.
	h := wifHTTPHandler()
	h.Type = omniav1alpha1.HandlerTypeMCP
	h.MCPConfig = &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportStreamableHTTP}
	// HTTPConfig deliberately left non-nil (the stray block).
	if err := validateHandlerAuth(&h); err == nil {
		t.Fatal("MCP WIF with a stray HTTPConfig should be rejected (fail-closed)")
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

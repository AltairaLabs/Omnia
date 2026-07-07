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
	"strings"
	"testing"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func saHandler(name, audience string) omniav1alpha1.HandlerDefinition {
	return omniav1alpha1.HandlerDefinition{
		Name: name, Type: omniav1alpha1.HandlerTypeHTTP,
		HTTPConfig: &omniav1alpha1.HTTPConfig{Endpoint: "https://svc"},
		Auth: &omniav1alpha1.ToolAuth{
			Type:           omniav1alpha1.ToolAuthTypeServiceAccount,
			ServiceAccount: &omniav1alpha1.ToolAuthServiceAccount{Audience: audience},
		},
	}
}

func TestCollectToolSAHandlers(t *testing.T) {
	tr := &omniav1alpha1.ToolRegistry{Spec: omniav1alpha1.ToolRegistrySpec{Handlers: []omniav1alpha1.HandlerDefinition{
		saHandler("sa1", "aud-1"),
		{Name: "bearer", Type: omniav1alpha1.HandlerTypeHTTP, HTTPConfig: &omniav1alpha1.HTTPConfig{Endpoint: "x"},
			Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer, SecretRef: &omniav1alpha1.SecretKeySelector{Name: "s", Key: "k"}}},
		saHandler("sa2", "aud-2"),
	}}}
	got := collectToolSAHandlers(tr)
	if len(got) != 2 {
		t.Fatalf("want 2 SA handlers, got %d (%+v)", len(got), got)
	}
	if got[0].handler != "sa1" || got[0].audience != "aud-1" || got[1].handler != "sa2" || got[1].audience != "aud-2" {
		t.Errorf("unexpected SA handlers: %+v", got)
	}
}

func TestToolSATokenVolume(t *testing.T) {
	tr := &omniav1alpha1.ToolRegistry{Spec: omniav1alpha1.ToolRegistrySpec{Handlers: []omniav1alpha1.HandlerDefinition{
		saHandler("sa1", "aud-1"),
	}}}
	vol, ok := toolSATokenVolume(tr)
	if !ok {
		t.Fatal("expected a projected volume")
	}
	if vol.Projected == nil || len(vol.Projected.Sources) != 1 {
		t.Fatalf("want 1 projection, got %+v", vol.Projected)
	}
	proj := vol.Projected.Sources[0].ServiceAccountToken
	if proj == nil || proj.Audience != "aud-1" || proj.Path != "sa1/"+toolSATokenFileName {
		t.Errorf("unexpected projection: %+v", proj)
	}

	if _, ok := toolSATokenVolume(&omniav1alpha1.ToolRegistry{}); ok {
		t.Error("no SA handlers should yield no volume")
	}
}

func TestAuthFieldsFor_ServiceAccount(t *testing.T) {
	h := saHandler("sa1", "aud-1")
	authType, path, ok := authFieldsFor(&h)
	if !ok {
		t.Fatal("expected auth fields for a serviceAccount handler")
	}
	if authType != omniav1alpha1.ToolAuthTypeBearer {
		t.Errorf("SA auth applied as %q, want bearer", authType)
	}
	if path != toolSATokenMountBase+"/sa1/"+toolSATokenFileName {
		t.Errorf("token path = %q", path)
	}
}

func TestBuildGRPCConfig_SetsAuth(t *testing.T) {
	h := &omniav1alpha1.HandlerDefinition{
		Name: "g1", Type: omniav1alpha1.HandlerTypeGRPC,
		GRPCConfig: &omniav1alpha1.GRPCConfig{Endpoint: "svc:9000"},
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer,
			SecretRef: &omniav1alpha1.SecretKeySelector{Name: "s", Key: "token"}},
	}
	cfg, err := buildGRPCConfig(h, "svc:9000")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cfg.AuthType != "bearer" || cfg.AuthTokenPath != ToolSecretsMountPath+"/g1" {
		t.Errorf("gRPC auth fields = %q / %q", cfg.AuthType, cfg.AuthTokenPath)
	}
}

func TestBuildMCPConfig_SetsAuth(t *testing.T) {
	endpoint := "https://mcp.example"
	h := &omniav1alpha1.HandlerDefinition{
		Name: "m1", Type: omniav1alpha1.HandlerTypeMCP,
		MCPConfig: &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportStreamableHTTP, Endpoint: &endpoint},
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeServiceAccount,
			ServiceAccount: &omniav1alpha1.ToolAuthServiceAccount{Audience: "aud"}},
	}
	cfg, err := buildMCPConfig(h)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cfg.AuthType != "bearer" || cfg.AuthTokenPath != toolSATokenPath("m1") {
		t.Errorf("MCP auth fields = %q / %q", cfg.AuthType, cfg.AuthTokenPath)
	}
}

func TestValidateToolAuthTypes(t *testing.T) {
	wiHandler := func(name string) omniav1alpha1.HandlerDefinition {
		return omniav1alpha1.HandlerDefinition{
			Name: name, Type: omniav1alpha1.HandlerTypeHTTP, HTTPConfig: &omniav1alpha1.HTTPConfig{Endpoint: "x"},
			Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeWorkloadIdentity,
				WorkloadIdentity: &omniav1alpha1.ToolAuthWorkloadIdentity{Cloud: "azure", Audience: "a"}},
		}
	}
	wiProvider := map[string]*omniav1alpha1.Provider{
		"p": {Spec: omniav1alpha1.ProviderSpec{Auth: &omniav1alpha1.AuthConfig{Type: omniav1alpha1.AuthMethodWorkloadIdentity}}},
	}

	t.Run("nil registry is fine", func(t *testing.T) {
		if err := validateToolAuthTypes(nil, nil); err != nil {
			t.Errorf("unexpected: %v", err)
		}
	})
	t.Run("serviceAccount is allowed", func(t *testing.T) {
		tr := &omniav1alpha1.ToolRegistry{Spec: omniav1alpha1.ToolRegistrySpec{Handlers: []omniav1alpha1.HandlerDefinition{saHandler("sa", "a")}}}
		if err := validateToolAuthTypes(tr, nil); err != nil {
			t.Errorf("serviceAccount should be allowed: %v", err)
		}
	})
	t.Run("workloadIdentity rejected (no provider collision)", func(t *testing.T) {
		tr := &omniav1alpha1.ToolRegistry{Spec: omniav1alpha1.ToolRegistrySpec{Handlers: []omniav1alpha1.HandlerDefinition{wiHandler("wi")}}}
		err := validateToolAuthTypes(tr, nil)
		if err == nil || !strings.Contains(err.Error(), "not yet available") {
			t.Errorf("want not-yet-available rejection, got %v", err)
		}
	})
	t.Run("workloadIdentity rejected with provider collision message", func(t *testing.T) {
		tr := &omniav1alpha1.ToolRegistry{Spec: omniav1alpha1.ToolRegistrySpec{Handlers: []omniav1alpha1.HandlerDefinition{wiHandler("wi")}}}
		err := validateToolAuthTypes(tr, wiProvider)
		if err == nil || !strings.Contains(err.Error(), "already uses workload identity") {
			t.Errorf("want collision message, got %v", err)
		}
	})
	t.Run("stdio MCP with auth rejected", func(t *testing.T) {
		tr := &omniav1alpha1.ToolRegistry{Spec: omniav1alpha1.ToolRegistrySpec{Handlers: []omniav1alpha1.HandlerDefinition{{
			Name: "mcp", Type: omniav1alpha1.HandlerTypeMCP,
			MCPConfig: &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportStdio},
			Auth:      &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer, SecretRef: &omniav1alpha1.SecretKeySelector{Name: "s", Key: "k"}},
		}}}}
		err := validateToolAuthTypes(tr, nil)
		if err == nil || !strings.Contains(err.Error(), "stdio MCP") {
			t.Errorf("want stdio-MCP rejection, got %v", err)
		}
	})
}

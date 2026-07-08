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

package tools

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/go-logr/logr"

	toolsv1 "github.com/altairalabs/omnia/pkg/tools/v1"
)

// --- resolveGRPCAuth (unit) ---

func TestResolveGRPCAuth_WorkloadIdentity(t *testing.T) {
	e := &OmniaExecutor{tokenAcquirer: fakeAcquirer{tok: "wtok"}}
	cfg := &GRPCCfg{AuthType: authTypeWorkloadIdentity, AuthCloud: "azure", AuthAudience: "api://tool"}
	name, val, err := e.resolveGRPCAuth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveGRPCAuth: %v", err)
	}
	if name != "authorization" || val != "Bearer wtok" {
		t.Fatalf("got (%q,%q), want (\"authorization\",\"Bearer wtok\")", name, val)
	}
}

func TestResolveGRPCAuth_WorkloadIdentity_CustomHeaderLowercased(t *testing.T) {
	e := &OmniaExecutor{tokenAcquirer: fakeAcquirer{tok: "wtok"}}
	cfg := &GRPCCfg{AuthType: authTypeWorkloadIdentity, AuthCloud: "azure", AuthAudience: "api://tool", AuthHeader: "X-Tool-Auth"}
	name, _, err := e.resolveGRPCAuth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveGRPCAuth: %v", err)
	}
	if name != "x-tool-auth" {
		t.Fatalf("header name = %q, want lowercased \"x-tool-auth\"", name)
	}
}

func TestResolveGRPCAuth_WorkloadIdentity_FailsLoudNoAcquirer(t *testing.T) {
	e := &OmniaExecutor{}
	cfg := &GRPCCfg{AuthType: authTypeWorkloadIdentity, AuthCloud: "azure", AuthAudience: "api://tool"}
	if _, _, err := e.resolveGRPCAuth(context.Background(), cfg); err == nil {
		t.Fatal("expected error when no tokenAcquirer is configured")
	}
}

func TestResolveGRPCAuth_StaticBearer(t *testing.T) {
	e := &OmniaExecutor{}
	cfg := &GRPCCfg{AuthType: "bearer", AuthToken: "btok"}
	name, val, err := e.resolveGRPCAuth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveGRPCAuth: %v", err)
	}
	if name != "authorization" || val != "Bearer btok" {
		t.Fatalf("got (%q,%q), want (\"authorization\",\"Bearer btok\")", name, val)
	}
}

func TestResolveGRPCAuth_None(t *testing.T) {
	e := &OmniaExecutor{}
	name, val, err := e.resolveGRPCAuth(context.Background(), &GRPCCfg{})
	if err != nil || name != "" || val != "" {
		t.Fatalf("got (%q,%q,%v), want empty/no error", name, val, err)
	}
}

// --- executeGRPC WIF (end-to-end via mock client) ---

func TestOmniaExecutor_ExecuteGRPC_WorkloadIdentity(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	e.tokenAcquirer = fakeAcquirer{tok: "wtok"}
	mock := &mockToolServiceClient{executeResp: &toolsv1.ToolResponse{ResultJson: `{"ok":true}`}}
	e.grpcClients["grpc-handler"] = mock
	e.handlers["grpc-handler"] = &HandlerEntry{
		Name: "grpc-handler",
		Type: ToolTypeGRPC,
		GRPCConfig: &GRPCCfg{
			AuthType:     authTypeWorkloadIdentity,
			AuthCloud:    "azure",
			AuthAudience: "api://tool",
		},
	}

	_, err := e.executeGRPC(context.Background(), "grpc-tool", "grpc-handler", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("executeGRPC: %v", err)
	}
	got := mock.capturedMD.Get("authorization")
	if len(got) != 1 || got[0] != "Bearer wtok" {
		t.Fatalf("captured authorization metadata = %v, want [\"Bearer wtok\"]", got)
	}
}

func TestOmniaExecutor_ExecuteGRPC_WorkloadIdentity_FailsLoud(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	// No tokenAcquirer configured.
	mock := &mockToolServiceClient{executeResp: &toolsv1.ToolResponse{ResultJson: `{"ok":true}`}}
	e.grpcClients["grpc-handler"] = mock
	e.handlers["grpc-handler"] = &HandlerEntry{
		Name: "grpc-handler",
		Type: ToolTypeGRPC,
		GRPCConfig: &GRPCCfg{
			AuthType:     authTypeWorkloadIdentity,
			AuthCloud:    "azure",
			AuthAudience: "api://tool",
		},
	}

	_, err := e.executeGRPC(context.Background(), "grpc-tool", "grpc-handler", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error when no tokenAcquirer is configured")
	}
}

func TestOmniaExecutor_ExecuteGRPC_StaticBearerStillWorks(t *testing.T) {
	e := NewOmniaExecutor(logr.Discard(), nil)
	mock := &mockToolServiceClient{executeResp: &toolsv1.ToolResponse{ResultJson: `{"ok":true}`}}
	e.grpcClients["grpc-handler"] = mock
	e.handlers["grpc-handler"] = &HandlerEntry{
		Name: "grpc-handler",
		Type: ToolTypeGRPC,
		GRPCConfig: &GRPCCfg{
			AuthType:  "bearer",
			AuthToken: "btok",
		},
	}

	_, err := e.executeGRPC(context.Background(), "grpc-tool", "grpc-handler", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("executeGRPC: %v", err)
	}
	got := mock.capturedMD.Get("authorization")
	if len(got) != 1 || got[0] != "Bearer btok" {
		t.Fatalf("captured authorization metadata = %v, want [\"Bearer btok\"]", got)
	}
}

// TestResolveGRPCAuth_FileToken_Reread proves the gRPC auth path re-reads the
// token file each call, so a rotated projected serviceAccount token is used
// rather than a value cached at startup (#1797).
func TestResolveGRPCAuth_FileToken_Reread(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/token"
	if err := os.WriteFile(path, []byte("tok-A"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := &OmniaExecutor{}
	cfg := &GRPCCfg{AuthType: "bearer", AuthTokenPath: path}

	_, val, err := e.resolveGRPCAuth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveGRPCAuth: %v", err)
	}
	if val != "Bearer tok-A" {
		t.Fatalf("first: got %q, want Bearer tok-A", val)
	}
	if err := os.WriteFile(path, []byte("tok-B"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, val, err = e.resolveGRPCAuth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("resolveGRPCAuth: %v", err)
	}
	if val != "Bearer tok-B" {
		t.Fatalf("second: got %q, want fresh Bearer tok-B", val)
	}
}

func TestResolveGRPCAuth_FileToken_MissingFileErrors(t *testing.T) {
	e := &OmniaExecutor{}
	cfg := &GRPCCfg{AuthType: "bearer", AuthTokenPath: "/nonexistent/token"}
	if _, _, err := e.resolveGRPCAuth(context.Background(), cfg); err == nil {
		t.Fatal("expected error when the token file is unreadable")
	}
}

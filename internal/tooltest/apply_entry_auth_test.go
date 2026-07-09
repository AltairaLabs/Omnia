/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package tooltest

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/runtime/tools"
)

func newAuthTester(t *testing.T, objs ...client.Object) *Tester {
	t.Helper()
	s := testScheme()
	c := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	return NewTester(c, zap.New(zap.UseDevMode(true)))
}

func bearerSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cred", Namespace: "default"},
		Data:       map[string][]byte{"token": []byte("secret-tok")},
	}
}

func TestApplyEntryAuth_GRPCBearer_SetsRuntimeAuth(t *testing.T) {
	tester := newAuthTester(t, bearerSecret())
	h := &omniav1alpha1.HandlerDefinition{
		Name: "g", Type: omniav1alpha1.HandlerTypeGRPC,
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer,
			SecretRef: &omniav1alpha1.SecretKeySelector{Name: "cred", Key: "token"}},
	}
	entry := &tools.HandlerEntry{GRPCConfig: &tools.GRPCCfg{Endpoint: "svc:50051"}}

	warn, err := tester.applyEntryAuth(context.Background(), "default", h, entry)
	if err != nil {
		t.Fatalf("applyEntryAuth: %v", err)
	}
	if warn != "" {
		t.Fatalf("unexpected warning: %q", warn)
	}
	if entry.GRPCConfig.AuthType != "bearer" || entry.GRPCConfig.AuthToken != "secret-tok" {
		t.Fatalf("gRPC auth not set: type=%q token=%q", entry.GRPCConfig.AuthType, entry.GRPCConfig.AuthToken)
	}
}

func TestApplyEntryAuth_MCPBearer_SetsRuntimeAuth(t *testing.T) {
	tester := newAuthTester(t, bearerSecret())
	h := &omniav1alpha1.HandlerDefinition{
		Name: "m", Type: omniav1alpha1.HandlerTypeMCP,
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer,
			SecretRef: &omniav1alpha1.SecretKeySelector{Name: "cred", Key: "token"}},
	}
	entry := &tools.HandlerEntry{MCPConfig: &tools.MCPCfg{Transport: "sse"}}

	warn, err := tester.applyEntryAuth(context.Background(), "default", h, entry)
	if err != nil {
		t.Fatalf("applyEntryAuth: %v", err)
	}
	if warn != "" {
		t.Fatalf("unexpected warning: %q", warn)
	}
	if entry.MCPConfig.AuthType != "bearer" || entry.MCPConfig.AuthToken != "secret-tok" {
		t.Fatalf("MCP auth not set: type=%q token=%q", entry.MCPConfig.AuthType, entry.MCPConfig.AuthToken)
	}
}

func TestApplyEntryAuth_ServiceAccount_Warns(t *testing.T) {
	tester := newAuthTester(t)
	h := &omniav1alpha1.HandlerDefinition{
		Name: "s", Type: omniav1alpha1.HandlerTypeHTTP,
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeServiceAccount,
			ServiceAccount: &omniav1alpha1.ToolAuthServiceAccount{Audience: "api://x"}},
	}
	entry := &tools.HandlerEntry{HTTPConfig: &tools.HTTPCfg{}}

	warn, err := tester.applyEntryAuth(context.Background(), "default", h, entry)
	if err != nil {
		t.Fatalf("applyEntryAuth: %v", err)
	}
	if warn == "" {
		t.Fatal("expected a warning for serviceAccount auth")
	}
}

func TestApplyEntryAuth_WorkloadIdentity_Warns(t *testing.T) {
	tester := newAuthTester(t)
	h := &omniav1alpha1.HandlerDefinition{
		Name: "w", Type: omniav1alpha1.HandlerTypeGRPC,
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeWorkloadIdentity,
			WorkloadIdentity: &omniav1alpha1.ToolAuthWorkloadIdentity{Cloud: "azure", Audience: "api://x"}},
	}
	entry := &tools.HandlerEntry{GRPCConfig: &tools.GRPCCfg{}}

	warn, err := tester.applyEntryAuth(context.Background(), "default", h, entry)
	if err != nil {
		t.Fatalf("applyEntryAuth: %v", err)
	}
	if warn == "" {
		t.Fatal("expected a warning for workloadIdentity auth")
	}
	if entry.GRPCConfig.AuthType != "" {
		t.Fatalf("workloadIdentity must not set a static credential, got %q", entry.GRPCConfig.AuthType)
	}
}

func TestApplyEntryAuth_None_NoWarnNoAuth(t *testing.T) {
	tester := newAuthTester(t)
	h := &omniav1alpha1.HandlerDefinition{Name: "n", Type: omniav1alpha1.HandlerTypeGRPC}
	entry := &tools.HandlerEntry{GRPCConfig: &tools.GRPCCfg{}}

	warn, err := tester.applyEntryAuth(context.Background(), "default", h, entry)
	if err != nil {
		t.Fatalf("applyEntryAuth: %v", err)
	}
	if warn != "" || entry.GRPCConfig.AuthType != "" {
		t.Fatalf("expected no-op for no auth; warn=%q type=%q", warn, entry.GRPCConfig.AuthType)
	}
}

func TestApplyEntryAuth_HTTPBearer_LeftToResolveAuthSecrets(t *testing.T) {
	// HTTP bearer is handled by resolveAuthSecrets (header injection); applyEntryAuth
	// must not double-apply or warn.
	tester := newAuthTester(t, bearerSecret())
	h := &omniav1alpha1.HandlerDefinition{
		Name: "h", Type: omniav1alpha1.HandlerTypeHTTP,
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer,
			SecretRef: &omniav1alpha1.SecretKeySelector{Name: "cred", Key: "token"}},
	}
	entry := &tools.HandlerEntry{HTTPConfig: &tools.HTTPCfg{}}

	warn, err := tester.applyEntryAuth(context.Background(), "default", h, entry)
	if err != nil {
		t.Fatalf("applyEntryAuth: %v", err)
	}
	if warn != "" || entry.HTTPConfig.AuthType != "" {
		t.Fatalf("HTTP bearer should be a no-op here; warn=%q type=%q", warn, entry.HTTPConfig.AuthType)
	}
}

func TestApplyEntryAuth_MCPStdioWithAuth_WarnsNoInertCredential(t *testing.T) {
	tester := newAuthTester(t, bearerSecret())
	cmd := "/usr/local/bin/mcp"
	h := &omniav1alpha1.HandlerDefinition{
		Name: "s", Type: omniav1alpha1.HandlerTypeMCP,
		MCPConfig: &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportStdio, Command: &cmd},
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer,
			SecretRef: &omniav1alpha1.SecretKeySelector{Name: "cred", Key: "token"}},
	}
	entry := &tools.HandlerEntry{MCPConfig: &tools.MCPCfg{Transport: "stdio"}}

	warn, err := tester.applyEntryAuth(context.Background(), "default", h, entry)
	if err != nil {
		t.Fatalf("applyEntryAuth: %v", err)
	}
	if warn == "" {
		t.Fatal("expected a warning for stdio MCP + auth")
	}
	if entry.MCPConfig.AuthType != "" {
		t.Fatalf("stdio MCP must not get an inert credential, got %q", entry.MCPConfig.AuthType)
	}
}

func TestApplyEntryAuth_GRPCBearer_MissingSecretErrors(t *testing.T) {
	tester := newAuthTester(t) // no secret objects
	h := &omniav1alpha1.HandlerDefinition{
		Name: "g", Type: omniav1alpha1.HandlerTypeGRPC,
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer,
			SecretRef: &omniav1alpha1.SecretKeySelector{Name: "missing", Key: "token"}},
	}
	entry := &tools.HandlerEntry{GRPCConfig: &tools.GRPCCfg{}}

	if _, err := tester.applyEntryAuth(context.Background(), "default", h, entry); err == nil {
		t.Fatal("expected an error when the referenced secret is missing")
	}
}

func TestApplyEntryAuth_MCPBearer_MissingSecretErrors(t *testing.T) {
	tester := newAuthTester(t)
	h := &omniav1alpha1.HandlerDefinition{
		Name: "m", Type: omniav1alpha1.HandlerTypeMCP,
		MCPConfig: &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportSSE},
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer,
			SecretRef: &omniav1alpha1.SecretKeySelector{Name: "missing", Key: "token"}},
	}
	entry := &tools.HandlerEntry{MCPConfig: &tools.MCPCfg{Transport: "sse"}}
	if _, err := tester.applyEntryAuth(context.Background(), "default", h, entry); err == nil {
		t.Fatal("expected an error when the referenced secret is missing")
	}
}

func TestApplyEntryAuth_BearerNoSecretRef_NoOp(t *testing.T) {
	tester := newAuthTester(t)
	h := &omniav1alpha1.HandlerDefinition{
		Name: "g", Type: omniav1alpha1.HandlerTypeGRPC,
		Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer}, // no SecretRef
	}
	entry := &tools.HandlerEntry{GRPCConfig: &tools.GRPCCfg{}}
	warn, err := tester.applyEntryAuth(context.Background(), "default", h, entry)
	if err != nil || warn != "" || entry.GRPCConfig.AuthType != "" {
		t.Fatalf("expected no-op; warn=%q err=%v type=%q", warn, err, entry.GRPCConfig.AuthType)
	}
}

func TestApplyEntryAuth_BearerNilEntryConfigs_NoOp(t *testing.T) {
	tester := newAuthTester(t, bearerSecret())
	for _, typ := range []omniav1alpha1.HandlerType{omniav1alpha1.HandlerTypeGRPC, omniav1alpha1.HandlerTypeMCP} {
		h := &omniav1alpha1.HandlerDefinition{
			Name: "x", Type: typ,
			MCPConfig: &omniav1alpha1.MCPClientConfig{Transport: omniav1alpha1.MCPTransportSSE},
			Auth: &omniav1alpha1.ToolAuth{Type: omniav1alpha1.ToolAuthTypeBearer,
				SecretRef: &omniav1alpha1.SecretKeySelector{Name: "cred", Key: "token"}},
		}
		entry := &tools.HandlerEntry{} // no GRPCConfig/MCPConfig set
		if _, err := tester.applyEntryAuth(context.Background(), "default", h, entry); err != nil {
			t.Fatalf("nil entry config for %s should be a no-op, got %v", typ, err)
		}
	}
}

func TestApplyEntryAuth_UnknownType_NoOp(t *testing.T) {
	tester := newAuthTester(t)
	h := &omniav1alpha1.HandlerDefinition{
		Name: "u", Type: omniav1alpha1.HandlerTypeHTTP,
		Auth: &omniav1alpha1.ToolAuth{Type: "some-future-type"},
	}
	entry := &tools.HandlerEntry{HTTPConfig: &tools.HTTPCfg{}}
	warn, err := tester.applyEntryAuth(context.Background(), "default", h, entry)
	if err != nil || warn != "" {
		t.Fatalf("unknown auth type should be a silent no-op; warn=%q err=%v", warn, err)
	}
}

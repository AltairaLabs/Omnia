package tools

import (
	"context"
	"fmt"
	"testing"
)

type fakeAcquirer struct {
	tok string
	err error
}

func (f fakeAcquirer) Token(context.Context, string) (string, error) { return f.tok, f.err }

func TestResolveWIF_Azure(t *testing.T) {
	name, val, err := resolveWorkloadIdentityHeader(context.Background(), fakeAcquirer{tok: "wtok"}, "azure", "api://tool", "")
	if err != nil || name != defaultAuthHeader || val != "Bearer wtok" {
		t.Fatalf("got (%q,%q,%v)", name, val, err)
	}
}

func TestResolveWIF_CustomHeader(t *testing.T) {
	name, _, _ := resolveWorkloadIdentityHeader(context.Background(), fakeAcquirer{tok: "x"}, "azure", "api://tool", "X-Tool-Auth")
	if name != "X-Tool-Auth" {
		t.Fatalf("header override ignored: %q", name)
	}
}

func TestResolveWIF_UnsupportedCloud(t *testing.T) {
	if _, _, err := resolveWorkloadIdentityHeader(context.Background(), fakeAcquirer{tok: "x"}, "aws", "a", ""); err == nil {
		t.Fatal("expected error for non-azure cloud")
	}
}

func TestResolveWIF_AcquireFailsLoud(t *testing.T) {
	if _, _, err := resolveWorkloadIdentityHeader(context.Background(), fakeAcquirer{err: fmt.Errorf("boom")}, "azure", "a", ""); err == nil {
		t.Fatal("expected error when acquisition fails")
	}
}

func TestResolveWIF_NilAcquirerFailsLoud(t *testing.T) {
	if _, _, err := resolveWorkloadIdentityHeader(context.Background(), nil, "azure", "a", ""); err == nil {
		t.Fatal("expected error when acquirer is nil")
	}
}

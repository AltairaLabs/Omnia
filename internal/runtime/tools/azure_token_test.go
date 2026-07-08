package tools

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

type fakeCred struct {
	calls int
	tok   string
	exp   time.Time
	err   error
}

func (f *fakeCred) GetToken(context.Context, policy.TokenRequestOptions) (azcore.AccessToken, error) {
	f.calls++
	if f.err != nil {
		return azcore.AccessToken{}, f.err
	}
	return azcore.AccessToken{Token: f.tok, ExpiresOn: f.exp}, nil
}

func TestAzureTokenAcquirer_CachesPerAudience(t *testing.T) {
	fc := &fakeCred{tok: "abc", exp: time.Now().Add(time.Hour)}
	a := &azureTokenAcquirer{cred: fc, cache: map[string]cachedToken{}}
	for i := 0; i < 3; i++ {
		got, err := a.Token(context.Background(), "api://tool")
		if err != nil || got != "abc" {
			t.Fatalf("got %q, %v", got, err)
		}
	}
	if fc.calls != 1 {
		t.Fatalf("expected 1 credential call (cached), got %d", fc.calls)
	}
}

func TestAzureTokenAcquirer_RefreshesNearExpiry(t *testing.T) {
	fc := &fakeCred{tok: "abc", exp: time.Now().Add(time.Minute)}
	a := &azureTokenAcquirer{cred: fc, cache: map[string]cachedToken{}}
	for i := 0; i < 3; i++ {
		got, err := a.Token(context.Background(), "api://tool")
		if err != nil || got != "abc" {
			t.Fatalf("got %q, %v", got, err)
		}
	}
	if fc.calls != 3 {
		t.Fatalf("expected 3 credential calls (refreshed near expiry), got %d", fc.calls)
	}
}

func TestAzureTokenAcquirer_FailsLoudOnCredError(t *testing.T) {
	credErr := errors.New("boom")
	fc := &fakeCred{err: credErr}
	a := &azureTokenAcquirer{cred: fc, cache: map[string]cachedToken{}}

	got, err := a.Token(context.Background(), "api://tool")
	if got != "" {
		t.Fatalf("expected empty token on error, got %q", got)
	}
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !errors.Is(err, credErr) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
	if !strings.Contains(err.Error(), "api://tool") {
		t.Fatalf("expected error to name the audience, got %v", err)
	}
}

func TestNewAzureTokenAcquirer_Success(t *testing.T) {
	fc := &fakeCred{tok: "abc", exp: time.Now().Add(time.Hour)}
	orig := newDefaultAzureCredential
	defer func() { newDefaultAzureCredential = orig }()
	newDefaultAzureCredential = func(*azidentity.DefaultAzureCredentialOptions) (azcore.TokenCredential, error) {
		return fc, nil
	}

	a, err := newAzureTokenAcquirer()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a == nil || a.cred != fc || a.cache == nil {
		t.Fatalf("acquirer not wired: %#v", a)
	}
}

func TestNewAzureTokenAcquirer_CredError(t *testing.T) {
	credErr := errors.New("no identity")
	orig := newDefaultAzureCredential
	defer func() { newDefaultAzureCredential = orig }()
	newDefaultAzureCredential = func(*azidentity.DefaultAzureCredentialOptions) (azcore.TokenCredential, error) {
		return nil, credErr
	}

	a, err := newAzureTokenAcquirer()
	if a != nil {
		t.Fatalf("expected nil acquirer on error, got %#v", a)
	}
	if !errors.Is(err, credErr) {
		t.Fatalf("expected wrapped underlying error, got %v", err)
	}
}

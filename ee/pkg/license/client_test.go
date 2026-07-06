/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package license

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// licenseServer spins up an httptest server that serves the given license at
// /api/v1/license (the arena-controller DTO shape), counting hits.
func licenseServer(t *testing.T, lic *License) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		assert.Equal(t, LicensePath, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":        lic.ID,
			"tier":      lic.Tier,
			"customer":  lic.Customer,
			"features":  lic.Features,
			"limits":    lic.Limits,
			"issuedAt":  lic.IssuedAt.Format(time.RFC3339),
			"expiresAt": lic.ExpiresAt.Format(time.RFC3339),
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestNewClient_AppliesOptions(t *testing.T) {
	srv, _ := licenseServer(t, DevLicense())
	custom := &http.Client{Timeout: 42 * time.Second}
	c := NewClient(srv.URL,
		WithClientHTTP(custom),
		WithClientLogger(logr.Discard()),
		WithClientTTL(2*time.Minute),
	)

	assert.Same(t, custom, c.httpC, "WithClientHTTP should install the provided client")
	assert.Equal(t, 2*time.Minute, c.ttl)
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	// A trailing slash in the base URL must not produce a `//api/v1/license`
	// path that silently 404s and degrades a validly-licensed service.
	srv, hits := licenseServer(t, DevLicense())
	c := NewClient(srv.URL + "/")

	_, err := c.Refresh(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(hits))
	assert.True(t, c.License().CanUseMemoryEnterprise())
}

func TestClient_License_BeforeFetchIsOpenCore(t *testing.T) {
	c := NewClient("http://unused")
	lic := c.License()
	require.NotNil(t, lic)
	assert.Equal(t, TierOpenCore, lic.Tier)
	assert.False(t, lic.CanUseMemoryEnterprise())
}

func TestClient_Refresh_Enterprise(t *testing.T) {
	srv, _ := licenseServer(t, DevLicense())
	c := NewClient(srv.URL)

	lic, err := c.Refresh(context.Background())
	require.NoError(t, err)
	assert.True(t, lic.CanUseMemoryEnterprise())
	assert.True(t, lic.CanUsePrivacyEnterprise())
	assert.True(t, lic.CanUseToolPolicy())
	// Cache reflects the fetch.
	assert.True(t, c.License().CanUseMemoryEnterprise())
}

func TestClient_Refresh_OpenCore(t *testing.T) {
	srv, _ := licenseServer(t, OpenCoreLicense())
	c := NewClient(srv.URL)

	_, err := c.Refresh(context.Background())
	require.NoError(t, err)
	assert.False(t, c.License().CanUseMemoryEnterprise())
	assert.False(t, c.License().CanUsePrivacyEnterprise())
	assert.False(t, c.License().CanUseToolPolicy())
}

func TestClient_Refresh_Expired(t *testing.T) {
	expired := DevLicense()
	expired.ExpiresAt = time.Now().Add(-1 * time.Hour)
	srv, _ := licenseServer(t, expired)
	c := NewClient(srv.URL)

	_, err := c.Refresh(context.Background())
	require.NoError(t, err)
	// Feature bits are set, but expiry must be observable so the gate degrades.
	assert.True(t, c.License().IsExpired())
}

func TestClient_Refresh_ServerErrorReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)

	_, err := c.Refresh(context.Background())
	assert.Error(t, err, "a non-200 must surface as an error so startup can distinguish it")
	// Nothing was ever cached → License degrades to open-core.
	assert.Equal(t, TierOpenCore, c.License().Tier)
}

func TestClient_Refresh_UnreachableReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := NewClient(url)

	_, err := c.Refresh(context.Background())
	assert.Error(t, err)
	assert.Equal(t, TierOpenCore, c.License().Tier)
}

func TestClient_Refresh_BadJSONReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)

	_, err := c.Refresh(context.Background())
	assert.Error(t, err)
}

func TestClient_Refresh_ErrorKeepsLastGood(t *testing.T) {
	// First refresh succeeds (enterprise); a subsequent failure must leave the
	// cached enterprise license intact — last-good survives a transient outage.
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		lic := DevLicense()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tier": lic.Tier, "features": lic.Features,
			"issuedAt": lic.IssuedAt.Format(time.RFC3339), "expiresAt": lic.ExpiresAt.Format(time.RFC3339),
		})
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL)

	_, err := c.Refresh(context.Background())
	require.NoError(t, err)
	assert.True(t, c.License().CanUseMemoryEnterprise())

	fail.Store(true)
	_, err = c.Refresh(context.Background())
	assert.Error(t, err)
	assert.True(t, c.License().CanUseMemoryEnterprise(),
		"last-good license must survive a failed refresh")
}

func TestClient_License_ReturnsIndependentCopy(t *testing.T) {
	// A caller mutating the returned license must not corrupt the shared cache.
	srv, _ := licenseServer(t, DevLicense())
	c := NewClient(srv.URL)
	_, err := c.Refresh(context.Background())
	require.NoError(t, err)

	got := c.License()
	got.Features.MemoryEnterprise = false // mutate the caller's copy

	assert.True(t, c.License().CanUseMemoryEnterprise(),
		"cache must be unaffected by a caller mutating a returned license")
}

func TestClient_Start_RefreshesInBackground(t *testing.T) {
	srv, hits := licenseServer(t, DevLicense())
	c := NewClient(srv.URL, WithClientTTL(15*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c.Start(ctx)

	assert.Eventually(t, func() bool {
		return atomic.LoadInt32(hits) >= 1 && c.License().CanUseMemoryEnterprise()
	}, 2*time.Second, 5*time.Millisecond, "background refresher should warm the cache")
}

func TestClient_ConcurrentRefreshAndReadIsRaceFree(t *testing.T) {
	// Exercised under -race: concurrent Refresh + License must not race and the
	// serialized fetches must not panic.
	srv, _ := licenseServer(t, DevLicense())
	c := NewClient(srv.URL)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Refresh(context.Background())
			_ = c.License().CanUseMemoryEnterprise()
		}()
	}
	wg.Wait()
	assert.True(t, c.License().CanUseMemoryEnterprise())
}

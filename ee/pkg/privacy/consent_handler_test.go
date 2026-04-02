/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/internal/session/api"
)

// mockConsentAuditLogger captures emitted audit events for assertions.
type mockConsentAuditLogger struct {
	events []*api.AuditEntry
}

func (m *mockConsentAuditLogger) LogEvent(_ context.Context, entry *api.AuditEntry) {
	m.events = append(m.events, entry)
}

func (m *mockConsentAuditLogger) Close() error {
	return nil
}

// newTestConsentHandler builds a ConsentHandler backed by a prefsMockPool.
func newTestConsentHandler(pool dbPool, audit api.AuditLogger) *ConsentHandler {
	log := zap.New(zap.UseDevMode(true))
	store := NewPreferencesStore(pool)
	return NewConsentHandler(store, audit, log)
}

// consentHandlerViaRouter registers routes on a ServeMux and serves a request,
// returning the recorder. This exercises PathValue routing.
func serveConsentRequest(h *ConsentHandler, method, target string, body []byte) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// successExecPool returns a pool whose Exec always succeeds and QueryRow scans the provided grants.
func successExecPool(responseGrants []string) *prefsMockPool {
	return &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*[]string) = responseGrants
				return nil
			}}
		},
	}
}

// ---- PUT /consent tests ----

func TestConsentHandlerPUT_ValidGrants(t *testing.T) {
	pool := successExecPool([]string{string(ConsentMemoryIdentity)})
	h := newTestConsentHandler(pool, nil)

	body, _ := json.Marshal(ConsentRequest{
		Grants: []ConsentCategory{ConsentMemoryIdentity},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ConsentResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Contains(t, resp.Grants, ConsentMemoryIdentity)
	assert.NotEmpty(t, resp.Defaults)
}

func TestConsentHandlerPUT_ValidRevocations(t *testing.T) {
	// Exec returns UPDATE 1 for revocation; QueryRow returns no grants.
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 1"), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*[]string) = []string{}
				return nil
			}}
		},
	}
	h := newTestConsentHandler(pool, nil)

	body, _ := json.Marshal(ConsentRequest{
		Revocations: []ConsentCategory{ConsentMemoryIdentity},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp ConsentResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Empty(t, resp.Grants)
}

func TestConsentHandlerPUT_RevocationIgnoresNotFound(t *testing.T) {
	// removeArrayElement returns UPDATE 0 → ErrPreferencesNotFound, handler must swallow it.
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("UPDATE 0"), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*[]string) = []string{}
				return nil
			}}
		},
	}
	h := newTestConsentHandler(pool, nil)

	body, _ := json.Marshal(ConsentRequest{
		Revocations: []ConsentCategory{ConsentMemoryLocation},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestConsentHandlerPUT_UnknownCategory(t *testing.T) {
	h := newTestConsentHandler(&prefsMockPool{}, nil)

	body, _ := json.Marshal(ConsentRequest{
		Grants: []ConsentCategory{"unknown:category"},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConsentHandlerPUT_UnknownCategoryInRevocations(t *testing.T) {
	h := newTestConsentHandler(&prefsMockPool{}, nil)

	body, _ := json.Marshal(ConsentRequest{
		Revocations: []ConsentCategory{"bad:cat"},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConsentHandlerPUT_EmptyUserID(t *testing.T) {
	// Route won't match with empty segment — use handleSetConsent directly.
	h := newTestConsentHandler(&prefsMockPool{}, nil)
	log := zap.New(zap.UseDevMode(true))
	h.log = log

	req := httptest.NewRequest(http.MethodPut, "/api/v1/privacy/preferences//consent", nil)
	rec := httptest.NewRecorder()
	h.handleSetConsent(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConsentHandlerPUT_InvalidJSON(t *testing.T) {
	h := newTestConsentHandler(&prefsMockPool{}, nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	req := httptest.NewRequest(http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent",
		bytes.NewReader([]byte("not-json")))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConsentHandlerPUT_StoreErrorOnGrant(t *testing.T) {
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.CommandTag{}, assert.AnError
		},
	}
	h := newTestConsentHandler(pool, nil)

	body, _ := json.Marshal(ConsentRequest{
		Grants: []ConsentCategory{ConsentMemoryIdentity},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestConsentHandlerPUT_StoreErrorOnQueryAfterGrant(t *testing.T) {
	// Grant succeeds, but GetConsentGrants (QueryRow) fails.
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error {
				return assert.AnError
			}}
		},
	}
	h := newTestConsentHandler(pool, nil)

	body, _ := json.Marshal(ConsentRequest{
		Grants: []ConsentCategory{ConsentMemoryIdentity},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestConsentHandlerPUT_AuditEventsEmitted(t *testing.T) {
	pool := successExecPool([]string{
		string(ConsentMemoryIdentity),
		string(ConsentMemoryPreferences),
	})
	audit := &mockConsentAuditLogger{}
	h := newTestConsentHandler(pool, audit)

	body, _ := json.Marshal(ConsentRequest{
		Grants:      []ConsentCategory{ConsentMemoryIdentity, ConsentMemoryPreferences},
		Revocations: []ConsentCategory{ConsentMemoryLocation},
	})

	// Exec: 2 grants succeed (INSERT), then 1 revocation returns UPDATE 0 (ErrPreferencesNotFound, swallowed).
	callCount := 0
	pool.execFn = func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
		callCount++
		if callCount <= 2 {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		}
		// revocation: UPDATE 0 → ErrPreferencesNotFound, swallowed
		return pgconn.NewCommandTag("UPDATE 0"), nil
	}

	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, audit.events, 3)

	assert.Equal(t, "consent_granted", audit.events[0].EventType)
	assert.Equal(t, "user1", audit.events[0].Metadata["user_id"])
	assert.Equal(t, string(ConsentMemoryIdentity), audit.events[0].Metadata["category"])

	assert.Equal(t, "consent_granted", audit.events[1].EventType)
	assert.Equal(t, string(ConsentMemoryPreferences), audit.events[1].Metadata["category"])

	assert.Equal(t, "consent_revoked", audit.events[2].EventType)
	assert.Equal(t, string(ConsentMemoryLocation), audit.events[2].Metadata["category"])
}

func TestConsentHandlerPUT_NoAuditWhenLoggerNil(t *testing.T) {
	pool := successExecPool([]string{string(ConsentMemoryIdentity)})
	h := newTestConsentHandler(pool, nil) // nil audit logger

	body, _ := json.Marshal(ConsentRequest{
		Grants: []ConsentCategory{ConsentMemoryIdentity},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	// No panic and request succeeds.
	assert.Equal(t, http.StatusOK, rec.Code)
}

// ---- GET /consent tests ----

func TestConsentHandlerGET_WithGrants(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*[]string) = []string{
					string(ConsentMemoryIdentity),
				}
				return nil
			}}
		},
	}
	h := newTestConsentHandler(pool, nil)

	rec := serveConsentRequest(h, http.MethodGet,
		"/api/v1/privacy/preferences/user1/consent", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ConsentResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	// ConsentMemoryIdentity is explicit-grant, so it should appear in grants, not denied.
	assert.Contains(t, resp.Grants, ConsentMemoryIdentity)
	assert.NotContains(t, resp.Denied, ConsentMemoryIdentity)
	assert.NotContains(t, resp.Defaults, ConsentMemoryIdentity)

	// Default categories (requiresGrant=false) should appear in defaults.
	assert.Contains(t, resp.Defaults, ConsentMemoryPreferences)
	assert.Contains(t, resp.Defaults, ConsentMemoryContext)
	assert.Contains(t, resp.Defaults, ConsentMemoryHistory)

	// Explicit-grant categories that weren't granted go to denied.
	assert.Contains(t, resp.Denied, ConsentMemoryLocation)
	assert.Contains(t, resp.Denied, ConsentMemoryHealth)
}

func TestConsentHandlerGET_NoPreferences(t *testing.T) {
	// GetConsentGrants returns empty when user has no preferences row.
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error {
				return pgx.ErrNoRows
			}}
		},
	}
	h := newTestConsentHandler(pool, nil)

	rec := serveConsentRequest(h, http.MethodGet,
		"/api/v1/privacy/preferences/newuser/consent", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp ConsentResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))

	assert.Empty(t, resp.Grants)
	assert.NotEmpty(t, resp.Defaults)
	// All explicit-grant categories should be denied.
	assert.Contains(t, resp.Denied, ConsentMemoryIdentity)
	assert.Contains(t, resp.Denied, ConsentMemoryLocation)
	assert.Contains(t, resp.Denied, ConsentMemoryHealth)
	assert.Contains(t, resp.Denied, ConsentAnalyticsAggregate)
}

func TestConsentHandlerGET_StoreError(t *testing.T) {
	pool := &prefsMockPool{
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(_ ...any) error {
				return assert.AnError
			}}
		},
	}
	h := newTestConsentHandler(pool, nil)

	rec := serveConsentRequest(h, http.MethodGet,
		"/api/v1/privacy/preferences/user1/consent", nil)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestConsentHandlerGET_EmptyUserID(t *testing.T) {
	h := newTestConsentHandler(&prefsMockPool{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/privacy/preferences//consent", nil)
	rec := httptest.NewRecorder()
	h.handleGetConsent(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestConsentHandlerRegisterRoutes(t *testing.T) {
	pool := successExecPool([]string{})
	h := newTestConsentHandler(pool, nil)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// PUT
	body, _ := json.Marshal(ConsentRequest{})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPut,
		"/api/v1/privacy/preferences/u1/consent", bytes.NewReader(body)))
	assert.Equal(t, http.StatusOK, rec.Code)

	// GET
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet,
		"/api/v1/privacy/preferences/u1/consent", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestValidateCategories_Valid(t *testing.T) {
	err := validateCategories(
		[]ConsentCategory{ConsentMemoryIdentity, ConsentMemoryLocation},
		[]ConsentCategory{ConsentMemoryHealth},
	)
	assert.NoError(t, err)
}

func TestValidateCategories_InvalidInGrants(t *testing.T) {
	err := validateCategories([]ConsentCategory{"bad:cat"}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bad:cat")
}

func TestValidateCategories_InvalidInRevocations(t *testing.T) {
	err := validateCategories(nil, []ConsentCategory{"also:bad"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "also:bad")
}

func TestConsentHandlerPUT_MixedGrantsAndRevocations(t *testing.T) {
	execCalls := 0
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			execCalls++
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*[]string) = []string{string(ConsentMemoryIdentity)}
				return nil
			}}
		},
	}
	h := newTestConsentHandler(pool, nil)

	body, _ := json.Marshal(ConsentRequest{
		Grants:      []ConsentCategory{ConsentMemoryIdentity},
		Revocations: []ConsentCategory{ConsentMemoryLocation},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user1/consent", body)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 2, execCalls, "expected one exec per grant and one per revocation")
}

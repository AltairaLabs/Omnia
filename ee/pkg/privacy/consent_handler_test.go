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
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/internal/session/api"
)

// fakeOutboxID is the outbox row id returned by the test store fakes.
const fakeOutboxID = "fake-outbox-id"

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
	// RemoveConsentGrantWithOutbox: tx.Exec returns UPDATE 1, tx.QueryRow returns a fake outbox id.
	// No notifier → MarkOutboxDelivered is never called; pool.QueryRow returns no grants.
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
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return &mockPgxTx{
				execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
				queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
					return &prefsMockRow{scanFn: func(dest ...any) error {
						*dest[0].(*string) = fakeOutboxID
						return nil
					}}
				},
			}, nil
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
	// RemoveConsentGrantWithOutbox: tx.Exec returns UPDATE 0 (category not currently
	// granted). The outbox is a no-op: ("", nil) is returned and the handler must
	// continue without error, yielding 200.
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
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return &mockPgxTx{
				execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
					// No rows affected → no-op revocation, no outbox row.
					return pgconn.NewCommandTag("UPDATE 0"), nil
				},
			}, nil
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
	audit := &mockConsentAuditLogger{}
	// Grants use pool.Exec (INSERT); revocation of ConsentMemoryLocation goes through
	// RemoveConsentGrantWithOutbox → pool.Begin → tx.Exec. The category is not currently
	// granted, so tx.Exec returns UPDATE 0 (no-op → outboxID="" → no notification).
	pool := &prefsMockPool{
		execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
			return pgconn.NewCommandTag("INSERT 0 1"), nil
		},
		queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
			return &prefsMockRow{scanFn: func(dest ...any) error {
				*dest[0].(*[]string) = []string{
					string(ConsentMemoryIdentity),
					string(ConsentMemoryPreferences),
				}
				return nil
			}}
		},
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return &mockPgxTx{
				execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
					// Revocation: category not currently granted → no-op.
					return pgconn.NewCommandTag("UPDATE 0"), nil
				},
			}, nil
		},
	}
	h := newTestConsentHandler(pool, audit)

	body, _ := json.Marshal(ConsentRequest{
		Grants:      []ConsentCategory{ConsentMemoryIdentity, ConsentMemoryPreferences},
		Revocations: []ConsentCategory{ConsentMemoryLocation},
	})

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

// ---- ConsentNotifier integration tests ----

// spyNotifier records calls made to NotifyRevocation for assertion.
type spyNotifier struct {
	mu    sync.Mutex
	calls []notifyCall
}

type notifyCall struct {
	userID   string
	category ConsentCategory
}

func (s *spyNotifier) NotifyRevocation(_ context.Context, userID string, category ConsentCategory) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, notifyCall{userID: userID, category: category})
	return true, nil
}

func (s *spyNotifier) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *spyNotifier) callAt(i int) notifyCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls[i]
}

// TestConsentHandlerPUT_RevocationTriggersNotifier asserts that a successful
// revocation fires NotifyRevocation with the correct (userID, category).
func TestConsentHandlerPUT_RevocationTriggersNotifier(t *testing.T) {
	// pool.Exec: used by MarkOutboxDelivered after spy returns delivered=true.
	// pool.Begin/tx: used by RemoveConsentGrantWithOutbox.
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
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return &mockPgxTx{
				execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
				queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
					return &prefsMockRow{scanFn: func(dest ...any) error {
						*dest[0].(*string) = fakeOutboxID
						return nil
					}}
				},
			}, nil
		},
	}
	spy := &spyNotifier{}
	h := newTestConsentHandler(pool, nil).WithConsentNotifier(spy)

	body, _ := json.Marshal(ConsentRequest{
		Revocations: []ConsentCategory{ConsentMemoryIdentity},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user42/consent", body)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, 1, spy.callCount(), "expected one NotifyRevocation call")
	call := spy.callAt(0)
	assert.Equal(t, "user42", call.userID)
	assert.Equal(t, ConsentMemoryIdentity, call.category)
}

// TestConsentHandlerPUT_GrantDoesNotTriggerNotifier asserts that granting
// consent never calls NotifyRevocation.
func TestConsentHandlerPUT_GrantDoesNotTriggerNotifier(t *testing.T) {
	pool := successExecPool([]string{string(ConsentMemoryIdentity)})
	spy := &spyNotifier{}
	h := newTestConsentHandler(pool, nil).WithConsentNotifier(spy)

	body, _ := json.Marshal(ConsentRequest{
		Grants: []ConsentCategory{ConsentMemoryIdentity},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user7/consent", body)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, spy.callCount(), "grant must not trigger NotifyRevocation")
}

// TestConsentHandlerPUT_MultipleRevocationsNotifiedEach asserts that each
// revocation in a batch is individually forwarded to the notifier.
func TestConsentHandlerPUT_MultipleRevocationsNotifiedEach(t *testing.T) {
	// Two revocations: each calls pool.Begin independently; spy is called once per revocation.
	// spy returns delivered=true for each → MarkOutboxDelivered called twice via pool.Exec.
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
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return &mockPgxTx{
				execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
				queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
					return &prefsMockRow{scanFn: func(dest ...any) error {
						*dest[0].(*string) = fakeOutboxID
						return nil
					}}
				},
			}, nil
		},
	}
	spy := &spyNotifier{}
	h := newTestConsentHandler(pool, nil).WithConsentNotifier(spy)

	body, _ := json.Marshal(ConsentRequest{
		Revocations: []ConsentCategory{ConsentMemoryIdentity, ConsentMemoryLocation},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/userN/consent", body)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 2, spy.callCount(), "each revocation must trigger one notify call")
}

// TestConsentHandlerPUT_NotifierErrorIsSwallowed asserts that a notifier that
// returns an error does not fail the HTTP request (best-effort contract).
func TestConsentHandlerPUT_NotifierErrorIsSwallowed(t *testing.T) {
	// errorNotifier returns (false, error). The error is discarded; delivered=false
	// means MarkOutboxDelivered is skipped. The consent write must still return 200.
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
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return &mockPgxTx{
				execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
				queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
					return &prefsMockRow{scanFn: func(dest ...any) error {
						*dest[0].(*string) = fakeOutboxID
						return nil
					}}
				},
			}, nil
		},
	}
	// A notifier that always errors.
	errNotifier := errorNotifier{}
	h := newTestConsentHandler(pool, nil).WithConsentNotifier(errNotifier)

	body, _ := json.Marshal(ConsentRequest{
		Revocations: []ConsentCategory{ConsentMemoryIdentity},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/user99/consent", body)

	// The revocation store write succeeded; notifier error must not surface as HTTP 500.
	assert.Equal(t, http.StatusOK, rec.Code)
}

// errorNotifier always returns an error from NotifyRevocation.
type errorNotifier struct{}

func (errorNotifier) NotifyRevocation(_ context.Context, _ string, _ ConsentCategory) (bool, error) {
	return false, errors.New("simulated notifier failure")
}

// alwaysUndeliveredNotifier returns delivered=false without error — used to test
// that the outbox row is left undelivered when the notifier does not succeed.
type alwaysUndeliveredNotifier struct{}

func (alwaysUndeliveredNotifier) NotifyRevocation(_ context.Context, _ string, _ ConsentCategory) (bool, error) {
	return false, nil
}

func TestConsentHandlerPUT_MixedGrantsAndRevocations(t *testing.T) {
	// Grants use pool.Exec; revocations use pool.Begin → tx.Exec (transactional outbox).
	// No notifier → MarkOutboxDelivered not called; only one pool.Exec call (the grant).
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
		beginFn: func(_ context.Context) (pgx.Tx, error) {
			return &mockPgxTx{
				execFn: func(_ context.Context, _ string, _ ...any) (pgconn.CommandTag, error) {
					return pgconn.NewCommandTag("UPDATE 1"), nil
				},
				queryRowFn: func(_ context.Context, _ string, _ ...any) pgx.Row {
					return &prefsMockRow{scanFn: func(dest ...any) error {
						*dest[0].(*string) = fakeOutboxID
						return nil
					}}
				},
			}, nil
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
	assert.Equal(t, 1, execCalls, "grant uses pool.Exec; revocation uses pool.Begin (transactional outbox)")
}

// ---- outbox integration tests (real Postgres via testcontainers) ----

// TestConsentHandlerPUT_RevocationOutboxMarkedDelivered_RealPostgres verifies that
// when a revocation succeeds and the notifier returns delivered=true, the handler
// records an outbox row AND immediately marks it delivered. The PUT returns 200.
func TestConsentHandlerPUT_RevocationOutboxMarkedDelivered_RealPostgres(t *testing.T) {
	pool := outboxTestPool(t)
	const (
		userID   = "handler-outbox-delivered-user"
		category = ConsentMemoryIdentity
	)
	seedConsentGrant(t, pool, userID, category)

	spy := &spyNotifier{} // NotifyRevocation returns (true, nil)
	h := newTestConsentHandler(pool, nil).WithConsentNotifier(spy)

	body, _ := json.Marshal(ConsentRequest{
		Revocations: []ConsentCategory{category},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/"+userID+"/consent", body)

	require.Equal(t, http.StatusOK, rec.Code, "revocation must return 200")
	require.Equal(t, 1, spy.callCount(), "notifier must be called once")

	assert.Equal(t, int64(1), outboxRowCount(t, pool), "exactly one outbox row must be created")

	var deliveredAt *time.Time
	err := pool.QueryRow(
		context.Background(),
		`SELECT delivered_at FROM consent_revocation_outbox WHERE user_id = $1`, userID,
	).Scan(&deliveredAt)
	require.NoError(t, err)
	assert.NotNil(t, deliveredAt, "delivered_at must be set when notifier returns delivered=true")
}

// TestConsentHandlerPUT_RevocationOutboxLeftUndelivered_RealPostgres verifies that
// when the notifier returns delivered=false the outbox row is created but left
// undelivered (delivered_at is NULL). The PUT still returns 200 (best-effort).
func TestConsentHandlerPUT_RevocationOutboxLeftUndelivered_RealPostgres(t *testing.T) {
	pool := outboxTestPool(t)
	const (
		userID   = "handler-outbox-undelivered-user"
		category = ConsentMemoryIdentity
	)
	seedConsentGrant(t, pool, userID, category)

	h := newTestConsentHandler(pool, nil).WithConsentNotifier(alwaysUndeliveredNotifier{})

	body, _ := json.Marshal(ConsentRequest{
		Revocations: []ConsentCategory{category},
	})
	rec := serveConsentRequest(h, http.MethodPut,
		"/api/v1/privacy/preferences/"+userID+"/consent", body)

	require.Equal(t, http.StatusOK, rec.Code, "revocation must return 200 even when delivery fails")

	assert.Equal(t, int64(1), outboxRowCount(t, pool), "exactly one outbox row must be created")

	var deliveredAt *time.Time
	err := pool.QueryRow(
		context.Background(),
		`SELECT delivered_at FROM consent_revocation_outbox WHERE user_id = $1`, userID,
	).Scan(&deliveredAt)
	require.NoError(t, err)
	assert.Nil(t, deliveredAt, "delivered_at must remain NULL when notifier returns delivered=false")
}

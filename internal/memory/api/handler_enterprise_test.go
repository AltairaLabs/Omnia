package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

const (
	institutionalRoute     = "/api/v1/institutional/memories"
	enterpriseRequiredJSON = `{"error":"enterprise_required"}`
)

func TestRequireEnterprise_BlocksWhenOff(t *testing.T) {
	h := &Handler{enterprise: false, log: logr.Discard()}
	called := false
	guarded := h.requireEnterprise(func(http.ResponseWriter, *http.Request) { called = true })

	rec := httptest.NewRecorder()
	guarded(rec, httptest.NewRequest(http.MethodPost, institutionalRoute, nil))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, enterpriseRequiredJSON, rec.Body.String())
	assert.False(t, called, "guarded handler must not run when disabled")
}

func TestRequireEnterprise_PassesWhenOn(t *testing.T) {
	h := &Handler{enterprise: true, log: logr.Discard()}
	called := false
	guarded := h.requireEnterprise(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	guarded(rec, httptest.NewRequest(http.MethodPost, institutionalRoute, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}

func TestRequireEnterprise_LivePredicateBlocksWhenFalse(t *testing.T) {
	// A live predicate takes precedence over the static flag: even with the
	// static bool set true, an unlicensed predicate returns 403.
	h := (&Handler{enterprise: true, log: logr.Discard()}).WithEnterpriseFunc(func() bool { return false })
	called := false
	guarded := h.requireEnterprise(func(http.ResponseWriter, *http.Request) { called = true })

	rec := httptest.NewRecorder()
	guarded(rec, httptest.NewRequest(http.MethodPost, institutionalRoute, nil))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, enterpriseRequiredJSON, rec.Body.String())
	assert.False(t, called)
}

func TestRequireEnterprise_LivePredicatePassesWhenTrue(t *testing.T) {
	h := (&Handler{enterprise: false, log: logr.Discard()}).WithEnterpriseFunc(func() bool { return true })
	called := false
	guarded := h.requireEnterprise(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	guarded(rec, httptest.NewRequest(http.MethodPost, institutionalRoute, nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}

func TestRequireEnterprise_LivePredicateDegradesMidRun(t *testing.T) {
	// Model a license lapsing while the process runs: the predicate flips to
	// false and the paid endpoint starts returning 403 with no restart.
	entitled := true
	h := (&Handler{log: logr.Discard()}).WithEnterpriseFunc(func() bool { return entitled })
	guarded := h.requireEnterprise(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	rec1 := httptest.NewRecorder()
	guarded(rec1, httptest.NewRequest(http.MethodGet, "/api/v1/memories/projection", nil))
	assert.Equal(t, http.StatusOK, rec1.Code, "licensed request should pass")

	entitled = false // license expires / downgrades

	rec2 := httptest.NewRecorder()
	guarded(rec2, httptest.NewRequest(http.MethodGet, "/api/v1/memories/projection", nil))
	assert.Equal(t, http.StatusForbidden, rec2.Code, "lapsed license should degrade to 403")
}

func TestAggregateRoute_GatedWhenOff(t *testing.T) {
	h := &Handler{enterprise: false, log: logr.Discard()}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=test", nil))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, enterpriseRequiredJSON, rec.Body.String())
}

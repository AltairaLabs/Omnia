package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestRequireEnterprise_BlocksWhenOff(t *testing.T) {
	h := &Handler{enterprise: false, log: logr.Discard()}
	called := false
	guarded := h.requireEnterprise(func(http.ResponseWriter, *http.Request) { called = true })

	rec := httptest.NewRecorder()
	guarded(rec, httptest.NewRequest(http.MethodPost, "/api/v1/institutional/memories", nil))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, `{"error":"enterprise_required"}`, rec.Body.String())
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
	guarded(rec, httptest.NewRequest(http.MethodPost, "/api/v1/institutional/memories", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}

func TestAggregateRoute_GatedWhenOff(t *testing.T) {
	h := &Handler{enterprise: false, log: logr.Discard()}
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=test", nil))

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.JSONEq(t, `{"error":"enterprise_required"}`, rec.Body.String())
}

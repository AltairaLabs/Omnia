package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"

	memoryapi "github.com/altairalabs/omnia/internal/memory/api"
	"github.com/altairalabs/omnia/internal/memory/ingestion"
)

// licenseGateServer serves a GET /api/v1/license response with the given
// memoryEnterprise entitlement and expiry, mimicking the arena-controller DTO.
func licenseGateServer(t *testing.T, memoryEnterprise bool, expiresAt time.Time) string {
	t.Helper()
	body := fmt.Sprintf(
		`{"tier":"enterprise","features":{"memoryEnterprise":%t},"expiresAt":%q}`,
		memoryEnterprise, expiresAt.Format(time.RFC3339),
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestResolveEnterpriseGate_FlagOff(t *testing.T) {
	licensed, entitled := resolveEnterpriseGate(context.Background(),
		&flags{enterprise: false, operatorAPIURL: "http://unused"}, logr.Discard())

	assert.False(t, licensed, "gate must stay closed when ENTERPRISE_ENABLED is off")
	assert.Nil(t, entitled, "no live predicate when enterprise is off")
}

func TestResolveEnterpriseGate_FlagOnButNoOperatorURL(t *testing.T) {
	// Enterprise deployed but enforcement not wired (no operator URL): fall back
	// to the flag so existing deployments keep working. A nil predicate leaves
	// the HTTP gate on the static flag — this is the non-breaking, dormant path.
	licensed, entitled := resolveEnterpriseGate(context.Background(),
		&flags{enterprise: true, operatorAPIURL: ""}, logr.Discard())

	assert.True(t, licensed, "unwired enforcement must honor ENTERPRISE_ENABLED, not disable features")
	assert.Nil(t, entitled, "no live enforcement predicate when enforcement is not wired")
}

func gateCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel) // stop the client's background refresher when the test ends
	return ctx
}

func TestResolveEnterpriseGate_Licensed(t *testing.T) {
	url := licenseGateServer(t, true, time.Now().Add(24*time.Hour))
	licensed, entitled := resolveEnterpriseGate(gateCtx(t),
		&flags{enterprise: true, operatorAPIURL: url}, logr.Discard())

	assert.True(t, licensed, "a license granting memoryEnterprise should enable the features")
	assert.True(t, entitled())
}

func TestResolveEnterpriseGate_LicenseLacksEntitlement(t *testing.T) {
	url := licenseGateServer(t, false, time.Now().Add(24*time.Hour))
	licensed, entitled := resolveEnterpriseGate(gateCtx(t),
		&flags{enterprise: true, operatorAPIURL: url}, logr.Discard())

	assert.False(t, licensed, "an enterprise license without memoryEnterprise must degrade")
	assert.False(t, entitled())
}

func TestResolveEnterpriseGate_ExpiredLicenseDegrades(t *testing.T) {
	url := licenseGateServer(t, true, time.Now().Add(-1*time.Hour))
	licensed, entitled := resolveEnterpriseGate(gateCtx(t),
		&flags{enterprise: true, operatorAPIURL: url}, logr.Discard())

	assert.False(t, licensed, "an expired license must degrade even if the feature bit is set")
	assert.False(t, entitled())
}

// buildLiveGateMux builds a memory-api mux with enterprise=true and the given
// live entitlement predicate, so we can assert the predicate reaches the HTTP
// gate (buildAPIMux -> handler.WithEnterpriseFunc wiring).
func buildLiveGateMux(t *testing.T, entitled func() bool) http.Handler {
	t.Helper()
	freshPromRegistry(t)
	handler, cleanup := buildAPIMux(
		context.Background(),
		fakeMemoryStore{},
		nil,
		memoryapi.MemoryServiceConfig{},
		nil,
		true, // enterprise wired at startup
		nil,
		nil,
		nil,
		logr.Discard(),
		memoryapi.IngestOptions{Fallback: ingestion.Config{
			Strategy: ingestion.StrategyChunk, ChunkSize: 200, ChunkOverlap: 40,
		}},
		"", "",
		nil,
		entitled, // live gate
	)
	t.Cleanup(cleanup)
	return handler
}

func TestBuildAPIMux_LiveGateBlocksPaidRouteWhenDenied(t *testing.T) {
	handler := buildLiveGateMux(t, func() bool { return false })

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws-1", nil))

	assert.Equal(t, http.StatusForbidden, rr.Code,
		"a denying live predicate must degrade the paid route to 403 even though enterprise wiring is on")
}

func TestBuildAPIMux_LiveGateAllowsPaidRouteWhenGranted(t *testing.T) {
	handler := buildLiveGateMux(t, func() bool { return true })

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/memories/aggregate?workspace=ws-1", nil))

	assert.NotEqual(t, http.StatusForbidden, rr.Code,
		"a granting live predicate must not block the paid route on enterprise_required")
}

func TestResolveEnterpriseGate_UnreachableOperatorIsOptimistic(t *testing.T) {
	// A URL that refuses connections → license can't be verified at startup.
	// The startup decision is optimistic (features stay wired) so a transient
	// boot-time outage doesn't permanently downgrade a licensed deployment; the
	// live predicate stays closed until the operator becomes reachable.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	licensed, entitled := resolveEnterpriseGate(gateCtx(t),
		&flags{enterprise: true, operatorAPIURL: url}, logr.Discard())

	assert.True(t, licensed, "an unverifiable license at startup must stay wired, not permanently downgrade")
	assert.False(t, entitled(), "the live gate stays closed until the operator confirms the entitlement")
}

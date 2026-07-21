/*
Copyright 2026 Altaira Labs.

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

package main

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/session/sessiontest"
)

const probeAgentName = "probe"

// TestBuildWebSocketServer_WiresRedisRouteStore verifies that when
// OMNIA_ROUTE_REDIS_URL is set, buildWebSocketServer wires the Redis-backed
// RouteStore into the facade via WithRouteStore. Without this, parked
// realtime sessions cannot publish a pod-address hint and blip-resume
// falls back to the no-op store silently.
func TestBuildWebSocketServer_WiresRedisRouteStore(t *testing.T) {
	freshPromRegistry(t)

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer mr.Close()

	t.Setenv("OMNIA_ROUTE_REDIS_URL", "redis://"+mr.Addr())
	t.Setenv("POD_IP", "10.0.0.1")

	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	cfg := &agent.Config{
		AgentName:  probeAgentName,
		Namespace:  "ns",
		FacadePort: 8080,
	}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: probeAgentName}

	servers, err := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildWebSocketServer: %v", err)
	}
	srv := servers.external

	if !srv.HasRouteStore() {
		t.Error("facade reports no RouteStore wired; buildWebSocketServer " +
			"is not forwarding the Redis RouteStore via facade.WithRouteStore — " +
			"blip-resume parked sessions will not publish pod-address hints")
	}
}

// TestBuildWebSocketServer_NoopRouteStoreWhenEnvUnset verifies that when
// OMNIA_ROUTE_REDIS_URL is not set, the facade server uses the noop
// RouteStore (no Redis required, no error). This is the expected state for
// text-only agents without realtime audio.
func TestBuildWebSocketServer_NoopRouteStoreWhenEnvUnset(t *testing.T) {
	freshPromRegistry(t)

	// Explicitly unset to avoid leakage from other tests in the package.
	t.Setenv("OMNIA_ROUTE_REDIS_URL", "")

	store := sessiontest.NewStore()
	t.Cleanup(func() { _ = store.Close() })

	cfg := &agent.Config{
		AgentName:  "probe-no-redis",
		Namespace:  "ns",
		FacadePort: 8080,
	}
	metrics := agent.NewMetrics(cfg.AgentName, cfg.Namespace)
	handler := &captureHandler{name: "probe-no-redis"}

	servers, err := buildWebSocketServer(cfg, logr.Discard(), store, handler, metrics, nil, nil, nil)
	if err != nil {
		t.Fatalf("buildWebSocketServer: %v", err)
	}
	srv := servers.external

	if srv.HasRouteStore() {
		t.Error("facade reports a real RouteStore wired when OMNIA_ROUTE_REDIS_URL is unset; " +
			"expected noop store")
	}
}

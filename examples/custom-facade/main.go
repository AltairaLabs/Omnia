/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

// Command custom-facade is a minimal, buildable reference "bring your own"
// facade for Omnia (facade type: custom, #1768). It demonstrates the full
// contract a third-party facade image must honour:
//
//   - authenticate its OWN protocol (here: a static bearer token) and map the
//     result onto the platform's flat x-omnia-* identity/claims metadata;
//   - speak the runtime gRPC contract (RuntimeService/Converse) directly,
//     attaching that metadata so the runtime + policy-broker see the caller's
//     id, roles, full claim map, origin and workspace;
//   - serve /healthz and /readyz on the health port (:8081) so the operator's
//     probes mark the pod Ready — a BYO image that skips this never goes Ready;
//   - optionally serve the management-plane twin listener (:18080) and verify
//     the dashboard's RS256 JWT (JWKS at OMNIA_MGMT_PLANE_JWKS_URL), failing
//     closed on a missing/bad token.
//
// It is intentionally minimal but real: `docker build` it and deploy it as the
// image of a `type: custom` facade. The reusable core lives in ./facade so it
// is unit- and contract-tested.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/altairalabs/omnia/examples/custom-facade/facade"
	"github.com/altairalabs/omnia/pkg/policy"
)

const (
	// healthAddr is the operator-probed health port. MUST match the built-in
	// facade contract (internal/controller: DefaultFacadeHealthPort = 8081) or
	// the readiness probe never passes and the pod never goes Ready.
	healthAddr = ":8081"
	// dataPlaneAddr is the external data-plane port (DefaultFacadePort = 8080).
	dataPlaneAddr = ":8080"
	// mgmtAddr is the management-plane twin port (DefaultInternalFacadePort =
	// 18080). Served only when OMNIA_MGMT_PLANE_JWKS_URL is set.
	mgmtAddr = ":18080"

	envRuntimeAddress = "OMNIA_RUNTIME_ADDRESS"
	envAgentName      = "OMNIA_AGENT_NAME"
	envMgmtJWKSURL    = "OMNIA_MGMT_PLANE_JWKS_URL"
)

// demoTokens is the reference facade's trivial credential table. A real facade
// replaces this with its own credential validation. The demo principal carries
// a rich identity so the whole x-omnia-* contract is exercised end to end.
func demoTokens() map[string]*facade.Principal {
	return map[string]*facade.Principal{
		"demo-token": {
			UserID:    "user-42",
			Roles:     []string{policy.RoleAdmin, policy.RoleEditor},
			Workspace: "acme",
			Origin:    policy.OriginSharedToken,
			Claims: map[string]string{
				"tier":   "gold",
				"team":   "finance",
				"region": "emea",
			},
		},
	}
}

func main() {
	log.SetFlags(0)
	agentName := os.Getenv(envAgentName)
	runtimeAddr := os.Getenv(envRuntimeAddress)
	if runtimeAddr == "" {
		runtimeAddr = "localhost:9000"
	}

	rc, err := facade.Dial(runtimeAddr, agentName)
	if err != nil {
		log.Fatalf("custom-facade: %v", err)
	}
	defer func() { _ = rc.Close() }()

	auth := facade.NewAuthenticator(demoTokens())
	srv := &server{auth: auth, runtime: rc, agentName: agentName}

	go serve("health", healthAddr, healthMux())
	go serve("data-plane", dataPlaneAddr, srv.dataPlaneMux())

	if jwksURL := os.Getenv(envMgmtJWKSURL); jwksURL != "" {
		verifier := facade.NewMgmtVerifier(facade.NewJWKSResolver(jwksURL))
		go serve("mgmt-twin", mgmtAddr, verifier.Middleware(srv.dataPlaneMux()))
		log.Printf("custom-facade: management-plane twin enabled on %s (jwks=%s)", mgmtAddr, jwksURL)
	} else {
		log.Printf("custom-facade: management-plane twin disabled (%s unset)", envMgmtJWKSURL)
	}

	select {} // block forever; the HTTP servers own the process lifetime.
}

// serve runs an HTTP server on addr, fatally exiting if it stops.
func serve(name, addr string, handler http.Handler) {
	log.Printf("custom-facade: %s listening on %s", name, addr)
	s := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("custom-facade: %s server: %v", name, err)
	}
}

// healthMux serves the operator-probed /healthz and /readyz endpoints.
func healthMux() http.Handler {
	mux := http.NewServeMux()
	ok := func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }
	mux.HandleFunc("/healthz", ok)
	mux.HandleFunc("/readyz", ok)
	return mux
}

// server holds the data-plane dependencies.
type server struct {
	auth      *facade.Authenticator
	runtime   *facade.RuntimeClient
	agentName string
}

// chatRequest is the reference facade's trivial external protocol.
type chatRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
}

// chatResponse is what the facade returns to its client.
type chatResponse struct {
	Reply string `json:"reply"`
}

// dataPlaneMux serves POST /chat: authenticate the bearer token, then run one
// Converse turn against the runtime with the caller's identity attached.
func (s *server) dataPlaneMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat", s.handleChat)
	return mux
}

func (s *server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	principal, err := s.auth.Authenticate(r.Header.Get("Authorization"))
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	reply, err := s.runtime.Converse(ctx, principal, req.SessionID, req.Message)
	if err != nil {
		http.Error(w, "runtime error", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chatResponse{Reply: reply})
}

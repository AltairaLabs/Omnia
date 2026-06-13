/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"
	"net/http"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/altairalabs/omnia/internal/agent"
)

// newHealthServer builds the shared health endpoint surface used by facade modes.
// It serves /healthz, /readyz, and /metrics on the dedicated health port.
func newHealthServer(cfg *agent.Config, readyHandler http.HandlerFunc) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/readyz", readyHandler)
	mux.Handle("/metrics", promhttp.Handler())

	return &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.HealthPort),
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
}

// readyzOKHandler is a static readiness endpoint used by components that only
// need to report process-level readiness.
func readyzOKHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// runPrimaryAndHealthServers starts two HTTP servers and waits for either a
// shutdown signal or a non-graceful server error.
func runPrimaryAndHealthServers(log logr.Logger, primaryName string, primary, health *http.Server) {
	errChan := make(chan error, 2)
	startServerGoroutine(primaryName, primary, errChan, log)
	startServerGoroutine("health server", health, errChan, log)
	waitForShutdownSignal(log, errChan)
}

// shutdownPrimaryAndHealthServers gracefully shuts down a primary HTTP server
// and the shared health server using the common shutdown helper.
func shutdownPrimaryAndHealthServers(log logr.Logger, primaryName string, primary, health *http.Server) {
	shutdownServers(log, map[string]*http.Server{
		primaryName:     primary,
		"health server": health,
	})
}

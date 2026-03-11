/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/logr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/agent"
	facadea2a "github.com/altairalabs/omnia/internal/facade/a2a"
	"github.com/altairalabs/omnia/internal/tracing"

	a2aserver "github.com/AltairaLabs/PromptKit/server/a2a"
)

// runA2AFacade starts the A2A JSON-RPC facade with PromptKit SDK in-process.
// Unlike the WebSocket facade, A2A does not use a separate runtime sidecar —
// the SDK handles LLM calls directly.
func runA2AFacade(cfg *agent.Config, log logr.Logger, _ *tracing.Provider) {
	log.Info("starting A2A facade",
		"port", cfg.FacadePort,
		"taskTTL", cfg.A2ATaskTTL,
		"conversationTTL", cfg.A2AConversationTTL,
	)

	// Build authenticator
	var auth a2aserver.Authenticator
	if cfg.A2AAuthToken != "" {
		auth = facadea2a.NewBearerAuthenticator(cfg.A2AAuthToken)
		log.Info("A2A bearer auth enabled")
	}

	// Build card provider from CRD config
	cardProvider := buildCardProvider(cfg, log)

	// Pack path: for A2A, the SDK reads the pack directly
	packPath := cfg.PromptPackPath + "/pack.json"

	a2aSrv := facadea2a.NewServer(facadea2a.ServerConfig{
		PackPath:        packPath,
		PromptName:      "default",
		Port:            cfg.FacadePort,
		TaskTTL:         cfg.A2ATaskTTL,
		ConversationTTL: cfg.A2AConversationTTL,
		CardProvider:    cardProvider,
		Authenticator:   auth,
		Log:             log,
	})

	// Serve A2A handler on the facade port.
	// The A2A server handler includes /healthz and /readyz endpoints.
	facadeServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.FacadePort),
		Handler: a2aSrv.Handler(),
	}

	errChan := make(chan error, 1)
	go func() {
		log.Info("starting A2A server", "addr", facadeServer.Addr)
		if err := facadeServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- fmt.Errorf("a2a server error: %w", err)
		}
	}()

	// Wait for shutdown signal or error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Info("received shutdown signal", "signal", sig)
	case err := <-errChan:
		log.Error(err, "server error")
	}

	// Graceful shutdown
	log.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := a2aSrv.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down A2A server")
	}
	if err := facadeServer.Shutdown(ctx); err != nil {
		log.Error(err, "error shutting down HTTP server")
	}

	log.Info("shutdown complete")
}

// buildCardProvider creates the agent card provider from config.
func buildCardProvider(cfg *agent.Config, log logr.Logger) a2aserver.AgentCardProvider {
	log.V(1).Info("building default agent card", "agentName", cfg.AgentName)

	spec := &omniav1alpha1.AgentCardSpec{
		Name:        cfg.AgentName,
		Description: fmt.Sprintf("Omnia agent: %s", cfg.AgentName),
	}

	endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
		cfg.AgentName, cfg.Namespace, cfg.FacadePort)

	return facadea2a.NewCRDCardProvider(spec, endpoint)
}

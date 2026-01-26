/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	"github.com/altairalabs/omnia/ee/cmd/promptkit-lsp/server"
)

func main() {
	var (
		addr            string
		healthAddr      string
		dashboardAPIURL string
		devMode         bool
	)

	flag.StringVar(&addr, "addr", ":8080", "Address for HTTP/WebSocket server")
	flag.StringVar(&healthAddr, "health-addr", ":8081", "Address for health probes")
	flag.StringVar(&dashboardAPIURL, "dashboard-api-url",
		"http://omnia-dashboard:3000", "Dashboard API base URL for file access")
	flag.BoolVar(&devMode, "dev-mode", false, "Enable development mode (disables license validation)")
	flag.Parse()

	// Setup logger
	zapLog, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = zapLog.Sync() }()
	log := zapr.NewLogger(zapLog)

	// Create and start the server
	srv, err := server.New(server.Config{
		Addr:            addr,
		HealthAddr:      healthAddr,
		DashboardAPIURL: dashboardAPIURL,
		DevMode:         devMode,
	}, log)
	if err != nil {
		log.Error(err, "failed to create server")
		os.Exit(1)
	}

	// Setup signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		log.Info("starting promptkit-lsp server", "addr", addr, "healthAddr", healthAddr)
		if err := srv.Start(ctx); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for signal or error
	select {
	case sig := <-sigCh:
		log.Info("received signal, shutting down", "signal", sig.String())
	case err := <-errCh:
		if err != nil {
			log.Error(err, "server error")
		}
	}

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error(err, "shutdown error")
		os.Exit(1)
	}

	log.Info("server stopped")
}

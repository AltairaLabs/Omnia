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

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	"github.com/altairalabs/omnia/ee/cmd/promptkit-lsp/server"
)

// parseFlagsIntoConfig parses the four binary-level flags into a
// server.Config. Extracted from main() so a wiring test can assert
// each flag flows into the correct Config field with the documented
// default. The fs argument lets tests supply a fresh FlagSet without
// touching the global default.
func parseFlagsIntoConfig(fs *flag.FlagSet, args []string) server.Config {
	var cfg server.Config
	fs.StringVar(&cfg.Addr, "addr", ":8080", "Address for HTTP/WebSocket server")
	fs.StringVar(&cfg.HealthAddr, "health-addr", ":8081", "Address for health probes")
	fs.StringVar(&cfg.DashboardAPIURL, "dashboard-api-url",
		"http://omnia-dashboard:3000", "Dashboard API base URL for file access")
	fs.BoolVar(&cfg.DevMode, "dev-mode", false, "Enable development mode (disables license validation)")
	_ = fs.Parse(args)
	return cfg
}

// setupServer is the binary-level wiring contract: parse args into a
// server.Config and hand that Config to server.New. Returning srv + cfg
// + err lets a wiring test assert (a) every flag flows into the right
// Config field and (b) the resulting Config builds a non-nil server,
// without spinning up a real listener or signal loop.
func setupServer(args []string, log logr.Logger) (*server.Server, server.Config, error) {
	cfg := parseFlagsIntoConfig(flag.NewFlagSet("promptkit-lsp", flag.ContinueOnError), args)
	srv, err := server.New(cfg, log)
	return srv, cfg, err
}

func main() {
	// Setup logger
	zapLog, err := zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = zapLog.Sync() }()
	log := zapr.NewLogger(zapLog)

	srv, cfg, err := setupServer(os.Args[1:], log)
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
		log.Info("starting promptkit-lsp server", "addr", cfg.Addr, "healthAddr", cfg.HealthAddr)
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

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
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/altairalabs/omnia/ee/pkg/policy"
)

const (
	defaultListenAddr = ":8443"
	defaultHealthAddr = ":8444"
	shutdownTimeout   = 5 * time.Second
	readHeaderTimeout = 10 * time.Second
	envUpstreamURL    = "UPSTREAM_URL"
)

func main() {
	var listenAddr string
	var healthAddr string
	var upstreamRaw string
	var auditLog bool

	flag.StringVar(&listenAddr, "listen-addr", defaultListenAddr, "Address to listen on for proxied requests")
	flag.StringVar(&healthAddr, "health-addr", defaultHealthAddr, "Address to listen on for health checks")
	flag.StringVar(&upstreamRaw, "upstream-url", "", "Upstream URL to forward requests to (or UPSTREAM_URL env)")
	flag.BoolVar(&auditLog, "audit-log", false, "Enable audit logging of policy decisions")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("policy-proxy")

	// Resolve upstream URL from flag or env
	if upstreamRaw == "" {
		upstreamRaw = os.Getenv(envUpstreamURL)
	}
	if upstreamRaw == "" {
		log.Error(nil, "upstream URL is required (--upstream-url or UPSTREAM_URL)")
		os.Exit(1)
	}

	upstreamURL, err := url.Parse(upstreamRaw)
	if err != nil {
		log.Error(err, "failed to parse upstream URL")
		os.Exit(1)
	}

	// Create CEL evaluator
	evaluator, err := policy.NewEvaluator()
	if err != nil {
		log.Error(err, "failed to create CEL evaluator")
		os.Exit(1)
	}

	// Create dynamic Kubernetes client
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Error(err, "failed to get in-cluster config")
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		log.Error(err, "failed to create dynamic client")
		os.Exit(1)
	}

	// Start ToolPolicy watcher
	watcher := policy.NewWatcher(dynamicClient, evaluator, log)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := watcher.Start(ctx); err != nil {
			log.Error(err, "watcher failed")
			os.Exit(1)
		}
	}()

	// Create proxy handler
	proxyHandler := policy.NewProxyHandler(evaluator, upstreamURL, log, auditLog)

	// Proxy server
	proxySrv := &http.Server{
		Addr:              listenAddr,
		Handler:           proxyHandler,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// Health server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", policy.HealthHandler())
	healthMux.HandleFunc("/readyz", policy.HealthHandler())
	healthSrv := &http.Server{
		Addr:              healthAddr,
		Handler:           healthMux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	// Start servers
	go func() {
		log.Info("starting health server", "addr", healthAddr)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "health server error")
		}
	}()

	go func() {
		log.Info("starting policy proxy", "addr", listenAddr, "upstream", upstreamRaw)
		if err := proxySrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error(err, "proxy server error")
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh
	log.Info("received signal, shutting down", "signal", fmt.Sprintf("%v", sig))

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	_ = proxySrv.Shutdown(shutdownCtx)
	_ = healthSrv.Shutdown(shutdownCtx)
}

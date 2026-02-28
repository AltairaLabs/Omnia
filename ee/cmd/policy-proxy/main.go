/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/policy"
)

const (
	defaultListenAddr  = ":8080"
	defaultHealthAddr  = ":8081"
	defaultUpstreamURL = "http://localhost:9090"
	shutdownTimeout    = 5 * time.Second

	envListenAddr  = "POLICY_PROXY_LISTEN_ADDR"
	envHealthAddr  = "POLICY_PROXY_HEALTH_ADDR"
	envUpstreamURL = "POLICY_PROXY_UPSTREAM_URL"
	envNamespace   = "OMNIA_NAMESPACE"
	envAgentName   = "OMNIA_AGENT_NAME"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("policy proxy failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	listenAddr := getEnvOrDefault(envListenAddr, defaultListenAddr)
	healthAddr := getEnvOrDefault(envHealthAddr, defaultHealthAddr)
	upstreamStr := getEnvOrDefault(envUpstreamURL, defaultUpstreamURL)
	namespace := os.Getenv(envNamespace)
	agentName := os.Getenv(envAgentName)

	logger.Info("starting policy proxy",
		"listenAddr", listenAddr,
		"healthAddr", healthAddr,
		"upstream", upstreamStr,
		"namespace", namespace,
		"agentName", agentName)

	upstreamURL, err := url.Parse(upstreamStr)
	if err != nil {
		return fmt.Errorf("invalid upstream URL %q: %w", upstreamStr, err)
	}

	evaluator, err := policy.NewEvaluator()
	if err != nil {
		return fmt.Errorf("failed to create evaluator: %w", err)
	}

	k8sClient, scheme, err := createK8sClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	policy.RegisterMetrics(prometheus.DefaultRegisterer)

	watcher := policy.NewWatcher(evaluator, k8sClient, scheme, namespace, logger)
	proxyHandler := policy.NewProxyHandler(evaluator, upstreamURL, logger)

	proxySrv := &http.Server{
		Addr:              listenAddr,
		Handler:           proxyHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", policy.HealthHandler())
	healthMux.HandleFunc("/readyz", policy.HealthHandler())
	healthMux.Handle("/metrics", promhttp.Handler())
	healthSrv := &http.Server{
		Addr:              healthAddr,
		Handler:           healthMux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		if watchErr := watcher.Start(ctx); watchErr != nil && !errors.Is(watchErr, context.Canceled) {
			logger.Error("watcher error", "error", watchErr.Error())
		}
	}()

	go func() {
		logger.Info("health server starting", "addr", healthAddr)
		if srvErr := healthSrv.ListenAndServe(); srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
			logger.Error("health server error", "error", srvErr.Error())
		}
	}()

	go func() {
		logger.Info("proxy server starting", "addr", listenAddr)
		if srvErr := proxySrv.ListenAndServe(); srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
			logger.Error("proxy server error", "error", srvErr.Error())
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	_ = proxySrv.Shutdown(shutdownCtx)
	_ = healthSrv.Shutdown(shutdownCtx)

	return nil
}

func createK8sClient() (client.Client, *runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(omniav1alpha1.AddToScheme(scheme))

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	return c, scheme, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultValue
}

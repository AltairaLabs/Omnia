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
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	eelicense "github.com/altairalabs/omnia/ee/pkg/license"
	"github.com/altairalabs/omnia/ee/pkg/policy"
)

const (
	defaultListenAddr = ":8090"
	defaultHealthAddr = ":8091"
	decisionPath      = "/v1/decision"
	shutdownTimeout   = 5 * time.Second

	envListenAddr = "POLICY_BROKER_LISTEN_ADDR"
	envHealthAddr = "POLICY_BROKER_HEALTH_ADDR"
	envNamespace  = "OMNIA_NAMESPACE"
	envAgentName  = "OMNIA_AGENT_NAME"
	// envOperatorAPIURL points at the operator/arena-controller license
	// endpoint. When set and the license is not valid, the broker logs a
	// startup reminder. Never blocks.
	envOperatorAPIURL = "OPERATOR_API_URL"
)

// nagLicenseAtStartup fetches the operator license once and logs a reminder when
// the policy-broker sidecar runs without a valid license. The broker is
// enterprise-only, so any non-valid license (open-core, absent, or expired)
// nags. It never blocks — enforcement keeps running. The "startup license
// check" line is always emitted so the check is observable when silent.
func nagLicenseAtStartup(ctx context.Context, logger *slog.Logger) {
	operatorURL := os.Getenv(envOperatorAPIURL)
	if operatorURL == "" {
		logger.Info("startup license check skipped", "reason", "no OPERATOR_API_URL configured")
		return
	}
	log := logr.FromSlogHandler(logger.Handler())
	licClient := eelicense.NewClient(operatorURL, eelicense.WithClientLogger(log.WithName("license")))
	lic, err := licClient.Refresh(ctx)
	if err != nil {
		// Operator unreachable — degrade to the open-core fallback and nag.
		lic = licClient.License()
	}
	logger.Info("startup license check",
		"valid", lic.IsValidEnterprise(),
		"tier", string(lic.Tier),
		"licenseID", lic.ID,
		"operatorURL", operatorURL)
	eelicense.NagIfUnlicensed(lic, log)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("policy broker failed", "error", err.Error())
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	listenAddr := getEnvOrDefault(envListenAddr, defaultListenAddr)
	healthAddr := getEnvOrDefault(envHealthAddr, defaultHealthAddr)
	namespace := os.Getenv(envNamespace)
	agentName := os.Getenv(envAgentName)

	logger.Info("starting policy broker",
		"listenAddr", listenAddr,
		"healthAddr", healthAddr,
		"namespace", namespace,
		"agentName", agentName)

	evaluator, err := policy.NewEvaluator()
	if err != nil {
		return fmt.Errorf("failed to create evaluator: %w", err)
	}

	k8sClient, scheme, err := createK8sClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	watcher := policy.NewWatcher(evaluator, k8sClient, scheme, namespace, logger)
	brokerHandler := policy.NewBrokerHandler(evaluator, logger)

	brokerSrv := &http.Server{
		Addr:              listenAddr,
		Handler:           buildDecisionMux(brokerHandler),
		ReadHeaderTimeout: 10 * time.Second,
	}

	healthSrv := &http.Server{
		Addr:              healthAddr,
		Handler:           buildHealthMux(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// License-awareness nag (#1682): remind if enterprise runs unlicensed.
	nagLicenseAtStartup(ctx, logger)

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
		logger.Info("broker server starting", "addr", listenAddr)
		if srvErr := brokerSrv.ListenAndServe(); srvErr != nil && !errors.Is(srvErr, http.ErrServerClosed) {
			logger.Error("broker server error", "error", srvErr.Error())
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	_ = brokerSrv.Shutdown(shutdownCtx)
	_ = healthSrv.Shutdown(shutdownCtx)

	return nil
}

// buildDecisionMux registers the decision endpoint against the shared
// policy.BrokerHandler. Extracted so a wiring test can assert the route is
// registered without spinning up a real listener.
func buildDecisionMux(handler *policy.BrokerHandler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle(decisionPath, handler)
	return mux
}

// buildHealthMux registers /healthz and /readyz against the shared
// policy.HealthHandler. Extracted so a wiring test can assert both routes
// are registered without spinning up a real listener.
func buildHealthMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", policy.HealthHandler())
	mux.HandleFunc("/readyz", policy.HealthHandler())
	return mux
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

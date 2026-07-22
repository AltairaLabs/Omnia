/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

// lazyPool builds a non-nil *pgxpool.Pool without connecting (pgx v5 connects
// lazily on first use), so wiring tests can exercise the pool-present branch
// without a database.
func lazyPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("postgres://u:p@127.0.0.1:5432/db")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// TestBuildAuditForwarder_StartedWhenEnterpriseAndURLPresent is the wiring test
// (repo policy): the audit drain-forwarder must actually be constructed when
// enterprise audit is on AND a privacy-api URL is resolvable, with a pool to
// drain. A built-but-unwired forwarder is the silent failure this guards against.
func TestBuildAuditForwarder_StartedWhenEnterpriseAndURLPresent(t *testing.T) {
	fwd := buildAuditForwarder(true, lazyPool(t), "http://privacy-api.omnia-system:8080",
		prometheus.NewRegistry(), logr.Discard())
	if fwd == nil {
		t.Fatal("expected a forwarder when enterprise, pool and privacy URL are present")
	}
}

func TestBuildAuditForwarder_NilWhenEnterpriseOff(t *testing.T) {
	fwd := buildAuditForwarder(false, nil, "http://privacy-api:8080",
		prometheus.NewRegistry(), logr.Discard())
	if fwd != nil {
		t.Fatal("expected nil forwarder when enterprise is disabled")
	}
}

// TestBuildAuditForwarder_NilWhenNoURL verifies no forwarder is built when the
// privacy-api URL cannot be resolved, even with enterprise on.
func TestBuildAuditForwarder_NilWhenNoURL(t *testing.T) {
	fwd := buildAuditForwarder(true, nil, "", prometheus.NewRegistry(), logr.Discard())
	if fwd != nil {
		t.Fatal("expected nil forwarder when no privacy URL resolves")
	}
}

// TestBuildAuditForwarder_NilWhenNoPool verifies the pool gate: enterprise on
// and a URL present but a nil pool (no DB) must not build a forwarder.
func TestBuildAuditForwarder_NilWhenNoPool(t *testing.T) {
	fwd := buildAuditForwarder(true, nil, "http://privacy-api:8080",
		prometheus.NewRegistry(), logr.Discard())
	if fwd != nil {
		t.Fatal("expected nil forwarder when pool is nil")
	}
}

// TestResolvePrivacyURL_EnvOverride verifies PRIVACY_API_URL is used as-is.
// TestResolvePrivacyURL_EmptyWhenNoEnvNoWorkspace verifies resolution returns ""
// (forwarder skipped) when neither env nor a workspace lookup is available.
func TestResolvePrivacyURL_EmptyWhenNoEnvNoWorkspace(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "")
	got := resolvePrivacyURL(context.Background(), "", "", logr.Discard())
	if got != "" {
		t.Errorf("expected empty URL, got %q", got)
	}
}

// PRIVACY_API_URL is no longer honoured: privacy-api is per-workspace like
// every other service, so its endpoint comes from the Workspace. With no
// workspace supplied there is nothing to resolve, and the env var must not
// stand in for it.
func TestResolvePrivacyURL_IgnoresEnvOverride(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "http://from-env:8080")

	got := resolvePrivacyURL(context.Background(), "", "", logr.Discard())

	if got != "" {
		t.Fatalf("PRIVACY_API_URL still honoured: %s", got)
	}
}

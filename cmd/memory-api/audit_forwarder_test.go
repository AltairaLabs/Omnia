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

	eeaudit "github.com/altairalabs/omnia/ee/pkg/audit"
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

// TestBuildAuditForwarder_StartedWhenAuditAndURLPresent is the wiring test
// (repo policy): the audit drain-forwarder must actually be constructed when
// audit logging is on AND a privacy-api URL is resolvable. A built-but-unwired
// forwarder is the silent failure this guards against.
func TestBuildAuditForwarder_StartedWhenAuditAndURLPresent(t *testing.T) {
	logger := eeaudit.NewLogger(nil, logr.Discard(), nil, eeaudit.LoggerConfig{})
	t.Cleanup(func() { _ = logger.Close() })

	fwd := buildAuditForwarder(logger, lazyPool(t), "http://privacy-api.omnia-system:8080",
		prometheus.NewRegistry(), logr.Discard())
	if fwd == nil {
		t.Fatal("expected a forwarder when audit logger, pool and privacy URL are present")
	}
}

// TestBuildAuditForwarder_NilWhenAuditDisabled verifies no forwarder is built
// when audit logging is off (nil logger), even if a URL is configured.
func TestBuildAuditForwarder_NilWhenAuditDisabled(t *testing.T) {
	fwd := buildAuditForwarder(nil, nil, "http://privacy-api:8080",
		prometheus.NewRegistry(), logr.Discard())
	if fwd != nil {
		t.Fatal("expected nil forwarder when audit logging is disabled")
	}
}

// TestBuildAuditForwarder_NilWhenNoURL verifies no forwarder is built when the
// privacy-api URL cannot be resolved, even with audit logging on.
func TestBuildAuditForwarder_NilWhenNoURL(t *testing.T) {
	logger := eeaudit.NewLogger(nil, logr.Discard(), nil, eeaudit.LoggerConfig{})
	t.Cleanup(func() { _ = logger.Close() })

	fwd := buildAuditForwarder(logger, nil, "", prometheus.NewRegistry(), logr.Discard())
	if fwd != nil {
		t.Fatal("expected nil forwarder when no privacy URL resolves")
	}
}

// TestResolvePrivacyURL_EnvOverride verifies PRIVACY_API_URL is used as-is.
// PRIVACY_API_URL is no longer honoured: privacy-api is per-workspace like
// every other service, so its endpoint comes from the Workspace.
func TestResolvePrivacyURL_IgnoresEnvOverride(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "http://privacy-api.omnia-system:8080")
	got := resolvePrivacyURL(context.Background(), "", "", logr.Discard())
	if got != "" {
		t.Errorf("PRIVACY_API_URL still honoured: %q", got)
	}
}

// TestResolvePrivacyURL_EmptyWhenNoEnvNoWorkspace verifies resolution returns ""
// (forwarder skipped) when neither env nor a workspace lookup is available.
func TestResolvePrivacyURL_EmptyWhenNoEnvNoWorkspace(t *testing.T) {
	t.Setenv("PRIVACY_API_URL", "")
	got := resolvePrivacyURL(context.Background(), "", "", logr.Discard())
	if got != "" {
		t.Errorf("expected empty URL, got %q", got)
	}
}

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"flag"
	"testing"

	"github.com/go-logr/logr"
)

// TestParseFlagsIntoConfig_Defaults asserts the four binary-level flags
// produce the documented defaults when no arguments are passed. A
// regression that renames a flag or changes a default would silently
// affect every deployment until someone tested it.
func TestParseFlagsIntoConfig_Defaults(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := parseFlagsIntoConfig(fs, nil)
	if cfg.Addr != ":8080" {
		t.Errorf("Addr default = %q, want :8080", cfg.Addr)
	}
	if cfg.HealthAddr != ":8081" {
		t.Errorf("HealthAddr default = %q, want :8081", cfg.HealthAddr)
	}
	if cfg.DashboardAPIURL != "http://omnia-dashboard:3000" {
		t.Errorf("DashboardAPIURL default = %q, want http://omnia-dashboard:3000",
			cfg.DashboardAPIURL)
	}
	if cfg.DevMode {
		t.Errorf("DevMode default = true, want false")
	}
}

// TestParseFlagsIntoConfig_AllOverrides asserts each flag actually writes
// into the corresponding Config field. Catches the class of bug where a
// flag is renamed but the StringVar target is left pointing at the
// old variable.
func TestParseFlagsIntoConfig_AllOverrides(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cfg := parseFlagsIntoConfig(fs, []string{
		"--addr=:9000",
		"--health-addr=:9001",
		"--dashboard-api-url=http://custom-dashboard:8000",
		"--dev-mode",
	})
	if cfg.Addr != ":9000" {
		t.Errorf("Addr = %q, want :9000", cfg.Addr)
	}
	if cfg.HealthAddr != ":9001" {
		t.Errorf("HealthAddr = %q, want :9001", cfg.HealthAddr)
	}
	if cfg.DashboardAPIURL != "http://custom-dashboard:8000" {
		t.Errorf("DashboardAPIURL = %q, want http://custom-dashboard:8000",
			cfg.DashboardAPIURL)
	}
	if !cfg.DevMode {
		t.Errorf("DevMode = false, want true")
	}
}

// TestSetupServer_FromDefaults asserts the binary-level wiring contract:
// args → parseFlagsIntoConfig → server.New. Catches regressions where
// server.Config gains a required field the binary doesn't populate,
// and exercises the code path main() takes to build its server.
func TestSetupServer_FromDefaults(t *testing.T) {
	srv, cfg, err := setupServer(nil, logr.Discard())
	if err != nil {
		t.Fatalf("setupServer with no args returned error: %v", err)
	}
	if srv == nil {
		t.Fatal("setupServer returned nil server with no error")
	}
	if cfg.Addr != ":8080" {
		t.Errorf("Config.Addr = %q, want :8080", cfg.Addr)
	}
}

// TestSetupServer_FlagsFlowIntoConfig asserts overrides reach server.New
// via the same path main() uses.
func TestSetupServer_FlagsFlowIntoConfig(t *testing.T) {
	srv, cfg, err := setupServer([]string{"--addr=:9090", "--dev-mode"}, logr.Discard())
	if err != nil {
		t.Fatalf("setupServer returned error: %v", err)
	}
	if srv == nil {
		t.Fatal("setupServer returned nil server")
	}
	if cfg.Addr != ":9090" {
		t.Errorf("cfg.Addr = %q, want :9090", cfg.Addr)
	}
	if !cfg.DevMode {
		t.Errorf("cfg.DevMode = false, want true")
	}
}

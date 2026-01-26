/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package server

import (
	"testing"

	"github.com/go-logr/logr"
)

func TestGetKindCompletions(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	items := srv.getKindCompletions()

	if len(items) == 0 {
		t.Fatal("expected kind completions")
	}

	// Check for known kinds
	kinds := make(map[string]bool)
	for _, item := range items {
		kinds[item.Label] = true
	}

	// Check that we have some expected kinds
	expectedKinds := []string{"Tool", "Provider", "Scenario", "Arena", "Persona"}
	for _, kind := range expectedKinds {
		if !kinds[kind] {
			t.Errorf("expected kind %q in completions", kind)
		}
	}
}

func TestGetProviderTypeCompletions(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	items := srv.getProviderTypeCompletions()

	if len(items) == 0 {
		t.Fatal("expected type completions")
	}

	// Check for known provider types
	types := make(map[string]bool)
	for _, item := range items {
		types[item.Label] = true
	}

	expectedTypes := []string{"openai", "anthropic", "azure", "bedrock"}
	for _, typ := range expectedTypes {
		if !types[typ] {
			t.Errorf("expected provider type %q in completions", typ)
		}
	}
}

func TestGetTopLevelCompletions(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	items := srv.getTopLevelCompletions()

	if len(items) == 0 {
		t.Fatal("expected top-level completions")
	}

	// Check for some expected fields
	fields := make(map[string]bool)
	for _, item := range items {
		fields[item.Label] = true
	}

	// At minimum we expect kind and spec
	if !fields["kind"] {
		t.Error("expected 'kind' in top-level completions")
	}
	if !fields["spec"] {
		t.Error("expected 'spec' in top-level completions")
	}
}

func TestGetFieldCompletions(t *testing.T) {
	srv, _ := New(Config{
		Addr:            ":8080",
		DashboardAPIURL: "http://localhost:3000",
	}, logr.Discard())

	items := srv.getFieldCompletions(nil, Position{})

	if len(items) == 0 {
		t.Fatal("expected field completions")
	}

	// Should have at least some common fields
	fields := make(map[string]bool)
	for _, item := range items {
		fields[item.Label] = true
	}

	if !fields["name"] {
		t.Error("expected 'name' field in completions")
	}
	if !fields["description"] {
		t.Error("expected 'description' field in completions")
	}
}

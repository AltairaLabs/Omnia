/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package snowflake

import (
	"strings"
	"testing"
)

func TestConfig_Validate_AllRequired(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{"missing account", Config{User: "u", Password: "p", Database: "d", Warehouse: "w"}, "account is required"},
		{"missing user", Config{Account: "a", Password: "p", Database: "d", Warehouse: "w"}, "user is required"},
		{"missing password", Config{Account: "a", User: "u", Database: "d", Warehouse: "w"}, "password is required"},
		{"missing database", Config{Account: "a", User: "u", Password: "p", Warehouse: "w"}, "database is required"},
		{"missing warehouse", Config{Account: "a", User: "u", Password: "p", Database: "d"}, "warehouse is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestConfig_Validate_Defaults(t *testing.T) {
	cfg := Config{
		Account:   "a",
		User:      "u",
		Password:  "p",
		Database:  "d",
		Warehouse: "w",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Schema != DefaultSchema {
		t.Errorf("expected schema %q, got %q", DefaultSchema, cfg.Schema)
	}
	if cfg.DefaultBatchSize != DefaultBatchSize {
		t.Errorf("expected batch size %d, got %d", DefaultBatchSize, cfg.DefaultBatchSize)
	}
}

func TestConfig_Validate_PreservesExisting(t *testing.T) {
	cfg := Config{
		Account:          "a",
		User:             "u",
		Password:         "p",
		Database:         "d",
		Warehouse:        "w",
		Schema:           "CUSTOM",
		DefaultBatchSize: 500,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Schema != "CUSTOM" {
		t.Errorf("expected schema 'CUSTOM', got %q", cfg.Schema)
	}
	if cfg.DefaultBatchSize != 500 {
		t.Errorf("expected batch size 500, got %d", cfg.DefaultBatchSize)
	}
}

func TestConfig_DSN_WithoutRole(t *testing.T) {
	cfg := Config{
		Account:   "org-acct",
		User:      "myuser",
		Password:  "mypass",
		Database:  "mydb",
		Schema:    "PUBLIC",
		Warehouse: "mywh",
	}
	dsn := cfg.DSN()
	expected := "myuser:mypass@org-acct/mydb/PUBLIC?warehouse=mywh"
	if dsn != expected {
		t.Errorf("expected DSN %q, got %q", expected, dsn)
	}
}

func TestConfig_DSN_WithRole(t *testing.T) {
	cfg := Config{
		Account:   "org-acct",
		User:      "myuser",
		Password:  "mypass",
		Database:  "mydb",
		Schema:    "PUBLIC",
		Warehouse: "mywh",
		Role:      "ANALYST",
	}
	dsn := cfg.DSN()
	if !strings.Contains(dsn, "&role=ANALYST") {
		t.Errorf("expected DSN to contain role, got %q", dsn)
	}
}

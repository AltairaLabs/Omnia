/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package analyticsfactory_test

import (
	"context"
	"testing"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/analyticsfactory"
)

func TestFactory_Ping_UnsupportedProvider_BigQuery(t *testing.T) {
	f := &analyticsfactory.Factory{}
	spec := corev1alpha1.SessionAnalyticsSyncSpec{
		Provider: corev1alpha1.AnalyticsProviderBigQuery,
	}
	err := f.Ping(context.Background(), spec)
	if err == nil {
		t.Fatal("expected error for unsupported provider, got nil")
	}
	want := `unsupported analytics provider: "bigquery"`
	if err.Error() != want {
		t.Errorf("unexpected error message:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestFactory_Ping_UnsupportedProvider_ClickHouse(t *testing.T) {
	f := &analyticsfactory.Factory{}
	spec := corev1alpha1.SessionAnalyticsSyncSpec{
		Provider: corev1alpha1.AnalyticsProviderClickHouse,
	}
	err := f.Ping(context.Background(), spec)
	if err == nil {
		t.Fatal("expected error for unsupported provider, got nil")
	}
	want := `unsupported analytics provider: "clickhouse"`
	if err.Error() != want {
		t.Errorf("unexpected error message:\n  got:  %q\n  want: %q", err.Error(), want)
	}
}

func TestFactory_Ping_Snowflake_MissingConfig(t *testing.T) {
	f := &analyticsfactory.Factory{}
	spec := corev1alpha1.SessionAnalyticsSyncSpec{
		Provider: corev1alpha1.AnalyticsProviderSnowflake,
		// Snowflake is nil — no config provided
	}
	err := f.Ping(context.Background(), spec)
	if err == nil {
		t.Fatal("expected error when snowflake config is nil, got nil")
	}
	if err.Error() != "snowflake configuration is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFactory_Ping_Snowflake_InvalidConfig(t *testing.T) {
	f := &analyticsfactory.Factory{}
	spec := corev1alpha1.SessionAnalyticsSyncSpec{
		Provider: corev1alpha1.AnalyticsProviderSnowflake,
		Snowflake: &corev1alpha1.SnowflakeConfig{
			Account:   "invalid-account",
			Database:  "test_db",
			Schema:    "public",
			Warehouse: "test_wh",
		},
	}
	// Init will fail because the account doesn't exist — exercises the
	// pingSnowflake path through Config construction and Init error.
	err := f.Ping(context.Background(), spec)
	if err == nil {
		t.Fatal("expected error for invalid snowflake account, got nil")
	}
	// The error should be wrapped with "snowflake ping:" prefix
	if len(err.Error()) < 16 || err.Error()[:16] != "snowflake ping: " {
		t.Errorf("expected error prefixed with 'snowflake ping: ', got: %v", err)
	}
}

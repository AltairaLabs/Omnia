/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

// Package analyticsfactory provides the AnalyticsProviderFactory implementation
// used by the SessionAnalyticsSync controller for connectivity checks.
package analyticsfactory

import (
	"context"
	"fmt"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/ee/pkg/analytics/snowflake"
)

// Factory implements the AnalyticsProviderFactory interface used by the
// SessionAnalyticsSync controller to perform connectivity checks.
type Factory struct{}

// Ping verifies connectivity to the analytics provider described by spec.
// It creates a temporary provider, initialises it (which pings the backend),
// then closes it. The provider is never used for sync — only for the Ping.
func (f *Factory) Ping(ctx context.Context, spec corev1alpha1.SessionAnalyticsSyncSpec) error {
	switch spec.Provider {
	case corev1alpha1.AnalyticsProviderSnowflake:
		return pingSnowflake(ctx, spec)
	default:
		return fmt.Errorf("unsupported analytics provider: %q", spec.Provider)
	}
}

// pingSnowflake creates a temporary Snowflake provider, calls Init (which
// opens the connection and pings the database), then closes it.
func pingSnowflake(ctx context.Context, spec corev1alpha1.SessionAnalyticsSyncSpec) error {
	if spec.Snowflake == nil {
		return fmt.Errorf("snowflake configuration is required")
	}

	cfg := &snowflake.Config{
		Account:   spec.Snowflake.Account,
		Database:  spec.Snowflake.Database,
		Schema:    spec.Snowflake.Schema,
		Warehouse: spec.Snowflake.Warehouse,
		Role:      spec.Snowflake.Role,
	}

	p := snowflake.NewProvider(cfg, nil)
	if err := p.Init(ctx); err != nil {
		return fmt.Errorf("snowflake ping: %w", err)
	}
	return p.Close()
}

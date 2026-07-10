/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package license

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestLicense_CanUseCustomFacade is the truth table for the custom-facade
// entitlement (#1774): enterprise tier grants it via IsEnterprise(), a
// non-enterprise license only when the per-feature bit is set, and open-core
// never.
func TestLicense_CanUseCustomFacade(t *testing.T) {
	tests := []struct {
		name    string
		license *License
		want    bool
	}{
		{
			name:    "dev license grants custom facade",
			license: DevLicense(),
			want:    true,
		},
		{
			name:    "open-core license denies custom facade",
			license: OpenCoreLicense(),
			want:    false,
		},
		{
			name: "enterprise tier grants even without the feature bit",
			license: &License{
				Tier:     TierEnterprise,
				Features: Features{CustomFacade: false},
			},
			want: true,
		},
		{
			name: "feature bit grants on a non-enterprise tier",
			license: &License{
				Tier:     TierOpenCore,
				Features: Features{CustomFacade: true},
			},
			want: true,
		},
		{
			name: "open-core tier without the bit denies",
			license: &License{
				Tier:     TierOpenCore,
				Features: Features{CustomFacade: false},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.license.CanUseCustomFacade(); got != tt.want {
				t.Errorf("CanUseCustomFacade() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestValidator_ValidateCustomFacade covers allow (dev license), deny
// (open-core), and expired-license paths.
func TestValidator_ValidateCustomFacade(t *testing.T) {
	t.Run("dev license allows", func(t *testing.T) {
		v, err := NewValidator(nil, WithDevMode())
		if err != nil {
			t.Fatalf("NewValidator: %v", err)
		}
		if err := v.ValidateCustomFacade(context.Background()); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("open-core denies with a custom-facade error", func(t *testing.T) {
		// A validator with no client and no dev mode degrades to open-core.
		v, err := NewValidator(nil)
		if err != nil {
			t.Fatalf("NewValidator: %v", err)
		}
		err = v.ValidateCustomFacade(context.Background())
		if err == nil {
			t.Fatal("expected an error on open-core, got nil")
		}
		var valErr *ValidationError
		if !errors.As(err, &valErr) {
			t.Fatalf("expected *ValidationError, got %T", err)
		}
		if valErr.Feature != "custom_facade" {
			t.Errorf("Feature = %q, want %q", valErr.Feature, "custom_facade")
		}
		if valErr.UpgradeURL != DefaultUpgradeURL {
			t.Errorf("UpgradeURL = %q, want %q", valErr.UpgradeURL, DefaultUpgradeURL)
		}
	})

	t.Run("expired enterprise license reports expiry", func(t *testing.T) {
		v, err := NewValidator(nil)
		if err != nil {
			t.Fatalf("NewValidator: %v", err)
		}
		// Prime the cache with an expired enterprise license so
		// GetLicenseOrDefault returns it directly.
		v.cache = &License{
			Tier:      TierEnterprise,
			Features:  Features{CustomFacade: true},
			ExpiresAt: time.Now().Add(-time.Hour),
		}
		v.cacheExp = time.Now().Add(time.Hour)

		err = v.ValidateCustomFacade(context.Background())
		if err == nil {
			t.Fatal("expected an expiry error, got nil")
		}
		var valErr *ValidationError
		if !errors.As(err, &valErr) {
			t.Fatalf("expected *ValidationError, got %T", err)
		}
		if valErr.Feature != "license_expired" {
			t.Errorf("Feature = %q, want %q", valErr.Feature, "license_expired")
		}
	})
}

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestExpandDays(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain hours", "6h", "6h"},
		{"single day", "1d", "24h"},
		{"multi day", "30d", "720h"},
		{"big value", "365d", "8760h"},
		{"days plus hours", "1d2h", "24h2h"},
		{"mixed order", "2h30m", "2h30m"},
		{"no digits empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := expandDays(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestExpandDays_DanglingD(t *testing.T) {
	_, err := expandDays("d")
	assert.Error(t, err)
}

func TestParseExtendedDuration(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"6h", 6 * time.Hour},
		{"90d", 90 * 24 * time.Hour},
		{"1d12h", 36 * time.Hour},
		{"30m", 30 * time.Minute},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseExtendedDuration(tc.in)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseExtendedDuration_Empty(t *testing.T) {
	_, err := parseExtendedDuration("")
	assert.Error(t, err)
}

func TestValidateCronSchedule(t *testing.T) {
	good := []string{
		"0 3 * * *",
		"*/15 * * * *",
		"0 */6 * * *",
		"@every 10m",
		"@daily",
		"@hourly",
	}
	for _, s := range good {
		if err := validateCronSchedule(s); err != nil {
			t.Errorf("valid schedule %q rejected: %v", s, err)
		}
	}

	bad := []string{
		"",
		"not a cron",
		"every minute",
		"@monthlyly",
	}
	for _, s := range bad {
		if err := validateCronSchedule(s); err == nil {
			t.Errorf("invalid schedule %q accepted", s)
		}
	}
}

func TestValidateWeight(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"empty ok", "", false},
		{"zero", "0", false},
		{"one", "1", false},
		{"mid", "0.5", false},
		{"bad format", "abc", true},
		{"negative", "-0.1", true},
		{"too large", "1.5", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWeight("field", tc.in)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateTTL_DefaultExceedsMaxAge(t *testing.T) {
	cfg := &omniav1alpha1.MemoryTTLConfig{Default: "180d", MaxAge: "90d"}
	err := validateTTL(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must not exceed maxAge")
}

func TestValidateTTL_DefaultWithinMaxAgeOK(t *testing.T) {
	cfg := &omniav1alpha1.MemoryTTLConfig{Default: "30d", MaxAge: "90d"}
	assert.NoError(t, validateTTL(cfg))
}

func TestValidateTTL_BadDuration(t *testing.T) {
	cfg := &omniav1alpha1.MemoryTTLConfig{Default: "not-a-duration"}
	assert.Error(t, validateTTL(cfg))
}

func TestValidateDecay_BadMinScore(t *testing.T) {
	cfg := &omniav1alpha1.MemoryDecayConfig{MinScore: "2.0"}
	err := validateDecay(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "between 0 and 1")
}

func TestValidateDecay_HappyPath(t *testing.T) {
	cfg := &omniav1alpha1.MemoryDecayConfig{
		MinScore: "0.2",
		ScoreFormula: &omniav1alpha1.MemoryDecayScoreFormula{
			ConfidenceWeight:      "0.5",
			AccessFrequencyWeight: "0.3",
			RecencyWeight:         "0.2",
		},
	}
	assert.NoError(t, validateDecay(cfg))
}

func TestValidateTierConfig_NestedCategoryInvalid(t *testing.T) {
	cfg := &omniav1alpha1.MemoryTierConfig{
		PerCategory: map[string]omniav1alpha1.MemoryTierLeafConfig{
			"memory:health": {
				TTL: &omniav1alpha1.MemoryTTLConfig{Default: "nope"},
			},
		},
	}
	err := validateTierConfig(cfg, "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "perCategory[memory:health]")
}

func TestValidateRetentionDefaults_BadSchedule(t *testing.T) {
	d := &omniav1alpha1.MemoryRetentionDefaults{
		Tiers:    omniav1alpha1.MemoryRetentionTierSet{},
		Schedule: "not a cron",
	}
	err := validateRetentionDefaults(d)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schedule")
}

func TestValidateRetentionDefaults_AllModes(t *testing.T) {
	d := &omniav1alpha1.MemoryRetentionDefaults{
		Tiers: omniav1alpha1.MemoryRetentionTierSet{
			Institutional: &omniav1alpha1.MemoryTierConfig{Mode: omniav1alpha1.MemoryRetentionModeManual},
			Agent: &omniav1alpha1.MemoryTierConfig{
				Mode: omniav1alpha1.MemoryRetentionModeComposite,
				TTL:  &omniav1alpha1.MemoryTTLConfig{Default: "180d", MaxAge: "365d"},
				Decay: &omniav1alpha1.MemoryDecayConfig{
					MinScore: "0.2",
				},
				LRU: &omniav1alpha1.MemoryLRUConfig{StaleAfter: "120d"},
			},
			User: &omniav1alpha1.MemoryTierConfig{
				Mode: omniav1alpha1.MemoryRetentionModeTTL,
				TTL:  &omniav1alpha1.MemoryTTLConfig{Default: "90d", MaxAge: "365d"},
			},
		},
		Schedule: "0 3 * * *",
	}
	assert.NoError(t, validateRetentionDefaults(d))
}

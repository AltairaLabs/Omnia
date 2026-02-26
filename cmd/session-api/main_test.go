/*
Copyright 2025.

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

package main

import (
	"testing"
	"time"
)

func TestEnvInt32(t *testing.T) {
	tests := []struct {
		name string
		env  string
		def  int32
		want int32
	}{
		{"empty returns default", "", 25, 25},
		{"valid value", "10", 25, 10},
		{"invalid value returns default", "abc", 25, 25},
		{"zero is valid", "0", 25, 0},
		{"negative value", "-5", 25, -5},
		{"overflow returns default", "9999999999999", 25, 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_INT32_" + tt.name
			if tt.env != "" {
				t.Setenv(key, tt.env)
			}
			got := envInt32(key, tt.def)
			if got != tt.want {
				t.Errorf("envInt32(%q, %d) = %d, want %d", key, tt.def, got, tt.want)
			}
		})
	}
}

func TestEnvDuration(t *testing.T) {
	tests := []struct {
		name string
		env  string
		def  time.Duration
		want time.Duration
	}{
		{"empty returns default", "", time.Hour, time.Hour},
		{"valid duration", "5m", time.Hour, 5 * time.Minute},
		{"invalid value returns default", "not-a-duration", time.Hour, time.Hour},
		{"zero is valid", "0s", time.Hour, 0},
		{"complex duration", "1h30m", time.Hour, 90 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_DURATION_" + tt.name
			if tt.env != "" {
				t.Setenv(key, tt.env)
			}
			got := envDuration(key, tt.def)
			if got != tt.want {
				t.Errorf("envDuration(%q, %v) = %v, want %v", key, tt.def, got, tt.want)
			}
		})
	}
}

func TestEnvFallback(t *testing.T) {
	tests := []struct {
		name       string
		initial    string
		defaultVal string
		envVal     string
		want       string
	}{
		{"env overrides default", "", "", "from-env", "from-env"},
		{"flag value kept when non-default", "flag-val", "", "", "flag-val"},
		{"empty env ignored", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_FALLBACK_" + tt.name
			if tt.envVal != "" {
				t.Setenv(key, tt.envVal)
			}
			val := tt.initial
			envFallback(&val, tt.defaultVal, key)
			if val != tt.want {
				t.Errorf("envFallback() = %q, want %q", val, tt.want)
			}
		})
	}
}

func TestEnvBoolFallback(t *testing.T) {
	tests := []struct {
		name    string
		initial bool
		envVal  string
		want    bool
	}{
		{"true from env", false, "true", true},
		{"non-true env ignored", false, "yes", false},
		{"already true stays true", true, "", true},
		{"empty env keeps false", false, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_BOOL_" + tt.name
			if tt.envVal != "" {
				t.Setenv(key, tt.envVal)
			}
			val := tt.initial
			envBoolFallback(&val, key)
			if val != tt.want {
				t.Errorf("envBoolFallback() = %v, want %v", val, tt.want)
			}
		})
	}
}

func TestPoolConfigDefaults(t *testing.T) {
	if defaultMaxConns != 25 {
		t.Errorf("expected defaultMaxConns=25, got %d", defaultMaxConns)
	}
	if defaultMinConns != 5 {
		t.Errorf("expected defaultMinConns=5, got %d", defaultMinConns)
	}
	if defaultMaxConnLifetime != time.Hour {
		t.Errorf("expected defaultMaxConnLifetime=1h, got %v", defaultMaxConnLifetime)
	}
	if defaultMaxConnIdleTime != 30*time.Minute {
		t.Errorf("expected defaultMaxConnIdleTime=30m, got %v", defaultMaxConnIdleTime)
	}
}

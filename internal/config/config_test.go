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

package config

import (
	"crypto/tls"
	"testing"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()

	if opts.MetricsAddr != "0" {
		t.Errorf("expected MetricsAddr to be '0', got %q", opts.MetricsAddr)
	}
	if opts.ProbeAddr != ":8081" {
		t.Errorf("expected ProbeAddr to be ':8081', got %q", opts.ProbeAddr)
	}
	if opts.EnableLeaderElection {
		t.Error("expected EnableLeaderElection to be false")
	}
	if !opts.SecureMetrics {
		t.Error("expected SecureMetrics to be true")
	}
	if opts.EnableHTTP2 {
		t.Error("expected EnableHTTP2 to be false")
	}
	if opts.WebhookCertName != "tls.crt" {
		t.Errorf("expected WebhookCertName to be 'tls.crt', got %q", opts.WebhookCertName)
	}
	if opts.WebhookCertKey != "tls.key" {
		t.Errorf("expected WebhookCertKey to be 'tls.key', got %q", opts.WebhookCertKey)
	}
	if opts.MetricsCertName != "tls.crt" {
		t.Errorf("expected MetricsCertName to be 'tls.crt', got %q", opts.MetricsCertName)
	}
	if opts.MetricsCertKey != "tls.key" {
		t.Errorf("expected MetricsCertKey to be 'tls.key', got %q", opts.MetricsCertKey)
	}
}

func TestOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name:    "default options are valid",
			opts:    DefaultOptions(),
			wantErr: false,
		},
		{
			name: "custom options are valid",
			opts: Options{
				MetricsAddr:          ":8443",
				ProbeAddr:            ":9090",
				EnableLeaderElection: true,
				SecureMetrics:        false,
				EnableHTTP2:          true,
			},
			wantErr: false,
		},
		{
			name:    "empty options are valid",
			opts:    Options{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTLSConfig_IsConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  TLSConfig
		want bool
	}{
		{
			name: "configured with cert dir",
			cfg: TLSConfig{
				CertDir:  "/path/to/certs",
				CertName: "tls.crt",
				KeyName:  "tls.key",
			},
			want: true,
		},
		{
			name: "not configured - empty cert dir",
			cfg: TLSConfig{
				CertDir:  "",
				CertName: "tls.crt",
				KeyName:  "tls.key",
			},
			want: false,
		},
		{
			name: "not configured - zero value",
			cfg:  TLSConfig{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.IsConfigured(); got != tt.want {
				t.Errorf("IsConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOptions_GetWebhookTLSConfig(t *testing.T) {
	opts := Options{
		WebhookCertPath: "/webhook/certs",
		WebhookCertName: "webhook.crt",
		WebhookCertKey:  "webhook.key",
	}

	cfg := opts.GetWebhookTLSConfig()

	if cfg.CertDir != opts.WebhookCertPath {
		t.Errorf("expected CertDir %q, got %q", opts.WebhookCertPath, cfg.CertDir)
	}
	if cfg.CertName != opts.WebhookCertName {
		t.Errorf("expected CertName %q, got %q", opts.WebhookCertName, cfg.CertName)
	}
	if cfg.KeyName != opts.WebhookCertKey {
		t.Errorf("expected KeyName %q, got %q", opts.WebhookCertKey, cfg.KeyName)
	}
}

func TestOptions_GetMetricsTLSConfig(t *testing.T) {
	opts := Options{
		MetricsCertPath: "/metrics/certs",
		MetricsCertName: "metrics.crt",
		MetricsCertKey:  "metrics.key",
	}

	cfg := opts.GetMetricsTLSConfig()

	if cfg.CertDir != opts.MetricsCertPath {
		t.Errorf("expected CertDir %q, got %q", opts.MetricsCertPath, cfg.CertDir)
	}
	if cfg.CertName != opts.MetricsCertName {
		t.Errorf("expected CertName %q, got %q", opts.MetricsCertName, cfg.CertName)
	}
	if cfg.KeyName != opts.MetricsCertKey {
		t.Errorf("expected KeyName %q, got %q", opts.MetricsCertKey, cfg.KeyName)
	}
}

func TestDisableHTTP2TLSConfig(t *testing.T) {
	modifier := DisableHTTP2TLSConfig()

	cfg := &tls.Config{}
	modifier(cfg)

	if len(cfg.NextProtos) != 1 {
		t.Fatalf("expected 1 protocol, got %d", len(cfg.NextProtos))
	}
	if cfg.NextProtos[0] != "http/1.1" {
		t.Errorf("expected 'http/1.1', got %q", cfg.NextProtos[0])
	}
}

func TestOptions_BuildTLSOptions(t *testing.T) {
	tests := []struct {
		name        string
		enableHTTP2 bool
		wantLen     int
	}{
		{
			name:        "HTTP/2 disabled - should have modifier",
			enableHTTP2: false,
			wantLen:     1,
		},
		{
			name:        "HTTP/2 enabled - no modifier",
			enableHTTP2: true,
			wantLen:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{EnableHTTP2: tt.enableHTTP2}
			tlsOpts := opts.BuildTLSOptions()

			if len(tlsOpts) != tt.wantLen {
				t.Errorf("expected %d TLS options, got %d", tt.wantLen, len(tlsOpts))
			}

			// Verify the modifier works if present
			if len(tlsOpts) > 0 {
				cfg := &tls.Config{}
				tlsOpts[0](cfg)
				if len(cfg.NextProtos) == 0 || cfg.NextProtos[0] != "http/1.1" {
					t.Error("TLS modifier did not disable HTTP/2 correctly")
				}
			}
		})
	}
}

func TestLeaderElectionID(t *testing.T) {
	id := LeaderElectionID()
	expected := "4416a20d.altairalabs.ai"

	if id != expected {
		t.Errorf("expected leader election ID %q, got %q", expected, id)
	}
}

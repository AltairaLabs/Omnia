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

// Package config provides configuration management for the Omnia operator.
package config

import (
	"crypto/tls"
)

// Options holds all configuration options for the operator.
type Options struct {
	// MetricsAddr is the address the metrics endpoint binds to.
	MetricsAddr string

	// ProbeAddr is the address the probe endpoint binds to.
	ProbeAddr string

	// EnableLeaderElection enables leader election for controller manager.
	EnableLeaderElection bool

	// SecureMetrics indicates if the metrics endpoint should be served via HTTPS.
	SecureMetrics bool

	// EnableHTTP2 enables HTTP/2 for the metrics and webhook servers.
	EnableHTTP2 bool

	// WebhookCertPath is the directory that contains the webhook certificate.
	WebhookCertPath string

	// WebhookCertName is the name of the webhook certificate file.
	WebhookCertName string

	// WebhookCertKey is the name of the webhook key file.
	WebhookCertKey string

	// MetricsCertPath is the directory that contains the metrics server certificate.
	MetricsCertPath string

	// MetricsCertName is the name of the metrics server certificate file.
	MetricsCertName string

	// MetricsCertKey is the name of the metrics server key file.
	MetricsCertKey string
}

// DefaultOptions returns Options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		MetricsAddr:          "0",
		ProbeAddr:            ":8081",
		EnableLeaderElection: false,
		SecureMetrics:        true,
		EnableHTTP2:          false,
		WebhookCertName:      "tls.crt",
		WebhookCertKey:       "tls.key",
		MetricsCertName:      "tls.crt",
		MetricsCertKey:       "tls.key",
	}
}

// Validate checks if the Options are valid.
func (o *Options) Validate() error {
	// Currently no validation errors possible with these options
	// Future: validate port ranges, cert paths exist, etc.
	return nil
}

// TLSConfig holds TLS-related configuration.
type TLSConfig struct {
	// CertDir is the directory containing certificates.
	CertDir string

	// CertName is the certificate filename.
	CertName string

	// KeyName is the key filename.
	KeyName string
}

// IsConfigured returns true if the TLS config has a cert directory specified.
func (t *TLSConfig) IsConfigured() bool {
	return len(t.CertDir) > 0
}

// GetWebhookTLSConfig returns TLS configuration for webhooks.
func (o *Options) GetWebhookTLSConfig() TLSConfig {
	return TLSConfig{
		CertDir:  o.WebhookCertPath,
		CertName: o.WebhookCertName,
		KeyName:  o.WebhookCertKey,
	}
}

// GetMetricsTLSConfig returns TLS configuration for metrics server.
func (o *Options) GetMetricsTLSConfig() TLSConfig {
	return TLSConfig{
		CertDir:  o.MetricsCertPath,
		CertName: o.MetricsCertName,
		KeyName:  o.MetricsCertKey,
	}
}

// DisableHTTP2TLSConfig returns a TLS config modifier that disables HTTP/2.
// This is recommended due to HTTP/2 vulnerabilities (CVE-2023-44487, CVE-2023-39325).
func DisableHTTP2TLSConfig() func(*tls.Config) {
	return func(c *tls.Config) {
		c.NextProtos = []string{"http/1.1"}
	}
}

// BuildTLSOptions returns TLS options based on the configuration.
func (o *Options) BuildTLSOptions() []func(*tls.Config) {
	var tlsOpts []func(*tls.Config)
	if !o.EnableHTTP2 {
		tlsOpts = append(tlsOpts, DisableHTTP2TLSConfig())
	}
	return tlsOpts
}

// LeaderElectionID returns the leader election ID for this operator.
func LeaderElectionID() string {
	return "4416a20d.altairalabs.ai"
}

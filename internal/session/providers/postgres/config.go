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

package postgres

import (
	"crypto/tls"
	"time"
)

// Config holds connection and pool settings for the PostgreSQL warm store provider.
type Config struct {
	// ConnString is the PostgreSQL connection URI (e.g. "postgres://user:pass@host:5432/db").
	ConnString string
	// MaxConns is the maximum number of connections in the pool. Default: 10.
	MaxConns int32
	// MinConns is the minimum number of idle connections maintained. Default: 2.
	MinConns int32
	// MaxConnLifetime is the maximum lifetime of a connection. Default: 1h.
	MaxConnLifetime time.Duration
	// MaxConnIdleTime is the maximum time a connection can be idle. Default: 30m.
	MaxConnIdleTime time.Duration
	// HealthCheckPeriod is the interval between health checks on idle connections. Default: 1m.
	HealthCheckPeriod time.Duration
	// TLS enables TLS when non-nil.
	TLS *tls.Config
}

// DefaultConfig returns a Config with sensible pool defaults. Callers must
// still set ConnString.
func DefaultConfig() Config {
	return Config{
		MaxConns:          10,
		MinConns:          2,
		MaxConnLifetime:   time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: time.Minute,
	}
}

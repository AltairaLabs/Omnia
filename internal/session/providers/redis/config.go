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

package redis

import (
	"crypto/tls"
	"time"
)

const (
	defaultKeyPrefix  = "hot:"
	defaultMaxRetries = 3
)

// Config holds connection and behaviour settings for the Redis hot cache provider.
type Config struct {
	// Addrs lists Redis server addresses. A single address creates a standalone
	// client; multiple addresses create a cluster client.
	Addrs []string
	// Password is used for Redis AUTH.
	Password string
	// DB selects the database number. Ignored in cluster mode.
	DB int
	// KeyPrefix is prepended to every key written by the provider.
	// Default: "hot:".
	KeyPrefix string
	// MaxMessagesPerSession caps the message list length via LTRIM after each
	// append. Zero means unlimited.
	MaxMessagesPerSession int
	// PoolSize overrides the go-redis default connection pool size.
	// Zero uses the library default (10 * GOMAXPROCS).
	PoolSize int
	// MaxRetries is the maximum number of retries for a command. Default: 3.
	MaxRetries int
	// DialTimeout is the timeout for establishing new connections.
	DialTimeout time.Duration
	// ReadTimeout is the timeout for socket reads.
	ReadTimeout time.Duration
	// WriteTimeout is the timeout for socket writes.
	WriteTimeout time.Duration
	// TLS enables TLS when non-nil.
	TLS *tls.Config
}

// DefaultConfig returns a Config with sensible defaults. Callers must still
// set at least one address in Addrs.
func DefaultConfig() Config {
	return Config{
		KeyPrefix:  defaultKeyPrefix,
		MaxRetries: defaultMaxRetries,
	}
}

// Options carries non-connection settings used by NewFromClient.
type Options struct {
	// KeyPrefix is prepended to every key. Default: "hot:".
	KeyPrefix string
	// MaxMessagesPerSession caps the message list length. Zero means unlimited.
	MaxMessagesPerSession int
}

// DefaultOptions returns Options with sensible defaults.
func DefaultOptions() Options {
	return Options{
		KeyPrefix: defaultKeyPrefix,
	}
}

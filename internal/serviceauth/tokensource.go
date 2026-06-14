/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/credentials"
)

// DefaultTokenPath is the standard projected ServiceAccount token mount path.
const DefaultTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// defaultTTL is the cache TTL used when NewTokenSource is given a non-positive
// ttl. The kubelet rotates projected tokens well before expiry, so a short
// re-read interval keeps the cached value fresh without per-call file I/O.
const defaultTTL = 60 * time.Second

// TokenSource returns the current projected ServiceAccount token, re-reading the
// file from disk after a TTL (kubelet rotates it in place). It is safe for
// concurrent use.
//
// When the token file does not exist, Token returns ("", nil) so callers running
// outside a cluster (or with auth disabled) cleanly skip the Authorization
// header rather than erroring.
type TokenSource struct {
	path string
	ttl  time.Duration
	now  func() time.Time // injectable clock for tests

	mu       sync.Mutex
	cached   string
	lastRead time.Time
	loaded   bool
}

// NewTokenSource creates a TokenSource reading from path (defaults to
// DefaultTokenPath when empty), caching the value for ttl (defaults to 60s when
// non-positive).
func NewTokenSource(path string, ttl time.Duration) *TokenSource {
	if path == "" {
		path = DefaultTokenPath
	}
	if ttl <= 0 {
		ttl = defaultTTL
	}
	return &TokenSource{path: path, ttl: ttl, now: time.Now}
}

// Token returns the current token, re-reading from disk if the cached value has
// aged past the TTL. A missing token file yields ("", nil).
func (s *TokenSource) Token() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.loaded && s.now().Sub(s.lastRead) < s.ttl {
		return s.cached, nil
	}

	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			s.cached = ""
			s.lastRead = s.now()
			s.loaded = true
			return "", nil
		}
		return "", fmt.Errorf("read service account token %q: %w", s.path, err)
	}

	s.cached = strings.TrimSpace(string(data))
	s.lastRead = s.now()
	s.loaded = true
	return s.cached, nil
}

// Authorize sets the "Authorization: Bearer <token>" header on r using the
// current token. When the token is empty (file missing / outside cluster) the
// header is left unset, so the request proceeds without auth.
func (s *TokenSource) Authorize(r *http.Request) error {
	token, err := s.Token()
	if err != nil {
		return err
	}
	if token != "" {
		r.Header.Set("Authorization", bearerPrefix+token)
	}
	return nil
}

// PerRPCCredentials returns a gRPC credentials.PerRPCCredentials backed by this
// TokenSource, suitable for OTLP gRPC exporters. Transport security is not
// required because in-cluster traffic may be plaintext within the mesh.
func (s *TokenSource) PerRPCCredentials() credentials.PerRPCCredentials {
	return perRPCToken{src: s}
}

// perRPCToken adapts a TokenSource to grpc credentials.PerRPCCredentials.
type perRPCToken struct {
	src *TokenSource
}

func (p perRPCToken) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	token, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, nil
	}
	return map[string]string{metadataAuthKey: bearerPrefix + token}, nil
}

func (p perRPCToken) RequireTransportSecurity() bool { return false }

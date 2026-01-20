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

package license

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:embed keys/public.pem
var embeddedPublicKey []byte

// License secret configuration.
const (
	// LicenseSecretName is the name of the Secret containing the license.
	LicenseSecretName = "arena-license"
	// LicenseSecretNamespace is the namespace of the license Secret.
	LicenseSecretNamespace = "omnia-system"
	// LicenseSecretKey is the key within the Secret containing the JWT.
	LicenseSecretKey = "license"
)

// Default cache TTL.
const DefaultCacheTTL = 5 * time.Minute

// Validator validates licenses using RS256 JWT tokens.
type Validator struct {
	client    client.Client
	publicKey *rsa.PublicKey
	cache     *License
	cacheExp  time.Time
	cacheTTL  time.Duration
	mu        sync.RWMutex
}

// ValidatorOption configures the Validator.
type ValidatorOption func(*Validator)

// WithCacheTTL sets the cache TTL.
func WithCacheTTL(ttl time.Duration) ValidatorOption {
	return func(v *Validator) {
		v.cacheTTL = ttl
	}
}

// WithPublicKey sets a custom public key (for testing).
func WithPublicKey(key *rsa.PublicKey) ValidatorOption {
	return func(v *Validator) {
		v.publicKey = key
	}
}

// NewValidator creates a new license validator.
func NewValidator(c client.Client, opts ...ValidatorOption) (*Validator, error) {
	v := &Validator{
		client:   c,
		cacheTTL: DefaultCacheTTL,
	}

	for _, opt := range opts {
		opt(v)
	}

	// Parse embedded public key if not provided
	if v.publicKey == nil {
		key, err := parsePublicKey(embeddedPublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse embedded public key: %w", err)
		}
		v.publicKey = key
	}

	return v, nil
}

// GetLicense returns the current license, fetching from the Secret if needed.
// Returns OpenCoreLicense if no license is found or validation fails.
func (v *Validator) GetLicense(ctx context.Context) (*License, error) {
	// Check cache
	v.mu.RLock()
	if v.cache != nil && time.Now().Before(v.cacheExp) {
		license := v.cache
		v.mu.RUnlock()
		return license, nil
	}
	v.mu.RUnlock()

	// Fetch from Secret
	license, err := v.fetchAndValidate(ctx)
	if err != nil {
		// Return open-core license on any error
		return OpenCoreLicense(), err
	}

	// Update cache
	v.mu.Lock()
	v.cache = license
	v.cacheExp = time.Now().Add(v.cacheTTL)
	v.mu.Unlock()

	return license, nil
}

// GetLicenseOrDefault returns the current license, or OpenCoreLicense if not found.
// This is a convenience method that ignores errors.
func (v *Validator) GetLicenseOrDefault(ctx context.Context) *License {
	license, _ := v.GetLicense(ctx)
	return license
}

// InvalidateCache clears the license cache.
func (v *Validator) InvalidateCache() {
	v.mu.Lock()
	v.cache = nil
	v.cacheExp = time.Time{}
	v.mu.Unlock()
}

// fetchAndValidate fetches the license Secret and validates the JWT.
func (v *Validator) fetchAndValidate(ctx context.Context) (*License, error) {
	secret := &corev1.Secret{}
	err := v.client.Get(ctx, types.NamespacedName{
		Name:      LicenseSecretName,
		Namespace: LicenseSecretNamespace,
	}, secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, ErrLicenseNotFound
		}
		return nil, fmt.Errorf("failed to get license secret: %w", err)
	}

	tokenData, ok := secret.Data[LicenseSecretKey]
	if !ok {
		return nil, fmt.Errorf("license secret missing '%s' key", LicenseSecretKey)
	}

	return v.validateToken(string(tokenData))
}

// licenseClaims represents the JWT claims for a license.
type licenseClaims struct {
	jwt.RegisteredClaims
	LicenseID string   `json:"lid"`
	Tier      string   `json:"tier"`
	Customer  string   `json:"customer"`
	Features  Features `json:"features"`
	Limits    Limits   `json:"limits"`
}

// validateToken validates a JWT token and returns the license.
func (v *Validator) validateToken(tokenString string) (*License, error) {
	token, err := jwt.ParseWithClaims(tokenString, &licenseClaims{}, func(token *jwt.Token) (any, error) {
		// Ensure the signing method is RS256
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return v.publicKey, nil
	})
	if err != nil {
		// Check if the error is due to token expiration
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrLicenseExpired
		}
		return nil, fmt.Errorf("%w: %v", ErrInvalidSignature, err)
	}

	claims, ok := token.Claims.(*licenseClaims)
	if !ok || !token.Valid {
		return nil, ErrLicenseInvalid
	}

	// Parse times from claims
	issuedAt := time.Now()
	expiresAt := time.Now().AddDate(1, 0, 0)

	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.Time
	}
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}

	license := &License{
		ID:        claims.LicenseID,
		Tier:      Tier(claims.Tier),
		Customer:  claims.Customer,
		Features:  claims.Features,
		Limits:    claims.Limits,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}

	// Double-check expiration (in case JWT library didn't catch it)
	if license.IsExpired() {
		return nil, ErrLicenseExpired
	}

	return license, nil
}

// parsePublicKey parses a PEM-encoded RSA public key.
func parsePublicKey(pemData []byte) (*rsa.PublicKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaKey, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA public key")
	}

	return rsaKey, nil
}

// ValidateArenaSource validates that the source type is allowed by the license.
func (v *Validator) ValidateArenaSource(ctx context.Context, sourceType string) error {
	license := v.GetLicenseOrDefault(ctx)

	if license.IsExpired() {
		return NewLicenseExpiredError()
	}

	if !license.CanUseSourceType(sourceType) {
		return NewSourceTypeError(sourceType)
	}

	return nil
}

// ValidateArenaJob validates that the job configuration is allowed by the license.
func (v *Validator) ValidateArenaJob(ctx context.Context, jobType string, replicas int, hasSchedule bool) error {
	license := v.GetLicenseOrDefault(ctx)

	if license.IsExpired() {
		return NewLicenseExpiredError()
	}

	if !license.CanUseJobType(jobType) {
		return NewJobTypeError(jobType)
	}

	if !license.CanUseWorkerReplicas(replicas) {
		return NewWorkerReplicasError(replicas, license.Limits.MaxWorkerReplicas)
	}

	if hasSchedule && !license.CanUseScheduling() {
		return NewSchedulingError()
	}

	return nil
}

// ValidateScenarioCount validates that the scenario count is allowed by the license.
func (v *Validator) ValidateScenarioCount(ctx context.Context, count int) error {
	license := v.GetLicenseOrDefault(ctx)

	if license.IsExpired() {
		return NewLicenseExpiredError()
	}

	if !license.CanUseScenarioCount(count) {
		return NewScenarioCountError(count, license.Limits.MaxScenarios)
	}

	return nil
}

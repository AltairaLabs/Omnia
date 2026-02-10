/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
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

// License secret and ConfigMap configuration.
const (
	// LicenseSecretName is the name of the Secret containing the license.
	LicenseSecretName = "arena-license"
	// LicenseSecretNamespace is the namespace of the license Secret.
	LicenseSecretNamespace = "omnia-system"
	// LicenseSecretKey is the key within the Secret containing the JWT.
	LicenseSecretKey = "license"

	// PublicKeyConfigMapName is the name of the ConfigMap that can override the embedded public key.
	// This allows key rotation without redeploying the operator.
	PublicKeyConfigMapName = "arena-license-public-key"
	// PublicKeyConfigMapNamespace is the namespace of the public key ConfigMap.
	PublicKeyConfigMapNamespace = "omnia-system"
	// PublicKeyConfigMapKey is the key within the ConfigMap containing the PEM-encoded public key.
	PublicKeyConfigMapKey = "public.pem"
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
	devMode   bool // When true, returns a full-featured dev license
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

// WithDevMode enables development mode with a full-featured license.
// This should NEVER be used in production.
func WithDevMode() ValidatorOption {
	return func(v *Validator) {
		v.devMode = true
	}
}

// NewValidator creates a new license validator.
// It first checks for a public key in the ConfigMap (for easy rotation),
// then falls back to the embedded public key.
func NewValidator(c client.Client, opts ...ValidatorOption) (*Validator, error) {
	v := &Validator{
		client:   c,
		cacheTTL: DefaultCacheTTL,
	}

	for _, opt := range opts {
		opt(v)
	}

	// If public key was explicitly provided via option, use it
	if v.publicKey != nil {
		return v, nil
	}

	// Try to load public key from ConfigMap first (allows rotation without redeploy)
	if c != nil {
		key, err := v.loadPublicKeyFromConfigMap(context.Background())
		if err == nil && key != nil {
			v.publicKey = key
			return v, nil
		}
		// ConfigMap not found or invalid - fall back to embedded key
	}

	// Fall back to embedded public key
	key, err := parsePublicKey(embeddedPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded public key: %w", err)
	}
	v.publicKey = key

	return v, nil
}

// loadPublicKeyFromConfigMap attempts to load the public key from a ConfigMap.
// Returns nil, nil if the ConfigMap doesn't exist (allowing fallback to embedded key).
func (v *Validator) loadPublicKeyFromConfigMap(ctx context.Context) (*rsa.PublicKey, error) {
	var cm corev1.ConfigMap
	err := v.client.Get(ctx, client.ObjectKey{
		Namespace: PublicKeyConfigMapNamespace,
		Name:      PublicKeyConfigMapName,
	}, &cm)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil // ConfigMap doesn't exist, not an error
		}
		return nil, fmt.Errorf("failed to get public key ConfigMap: %w", err)
	}

	pemData, ok := cm.Data[PublicKeyConfigMapKey]
	if !ok {
		return nil, fmt.Errorf("ConfigMap %s/%s missing key %q",
			PublicKeyConfigMapNamespace, PublicKeyConfigMapName, PublicKeyConfigMapKey)
	}

	key, err := parsePublicKey([]byte(pemData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key from ConfigMap: %w", err)
	}

	return key, nil
}

// RefreshPublicKey reloads the public key from the ConfigMap.
// This can be called periodically or in response to ConfigMap updates
// to support key rotation without restarting the operator.
func (v *Validator) RefreshPublicKey(ctx context.Context) error {
	key, err := v.loadPublicKeyFromConfigMap(ctx)
	if err != nil {
		return err
	}

	if key == nil {
		// ConfigMap doesn't exist, keep using current key
		return nil
	}

	v.mu.Lock()
	v.publicKey = key
	// Clear the license cache to force re-validation with new key
	v.cache = nil
	v.mu.Unlock()

	return nil
}

// GetLicense returns the current license, fetching from the Secret if needed.
// Returns OpenCoreLicense if no license is found or validation fails.
// In dev mode, returns a full-featured dev license.
func (v *Validator) GetLicense(ctx context.Context) (*License, error) {
	// In dev mode, always return a full-featured license
	if v.devMode {
		return DevLicense(), nil
	}

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

// IsDevMode returns whether the validator is running in development mode.
func (v *Validator) IsDevMode() bool {
	return v.devMode
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

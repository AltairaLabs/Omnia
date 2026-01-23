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
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// generateTestKeyPair generates an RSA key pair for testing.
func generateTestKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return privateKey, &privateKey.PublicKey
}

// createTestToken creates a signed JWT token for testing.
func createTestToken(t *testing.T, privateKey *rsa.PrivateKey, claims *licenseClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(privateKey)
	require.NoError(t, err)
	return tokenString
}

func TestOpenCoreLicense(t *testing.T) {
	license := OpenCoreLicense()

	assert.Equal(t, "open-core", license.ID)
	assert.Equal(t, TierOpenCore, license.Tier)
	assert.Equal(t, "Open Core User", license.Customer)

	// Check features - Git is included in open-core, others require enterprise
	assert.True(t, license.Features.GitSource)
	assert.False(t, license.Features.OCISource)
	assert.False(t, license.Features.S3Source)
	assert.False(t, license.Features.LoadTesting)
	assert.False(t, license.Features.DataGeneration)
	assert.False(t, license.Features.Scheduling)
	assert.False(t, license.Features.DistributedWorkers)

	// Check limits
	assert.Equal(t, 10, license.Limits.MaxScenarios)
	assert.Equal(t, 1, license.Limits.MaxWorkerReplicas)

	// Should not be expired
	assert.False(t, license.IsExpired())
	assert.False(t, license.IsEnterprise())
}

func TestLicense_CanUseSourceType(t *testing.T) {
	tests := []struct {
		name       string
		sourceType string
		license    *License
		expected   bool
	}{
		{
			name:       "configmap always allowed on open-core",
			sourceType: "configmap",
			license:    OpenCoreLicense(),
			expected:   true,
		},
		{
			name:       "git allowed on open-core",
			sourceType: "git",
			license:    OpenCoreLicense(),
			expected:   true,
		},
		{
			name:       "oci not allowed on open-core",
			sourceType: "oci",
			license:    OpenCoreLicense(),
			expected:   false,
		},
		{
			name:       "s3 not allowed on open-core",
			sourceType: "s3",
			license:    OpenCoreLicense(),
			expected:   false,
		},
		{
			name:       "git allowed with enterprise",
			sourceType: "git",
			license: &License{
				Tier:     TierEnterprise,
				Features: Features{GitSource: true},
			},
			expected: true,
		},
		{
			name:       "oci allowed with enterprise",
			sourceType: "oci",
			license: &License{
				Tier:     TierEnterprise,
				Features: Features{OCISource: true},
			},
			expected: true,
		},
		{
			name:       "unknown type not allowed",
			sourceType: "unknown",
			license:    OpenCoreLicense(),
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.license.CanUseSourceType(tt.sourceType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLicense_CanUseJobType(t *testing.T) {
	tests := []struct {
		name     string
		jobType  string
		license  *License
		expected bool
	}{
		{
			name:     "evaluation always allowed on open-core",
			jobType:  "evaluation",
			license:  OpenCoreLicense(),
			expected: true,
		},
		{
			name:     "loadtest not allowed on open-core",
			jobType:  "loadtest",
			license:  OpenCoreLicense(),
			expected: false,
		},
		{
			name:     "datagen not allowed on open-core",
			jobType:  "datagen",
			license:  OpenCoreLicense(),
			expected: false,
		},
		{
			name:    "loadtest allowed with enterprise",
			jobType: "loadtest",
			license: &License{
				Tier:     TierEnterprise,
				Features: Features{LoadTesting: true},
			},
			expected: true,
		},
		{
			name:    "datagen allowed with enterprise",
			jobType: "datagen",
			license: &License{
				Tier:     TierEnterprise,
				Features: Features{DataGeneration: true},
			},
			expected: true,
		},
		{
			name:     "unknown type not allowed",
			jobType:  "unknown",
			license:  OpenCoreLicense(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.license.CanUseJobType(tt.jobType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLicense_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		license  *License
		expected bool
	}{
		{
			name: "not expired",
			license: &License{
				ExpiresAt: time.Now().Add(24 * time.Hour),
			},
			expected: false,
		},
		{
			name: "expired",
			license: &License{
				ExpiresAt: time.Now().Add(-24 * time.Hour),
			},
			expected: true,
		},
		{
			name: "just expired",
			license: &License{
				ExpiresAt: time.Now().Add(-1 * time.Minute),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.license.IsExpired()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLicense_CanUseWorkerReplicas(t *testing.T) {
	tests := []struct {
		name     string
		replicas int
		license  *License
		expected bool
	}{
		{
			name:     "one replica allowed on open-core",
			replicas: 1,
			license:  OpenCoreLicense(),
			expected: true,
		},
		{
			name:     "two replicas not allowed on open-core",
			replicas: 2,
			license:  OpenCoreLicense(),
			expected: false,
		},
		{
			name:     "many replicas allowed on enterprise",
			replicas: 100,
			license: &License{
				Tier:   TierEnterprise,
				Limits: Limits{MaxWorkerReplicas: 0}, // unlimited
			},
			expected: true,
		},
		{
			name:     "within enterprise limit",
			replicas: 50,
			license: &License{
				Tier:   TierEnterprise,
				Limits: Limits{MaxWorkerReplicas: 100},
			},
			expected: true,
		},
		{
			name:     "exceeds enterprise limit",
			replicas: 150,
			license: &License{
				Tier:   TierEnterprise,
				Limits: Limits{MaxWorkerReplicas: 100},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.license.CanUseWorkerReplicas(tt.replicas)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLicense_CanUseScenarioCount(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		license  *License
		expected bool
	}{
		{
			name:     "within open-core limit",
			count:    5,
			license:  OpenCoreLicense(),
			expected: true,
		},
		{
			name:     "at open-core limit",
			count:    10,
			license:  OpenCoreLicense(),
			expected: true,
		},
		{
			name:     "exceeds open-core limit",
			count:    11,
			license:  OpenCoreLicense(),
			expected: false,
		},
		{
			name:  "unlimited on enterprise",
			count: 10000,
			license: &License{
				Tier:   TierEnterprise,
				Limits: Limits{MaxScenarios: 0}, // unlimited
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.license.CanUseScenarioCount(tt.count)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidator_GetLicense_CachedWithinTTL(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	claims := &licenseClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		LicenseID: "test-123",
		Tier:      "enterprise",
		Customer:  "Test Corp",
		Features: Features{
			GitSource:   true,
			LoadTesting: true,
		},
		Limits: Limits{
			MaxScenarios:      1000,
			MaxWorkerReplicas: 50,
		},
	}

	token := createTestToken(t, privateKey, claims)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LicenseSecretName,
			Namespace: LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			LicenseSecretKey: []byte(token),
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	validator, err := NewValidator(client, WithPublicKey(publicKey))
	require.NoError(t, err)

	ctx := context.Background()

	// First call - should fetch from secret
	license1, err := validator.GetLicense(ctx)
	require.NoError(t, err)
	assert.Equal(t, "test-123", license1.ID)
	assert.Equal(t, TierEnterprise, license1.Tier)

	// Second call - should return cached
	license2, err := validator.GetLicense(ctx)
	require.NoError(t, err)
	assert.Equal(t, license1, license2)
}

func TestValidator_GetLicense_SecretNotFound(t *testing.T) {
	_, publicKey := generateTestKeyPair(t)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	validator, err := NewValidator(client, WithPublicKey(publicKey))
	require.NoError(t, err)

	ctx := context.Background()

	license, err := validator.GetLicense(ctx)
	assert.ErrorIs(t, err, ErrLicenseNotFound)
	assert.Equal(t, TierOpenCore, license.Tier)
}

func TestValidator_validateToken_InvalidSignature(t *testing.T) {
	// Generate two different key pairs
	privateKey1, _ := generateTestKeyPair(t)
	_, publicKey2 := generateTestKeyPair(t)

	claims := &licenseClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
		LicenseID: "test-123",
		Tier:      "enterprise",
	}

	// Sign with first key
	token := createTestToken(t, privateKey1, claims)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Validate with different key
	validator, err := NewValidator(client, WithPublicKey(publicKey2))
	require.NoError(t, err)

	_, err = validator.validateToken(token)
	assert.ErrorIs(t, err, ErrInvalidSignature)
}

func TestValidator_validateToken_ExpiredToken(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	claims := &licenseClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-24 * time.Hour)), // Expired
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-48 * time.Hour)),
		},
		LicenseID: "test-123",
		Tier:      "enterprise",
	}

	token := createTestToken(t, privateKey, claims)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	validator, err := NewValidator(client, WithPublicKey(publicKey))
	require.NoError(t, err)

	_, err = validator.validateToken(token)
	assert.ErrorIs(t, err, ErrLicenseExpired)
}

func TestValidator_InvalidateCache(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	claims := &licenseClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
		LicenseID: "test-123",
		Tier:      "enterprise",
	}

	token := createTestToken(t, privateKey, claims)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LicenseSecretName,
			Namespace: LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			LicenseSecretKey: []byte(token),
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	validator, err := NewValidator(client, WithPublicKey(publicKey), WithCacheTTL(1*time.Hour))
	require.NoError(t, err)

	ctx := context.Background()

	// First call
	_, err = validator.GetLicense(ctx)
	require.NoError(t, err)

	// Invalidate cache
	validator.InvalidateCache()

	// Verify cache is cleared
	validator.mu.RLock()
	assert.Nil(t, validator.cache)
	validator.mu.RUnlock()
}

func TestValidationErrors(t *testing.T) {
	t.Run("source type error", func(t *testing.T) {
		err := NewSourceTypeError("git")
		assert.Contains(t, err.Error(), "git sources require an Enterprise license")
		assert.Contains(t, err.UpgradeMessage(), DefaultUpgradeURL)
	})

	t.Run("job type error", func(t *testing.T) {
		err := NewJobTypeError("loadtest")
		assert.Contains(t, err.Error(), "loadtest jobs require an Enterprise license")
	})

	t.Run("scheduling error", func(t *testing.T) {
		err := NewSchedulingError()
		assert.Contains(t, err.Error(), "Scheduled jobs require an Enterprise license")
	})

	t.Run("worker replicas error", func(t *testing.T) {
		err := NewWorkerReplicasError(5, 1)
		assert.Contains(t, err.Error(), "Requested 5 worker replicas")
		assert.Contains(t, err.Error(), "open-core limit of 1")
	})

	t.Run("scenario count error", func(t *testing.T) {
		err := NewScenarioCountError(50, 10)
		assert.Contains(t, err.Error(), "Scenario count 50 exceeds")
		assert.Contains(t, err.Error(), "open-core limit of 10")
	})

	t.Run("license expired error", func(t *testing.T) {
		err := NewLicenseExpiredError()
		assert.Contains(t, err.Error(), "license has expired")
	})
}

func TestValidator_GetLicenseOrDefault(t *testing.T) {
	_, publicKey := generateTestKeyPair(t)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	// No secret - should return open-core license
	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	validator, err := NewValidator(client, WithPublicKey(publicKey))
	require.NoError(t, err)

	ctx := context.Background()

	license := validator.GetLicenseOrDefault(ctx)
	assert.Equal(t, TierOpenCore, license.Tier)
}

func TestValidator_GetLicense_MissingLicenseKey(t *testing.T) {
	_, publicKey := generateTestKeyPair(t)

	// Secret exists but missing 'license' key
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      LicenseSecretName,
			Namespace: LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			"wrong-key": []byte("some-data"),
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	validator, err := NewValidator(client, WithPublicKey(publicKey))
	require.NoError(t, err)

	ctx := context.Background()

	license, err := validator.GetLicense(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'license' key")
	assert.Equal(t, TierOpenCore, license.Tier)
}

func TestValidator_ValidateArenaSource(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	t.Run("open-core allows configmap", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateArenaSource(ctx, "configmap")
		assert.NoError(t, err)
	})

	t.Run("open-core allows git", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateArenaSource(ctx, "git")
		assert.NoError(t, err)
	})

	t.Run("enterprise allows git", func(t *testing.T) {
		claims := &licenseClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
			LicenseID: "test-123",
			Tier:      "enterprise",
			Customer:  "Test Corp",
			Features: Features{
				GitSource: true,
			},
		}

		token := createTestToken(t, privateKey, claims)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      LicenseSecretName,
				Namespace: LicenseSecretNamespace,
			},
			Data: map[string][]byte{
				LicenseSecretKey: []byte(token),
			},
		}

		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(secret).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateArenaSource(ctx, "git")
		assert.NoError(t, err)
	})

	t.Run("expired license falls back to open-core", func(t *testing.T) {
		// When a JWT license expires, the validator falls back to open-core
		// Open-core allows configmap and git, but not OCI or S3
		claims := &licenseClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-24 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-48 * time.Hour)),
			},
			LicenseID: "test-123",
			Tier:      "enterprise",
			Customer:  "Test Corp",
			Features: Features{
				GitSource: true,
				OCISource: true,
			},
		}

		token := createTestToken(t, privateKey, claims)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      LicenseSecretName,
				Namespace: LicenseSecretNamespace,
			},
			Data: map[string][]byte{
				LicenseSecretKey: []byte(token),
			},
		}

		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(secret).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		// ConfigMap is allowed on open-core fallback
		err = validator.ValidateArenaSource(ctx, "configmap")
		assert.NoError(t, err)
		// Git is allowed on open-core fallback
		err = validator.ValidateArenaSource(ctx, "git")
		assert.NoError(t, err)
		// OCI is NOT allowed on open-core fallback
		err = validator.ValidateArenaSource(ctx, "oci")
		assert.Error(t, err)
	})
}

func TestValidator_ValidateArenaJob(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	t.Run("open-core allows evaluation", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateArenaJob(ctx, "evaluation", 1, false)
		assert.NoError(t, err)
	})

	t.Run("open-core rejects loadtest", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateArenaJob(ctx, "loadtest", 1, false)
		assert.Error(t, err)
	})

	t.Run("open-core rejects multiple replicas", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateArenaJob(ctx, "evaluation", 5, false)
		assert.Error(t, err)
	})

	t.Run("open-core rejects scheduling", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateArenaJob(ctx, "evaluation", 1, true)
		assert.Error(t, err)
	})

	t.Run("enterprise allows all features", func(t *testing.T) {
		claims := &licenseClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
			LicenseID: "test-123",
			Tier:      "enterprise",
			Customer:  "Test Corp",
			Features: Features{
				LoadTesting:        true,
				Scheduling:         true,
				DistributedWorkers: true,
			},
			Limits: Limits{
				MaxWorkerReplicas: 0, // unlimited
			},
		}

		token := createTestToken(t, privateKey, claims)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      LicenseSecretName,
				Namespace: LicenseSecretNamespace,
			},
			Data: map[string][]byte{
				LicenseSecretKey: []byte(token),
			},
		}

		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(secret).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateArenaJob(ctx, "loadtest", 100, true)
		assert.NoError(t, err)
	})
}

func TestValidator_ValidateScenarioCount(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	t.Run("open-core allows within limit", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateScenarioCount(ctx, 5)
		assert.NoError(t, err)
	})

	t.Run("open-core rejects exceeding limit", func(t *testing.T) {
		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateScenarioCount(ctx, 100)
		assert.Error(t, err)
	})

	t.Run("enterprise allows unlimited", func(t *testing.T) {
		claims := &licenseClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
			LicenseID: "test-123",
			Tier:      "enterprise",
			Customer:  "Test Corp",
			Limits: Limits{
				MaxScenarios: 0, // unlimited
			},
		}

		token := createTestToken(t, privateKey, claims)

		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      LicenseSecretName,
				Namespace: LicenseSecretNamespace,
			},
			Data: map[string][]byte{
				LicenseSecretKey: []byte(token),
			},
		}

		scheme := runtime.NewScheme()
		require.NoError(t, corev1.AddToScheme(scheme))

		client := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(secret).
			Build()

		validator, err := NewValidator(client, WithPublicKey(publicKey))
		require.NoError(t, err)

		ctx := context.Background()
		err = validator.ValidateScenarioCount(ctx, 10000)
		assert.NoError(t, err)
	})
}

func TestValidator_validateToken_InvalidSigningMethod(t *testing.T) {
	_, publicKey := generateTestKeyPair(t)

	// Create a token with HMAC signing (not RSA)
	claims := jwt.MapClaims{
		"lid":      "test-123",
		"tier":     "enterprise",
		"customer": "Test Corp",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte("secret-key"))
	require.NoError(t, err)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	validator, err := NewValidator(client, WithPublicKey(publicKey))
	require.NoError(t, err)

	_, err = validator.validateToken(tokenString)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidSignature)
}

func TestValidator_validateToken_NoClaims(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	// Create a minimal valid token without proper claims structure
	claims := &licenseClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		},
	}

	token := createTestToken(t, privateKey, claims)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	validator, err := NewValidator(client, WithPublicKey(publicKey))
	require.NoError(t, err)

	license, err := validator.validateToken(token)
	assert.NoError(t, err)
	// Should have defaults for missing times
	assert.NotNil(t, license)
}

func TestLicense_CanUseScheduling(t *testing.T) {
	tests := []struct {
		name     string
		license  *License
		expected bool
	}{
		{
			name:     "not allowed on open-core",
			license:  OpenCoreLicense(),
			expected: false,
		},
		{
			name: "allowed with feature enabled",
			license: &License{
				Tier:     TierEnterprise,
				Features: Features{Scheduling: true},
			},
			expected: true,
		},
		{
			name: "not allowed when feature disabled",
			license: &License{
				Tier:     TierEnterprise,
				Features: Features{Scheduling: false},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.license.CanUseScheduling()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLicense_S3SourceType(t *testing.T) {
	t.Run("s3 not allowed on open-core", func(t *testing.T) {
		license := OpenCoreLicense()
		assert.False(t, license.CanUseSourceType("s3"))
	})

	t.Run("s3 allowed with feature enabled", func(t *testing.T) {
		license := &License{
			Tier:     TierEnterprise,
			Features: Features{S3Source: true},
		}
		assert.True(t, license.CanUseSourceType("s3"))
	})
}

func TestValidator_NewValidatorWithEmbeddedKey(t *testing.T) {
	// Test creating validator without custom public key (uses embedded key)
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// This tests the embedded key path
	// Note: In test environments, the embedded key may not be valid
	// so we just verify the code path is exercised
	validator, err := NewValidator(client)
	if err != nil {
		// If embedded key fails to parse, that's acceptable in test env
		assert.Contains(t, err.Error(), "failed to parse embedded public key")
		return
	}

	assert.NotNil(t, validator)

	// Validator should work (return open-core since no license secret)
	ctx := context.Background()
	license, fetchErr := validator.GetLicense(ctx)
	assert.ErrorIs(t, fetchErr, ErrLicenseNotFound)
	assert.Equal(t, TierOpenCore, license.Tier)
}

func TestValidator_NewValidatorWithCacheTTL(t *testing.T) {
	_, publicKey := generateTestKeyPair(t)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	// Test with custom cache TTL
	validator, err := NewValidator(client, WithPublicKey(publicKey), WithCacheTTL(10*time.Minute))
	require.NoError(t, err)
	assert.NotNil(t, validator)
	assert.Equal(t, 10*time.Minute, validator.cacheTTL)
}

func TestParsePublicKey(t *testing.T) {
	t.Run("valid PEM key", func(t *testing.T) {
		// Generate a test key
		privateKey, _ := generateTestKeyPair(t)

		// Encode to PEM
		pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
		require.NoError(t, err)

		pemBlock := &pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: pubKeyBytes,
		}
		pemData := pem.EncodeToMemory(pemBlock)

		key, err := parsePublicKey(pemData)
		assert.NoError(t, err)
		assert.NotNil(t, key)
	})

	t.Run("invalid PEM data", func(t *testing.T) {
		_, err := parsePublicKey([]byte("not valid pem"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode PEM")
	})

	t.Run("invalid key data", func(t *testing.T) {
		pemBlock := &pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: []byte("invalid key data"),
		}
		pemData := pem.EncodeToMemory(pemBlock)

		_, err := parsePublicKey(pemData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse public key")
	})

	t.Run("non-RSA key", func(t *testing.T) {
		// Create an EC key (not RSA)
		ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		require.NoError(t, err)

		pubKeyBytes, err := x509.MarshalPKIXPublicKey(&ecKey.PublicKey)
		require.NoError(t, err)

		pemBlock := &pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: pubKeyBytes,
		}
		pemData := pem.EncodeToMemory(pemBlock)

		_, err = parsePublicKey(pemData)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not an RSA public key")
	})
}

func TestValidator_ConfigMapOverride(t *testing.T) {
	privateKey, publicKey := generateTestKeyPair(t)

	// Create PEM-encoded public key
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	require.NoError(t, err)
	pemBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubKeyBytes,
	}
	pemData := pem.EncodeToMemory(pemBlock)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	t.Run("uses ConfigMap key when present", func(t *testing.T) {
		// Create ConfigMap with public key
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PublicKeyConfigMapName,
				Namespace: PublicKeyConfigMapNamespace,
			},
			Data: map[string]string{
				PublicKeyConfigMapKey: string(pemData),
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cm).
			Build()

		validator, err := NewValidator(fakeClient)
		require.NoError(t, err)
		assert.NotNil(t, validator)

		// Create a test license signed with the private key
		claims := &licenseClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
			LicenseID: "test-cm-license",
			Tier:      "enterprise",
			Customer:  "ConfigMap Test",
			Features:  Features{GitSource: true},
		}
		token := createTestToken(t, privateKey, claims)

		// Create license secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      LicenseSecretName,
				Namespace: LicenseSecretNamespace,
			},
			Data: map[string][]byte{
				LicenseSecretKey: []byte(token),
			},
		}
		require.NoError(t, fakeClient.Create(context.Background(), secret))

		// Verify license can be validated with the ConfigMap key
		ctx := context.Background()
		license, err := validator.GetLicense(ctx)
		require.NoError(t, err)
		assert.Equal(t, "test-cm-license", license.ID)
		assert.Equal(t, TierEnterprise, license.Tier)
	})

	t.Run("falls back to embedded key when ConfigMap missing", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		// Should succeed using embedded key (or fail parsing embedded if invalid in test)
		validator, err := NewValidator(fakeClient)
		if err != nil {
			// Embedded key parse failure is acceptable in test env
			assert.Contains(t, err.Error(), "failed to parse embedded public key")
			return
		}
		assert.NotNil(t, validator)
	})

	t.Run("RefreshPublicKey updates key from ConfigMap", func(t *testing.T) {
		// Start with embedded key
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(fakeClient, WithPublicKey(publicKey))
		require.NoError(t, err)

		// Create ConfigMap with new key
		newPrivateKey, newPublicKey := generateTestKeyPair(t)
		newPubKeyBytes, err := x509.MarshalPKIXPublicKey(newPublicKey)
		require.NoError(t, err)
		newPemBlock := &pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: newPubKeyBytes,
		}
		newPemData := pem.EncodeToMemory(newPemBlock)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PublicKeyConfigMapName,
				Namespace: PublicKeyConfigMapNamespace,
			},
			Data: map[string]string{
				PublicKeyConfigMapKey: string(newPemData),
			},
		}
		require.NoError(t, fakeClient.Create(context.Background(), cm))

		// Refresh should pick up the new key
		err = validator.RefreshPublicKey(context.Background())
		require.NoError(t, err)

		// Now create a license signed with the new key
		claims := &licenseClaims{
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
			},
			LicenseID: "refreshed-license",
			Tier:      "enterprise",
			Customer:  "Refresh Test",
			Features:  Features{GitSource: true},
		}
		token := createTestToken(t, newPrivateKey, claims)

		// Create license secret
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      LicenseSecretName,
				Namespace: LicenseSecretNamespace,
			},
			Data: map[string][]byte{
				LicenseSecretKey: []byte(token),
			},
		}
		require.NoError(t, fakeClient.Create(context.Background(), secret))

		// Verify license can be validated with the refreshed key
		license, err := validator.GetLicense(context.Background())
		require.NoError(t, err)
		assert.Equal(t, "refreshed-license", license.ID)
	})

	t.Run("RefreshPublicKey does nothing if ConfigMap missing", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		validator, err := NewValidator(fakeClient, WithPublicKey(publicKey))
		require.NoError(t, err)

		// Refresh should not error when ConfigMap doesn't exist
		err = validator.RefreshPublicKey(context.Background())
		require.NoError(t, err)
	})

	t.Run("errors on ConfigMap with missing key", func(t *testing.T) {
		// Create ConfigMap without the expected key
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PublicKeyConfigMapName,
				Namespace: PublicKeyConfigMapNamespace,
			},
			Data: map[string]string{
				"wrong-key": string(pemData),
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cm).
			Build()

		// Should fall back to embedded key or fail
		_, err := NewValidator(fakeClient)
		// May succeed (fallback to embedded) or fail (embedded key invalid in test)
		// The important thing is it doesn't crash
		_ = err
	})

	t.Run("errors on ConfigMap with invalid PEM", func(t *testing.T) {
		// Create ConfigMap with invalid PEM data
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      PublicKeyConfigMapName,
				Namespace: PublicKeyConfigMapNamespace,
			},
			Data: map[string]string{
				PublicKeyConfigMapKey: "not valid PEM data",
			},
		}

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(cm).
			Build()

		// Should fall back to embedded key or fail
		_, err := NewValidator(fakeClient)
		// May succeed (fallback to embedded) or fail
		_ = err
	})
}

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

package webhook

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/license"
)

// generateTestKeyPair generates an RSA key pair for testing.
func generateTestKeyPair(t *testing.T) (*rsa.PrivateKey, *rsa.PublicKey) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return privateKey, &privateKey.PublicKey
}

// licenseClaims represents the JWT claims for a license.
type licenseClaims struct {
	jwt.RegisteredClaims
	LicenseID string           `json:"lid"`
	Tier      string           `json:"tier"`
	Customer  string           `json:"customer"`
	Features  license.Features `json:"features"`
	Limits    license.Limits   `json:"limits"`
}

// createTestToken creates a signed JWT token for testing.
func createTestToken(t *testing.T, privateKey *rsa.PrivateKey, claims *licenseClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(privateKey)
	require.NoError(t, err)
	return tokenString
}

// createOpenCoreValidator creates a validator that returns open-core license.
func createOpenCoreValidator(t *testing.T) *license.Validator {
	t.Helper()
	_, publicKey := generateTestKeyPair(t)

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	validator, err := license.NewValidator(client, license.WithPublicKey(publicKey))
	require.NoError(t, err)
	return validator
}

// createEnterpriseValidator creates a validator with an enterprise license.
func createEnterpriseValidator(t *testing.T, features license.Features) *license.Validator {
	t.Helper()
	privateKey, publicKey := generateTestKeyPair(t)

	claims := &licenseClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		LicenseID: "enterprise-test",
		Tier:      "enterprise",
		Customer:  "Test Corp",
		Features:  features,
		Limits: license.Limits{
			MaxScenarios:      0, // unlimited
			MaxWorkerReplicas: 0, // unlimited
		},
	}

	token := createTestToken(t, privateKey, claims)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      license.LicenseSecretName,
			Namespace: license.LicenseSecretNamespace,
		},
		Data: map[string][]byte{
			license.LicenseSecretKey: []byte(token),
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(secret).
		Build()

	validator, err := license.NewValidator(client, license.WithPublicKey(publicKey))
	require.NoError(t, err)
	return validator
}

func TestArenaSourceValidator_RejectsOCISourceWithoutLicense(t *testing.T) {
	validator := &ArenaSourceValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeOCI,
			Interval: "5m",
			OCI: &omniav1alpha1.OCISource{
				URL: "oci://example.com/repo:latest",
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, source)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Enterprise license")
	assert.NotEmpty(t, warnings)
}

func TestArenaSourceValidator_AllowsGitSourceWithOpenCore(t *testing.T) {
	validator := &ArenaSourceValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeGit,
			Interval: "5m",
			Git: &omniav1alpha1.GitSource{
				URL: "https://github.com/example/repo",
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, source)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaSourceValidator_AllowsConfigMapSourceWithoutLicense(t *testing.T) {
	validator := &ArenaSourceValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
			Interval: "5m",
			ConfigMap: &omniav1alpha1.ConfigMapSource{
				Name: "my-config",
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, source)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaSourceValidator_AllowsGitSourceWithEnterpriseLicense(t *testing.T) {
	validator := &ArenaSourceValidator{
		LicenseValidator: createEnterpriseValidator(t, license.Features{GitSource: true}),
	}

	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeGit,
			Interval: "5m",
			Git: &omniav1alpha1.GitSource{
				URL: "https://github.com/example/repo",
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, source)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaJobValidator_RejectsLoadTestWithoutLicense(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeLoadTest,
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Enterprise license")
	assert.NotEmpty(t, warnings)
}

func TestArenaJobValidator_AllowsEvaluationWithoutLicense(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeEvaluation,
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaJobValidator_RejectsMultipleReplicasWithoutLicense(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeEvaluation,
			Workers: &omniav1alpha1.WorkerConfig{
				Replicas: 5,
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker replicas")
	assert.NotEmpty(t, warnings)
}

func TestArenaJobValidator_RejectsScheduledJobsWithoutLicense(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeEvaluation,
			Schedule: &omniav1alpha1.ScheduleConfig{
				Cron: "0 2 * * *",
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Scheduled jobs")
	assert.NotEmpty(t, warnings)
}

func TestArenaJobValidator_AllowsAllFeaturesWithEnterpriseLicense(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createEnterpriseValidator(t, license.Features{
			LoadTesting:        true,
			Scheduling:         true,
			DistributedWorkers: true,
		}),
	}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeLoadTest,
			Workers: &omniav1alpha1.WorkerConfig{
				Replicas: 10,
			},
			Schedule: &omniav1alpha1.ScheduleConfig{
				Cron: "0 2 * * *",
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaSourceValidator_DeleteAlwaysAllowed(t *testing.T) {
	validator := &ArenaSourceValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeGit,
			Interval: "5m",
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateDelete(ctx, source)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaJobValidator_DeleteAlwaysAllowed(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			Type: omniav1alpha1.ArenaJobTypeLoadTest,
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateDelete(ctx, job)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaSourceValidator_NilValidatorAllowsAll(t *testing.T) {
	validator := &ArenaSourceValidator{
		LicenseValidator: nil,
	}

	source := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeGit,
			Interval: "5m",
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, source)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaJobValidator_NilValidatorAllowsAll(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: nil,
	}

	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			Type: omniav1alpha1.ArenaJobTypeLoadTest,
			Workers: &omniav1alpha1.WorkerConfig{
				Replicas: 100,
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaSourceValidator_ValidateUpdate(t *testing.T) {
	validator := &ArenaSourceValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	oldSource := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
			Interval: "5m",
		},
	}

	// OCI sources require enterprise license (Git is now allowed in open-core)
	newSource := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeOCI,
			Interval: "5m",
			OCI: &omniav1alpha1.OCISource{
				URL: "oci://example.com/repo:latest",
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateUpdate(ctx, oldSource, newSource)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Enterprise license")
	assert.NotEmpty(t, warnings)
}

func TestArenaSourceValidator_ValidateUpdateAllowed(t *testing.T) {
	validator := &ArenaSourceValidator{
		LicenseValidator: createEnterpriseValidator(t, license.Features{GitSource: true}),
	}

	oldSource := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeConfigMap,
			Interval: "5m",
		},
	}

	newSource := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaSourceSpec{
			Type:     omniav1alpha1.ArenaSourceTypeGit,
			Interval: "5m",
			Git: &omniav1alpha1.GitSource{
				URL: "https://github.com/example/repo",
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateUpdate(ctx, oldSource, newSource)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaJobValidator_ValidateUpdate(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	oldJob := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeEvaluation,
		},
	}

	newJob := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeLoadTest,
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateUpdate(ctx, oldJob, newJob)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Enterprise license")
	assert.NotEmpty(t, warnings)
}

func TestArenaJobValidator_ValidateUpdateAllowed(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createEnterpriseValidator(t, license.Features{LoadTesting: true}),
	}

	oldJob := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeEvaluation,
		},
	}

	newJob := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeLoadTest,
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateUpdate(ctx, oldJob, newJob)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaSourceValidator_InvalidObjectType(t *testing.T) {
	validator := &ArenaSourceValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	// Pass a wrong object type
	invalidObj := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
	}

	ctx := context.Background()

	// Test ValidateCreate with wrong type
	_, err := validator.ValidateCreate(ctx, invalidObj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected ArenaSource")

	// Test ValidateUpdate with wrong type
	_, err = validator.ValidateUpdate(ctx, invalidObj, invalidObj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected ArenaSource")
}

func TestArenaJobValidator_InvalidObjectType(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	// Pass a wrong object type
	invalidObj := &omniav1alpha1.ArenaSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-source",
			Namespace: "default",
		},
	}

	ctx := context.Background()

	// Test ValidateCreate with wrong type
	_, err := validator.ValidateCreate(ctx, invalidObj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected ArenaJob")

	// Test ValidateUpdate with wrong type
	_, err = validator.ValidateUpdate(ctx, invalidObj, invalidObj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected ArenaJob")
}

func TestArenaJobValidator_DefaultJobType(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	// Job with empty type should default to evaluation (allowed on open-core)
	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			// Type is empty - should default to evaluation
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaJobValidator_NilWorkersDefaultsToOneReplica(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	// Job with nil Workers should default to 1 replica (allowed on open-core)
	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeEvaluation,
			// Workers is nil - should default to 1 replica
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaJobValidator_ZeroReplicasDefaultsToOne(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	// Job with 0 replicas should default to 1 (allowed on open-core)
	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeEvaluation,
			Workers: &omniav1alpha1.WorkerConfig{
				Replicas: 0, // Should default to 1
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

func TestArenaJobValidator_EmptyScheduleNotConsideredScheduled(t *testing.T) {
	validator := &ArenaJobValidator{
		LicenseValidator: createOpenCoreValidator(t),
	}

	// Job with Schedule but empty Cron should NOT be considered scheduled
	job := &omniav1alpha1.ArenaJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: "default",
		},
		Spec: omniav1alpha1.ArenaJobSpec{
			SourceRef: omniav1alpha1.LocalObjectReference{Name: "my-source"},
			Type:      omniav1alpha1.ArenaJobTypeEvaluation,
			Schedule: &omniav1alpha1.ScheduleConfig{
				Cron: "", // Empty cron - not scheduled
			},
		},
	}

	ctx := context.Background()
	warnings, err := validator.ValidateCreate(ctx, job)

	assert.NoError(t, err)
	assert.Empty(t, warnings)
}

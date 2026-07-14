package controller

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/media"
)

func mediaEnvMap(vars []corev1.EnvVar) map[string]corev1.EnvVar {
	m := make(map[string]corev1.EnvVar, len(vars))
	for _, v := range vars {
		m[v.Name] = v
	}
	return m
}

func TestMediaStorageEnvVars_NilOrNone(t *testing.T) {
	if got := mediaStorageEnvVars(nil); len(got) != 0 {
		t.Errorf("nil config: got %d env vars, want 0", len(got))
	}
	none := &omniav1alpha1.MediaStorageConfig{Type: "none"}
	if got := mediaStorageEnvVars(none); len(got) != 0 {
		t.Errorf("none: got %d env vars, want 0", len(got))
	}
}

func TestMediaStorageEnvVars_S3(t *testing.T) {
	cfg := &omniav1alpha1.MediaStorageConfig{
		Type: "s3",
		S3:   &omniav1alpha1.S3MediaBackend{Bucket: "b", Region: "us-east-1", Prefix: "media", Endpoint: "http://minio:9000"},
	}
	m := mediaEnvMap(mediaStorageEnvVars(cfg))
	if m[media.EnvStorageType].Value != "s3" {
		t.Errorf("%s = %q, want s3", media.EnvStorageType, m[media.EnvStorageType].Value)
	}
	if m[media.EnvS3Bucket].Value != "b" {
		t.Errorf("%s = %q, want b", media.EnvS3Bucket, m[media.EnvS3Bucket].Value)
	}
	if m[media.EnvS3Endpoint].Value != "http://minio:9000" {
		t.Errorf("%s = %q", media.EnvS3Endpoint, m[media.EnvS3Endpoint].Value)
	}
}

func TestMediaStorageEnvVars_SecretRef_AzureKey(t *testing.T) {
	cfg := &omniav1alpha1.MediaStorageConfig{
		Type:      "azure",
		Azure:     &omniav1alpha1.AzureMediaBackend{Account: "acct", Container: "c", Prefix: "prefix"},
		SecretRef: &corev1.LocalObjectReference{Name: "media-creds"},
	}
	m := mediaEnvMap(mediaStorageEnvVars(cfg))
	if m[media.EnvAzurePrefix].Value != "prefix" {
		t.Errorf("%s = %q, want prefix", media.EnvAzurePrefix, m[media.EnvAzurePrefix].Value)
	}
	key := m[media.EnvAzureKey]
	if key.ValueFrom == nil || key.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("%s should be a SecretKeyRef", media.EnvAzureKey)
	}
	if key.ValueFrom.SecretKeyRef.Name != "media-creds" {
		t.Errorf("secret name = %q, want media-creds", key.ValueFrom.SecretKeyRef.Name)
	}
	if key.ValueFrom.SecretKeyRef.Key != mediaSecretKeyAzureAccountKey {
		t.Errorf("secret key = %q, want %q", key.ValueFrom.SecretKeyRef.Key, mediaSecretKeyAzureAccountKey)
	}
}

func TestMediaStorageEnvVars_Local(t *testing.T) {
	cfg := &omniav1alpha1.MediaStorageConfig{
		Type:  string(media.BackendTypeLocal),
		Local: &omniav1alpha1.LocalMediaBackend{BasePath: "/data/media"},
	}
	m := mediaEnvMap(mediaStorageEnvVars(cfg))
	if m[media.EnvStorageType].Value != string(media.BackendTypeLocal) {
		t.Errorf("%s = %q, want local", media.EnvStorageType, m[media.EnvStorageType].Value)
	}
	if m[media.EnvStoragePath].Value != "/data/media" {
		t.Errorf("%s = %q, want /data/media", media.EnvStoragePath, m[media.EnvStoragePath].Value)
	}
}

// TestMediaStorageEnvVars_GCS_IgnoresSecretRef is the important negative case:
// GCS is workload-identity only, so a SecretRef on the config must not cause
// any secret-backed env var (nor any AWS_*/AZURE_* key) to be emitted.
func TestMediaStorageEnvVars_GCS_IgnoresSecretRef(t *testing.T) {
	cfg := &omniav1alpha1.MediaStorageConfig{
		Type:      "gcs",
		GCS:       &omniav1alpha1.GCSMediaBackend{Bucket: "b", Prefix: "media"},
		SecretRef: &corev1.LocalObjectReference{Name: "media-creds"},
	}
	vars := mediaStorageEnvVars(cfg)
	m := mediaEnvMap(vars)
	if m[media.EnvGCSBucket].Value != "b" {
		t.Errorf("%s = %q, want b", media.EnvGCSBucket, m[media.EnvGCSBucket].Value)
	}
	if m[media.EnvGCSPrefix].Value != "media" {
		t.Errorf("%s = %q, want media", media.EnvGCSPrefix, m[media.EnvGCSPrefix].Value)
	}
	for _, v := range vars {
		if v.ValueFrom != nil && v.ValueFrom.SecretKeyRef != nil {
			t.Errorf("unexpected secret-backed env var %s for GCS backend (workload-identity only)", v.Name)
		}
		switch v.Name {
		case mediaSecretKeyS3AccessKeyID, mediaSecretKeyS3SecretAccessKey, media.EnvAzureKey:
			t.Errorf("unexpected credential env var %s for GCS backend", v.Name)
		}
	}
}

func TestMediaStorageEnvVars_S3_SecretRef(t *testing.T) {
	cfg := &omniav1alpha1.MediaStorageConfig{
		Type:      "s3",
		S3:        &omniav1alpha1.S3MediaBackend{Bucket: "b"},
		SecretRef: &corev1.LocalObjectReference{Name: "media-creds"},
	}
	vars := mediaStorageEnvVars(cfg)

	accessKey := findEnvVar(vars, mediaSecretKeyS3AccessKeyID)
	if accessKey == nil || accessKey.ValueFrom == nil || accessKey.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("%s should be a SecretKeyRef", mediaSecretKeyS3AccessKeyID)
	}
	if accessKey.ValueFrom.SecretKeyRef.Name != "media-creds" {
		t.Errorf("secret name = %q, want media-creds", accessKey.ValueFrom.SecretKeyRef.Name)
	}
	if accessKey.ValueFrom.SecretKeyRef.Key != mediaSecretKeyS3AccessKeyID {
		t.Errorf("secret key = %q, want %q", accessKey.ValueFrom.SecretKeyRef.Key, mediaSecretKeyS3AccessKeyID)
	}

	secretKey := findEnvVar(vars, mediaSecretKeyS3SecretAccessKey)
	if secretKey == nil || secretKey.ValueFrom == nil || secretKey.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("%s should be a SecretKeyRef", mediaSecretKeyS3SecretAccessKey)
	}
	if secretKey.ValueFrom.SecretKeyRef.Name != "media-creds" {
		t.Errorf("secret name = %q, want media-creds", secretKey.ValueFrom.SecretKeyRef.Name)
	}
	if secretKey.ValueFrom.SecretKeyRef.Key != mediaSecretKeyS3SecretAccessKey {
		t.Errorf("secret key = %q, want %q", secretKey.ValueFrom.SecretKeyRef.Key, mediaSecretKeyS3SecretAccessKey)
	}
}

func TestMediaStorageEnvVars_Limits(t *testing.T) {
	maxSize := int64(5242880)
	cfg := &omniav1alpha1.MediaStorageConfig{
		Type:             "s3",
		S3:               &omniav1alpha1.S3MediaBackend{Bucket: "b"},
		MaxFileSizeBytes: &maxSize,
		DefaultTTL:       &metav1.Duration{Duration: time.Hour},
		UploadURLTTL:     &metav1.Duration{Duration: 30 * time.Minute},
		DownloadURLTTL:   &metav1.Duration{Duration: 2 * time.Hour},
	}
	m := mediaEnvMap(mediaStorageEnvVars(cfg))
	if m[media.EnvMaxFileSize].Value != "5242880" {
		t.Errorf("%s = %q, want 5242880", media.EnvMaxFileSize, m[media.EnvMaxFileSize].Value)
	}
	if m[media.EnvDefaultTTL].Value != "1h0m0s" {
		t.Errorf("%s = %q, want 1h0m0s", media.EnvDefaultTTL, m[media.EnvDefaultTTL].Value)
	}
	if m[media.EnvUploadURLTTL].Value != "30m0s" {
		t.Errorf("%s = %q, want 30m0s", media.EnvUploadURLTTL, m[media.EnvUploadURLTTL].Value)
	}
	if m[media.EnvDownloadURLTTL].Value != "2h0m0s" {
		t.Errorf("%s = %q, want 2h0m0s", media.EnvDownloadURLTTL, m[media.EnvDownloadURLTTL].Value)
	}
}

// TestMediaStorageEnvVars_LimitsOmittedWhenUnset asserts the upload/download
// TTL env vars are absent (not rendered as empty strings) when the CRD
// doesn't set them — the facade/runtime binaries fall back to their own
// hardcoded defaults (media.DefaultUploadURLTTL / DefaultDownloadURLTTL) in
// that case, which only works if the env var is fully absent.
func TestMediaStorageEnvVars_LimitsOmittedWhenUnset(t *testing.T) {
	cfg := &omniav1alpha1.MediaStorageConfig{
		Type: "s3",
		S3:   &omniav1alpha1.S3MediaBackend{Bucket: "b"},
	}
	m := mediaEnvMap(mediaStorageEnvVars(cfg))
	if _, ok := m[media.EnvUploadURLTTL]; ok {
		t.Errorf("%s should be omitted when uploadURLTTL is unset", media.EnvUploadURLTTL)
	}
	if _, ok := m[media.EnvDownloadURLTTL]; ok {
		t.Errorf("%s should be omitted when downloadURLTTL is unset", media.EnvDownloadURLTTL)
	}
}

// TestMediaEnvVars_WiredIntoBuilders is the gap-closer: it exercises
// mediaStorageEnvVars via the real production call sites
// (buildFacadeEnvVars/buildRuntimeEnvVars) rather than calling it directly,
// proving spec.media.storage actually reaches both container env slices.
func TestMediaEnvVars_WiredIntoBuilders(t *testing.T) {
	r := &AgentRuntimeReconciler{}
	ar := &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Media: &omniav1alpha1.MediaConfig{
				Storage: &omniav1alpha1.MediaStorageConfig{
					Type: "s3",
					S3:   &omniav1alpha1.S3MediaBackend{Bucket: "b", Region: "us-east-1"},
				},
			},
		},
	}

	facadeEnv := envMap(r.buildFacadeEnvVars(ar))
	if facadeEnv["OMNIA_MEDIA_STORAGE_TYPE"] != "s3" {
		t.Errorf("facade OMNIA_MEDIA_STORAGE_TYPE = %q, want s3", facadeEnv["OMNIA_MEDIA_STORAGE_TYPE"])
	}
	if facadeEnv["OMNIA_MEDIA_S3_BUCKET"] != "b" {
		t.Errorf("facade OMNIA_MEDIA_S3_BUCKET = %q, want b", facadeEnv["OMNIA_MEDIA_S3_BUCKET"])
	}

	runtimeEnv := envMap(r.buildRuntimeEnvVars(ar, nil, nil))
	if runtimeEnv["OMNIA_MEDIA_STORAGE_TYPE"] != "s3" {
		t.Errorf("runtime OMNIA_MEDIA_STORAGE_TYPE = %q, want s3", runtimeEnv["OMNIA_MEDIA_STORAGE_TYPE"])
	}
	if runtimeEnv["OMNIA_MEDIA_S3_BUCKET"] != "b" {
		t.Errorf("runtime OMNIA_MEDIA_S3_BUCKET = %q, want b", runtimeEnv["OMNIA_MEDIA_S3_BUCKET"])
	}
	if _, ok := runtimeEnv["OMNIA_FACADE_PORT"]; !ok {
		t.Errorf("runtime env vars missing OMNIA_FACADE_PORT")
	}
}

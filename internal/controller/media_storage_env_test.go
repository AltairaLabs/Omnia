package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

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
		Azure:     &omniav1alpha1.AzureMediaBackend{Account: "acct", Container: "c"},
		SecretRef: &corev1.LocalObjectReference{Name: "media-creds"},
	}
	m := mediaEnvMap(mediaStorageEnvVars(cfg))
	key := m[media.EnvAzureKey]
	if key.ValueFrom == nil || key.ValueFrom.SecretKeyRef == nil {
		t.Fatalf("%s should be a SecretKeyRef", media.EnvAzureKey)
	}
	if key.ValueFrom.SecretKeyRef.Name != "media-creds" {
		t.Errorf("secret name = %q, want media-creds", key.ValueFrom.SecretKeyRef.Name)
	}
}

package agent

import (
	"testing"

	"github.com/altairalabs/omnia/api/v1alpha1"
)

// The facade's local storage path must come from the storage config env (which
// the operator renders from spec.media.storage.local.basePath) so it aligns
// with the runtime — spec.media.basePath must not override it.
func TestLoadMediaConfigFromCRD_StorageEnvWinsOverBasePath(t *testing.T) {
	t.Setenv(EnvMediaStorageType, "local")
	t.Setenv(EnvMediaStoragePath, "/data/media")

	cfg := &Config{}
	ar := &v1alpha1.AgentRuntime{Spec: v1alpha1.AgentRuntimeSpec{
		Media: &v1alpha1.MediaConfig{BasePath: "/etc/omnia/media"}, // legacy field set
	}}
	loadMediaConfigFromCRD(cfg, ar)

	if cfg.MediaStorageType != MediaStorageTypeLocal {
		t.Errorf("MediaStorageType = %q, want local", cfg.MediaStorageType)
	}
	if cfg.MediaStoragePath != "/data/media" {
		t.Errorf("MediaStoragePath = %q, want /data/media (storage env, not basePath)", cfg.MediaStoragePath)
	}
}

// With no storage env, spec.media.basePath is the legacy fallback.
func TestLoadMediaConfigFromCRD_LegacyBasePathFallback(t *testing.T) {
	t.Setenv(EnvMediaStorageType, "")
	cfg := &Config{}
	ar := &v1alpha1.AgentRuntime{Spec: v1alpha1.AgentRuntimeSpec{
		Media: &v1alpha1.MediaConfig{BasePath: "/legacy/media"},
	}}
	loadMediaConfigFromCRD(cfg, ar)
	if cfg.MediaStorageType != MediaStorageTypeLocal || cfg.MediaStoragePath != "/legacy/media" {
		t.Errorf("legacy fallback: got type=%q path=%q, want local /legacy/media", cfg.MediaStorageType, cfg.MediaStoragePath)
	}
}

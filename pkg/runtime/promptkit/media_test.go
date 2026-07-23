/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package promptkit

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/media"
)

func TestInitMediaStorage_Disabled(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")

	store, cleanup := initMediaStorage(logr.Discard())
	if store != nil {
		t.Errorf("store = %v, want nil when media storage is disabled", store)
	}
	if cleanup != nil {
		t.Error("cleanup func should be nil when media storage is disabled")
	}
}

func TestInitMediaStorage_ExplicitNone(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "none")

	store, cleanup := initMediaStorage(logr.Discard())
	if store != nil {
		t.Errorf("store = %v, want nil for storage type \"none\"", store)
	}
	if cleanup != nil {
		t.Error("cleanup func should be nil for storage type \"none\"")
	}
}

func TestInitMediaStorage_Local(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "local")
	t.Setenv("OMNIA_MEDIA_STORAGE_PATH", t.TempDir())

	store, cleanup := initMediaStorage(logr.Discard())
	if store == nil {
		t.Fatal("store is nil, want a constructed local backend")
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil, want a Close-wrapping func for a constructed backend")
	}
	cleanup() // exercises the Close() success path
}

// TestInitMediaStorage_UploadURLTTLFromEnv is the wiring test for the
// previously-dead spec.media.storage.uploadURLTTL/downloadURLTTL CRD fields
// (#1817 Task 5 follow-up): the controller renders OMNIA_MEDIA_UPLOAD_URL_TTL
// onto the runtime container, and initMediaStorage must actually honor it
// instead of the hardcoded 15m default.
func TestInitMediaStorage_UploadURLTTLFromEnv(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "local")
	t.Setenv("OMNIA_MEDIA_STORAGE_PATH", t.TempDir())
	t.Setenv("OMNIA_MEDIA_UPLOAD_URL_TTL", "30m")

	store, cleanup := initMediaStorage(logr.Discard())
	if store == nil {
		t.Fatal("store is nil, want a constructed local backend")
	}
	defer cleanup()

	creds, err := store.GetUploadURL(context.Background(), media.UploadRequest{
		SessionID: "sess",
		Filename:  "f.txt",
		MIMEType:  "text/plain",
		SizeBytes: 10,
	})
	if err != nil {
		t.Fatalf("GetUploadURL() error = %v", err)
	}

	wantExpiry := time.Now().Add(30 * time.Minute)
	if diff := creds.ExpiresAt.Sub(wantExpiry); diff < -5*time.Second || diff > 5*time.Second {
		t.Errorf("ExpiresAt = %v, want ~%v (30m TTL from env, not the 15m hardcoded default)", creds.ExpiresAt, wantExpiry)
	}
}

func TestInitMediaStorage_UnknownType(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "ftp")

	store, cleanup := initMediaStorage(logr.Discard())
	if store != nil {
		t.Errorf("store = %v, want nil for an unrecognized storage type", store)
	}
	if cleanup != nil {
		t.Error("cleanup func should be nil for an unrecognized storage type")
	}
}

func TestMediaStorageServerOpts_Disabled_NilCleanup(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_TYPE", "")

	opts, cleanup := mediaStorageServerOpts(logr.Discard())
	if len(opts) != 0 {
		t.Errorf("opts = %v, want empty when media storage is disabled", opts)
	}
	if cleanup != nil {
		t.Error("cleanup func should be nil when media storage is disabled")
	}
}

func TestLocalMediaBaseURL_ExplicitOverrideWins(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_BASE_URL", "https://media.example.internal")
	t.Setenv("OMNIA_FACADE_PORT", "9090") // must be ignored once the override is set

	got := localMediaBaseURL(logr.Discard())
	if got != "https://media.example.internal" {
		t.Errorf("localMediaBaseURL() = %q, want the explicit override", got)
	}
}

func TestLocalMediaBaseURL_FacadePortFallback(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_BASE_URL", "")
	t.Setenv("OMNIA_FACADE_PORT", "9090")

	got := localMediaBaseURL(logr.Discard())
	if got != "http://localhost:9090" {
		t.Errorf("localMediaBaseURL() = %q, want http://localhost:9090", got)
	}
}

func TestLocalMediaBaseURL_DefaultGuess(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_BASE_URL", "")
	t.Setenv("OMNIA_FACADE_PORT", "")

	got := localMediaBaseURL(logr.Discard())
	if got != "http://localhost:8080" {
		t.Errorf("localMediaBaseURL() = %q, want the default facade port guess", got)
	}
}

func TestLocalMediaBaseURL_UnparseableFacadePortFallsBackToDefault(t *testing.T) {
	t.Setenv("OMNIA_MEDIA_STORAGE_BASE_URL", "")
	t.Setenv("OMNIA_FACADE_PORT", "not-a-port")

	got := localMediaBaseURL(logr.Discard())
	if got != "http://localhost:8080" {
		t.Errorf("localMediaBaseURL() = %q, want the default facade port guess for an unparseable override", got)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	t.Setenv("OMNIA_TEST_ENV_OR_DEFAULT", "")
	if got := getEnvOrDefault("OMNIA_TEST_ENV_OR_DEFAULT", "fallback"); got != "fallback" {
		t.Errorf("getEnvOrDefault() = %q, want fallback for an unset var", got)
	}

	t.Setenv("OMNIA_TEST_ENV_OR_DEFAULT", "set-value")
	if got := getEnvOrDefault("OMNIA_TEST_ENV_OR_DEFAULT", "fallback"); got != "set-value" {
		t.Errorf("getEnvOrDefault() = %q, want set-value", got)
	}
}

func TestGetEnvInt64(t *testing.T) {
	t.Setenv("OMNIA_TEST_ENV_INT64", "")
	if got := getEnvInt64("OMNIA_TEST_ENV_INT64", 42); got != 42 {
		t.Errorf("getEnvInt64() = %d, want 42 for an unset var", got)
	}

	t.Setenv("OMNIA_TEST_ENV_INT64", "not-a-number")
	if got := getEnvInt64("OMNIA_TEST_ENV_INT64", 42); got != 42 {
		t.Errorf("getEnvInt64() = %d, want 42 for an unparseable var", got)
	}

	t.Setenv("OMNIA_TEST_ENV_INT64", "12345")
	if got := getEnvInt64("OMNIA_TEST_ENV_INT64", 42); got != 12345 {
		t.Errorf("getEnvInt64() = %d, want 12345", got)
	}
}

func TestGetEnvDuration(t *testing.T) {
	t.Setenv("OMNIA_TEST_ENV_DURATION", "")
	if got := getEnvDuration("OMNIA_TEST_ENV_DURATION", time.Hour); got != time.Hour {
		t.Errorf("getEnvDuration() = %v, want 1h for an unset var", got)
	}

	t.Setenv("OMNIA_TEST_ENV_DURATION", "not-a-duration")
	if got := getEnvDuration("OMNIA_TEST_ENV_DURATION", time.Hour); got != time.Hour {
		t.Errorf("getEnvDuration() = %v, want 1h for an unparseable var", got)
	}

	t.Setenv("OMNIA_TEST_ENV_DURATION", "30m")
	if got := getEnvDuration("OMNIA_TEST_ENV_DURATION", time.Hour); got != 30*time.Minute {
		t.Errorf("getEnvDuration() = %v, want 30m", got)
	}
}

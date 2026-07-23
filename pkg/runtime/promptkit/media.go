/*
Copyright 2026 Altaira Labs.

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

package promptkit

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/media"
	pkruntime "github.com/altairalabs/omnia/internal/runtime"
)

// envMediaStorageBaseURL lets an operator pin the exact base URL the runtime
// uses to build download URLs for the "local" media backend, bypassing the
// best-effort guess in localMediaBaseURL. Not part of the shared internal/media
// env contract (media.Env*) because it exists only to work around the runtime's
// lack of a reliable facade-port signal.
const envMediaStorageBaseURL = "OMNIA_MEDIA_STORAGE_BASE_URL"

// envFacadePortFallback mirrors the facade's own OMNIA_FACADE_PORT env var
// (internal/agent/config.go EnvFacadePort). The runtime binary intentionally
// does not import internal/agent (a facade-only config package) just to reuse
// this string literal.
const envFacadePortFallback = "OMNIA_FACADE_PORT"

// defaultFacadePortGuess mirrors agent.DefaultFacadePort (internal/agent/config.go)
// — the facade's well-known default port when nothing else says otherwise.
const defaultFacadePortGuess = 8080

// mediaStorageServerOpts builds the pkruntime.ServerOption slice (0 or 1
// elements) that wires the runtime's media.Storage backend, mirroring
// cmd/agent/main.go's initMediaStorage. Returns the options plus a cleanup func
// (nil when storage is disabled) for the caller to defer (#1817).
func mediaStorageServerOpts(log logr.Logger) ([]pkruntime.ServerOption, func()) {
	store, cleanup := initMediaStorage(log)
	if store == nil {
		return nil, cleanup
	}
	return []pkruntime.ServerOption{pkruntime.WithMediaStorage(store)}, cleanup
}

// initMediaStorage builds the media.Storage backend selected by the OMNIA_MEDIA_*
// environment contract (internal/media/env.go) — the same env vars the facade
// (cmd/agent) reads via internal/agent.Config. The runtime's pkruntime.Config is
// CRD-derived and intentionally doesn't carry these fields: media storage backend
// selection has no CRD representation today and is pod-env-only for both binaries.
func initMediaStorage(log logr.Logger) (media.Storage, func()) {
	storageType := media.BackendType(getEnvOrDefault(media.EnvStorageType, string(media.BackendTypeNone)))
	if storageType == media.BackendTypeNone || storageType == "" {
		log.Info("media storage disabled")
		return nil, nil
	}

	bcfg := media.BuilderConfig{
		Type:           storageType,
		DefaultTTL:     getEnvDuration(media.EnvDefaultTTL, media.DefaultDefaultTTL),
		MaxFileSize:    getEnvInt64(media.EnvMaxFileSize, media.DefaultMaxFileSize),
		UploadURLTTL:   getEnvDuration(media.EnvUploadURLTTL, media.DefaultUploadURLTTL),
		DownloadURLTTL: getEnvDuration(media.EnvDownloadURLTTL, media.DefaultDownloadURLTTL),
		LocalPath:      getEnvOrDefault(media.EnvStoragePath, media.DefaultStoragePath),
		S3Bucket:       os.Getenv(media.EnvS3Bucket),
		S3Region:       os.Getenv(media.EnvS3Region),
		S3Prefix:       os.Getenv(media.EnvS3Prefix),
		S3Endpoint:     os.Getenv(media.EnvS3Endpoint),
		GCSBucket:      os.Getenv(media.EnvGCSBucket),
		GCSPrefix:      os.Getenv(media.EnvGCSPrefix),
		AzureAccount:   os.Getenv(media.EnvAzureAccount),
		AzureContainer: os.Getenv(media.EnvAzureContainer),
		AzurePrefix:    os.Getenv(media.EnvAzurePrefix),
		AzureKey:       os.Getenv(media.EnvAzureKey),
	}
	if storageType == media.BackendTypeLocal {
		bcfg.LocalBaseURL = localMediaBaseURL(log)
	}

	store, err := media.Build(context.Background(), bcfg)
	if err != nil {
		log.Error(err, "failed to initialize media storage", "type", storageType)
		return nil, nil
	}

	log.Info("media storage initialized", "type", storageType)
	return store, func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Error(closeErr, "error closing media storage")
		}
	}
}

// localMediaBaseURL resolves the base URL the runtime uses to build download
// URLs for the "local" backend's presigned-ish links. Cloud backends issue
// self-authenticating presigned URLs, so only the local backend needs this. The
// local backend's download URL is facade-relative (localhost:<facadePort>),
// reachable because both containers share the pod network namespace.
//
// Resolution order: an explicit override, then a same-named env var in case an
// operator mirrored it onto this container, then the facade's documented default.
// The fallback paths log clearly so a misconfigured local backend is visible
// instead of silently serving broken download URLs.
func localMediaBaseURL(log logr.Logger) string {
	if v := os.Getenv(envMediaStorageBaseURL); v != "" {
		return v
	}

	port := defaultFacadePortGuess
	if p := os.Getenv(envFacadePortFallback); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	log.Info("media storage: guessing facade base URL for local backend",
		"baseURL", baseURL,
		"reason", "OMNIA_FACADE_PORT is not guaranteed to be set on the runtime container; "+
			"set OMNIA_MEDIA_STORAGE_BASE_URL explicitly if this guess is wrong")
	return baseURL
}

// getEnvInt64 parses the env var as an int64, returning def when unset or
// unparseable.
func getEnvInt64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return parsed
}

// getEnvDuration parses the env var as a time.Duration, returning def when unset
// or unparseable.
func getEnvDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return parsed
}

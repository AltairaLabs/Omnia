/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/agent"
	"github.com/altairalabs/omnia/internal/media"
)

func TestInitMediaStorage_None(t *testing.T) {
	cfg := &agent.Config{MediaStorageType: agent.MediaStorageTypeNone}

	store, cleanup := initMediaStorage(cfg, logr.Discard())
	if store != nil {
		t.Errorf("store = %v, want nil for MediaStorageTypeNone", store)
	}
	if cleanup != nil {
		t.Error("cleanup func should be nil for MediaStorageTypeNone")
	}
}

func TestInitMediaStorage_Local(t *testing.T) {
	cfg := &agent.Config{
		MediaStorageType: agent.MediaStorageTypeLocal,
		MediaStoragePath: t.TempDir(),
		MediaDefaultTTL:  24 * time.Hour,
		MediaMaxFileSize: 1024,
		FacadePort:       8080,
	}

	store, cleanup := initMediaStorage(cfg, logr.Discard())
	if store == nil {
		t.Fatal("store is nil, want a constructed local backend")
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil, want a Close-wrapping func for a constructed backend")
	}
	cleanup() // exercises the Close() success path
}

// TestInitMediaStorage_UploadURLTTLFromConfig is the wiring test for the
// previously-dead spec.media.storage.uploadURLTTL CRD field (#1817 Task 5
// follow-up): agent.Config.MediaUploadURLTTL (loaded from
// OMNIA_MEDIA_UPLOAD_URL_TTL by internal/agent) must actually reach the
// constructed media.Storage instead of the hardcoded 15m default.
func TestInitMediaStorage_UploadURLTTLFromConfig(t *testing.T) {
	cfg := &agent.Config{
		MediaStorageType:  agent.MediaStorageTypeLocal,
		MediaStoragePath:  t.TempDir(),
		MediaUploadURLTTL: 30 * time.Minute,
		FacadePort:        8080,
	}

	store, cleanup := initMediaStorage(cfg, logr.Discard())
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
		t.Errorf("ExpiresAt = %v, want ~%v (30m TTL from config, not the 15m hardcoded default)", creds.ExpiresAt, wantExpiry)
	}
}

func TestInitMediaStorage_S3(t *testing.T) {
	cfg := &agent.Config{
		MediaStorageType: agent.MediaStorageTypeS3,
		MediaS3Bucket:    "probe-bucket",
		MediaS3Region:    "us-east-1",
	}

	store, cleanup := initMediaStorage(cfg, logr.Discard())
	if store == nil {
		// The S3 client requires AWS credentials/region resolution that may be
		// unavailable in CI; skip rather than fail when construction can't happen.
		t.Skip("S3 backend not constructible in this environment (no credentials)")
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil, want a Close-wrapping func for a constructed backend")
	}
	cleanup()
}

func TestInitMediaStorage_GCS(t *testing.T) {
	cfg := &agent.Config{
		MediaStorageType: agent.MediaStorageTypeGCS,
		MediaGCSBucket:   "probe-bucket",
	}

	store, cleanup := initMediaStorage(cfg, logr.Discard())
	if store == nil {
		// The GCS client requires application-default credentials that are
		// unavailable in CI; skip rather than fail when construction can't happen.
		t.Skip("GCS backend not constructible in this environment (no credentials)")
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil, want a Close-wrapping func for a constructed backend")
	}
	cleanup()
}

func TestInitMediaStorage_Azure(t *testing.T) {
	cfg := &agent.Config{
		MediaStorageType:    agent.MediaStorageTypeAzure,
		MediaAzureAccount:   "probeacct",
		MediaAzureContainer: "probecontainer",
	}

	store, cleanup := initMediaStorage(cfg, logr.Discard())
	if store == nil {
		// The Azure client requires account credentials that may be unavailable
		// in CI; skip rather than fail when construction can't happen.
		t.Skip("Azure backend not constructible in this environment (no credentials)")
	}
	if cleanup == nil {
		t.Fatal("cleanup is nil, want a Close-wrapping func for a constructed backend")
	}
	cleanup()
}

func TestInitMediaStorage_UnknownType(t *testing.T) {
	cfg := &agent.Config{MediaStorageType: "ftp"}

	store, cleanup := initMediaStorage(cfg, logr.Discard())
	if store != nil {
		t.Errorf("store = %v, want nil for an unrecognized storage type", store)
	}
	if cleanup != nil {
		t.Error("cleanup func should be nil for an unrecognized storage type")
	}
}

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package main

import (
	"testing"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/internal/agent"
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

func TestInitMediaStorage_S3(t *testing.T) {
	cfg := &agent.Config{
		MediaStorageType: agent.MediaStorageTypeS3,
		MediaS3Bucket:    "probe-bucket",
		MediaS3Region:    "us-east-1",
	}

	store, cleanup := initMediaStorage(cfg, logr.Discard())
	if store == nil {
		t.Fatal("store is nil, want a constructed S3 backend")
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
		t.Fatal("store is nil, want a constructed GCS backend")
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
		t.Fatal("store is nil, want a constructed Azure backend")
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

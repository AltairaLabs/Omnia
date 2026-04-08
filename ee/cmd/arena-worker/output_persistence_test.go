/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// --- newS3UploadFunc ---

func TestNewS3UploadFunc(t *testing.T) {
	t.Run("returns error when bucket is empty", func(t *testing.T) {
		_, err := newS3UploadFunc(context.Background(), &omniav1alpha1.S3OutputConfig{
			Region: "us-east-1",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bucket")
	})

	t.Run("returns error when region is empty", func(t *testing.T) {
		_, err := newS3UploadFunc(context.Background(), &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "region")
	})

	t.Run("returns non-nil upload func for valid config", func(t *testing.T) {
		// Does not make real AWS calls — just verifies the function is constructed.
		fn, err := newS3UploadFunc(context.Background(), &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
			Region: "us-east-1",
		})
		require.NoError(t, err)
		assert.NotNil(t, fn)
	})

	t.Run("uses explicit credentials when env vars are set", func(t *testing.T) {
		t.Setenv("ARENA_S3_ACCESS_KEY_ID", "test-key")
		t.Setenv("ARENA_S3_SECRET_ACCESS_KEY", "test-secret")
		fn, err := newS3UploadFunc(context.Background(), &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
			Region: "us-east-1",
		})
		require.NoError(t, err)
		assert.NotNil(t, fn)
	})

	t.Run("configures custom endpoint", func(t *testing.T) {
		fn, err := newS3UploadFunc(context.Background(), &omniav1alpha1.S3OutputConfig{
			Bucket:   "my-bucket",
			Region:   "us-east-1",
			Endpoint: "http://minio:9000",
		})
		require.NoError(t, err)
		assert.NotNil(t, fn)
	})
}

// --- resolveOutputDir ---

func TestResolveOutputDir(t *testing.T) {
	t.Run("returns fallback when OutputConfig is nil", func(t *testing.T) {
		cfg := &Config{}
		got := resolveOutputDir(cfg)
		assert.Equal(t, "/tmp/arena-output", got)
	})

	t.Run("returns PVC mount path when OutputType is PVC", func(t *testing.T) {
		cfg := &Config{
			OutputConfig: &omniav1alpha1.OutputConfig{
				Type: omniav1alpha1.OutputTypePVC,
				PVC: &omniav1alpha1.PVCOutputConfig{
					ClaimName: "results-pvc",
				},
			},
			OutputDir: "/mnt/pvc-output",
		}
		got := resolveOutputDir(cfg)
		assert.Equal(t, "/mnt/pvc-output", got)
	})

	t.Run("returns fallback when OutputType is PVC but OutputDir not injected", func(t *testing.T) {
		cfg := &Config{
			OutputConfig: &omniav1alpha1.OutputConfig{
				Type: omniav1alpha1.OutputTypePVC,
				PVC: &omniav1alpha1.PVCOutputConfig{
					ClaimName: "results-pvc",
				},
			},
			OutputDir: "", // not set
		}
		got := resolveOutputDir(cfg)
		assert.Equal(t, "/tmp/arena-output", got)
	})

	t.Run("returns tmp dir for S3 output (files staged locally before upload)", func(t *testing.T) {
		cfg := &Config{
			OutputConfig: &omniav1alpha1.OutputConfig{
				Type: omniav1alpha1.OutputTypeS3,
				S3: &omniav1alpha1.S3OutputConfig{
					Bucket: "my-bucket",
					Region: "us-east-1",
				},
			},
		}
		got := resolveOutputDir(cfg)
		assert.Equal(t, "/tmp/arena-output", got)
	})
}

// --- uploadOutputToS3 ---

func TestUploadOutputToS3(t *testing.T) {
	t.Run("uploads files from output dir to S3", func(t *testing.T) {
		// Create a temp dir with some test files
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "results.json"), []byte(`{"status":"pass"}`), 0600))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "media.png"), []byte("PNG"), 0600))

		var uploaded []string
		mockUpload := func(_ context.Context, key string, _ []byte) error {
			uploaded = append(uploaded, key)
			return nil
		}

		s3Cfg := &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
			Region: "us-east-1",
			Prefix: "arena/results",
		}
		err := uploadOutputToS3(context.Background(), testLog(), dir, "test-job", "default", s3Cfg, mockUpload)
		require.NoError(t, err)

		assert.Len(t, uploaded, 2)
		for _, key := range uploaded {
			assert.Contains(t, key, "arena/results/")
			assert.Contains(t, key, "test-job/")
		}
	})

	t.Run("succeeds when output dir is empty", func(t *testing.T) {
		dir := t.TempDir()

		var uploaded []string
		mockUpload := func(_ context.Context, key string, _ []byte) error {
			uploaded = append(uploaded, key)
			return nil
		}

		s3Cfg := &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
			Region: "us-east-1",
		}
		err := uploadOutputToS3(context.Background(), testLog(), dir, "test-job", "default", s3Cfg, mockUpload)
		require.NoError(t, err)
		assert.Empty(t, uploaded)
	})

	t.Run("returns error when output dir does not exist", func(t *testing.T) {
		mockUpload := func(_ context.Context, key string, _ []byte) error {
			return nil
		}

		s3Cfg := &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
			Region: "us-east-1",
		}
		err := uploadOutputToS3(
			context.Background(), testLog(), "/nonexistent/path",
			"test-job", "default", s3Cfg, mockUpload,
		)
		require.Error(t, err)
	})

	t.Run("propagates upload errors", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "results.json"), []byte(`{}`), 0600))

		mockUpload := func(_ context.Context, _ string, _ []byte) error {
			return assert.AnError
		}

		s3Cfg := &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
			Region: "us-east-1",
		}
		err := uploadOutputToS3(context.Background(), testLog(), dir, "test-job", "default", s3Cfg, mockUpload)
		require.Error(t, err)
	})

	t.Run("skips directories", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "emptysubdir"), 0750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "output.json"), []byte(`{}`), 0600))

		var uploaded []string
		mockUpload := func(_ context.Context, key string, _ []byte) error {
			uploaded = append(uploaded, key)
			return nil
		}

		s3Cfg := &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
			Region: "us-east-1",
		}
		err := uploadOutputToS3(context.Background(), testLog(), dir, "test-job", "default", s3Cfg, mockUpload)
		require.NoError(t, err)
		assert.Len(t, uploaded, 1) // only output.json, not the directory itself
	})

	t.Run("key includes prefix and relative path", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "report.json"), []byte(`{}`), 0600))

		var keys []string
		mockUpload := func(_ context.Context, key string, _ []byte) error {
			keys = append(keys, key)
			return nil
		}

		s3Cfg := &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
			Region: "us-east-1",
			Prefix: "jobs",
		}
		err := uploadOutputToS3(context.Background(), testLog(), dir, "myjob", "ns1", s3Cfg, mockUpload)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		assert.Equal(t, "jobs/myjob/report.json", keys[0])
	})

	t.Run("key without prefix", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "report.json"), []byte(`{}`), 0600))

		var keys []string
		mockUpload := func(_ context.Context, key string, _ []byte) error {
			keys = append(keys, key)
			return nil
		}

		s3Cfg := &omniav1alpha1.S3OutputConfig{
			Bucket: "my-bucket",
			Region: "us-east-1",
		}
		err := uploadOutputToS3(context.Background(), testLog(), dir, "myjob", "ns1", s3Cfg, mockUpload)
		require.NoError(t, err)
		require.Len(t, keys, 1)
		assert.Equal(t, "myjob/report.json", keys[0])
	})
}

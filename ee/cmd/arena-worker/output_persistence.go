/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-logr/logr"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// defaultOutputDir is the fallback output directory used when no OutputConfig is present.
const defaultOutputDir = "/tmp/arena-output"

// s3UploadFunc is a function that uploads a single file to S3.
// Extracted for testability.
type s3UploadFunc func(ctx context.Context, key string, data []byte) error

// resolveOutputDir returns the directory where the engine should write output files.
// For PVC output, the controller mounts the PVC and injects ARENA_OUTPUT_DIR;
// for S3 output, files are staged in /tmp/arena-output and uploaded after execution;
// when no OutputConfig is set, /tmp/arena-output is used (original behavior).
func resolveOutputDir(cfg *Config) string {
	if cfg.OutputConfig == nil {
		return defaultOutputDir
	}
	switch cfg.OutputConfig.Type {
	case omniav1alpha1.OutputTypePVC:
		if cfg.OutputDir != "" {
			return cfg.OutputDir
		}
		return defaultOutputDir
	case omniav1alpha1.OutputTypeS3:
		// Stage locally; uploadOutputToS3 runs after all work items complete.
		return defaultOutputDir
	default:
		return defaultOutputDir
	}
}

// uploadOutputToS3 walks outputDir and uploads every file to S3 under
// <prefix>/<jobName>/<relative-path>. The uploadFn parameter is injected
// for testability; production callers pass newS3UploadFunc.
func uploadOutputToS3(
	ctx context.Context,
	log logr.Logger,
	outputDir string,
	jobName string,
	_ string, // namespace reserved for future use (e.g. multi-tenant prefix)
	s3Cfg *omniav1alpha1.S3OutputConfig,
	uploadFn s3UploadFunc,
) error {
	// Verify the output directory exists.
	if _, err := os.Stat(outputDir); err != nil {
		return fmt.Errorf("output dir not accessible: %w", err)
	}

	return filepath.WalkDir(outputDir, func(filePath string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil // skip directories — only upload files
		}

		rel, err := filepath.Rel(outputDir, filePath)
		if err != nil {
			return fmt.Errorf("failed to compute relative path for %s: %w", filePath, err)
		}

		// Build the S3 key: [prefix/]jobName/relative-path
		var key string
		if s3Cfg.Prefix != "" {
			key = path.Join(s3Cfg.Prefix, jobName, rel)
		} else {
			key = path.Join(jobName, rel)
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", filePath, err)
		}

		log.V(1).Info("uploading output file", "key", key, "bytes", len(data))
		if err := uploadFn(ctx, key, data); err != nil {
			return fmt.Errorf("failed to upload %s to S3 key %s: %w", filePath, key, err)
		}

		return nil
	})
}

// newS3UploadFunc creates a real s3UploadFunc from the OutputConfig.
// It reads credentials from the Kubernetes Secret referenced by SecretRef
// (the controller injects the values as env vars ARENA_S3_ACCESS_KEY_ID and
// ARENA_S3_SECRET_ACCESS_KEY so the worker doesn't need k8s API access here).
func newS3UploadFunc(ctx context.Context, s3Cfg *omniav1alpha1.S3OutputConfig) (s3UploadFunc, error) {
	if s3Cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 output config: bucket is required")
	}
	if s3Cfg.Region == "" {
		return nil, fmt.Errorf("S3 output config: region is required")
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(s3Cfg.Region),
	}

	// Credentials are injected by the controller from the SecretRef.
	accessKeyID := os.Getenv("ARENA_S3_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("ARENA_S3_SECRET_ACCESS_KEY")
	if accessKeyID != "" && secretAccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if s3Cfg.Endpoint != "" {
		endpoint := s3Cfg.Endpoint
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true // required for most S3-compatible endpoints
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)
	bucket := s3Cfg.Bucket

	return func(ctx context.Context, key string, data []byte) error {
		_, err := client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
			Body:   bytes.NewReader(data),
		})
		return err
	}, nil
}

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Config contains configuration for S3 result storage.
type S3Config struct {
	// Bucket is the S3 bucket name.
	Bucket string

	// Region is the AWS region.
	Region string

	// Prefix is the key prefix for all result objects.
	// Example: "arena/results" -> objects stored as "arena/results/{jobID}.json"
	Prefix string

	// Endpoint is an optional custom endpoint (for S3-compatible services like MinIO/GCS).
	Endpoint string

	// UsePathStyle forces path-style addressing (required for MinIO).
	UsePathStyle bool

	// AccessKeyID is the AWS access key ID (optional, uses IAM role if not set).
	AccessKeyID string

	// SecretAccessKey is the AWS secret access key (optional, uses IAM role if not set).
	SecretAccessKey string
}

// DefaultS3Config returns a configuration with sensible defaults.
func DefaultS3Config(bucket, region string) S3Config {
	return S3Config{
		Bucket: bucket,
		Region: region,
		Prefix: "arena/results",
	}
}

// s3Client defines the S3 operations used by S3Storage.
// This interface allows for mocking in tests.
type s3Client interface {
	PutObject(
		ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options),
	) (*s3.PutObjectOutput, error)
	GetObject(
		ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options),
	) (*s3.GetObjectOutput, error)
	HeadObject(
		ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options),
	) (*s3.HeadObjectOutput, error)
	DeleteObject(
		ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options),
	) (*s3.DeleteObjectOutput, error)
}

// s3ListClient extends s3Client with listing capabilities.
type s3ListClient interface {
	s3Client
	ListObjectsV2(
		ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options),
	) (*s3.ListObjectsV2Output, error)
}

// S3Storage implements ResultStorage using Amazon S3 or compatible services.
type S3Storage struct {
	client s3ListClient
	config S3Config
	mu     sync.RWMutex
	closed bool
}

// NewS3Storage creates a new S3 result storage backend.
func NewS3Storage(ctx context.Context, cfg S3Config) (*S3Storage, error) {
	if cfg.Bucket == "" {
		return nil, errors.New("bucket is required")
	}
	if cfg.Region == "" {
		return nil, errors.New("region is required")
	}

	// Build AWS config options
	opts := []func(*config.LoadOptions) error{
		config.WithRegion(cfg.Region),
	}

	// Add explicit credentials if provided
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		opts = append(opts, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		))
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Build S3 client options
	s3Opts := []func(*s3.Options){}
	if cfg.Endpoint != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		})
	}
	if cfg.UsePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)

	return &S3Storage{
		client: client,
		config: cfg,
	}, nil
}

// Store persists job results to S3.
func (s *S3Storage) Store(ctx context.Context, jobID string, results *JobResults) error {
	if jobID == "" {
		return ErrInvalidJobID
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrStorageClosed
	}
	s.mu.RUnlock()

	// Serialize results to JSON
	data, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("failed to serialize results: %w", err)
	}

	key := s.keyForJob(jobID)

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.config.Bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("failed to upload results: %w", err)
	}

	return nil
}

// Get retrieves job results from S3.
func (s *S3Storage) Get(ctx context.Context, jobID string) (*JobResults, error) {
	if jobID == "" {
		return nil, ErrInvalidJobID
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrStorageClosed
	}
	s.mu.RUnlock()

	key := s.keyForJob(jobID)

	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return nil, ErrResultNotFound
		}
		// Also check for generic not found in error message (some S3-compatible services)
		if strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "not found") {
			return nil, ErrResultNotFound
		}
		return nil, fmt.Errorf("failed to get results: %w", err)
	}
	defer func() { _ = output.Body.Close() }()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read results: %w", err)
	}

	var results JobResults
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("failed to parse results: %w", err)
	}

	return &results, nil
}

// List returns job IDs that match the given prefix.
func (s *S3Storage) List(ctx context.Context, prefix string) ([]string, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}

	fullPrefix := s.buildFullPrefix(prefix)
	var jobIDs []string

	err := s.iterateObjects(ctx, fullPrefix, func(obj types.Object) {
		if obj.Key == nil {
			return
		}
		if jobID := s.jobIDFromKey(*obj.Key); jobID != "" {
			jobIDs = append(jobIDs, jobID)
		}
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(jobIDs)
	return jobIDs, nil
}

// Delete removes job results from S3.
func (s *S3Storage) Delete(ctx context.Context, jobID string) error {
	if jobID == "" {
		return ErrInvalidJobID
	}

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return ErrStorageClosed
	}
	s.mu.RUnlock()

	key := s.keyForJob(jobID)

	// Check if object exists first (S3 DeleteObject doesn't error on missing objects)
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		if errors.As(err, &notFound) {
			return ErrResultNotFound
		}
		// Also check for generic not found
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
			return ErrResultNotFound
		}
		return fmt.Errorf("failed to check results existence: %w", err)
	}

	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.config.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete results: %w", err)
	}

	return nil
}

// Close releases resources and marks the storage as closed.
func (s *S3Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	return nil
}

// ListWithInfo returns result metadata for jobs matching the prefix.
func (s *S3Storage) ListWithInfo(ctx context.Context, prefix string) ([]ResultInfo, error) {
	if err := s.checkClosed(); err != nil {
		return nil, err
	}

	fullPrefix := s.buildFullPrefix(prefix)
	var infos []ResultInfo

	err := s.iterateObjects(ctx, fullPrefix, func(obj types.Object) {
		if obj.Key == nil {
			return
		}
		jobID := s.jobIDFromKey(*obj.Key)
		if jobID == "" {
			return
		}

		info := ResultInfo{JobID: jobID}
		if obj.Size != nil {
			info.SizeBytes = *obj.Size
		}
		if obj.LastModified != nil {
			info.CompletedAt = *obj.LastModified
		}
		// To get full info (TotalItems, PassedItems, etc.), we'd need to
		// fetch and parse each object. For efficiency, we only include
		// basic info from the listing. Use Get() for full details.
		infos = append(infos, info)
	})
	if err != nil {
		return nil, err
	}

	// Sort by job ID for consistent ordering
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].JobID < infos[j].JobID
	})

	return infos, nil
}

// keyForJob returns the S3 key for a job's results.
func (s *S3Storage) keyForJob(jobID string) string {
	if s.config.Prefix == "" {
		return jobID + ".json"
	}
	return path.Join(s.config.Prefix, jobID+".json")
}

// jobIDFromKey extracts the job ID from an S3 key.
func (s *S3Storage) jobIDFromKey(key string) string {
	// Remove prefix
	if s.config.Prefix != "" {
		prefix := s.config.Prefix
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		key = strings.TrimPrefix(key, prefix)
	}

	// Remove .json suffix
	if !strings.HasSuffix(key, ".json") {
		return ""
	}
	return strings.TrimSuffix(key, ".json")
}

// checkClosed returns ErrStorageClosed if the storage is closed.
func (s *S3Storage) checkClosed() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrStorageClosed
	}
	return nil
}

// buildFullPrefix constructs the full S3 prefix for listing.
func (s *S3Storage) buildFullPrefix(prefix string) string {
	fullPrefix := s.config.Prefix
	if fullPrefix != "" && !strings.HasSuffix(fullPrefix, "/") {
		fullPrefix += "/"
	}
	if prefix != "" {
		fullPrefix += prefix
	}
	return fullPrefix
}

// iterateObjects iterates over S3 objects with the given prefix, calling fn for each object.
func (s *S3Storage) iterateObjects(ctx context.Context, prefix string, fn func(types.Object)) error {
	var continuationToken *string

	for {
		output, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.config.Bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return fmt.Errorf("failed to list results: %w", err)
		}

		for _, obj := range output.Contents {
			fn(obj)
		}

		if output.IsTruncated == nil || !*output.IsTruncated {
			break
		}
		continuationToken = output.NextContinuationToken
	}

	return nil
}

// Ensure S3Storage implements both interfaces.
var (
	_ ResultStorage         = (*S3Storage)(nil)
	_ ListableResultStorage = (*S3Storage)(nil)
)

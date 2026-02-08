/*
Copyright 2025.

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

package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// parseFlags tests
// ---------------------------------------------------------------------------

func TestParseFlags_Defaults(t *testing.T) {
	// Reset global flag state.
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"compaction"}

	f := parseFlags()

	if f.retentionConfigPath != "/etc/omnia/retention/retention.yaml" {
		t.Errorf("unexpected retentionConfigPath: %s", f.retentionConfigPath)
	}
	if f.batchSize != 1000 {
		t.Errorf("unexpected batchSize: %d", f.batchSize)
	}
	if f.maxRetries != 3 {
		t.Errorf("unexpected maxRetries: %d", f.maxRetries)
	}
	if f.compression != "snappy" {
		t.Errorf("unexpected compression: %s", f.compression)
	}
	if f.dryRun {
		t.Error("expected dryRun == false")
	}
	if f.metricsAddr != ":9090" {
		t.Errorf("unexpected metricsAddr: %s", f.metricsAddr)
	}
	if f.coldBackend != "s3" {
		t.Errorf("unexpected coldBackend: %s", f.coldBackend)
	}
}

func TestParseFlags_WithArgs(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{
		"compaction",
		"--batch-size=500",
		"--max-retries=5",
		"--compression=zstd",
		"--dry-run",
		"--metrics-addr=:8080",
		"--postgres-conn=postgres://localhost/test",
		"--redis-addrs=localhost:6379",
		"--cold-backend=gcs",
		"--cold-bucket=my-bucket",
	}

	f := parseFlags()

	if f.batchSize != 500 {
		t.Errorf("expected batchSize 500, got %d", f.batchSize)
	}
	if f.maxRetries != 5 {
		t.Errorf("expected maxRetries 5, got %d", f.maxRetries)
	}
	if f.compression != "zstd" {
		t.Errorf("expected compression zstd, got %s", f.compression)
	}
	if !f.dryRun {
		t.Error("expected dryRun == true")
	}
	if f.metricsAddr != ":8080" {
		t.Errorf("expected metricsAddr :8080, got %s", f.metricsAddr)
	}
	if f.postgresConn != "postgres://localhost/test" {
		t.Errorf("unexpected postgresConn: %s", f.postgresConn)
	}
	if f.redisAddrs != "localhost:6379" {
		t.Errorf("unexpected redisAddrs: %s", f.redisAddrs)
	}
	if f.coldBackend != "gcs" {
		t.Errorf("unexpected coldBackend: %s", f.coldBackend)
	}
	if f.coldBucket != "my-bucket" {
		t.Errorf("unexpected coldBucket: %s", f.coldBucket)
	}
}

func TestParseFlags_EnvOverrides(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"compaction"}

	t.Setenv("POSTGRES_CONN", "postgres://envhost/db")
	t.Setenv("REDIS_ADDRS", "env-redis:6379")
	t.Setenv("COLD_BACKEND", "gcs")
	t.Setenv("COLD_BUCKET", "env-bucket")

	f := parseFlags()

	if f.postgresConn != "postgres://envhost/db" {
		t.Errorf("expected POSTGRES_CONN from env, got %s", f.postgresConn)
	}
	if f.redisAddrs != "env-redis:6379" {
		t.Errorf("expected REDIS_ADDRS from env, got %s", f.redisAddrs)
	}
	if f.coldBackend != "gcs" {
		t.Errorf("expected COLD_BACKEND from env, got %s", f.coldBackend)
	}
	if f.coldBucket != "env-bucket" {
		t.Errorf("expected COLD_BUCKET from env, got %s", f.coldBucket)
	}
}

func TestParseFlags_FlagOverridesEnv(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{
		"compaction",
		"--postgres-conn=postgres://flaghost/db",
	}

	t.Setenv("POSTGRES_CONN", "postgres://envhost/db")

	f := parseFlags()

	// Flag should take precedence over env.
	if f.postgresConn != "postgres://flaghost/db" {
		t.Errorf("expected flag to override env, got %s", f.postgresConn)
	}
}

// ---------------------------------------------------------------------------
// validateProviderFlags tests
// ---------------------------------------------------------------------------

func TestValidateProviderFlags_MissingPostgresConn(t *testing.T) {
	f := &flags{coldBucket: "bucket", coldBackend: "s3"}
	err := validateProviderFlags(f)
	if err == nil {
		t.Fatal("expected error for missing postgres-conn")
	}
	if !strings.Contains(err.Error(), "postgres-conn") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateProviderFlags_MissingColdBucket(t *testing.T) {
	f := &flags{
		postgresConn: "postgres://localhost/db",
		coldBackend:  "s3",
	}
	err := validateProviderFlags(f)
	if err == nil {
		t.Fatal("expected error for missing cold-bucket")
	}
	if !strings.Contains(err.Error(), "cold-bucket") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateProviderFlags_UnsupportedBackend(t *testing.T) {
	f := &flags{
		postgresConn: "postgres://localhost/db",
		coldBucket:   "bucket",
		coldBackend:  "invalid-backend",
	}
	err := validateProviderFlags(f)
	if err == nil {
		t.Fatal("expected error for unsupported backend")
	}
	if !strings.Contains(err.Error(), "unsupported cold backend") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateProviderFlags_ValidS3(t *testing.T) {
	f := &flags{
		postgresConn: "postgres://localhost/db",
		coldBucket:   "bucket",
		coldBackend:  "s3",
	}
	if err := validateProviderFlags(f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProviderFlags_ValidGCS(t *testing.T) {
	f := &flags{
		postgresConn: "postgres://localhost/db",
		coldBucket:   "bucket",
		coldBackend:  "gcs",
	}
	if err := validateProviderFlags(f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateProviderFlags_ValidAzure(t *testing.T) {
	f := &flags{
		postgresConn: "postgres://localhost/db",
		coldBucket:   "bucket",
		coldBackend:  "azure",
	}
	if err := validateProviderFlags(f); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// initProviders tests (validation paths only — no real network)
// ---------------------------------------------------------------------------

func TestInitProviders_MissingPostgresConn(t *testing.T) {
	f := &flags{coldBucket: "bucket", coldBackend: "s3"}
	_, _, _, _, err := initProviders(context.Background(), f)
	if err == nil {
		t.Fatal("expected error for missing postgres-conn")
	}
}

func TestInitProviders_MissingColdBucket(t *testing.T) {
	f := &flags{
		postgresConn: "postgres://localhost/db",
		coldBackend:  "s3",
	}
	_, _, _, _, err := initProviders(context.Background(), f)
	if err == nil {
		t.Fatal("expected error for missing cold-bucket")
	}
}

func TestInitProviders_UnsupportedBackend(t *testing.T) {
	f := &flags{
		postgresConn: "postgres://localhost/db",
		coldBucket:   "bucket",
		coldBackend:  "invalid",
	}
	_, _, _, _, err := initProviders(context.Background(), f)
	if err == nil {
		t.Fatal("expected error for unsupported backend")
	}
}

// ---------------------------------------------------------------------------
// runWithFlags tests
// ---------------------------------------------------------------------------

func TestRunWithFlags_RetentionConfigNotFound(t *testing.T) {
	f := &flags{
		retentionConfigPath: "/nonexistent/path/retention.yaml",
		metricsAddr:         ":0",
		postgresConn:        "postgres://localhost/db",
		coldBucket:          "bucket",
		coldBackend:         "s3",
	}
	err := runWithFlags(f)
	if err == nil {
		t.Fatal("expected error for missing retention config")
	}
	if !strings.Contains(err.Error(), "retention config") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunWithFlags_ColdArchiveDisabled(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "retention.yaml")
	content := `
default:
  warmStore:
    retentionDays: 7
  coldArchive:
    enabled: false
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	f := &flags{
		retentionConfigPath: cfgPath,
		metricsAddr:         ":0",
		postgresConn:        "postgres://localhost/db",
		coldBucket:          "bucket",
		coldBackend:         "s3",
	}
	err := runWithFlags(f)
	if err != nil {
		t.Fatalf("expected nil error for disabled cold archive, got: %v", err)
	}
}

func TestRunWithFlags_MissingPostgresConn(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "retention.yaml")
	content := `
default:
  warmStore:
    retentionDays: 7
  coldArchive:
    enabled: true
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	f := &flags{
		retentionConfigPath: cfgPath,
		metricsAddr:         ":0",
		coldBucket:          "bucket",
		coldBackend:         "s3",
	}
	err := runWithFlags(f)
	if err == nil {
		t.Fatal("expected error for missing postgres-conn")
	}
	if !strings.Contains(err.Error(), "postgres-conn") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunWithFlags_MissingColdBucket(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "retention.yaml")
	content := `
default:
  warmStore:
    retentionDays: 7
  coldArchive:
    enabled: true
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	f := &flags{
		retentionConfigPath: cfgPath,
		metricsAddr:         ":0",
		postgresConn:        "postgres://localhost/db",
		coldBackend:         "s3",
	}
	err := runWithFlags(f)
	if err == nil {
		t.Fatal("expected error for missing cold-bucket")
	}
	if !strings.Contains(err.Error(), "cold-bucket") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunWithFlags_UnsupportedBackend(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "retention.yaml")
	content := `
default:
  warmStore:
    retentionDays: 7
  coldArchive:
    enabled: true
`
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	f := &flags{
		retentionConfigPath: cfgPath,
		metricsAddr:         ":0",
		postgresConn:        "postgres://localhost/db",
		coldBucket:          "bucket",
		coldBackend:         "invalid-backend",
	}
	err := runWithFlags(f)
	if err == nil {
		t.Fatal("expected error for unsupported backend")
	}
	if !strings.Contains(err.Error(), "unsupported cold backend") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// run() tests (exercises parseFlags + runWithFlags together)
// ---------------------------------------------------------------------------

func TestRun_InvalidRetentionConfig(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{
		"compaction",
		"--retention-config=/nonexistent/path/retention.yaml",
		"--metrics-addr=:0",
	}

	err := run()
	if err == nil {
		t.Fatal("expected error for missing retention config")
	}
	if !strings.Contains(err.Error(), "retention config") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// initProviders deeper coverage
// ---------------------------------------------------------------------------

func TestInitProviders_PostgresConnectionFails(t *testing.T) {
	// Use a connection string that pgx can parse but immediately fails to
	// connect to (localhost on an unlikely port with a short timeout).
	f := &flags{
		postgresConn: "postgres://user:pass@localhost:1/db?connect_timeout=1",
		coldBucket:   "bucket",
		coldBackend:  "s3",
	}
	_, _, _, _, err := initProviders(context.Background(), f)
	if err == nil {
		t.Fatal("expected error for unreachable postgres")
	}
	if !strings.Contains(err.Error(), "creating postgres provider") {
		t.Errorf("unexpected error: %v", err)
	}
}

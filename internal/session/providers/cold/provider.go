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

package cold

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/altairalabs/omnia/internal/session"
	"github.com/altairalabs/omnia/internal/session/providers"
)

// Compile-time interface check.
var _ providers.ColdArchiveProvider = (*Provider)(nil)

// Provider implements ColdArchiveProvider using a BlobStore backend and
// Parquet serialization.
type Provider struct {
	store       BlobStore
	prefix      string
	compression string
	maxFileSize int64
	ownsStore   bool
}

// New creates a Provider from the given Config, instantiating the appropriate
// BlobStore backend and verifying connectivity.
func New(ctx context.Context, cfg Config) (*Provider, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("cold archive: bucket is required")
	}

	// Apply defaults.
	defaults := DefaultConfig()
	if cfg.Prefix == "" {
		cfg.Prefix = defaults.Prefix
	}
	if cfg.DefaultCompression == "" {
		cfg.DefaultCompression = defaults.DefaultCompression
	}
	if cfg.DefaultMaxFileSize == 0 {
		cfg.DefaultMaxFileSize = defaults.DefaultMaxFileSize
	}

	store, err := createBlobStore(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Verify connectivity.
	if err := store.Ping(ctx); err != nil {
		_ = store.Close()
		return nil, fmt.Errorf("cold archive: ping: %w", err)
	}

	return &Provider{
		store:       store,
		prefix:      cfg.Prefix,
		compression: cfg.DefaultCompression,
		maxFileSize: cfg.DefaultMaxFileSize,
		ownsStore:   true,
	}, nil
}

// createBlobStore instantiates the backend-specific BlobStore.
func createBlobStore(ctx context.Context, cfg Config) (BlobStore, error) {
	switch cfg.Backend {
	case BackendS3:
		if cfg.S3 == nil {
			return nil, fmt.Errorf("cold archive: S3 config is required for S3 backend")
		}
		return NewS3BlobStore(ctx, cfg.Bucket, *cfg.S3)
	case BackendGCS:
		gcsCfg := GCSConfig{}
		if cfg.GCS != nil {
			gcsCfg = *cfg.GCS
		}
		return NewGCSBlobStore(ctx, cfg.Bucket, gcsCfg)
	case BackendAzure:
		if cfg.Azure == nil {
			return nil, fmt.Errorf("cold archive: Azure config is required for Azure backend")
		}
		return NewAzureBlobStore(ctx, cfg.Bucket, *cfg.Azure)
	default:
		return nil, fmt.Errorf("cold archive: unsupported backend %q", cfg.Backend)
	}
}

// NewFromBlobStore wraps an existing BlobStore. The caller retains ownership
// of the store; Close will not close it.
func NewFromBlobStore(store BlobStore, opts Options) *Provider {
	defaults := DefaultOptions()
	if opts.Prefix == "" {
		opts.Prefix = defaults.Prefix
	}
	if opts.DefaultCompression == "" {
		opts.DefaultCompression = defaults.DefaultCompression
	}
	if opts.DefaultMaxFileSize == 0 {
		opts.DefaultMaxFileSize = defaults.DefaultMaxFileSize
	}

	return &Provider{
		store:       store,
		prefix:      opts.Prefix,
		compression: opts.DefaultCompression,
		maxFileSize: opts.DefaultMaxFileSize,
		ownsStore:   false,
	}
}

// WriteParquet serializes sessions into Parquet format and writes them to
// the configured object store.
func (p *Provider) WriteParquet(ctx context.Context, sessions []*session.Session, opts providers.WriteOpts) error {
	if len(sessions) == 0 {
		return nil
	}

	maxFileSize := p.maxFileSize
	if opts.MaxFileSize > 0 {
		maxFileSize = opts.MaxFileSize
	}

	prefix := p.prefix
	if opts.BasePath != "" {
		prefix = opts.BasePath
	}

	// Group sessions by Hive partition path.
	groups := make(map[string][]*session.Session)
	for _, s := range sessions {
		path := hivePath(prefix, s)
		groups[path] = append(groups[path], s)
	}

	for path, group := range groups {
		if err := p.writeGroup(ctx, path, group, maxFileSize); err != nil {
			return err
		}
	}

	return nil
}

// writeGroup writes a single Hive partition group and updates the manifest.
func (p *Provider) writeGroup(ctx context.Context, path string, group []*session.Session, maxFileSize int64) error {
	rows := make([]sessionRow, len(group))
	for i, s := range group {
		rows[i] = sessionToRow(s)
	}

	chunks := splitRows(rows, maxFileSize)

	existing, err := p.store.List(ctx, path)
	if err != nil {
		return fmt.Errorf("list existing files: %w", err)
	}
	startPart := len(existing)

	fileKeys := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		data, err := writeParquetBytes(chunk)
		if err != nil {
			return fmt.Errorf("write parquet: %w", err)
		}

		key := fmt.Sprintf("%spart-%04d.parquet", path, startPart+i)
		if err := p.store.Put(ctx, key, data, "application/octet-stream"); err != nil {
			return fmt.Errorf("put parquet file: %w", err)
		}
		fileKeys = append(fileKeys, key)
	}

	return updateManifest(ctx, p.store, p.prefix, func(m *Manifest) {
		p.updateSessionIndex(m, group, chunks, path, startPart)
		updateDateEntry(m, group, len(fileKeys))
	})
}

// updateSessionIndex maps each session ID to the file key containing it.
func (p *Provider) updateSessionIndex(m *Manifest, group []*session.Session, chunks [][]sessionRow, path string, startPart int) {
	idx := 0
	for ci, chunk := range chunks {
		fk := fmt.Sprintf("%spart-%04d.parquet", path, startPart+ci)
		for range chunk {
			m.SessionIndex[group[idx].ID] = fk
			idx++
		}
	}
}

// updateDateEntry adds or updates the DateEntry for the group's creation date.
func updateDateEntry(m *Manifest, group []*session.Session, fileCount int) {
	dateKey := group[0].CreatedAt.UTC().Truncate(24 * time.Hour)
	for i, d := range m.Dates {
		if d.Date.Equal(dateKey) {
			m.Dates[i].FileCount += fileCount
			m.Dates[i].SessionCount += len(group)
			sortDates(m)
			return
		}
	}
	m.Dates = append(m.Dates, DateEntry{
		Date:         dateKey,
		FileCount:    fileCount,
		SessionCount: len(group),
	})
	sortDates(m)
}

func sortDates(m *Manifest) {
	sort.Slice(m.Dates, func(i, j int) bool {
		return m.Dates[i].Date.Before(m.Dates[j].Date)
	})
}

// GetSession retrieves a single archived session by ID.
func (p *Provider) GetSession(ctx context.Context, sessionID string) (*session.Session, error) {
	m, err := readManifest(ctx, p.store, p.prefix)
	if err != nil {
		return nil, err
	}

	fileKey, ok := m.SessionIndex[sessionID]
	if !ok {
		return nil, session.ErrSessionNotFound
	}

	data, err := p.store.Get(ctx, fileKey)
	if err != nil {
		return nil, fmt.Errorf("get parquet file: %w", err)
	}

	rows, err := readParquetBytes(data)
	if err != nil {
		return nil, err
	}

	for _, r := range rows {
		if r.ID == sessionID {
			return rowToSession(r)
		}
	}

	return nil, session.ErrSessionNotFound
}

// ListAvailableDates returns the dates for which archived data exists.
func (p *Provider) ListAvailableDates(ctx context.Context) ([]time.Time, error) {
	m, err := readManifest(ctx, p.store, p.prefix)
	if err != nil {
		return nil, err
	}

	dates := make([]time.Time, len(m.Dates))
	for i, d := range m.Dates {
		dates[i] = d.Date
	}
	return dates, nil
}

// QuerySessions searches archived sessions using space-separated key=value pairs.
//
// Supported fields:
//   - agent_name, namespace, workspace_name, status
//   - created_after, created_before (RFC3339)
//   - tag (repeatable)
func (p *Provider) QuerySessions(ctx context.Context, query string) ([]*session.Session, error) {
	filters := parseQuery(query)

	m, err := readManifest(ctx, p.store, p.prefix)
	if err != nil {
		return nil, err
	}

	fileSet, err := p.collectParquetFiles(ctx, m, filters)
	if err != nil {
		return nil, err
	}

	return p.scanFiles(ctx, fileSet, filters)
}

// collectParquetFiles gathers all parquet file keys matching the query's date range.
func (p *Provider) collectParquetFiles(ctx context.Context, m *Manifest, f queryFilters) (map[string]struct{}, error) {
	datePrefixes := p.datePrefixesForQuery(m, f)

	fileSet := make(map[string]struct{})
	for _, dp := range datePrefixes {
		keys, err := p.store.List(ctx, dp)
		if err != nil {
			return nil, fmt.Errorf("list files: %w", err)
		}
		for _, k := range keys {
			if strings.HasSuffix(k, ".parquet") {
				fileSet[k] = struct{}{}
			}
		}
	}
	return fileSet, nil
}

// scanFiles reads parquet files and returns sessions matching the filters.
func (p *Provider) scanFiles(ctx context.Context, fileSet map[string]struct{}, filters queryFilters) ([]*session.Session, error) {
	const maxResults = 1000
	var results []*session.Session

	for key := range fileSet {
		if len(results) >= maxResults {
			break
		}

		data, err := p.store.Get(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("get parquet file: %w", err)
		}

		rows, err := readParquetBytes(data)
		if err != nil {
			return nil, err
		}

		for _, r := range rows {
			if len(results) >= maxResults {
				break
			}
			if !matchesFilters(r, filters) {
				continue
			}
			s, err := rowToSession(r)
			if err != nil {
				return nil, err
			}
			results = append(results, s)
		}
	}

	return results, nil
}

// DeleteOlderThan removes all archived data older than the cutoff date.
func (p *Provider) DeleteOlderThan(ctx context.Context, cutoff time.Time) error {
	cutoffDate := cutoff.UTC().Truncate(24 * time.Hour)

	return updateManifest(ctx, p.store, p.prefix, func(m *Manifest) {
		var kept []DateEntry
		for _, d := range m.Dates {
			if !d.Date.Before(cutoffDate) {
				kept = append(kept, d)
				continue
			}
			p.deleteDateObjects(ctx, m, d.Date)
		}
		m.Dates = kept
	})
}

// deleteDateObjects removes all objects and session index entries for a date.
func (p *Provider) deleteDateObjects(ctx context.Context, m *Manifest, date time.Time) {
	datePrefix := p.datePrefixForDate(date)
	keys, err := p.store.List(ctx, datePrefix)
	if err != nil {
		return
	}
	for _, k := range keys {
		_ = p.store.Delete(ctx, k)
	}
	for sid, fk := range m.SessionIndex {
		if strings.HasPrefix(fk, datePrefix) {
			delete(m.SessionIndex, sid)
		}
	}
}

// Ping checks connectivity to the underlying store.
func (p *Provider) Ping(ctx context.Context) error {
	return p.store.Ping(ctx)
}

// Close releases resources. If the Provider owns the store, it is closed.
func (p *Provider) Close() error {
	if p.ownsStore {
		return p.store.Close()
	}
	return nil
}

// --- Helpers ---

// agentNameRe matches characters not allowed in Hive partition agent names.
var agentNameRe = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// sanitizeAgentName replaces characters that are unsafe in object paths.
func sanitizeAgentName(name string) string {
	return agentNameRe.ReplaceAllString(name, "_")
}

// hivePath returns the Hive-style partition path for a session.
func hivePath(prefix string, s *session.Session) string {
	t := s.CreatedAt.UTC()
	return fmt.Sprintf("%syear=%04d/month=%02d/day=%02d/agent=%s/",
		prefix, t.Year(), int(t.Month()), t.Day(), sanitizeAgentName(s.AgentName))
}

// splitRows splits rows into chunks where each chunk's serialized size
// is approximately bounded by maxSize.
func splitRows(rows []sessionRow, maxSize int64) [][]sessionRow {
	if len(rows) == 0 {
		return nil
	}

	estimatedTotal := int64(0)
	for _, r := range rows {
		data, _ := json.Marshal(r)
		estimatedTotal += int64(len(data))
	}

	if estimatedTotal <= maxSize || maxSize <= 0 {
		return [][]sessionRow{rows}
	}

	avgRowSize := max(estimatedTotal/int64(len(rows)), 1)
	rowsPerChunk := max(int(maxSize/avgRowSize), 1)

	var chunks [][]sessionRow
	for i := 0; i < len(rows); i += rowsPerChunk {
		end := min(i+rowsPerChunk, len(rows))
		chunks = append(chunks, rows[i:end])
	}
	return chunks
}

// queryFilters holds parsed query conditions.
type queryFilters struct {
	agentName     string
	namespace     string
	workspaceName string
	status        string
	createdAfter  time.Time
	createdBefore time.Time
	tags          []string
}

// parseQuery parses a space-separated key=value query string.
func parseQuery(query string) queryFilters {
	var f queryFilters
	for _, part := range strings.Fields(query) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k, v := kv[0], kv[1]
		switch k {
		case "agent_name":
			f.agentName = v
		case "namespace":
			f.namespace = v
		case "workspace_name":
			f.workspaceName = v
		case "status":
			f.status = v
		case "created_after":
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				f.createdAfter = t
			}
		case "created_before":
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				f.createdBefore = t
			}
		case "tag":
			f.tags = append(f.tags, v)
		}
	}
	return f
}

// matchesFilters returns true if the row matches all non-zero filter fields.
func matchesFilters(r sessionRow, f queryFilters) bool {
	if f.agentName != "" && r.AgentName != f.agentName {
		return false
	}
	if f.namespace != "" && r.Namespace != f.namespace {
		return false
	}
	if f.workspaceName != "" && r.WorkspaceName != f.workspaceName {
		return false
	}
	if f.status != "" && r.Status != f.status {
		return false
	}
	createdAt := time.Unix(0, r.CreatedAt).UTC()
	if !f.createdAfter.IsZero() && createdAt.Before(f.createdAfter) {
		return false
	}
	if !f.createdBefore.IsZero() && createdAt.After(f.createdBefore) {
		return false
	}
	return matchesTags(r.Tags, f.tags)
}

// matchesTags returns true if the row's tags contain all required tags.
func matchesTags(rawTags string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	var rowTags []string
	if rawTags != "" && rawTags != jsonNull {
		_ = json.Unmarshal([]byte(rawTags), &rowTags)
	}
	tagSet := make(map[string]struct{}, len(rowTags))
	for _, t := range rowTags {
		tagSet[t] = struct{}{}
	}
	for _, t := range required {
		if _, ok := tagSet[t]; !ok {
			return false
		}
	}
	return true
}

// datePrefixForDate returns the object prefix for a given date.
func (p *Provider) datePrefixForDate(d time.Time) string {
	d = d.UTC()
	return fmt.Sprintf("%syear=%04d/month=%02d/day=%02d/",
		p.prefix, d.Year(), int(d.Month()), d.Day())
}

// datePrefixesForQuery returns the date prefixes that need scanning.
// If date range filters are set, it prunes to matching dates.
func (p *Provider) datePrefixesForQuery(m *Manifest, f queryFilters) []string {
	prefixes := make([]string, 0, len(m.Dates))
	for _, d := range m.Dates {
		if !f.createdAfter.IsZero() {
			dayAfter := f.createdAfter.UTC().Truncate(24 * time.Hour)
			if d.Date.Before(dayAfter) {
				continue
			}
		}
		if !f.createdBefore.IsZero() {
			dayBefore := f.createdBefore.UTC().Truncate(24 * time.Hour).Add(24 * time.Hour)
			if !d.Date.Before(dayBefore) {
				continue
			}
		}
		prefixes = append(prefixes, p.datePrefixForDate(d.Date))
	}

	// If no date filters, return all date prefixes.
	if len(prefixes) == 0 && f.createdAfter.IsZero() && f.createdBefore.IsZero() {
		for _, d := range m.Dates {
			prefixes = append(prefixes, p.datePrefixForDate(d.Date))
		}
	}

	return prefixes
}

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

// Package schema provides validation against the published PromptPack JSON Schema.
package schema

import (
	// embed is used to embed the promptpack.schema.json file for offline validation
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/xeipuuv/gojsonschema"
)

// Embedded schema for offline/air-gapped environments.
// Updated via: make update-schema (or ./hack/update-schema.sh)
//
//go:embed promptpack.schema.json
var embeddedSchema string

// PromptPackSchemaURL is the published PromptPack schema URL (latest version).
const PromptPackSchemaURL = "https://promptpack.org/schema/latest/promptpack.schema.json"

// Default cache duration for fetched schemas.
const defaultCacheDuration = 1 * time.Hour

// SchemaSource indicates where the schema was loaded from.
type SchemaSource string

const (
	SchemaSourceEmbedded SchemaSource = "embedded"
	SchemaSourceNetwork  SchemaSource = "network"
	SchemaSourceCache    SchemaSource = "cache"
)

// cachedSchema holds a cached schema with expiration.
type cachedSchema struct {
	loader    gojsonschema.JSONLoader
	expiresAt time.Time
}

// SchemaValidator validates pack.json against the published PromptPack JSON Schema.
type SchemaValidator struct {
	log           logr.Logger
	httpClient    *http.Client
	cacheDuration time.Duration

	// Cache for network-fetched schemas by URL
	cache   map[string]*cachedSchema
	cacheMu sync.RWMutex

	// Embedded schema loader (created once)
	embeddedLoader gojsonschema.JSONLoader
}

// NewSchemaValidatorWithOptions creates a validator with custom settings.
func NewSchemaValidatorWithOptions(log logr.Logger, httpClient *http.Client, cacheDuration time.Duration) *SchemaValidator {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 10 * time.Second,
		}
	}
	if cacheDuration <= 0 {
		cacheDuration = defaultCacheDuration
	}

	return &SchemaValidator{
		log:            log.WithName("schema-validator"),
		httpClient:     httpClient,
		cacheDuration:  cacheDuration,
		cache:          make(map[string]*cachedSchema),
		embeddedLoader: gojsonschema.NewStringLoader(embeddedSchema),
	}
}

// Validate validates pack.json data against the PromptPack schema.
// If the pack includes a $schema field, attempts to fetch that specific version.
// Falls back to embedded schema if network fetch fails.
func (v *SchemaValidator) Validate(data []byte) error {
	// Extract $schema URL from pack if present
	schemaURL := extractSchemaURL(data)
	if schemaURL == "" {
		schemaURL = PromptPackSchemaURL
	}

	loader, source := v.getSchemaLoader(schemaURL)

	v.log.V(1).Info("validating pack.json",
		"schemaURL", schemaURL,
		"source", source,
	)

	documentLoader := gojsonschema.NewBytesLoader(data)
	result, err := gojsonschema.Validate(loader, documentLoader)
	if err != nil {
		return fmt.Errorf("schema validation error: %w", err)
	}

	if !result.Valid() {
		var errors []string
		for _, desc := range result.Errors() {
			errors = append(errors, fmt.Sprintf("%s: %s", desc.Field(), desc.Description()))
		}
		return fmt.Errorf("invalid pack.json: %s", strings.Join(errors, "; "))
	}

	v.log.V(1).Info("pack.json validation passed", "schemaURL", schemaURL)
	return nil
}

// getSchemaLoader returns a schema loader, using cache when available.
// Falls back to embedded schema if network fetch fails.
func (v *SchemaValidator) getSchemaLoader(schemaURL string) (gojsonschema.JSONLoader, SchemaSource) {
	// Check cache first
	v.cacheMu.RLock()
	cached, ok := v.cache[schemaURL]
	v.cacheMu.RUnlock()

	if ok && time.Now().Before(cached.expiresAt) {
		v.log.V(2).Info("using cached schema", "url", schemaURL)
		return cached.loader, SchemaSourceCache
	}

	// Try to fetch from network
	loader, err := v.fetchSchema(schemaURL)
	if err != nil {
		v.log.Info("network fetch failed, using embedded schema",
			"url", schemaURL,
			"error", err.Error(),
		)
		return v.embeddedLoader, SchemaSourceEmbedded
	}

	// Cache the fetched schema
	v.cacheMu.Lock()
	v.cache[schemaURL] = &cachedSchema{
		loader:    loader,
		expiresAt: time.Now().Add(v.cacheDuration),
	}
	v.cacheMu.Unlock()

	v.log.V(1).Info("cached schema from network",
		"url", schemaURL,
		"cacheDuration", v.cacheDuration,
	)

	return loader, SchemaSourceNetwork
}

// fetchSchema fetches a schema from a URL.
func (v *SchemaValidator) fetchSchema(schemaURL string) (gojsonschema.JSONLoader, error) {
	v.log.V(2).Info("fetching schema from network", "url", schemaURL)

	req, err := http.NewRequest(http.MethodGet, schemaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "omnia-controller/1.0")

	resp, err := v.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Validate it's valid JSON
	var js json.RawMessage
	if err := json.Unmarshal(body, &js); err != nil {
		return nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	v.log.V(1).Info("successfully fetched schema",
		"url", schemaURL,
		"size", len(body),
	)

	return gojsonschema.NewStringLoader(string(body)), nil
}

// extractSchemaURL extracts the $schema URL from pack JSON data.
func extractSchemaURL(data []byte) string {
	var pack struct {
		Schema string `json:"$schema"`
	}
	if json.Unmarshal(data, &pack) != nil {
		return ""
	}
	return pack.Schema
}

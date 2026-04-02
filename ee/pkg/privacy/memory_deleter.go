/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/logging"
)

// defaultMemoryBatchSize is the number of memories deleted per batch call.
const defaultMemoryBatchSize = 500

// memoryBatchDeleteResponse is the response from the batch delete endpoint.
type memoryBatchDeleteResponse struct {
	Deleted int `json:"deleted"`
}

// MemoryHTTPDeleter implements MemoryDeleter by calling the memory-api batch
// delete endpoint in a loop until all memories for a user are removed.
type MemoryHTTPDeleter struct {
	baseURL   string
	client    *http.Client
	batchSize int
	log       logr.Logger
}

// NewMemoryHTTPDeleter creates a MemoryHTTPDeleter that targets the given baseURL.
func NewMemoryHTTPDeleter(baseURL string, log logr.Logger) *MemoryHTTPDeleter {
	return &MemoryHTTPDeleter{
		baseURL:   baseURL,
		client:    &http.Client{Timeout: 30 * time.Second},
		batchSize: defaultMemoryBatchSize,
		log:       log.WithName("memory-http-deleter"),
	}
}

// DeleteAllMemories deletes all memories for userID in workspace by calling the
// batch delete endpoint repeatedly until deleted == 0.
func (d *MemoryHTTPDeleter) DeleteAllMemories(ctx context.Context, userID, workspace string) error {
	for {
		n, err := d.deleteBatch(ctx, userID, workspace)
		if err != nil {
			return err
		}
		d.log.V(1).Info("memory batch deleted", "count", n, "userHash", logging.HashID(userID))
		if n == 0 {
			return nil
		}
	}
}

// deleteBatch sends a single DELETE request and returns the number of deleted memories.
func (d *MemoryHTTPDeleter) deleteBatch(ctx context.Context, userID, workspace string) (int, error) {
	url := fmt.Sprintf(
		"%s/api/v1/memories/batch?workspace=%s&user_id=%s&limit=%d",
		d.baseURL, workspace, userID, d.batchSize,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return 0, fmt.Errorf("building memory delete request: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("memory delete request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("memory delete returned status %d", resp.StatusCode)
	}

	var result memoryBatchDeleteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding memory delete response: %w", err)
	}

	return result.Deleted, nil
}

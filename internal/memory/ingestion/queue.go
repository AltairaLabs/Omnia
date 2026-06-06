/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrWorkItemNotFound is returned by Get when no pending work item matches.
var ErrWorkItemNotFound = errors.New("summary work item not found")

// WorkItem is one document awaiting agent summarization. It is self-contained:
// the strategy + chunk geometry captured at enqueue time make completion
// deterministic even if the live policy changes meanwhile.
type WorkItem struct {
	WorkspaceID  string    `json:"workspace_id"`
	Doc          SourceDoc `json:"doc"`
	Strategy     string    `json:"strategy"`
	ChunkSize    int       `json:"chunk_size"`
	ChunkOverlap int       `json:"chunk_overlap"`
	AboutKey     string    `json:"about_key"` // entity key (the document URL)
}

// SummaryQueue is the async work-queue for agent-backed document summarization.
// memory-api enqueues raw text; the summarizer agent lists candidates,
// summarizes, posts back, and the item is completed (deleted).
type SummaryQueue interface {
	Enqueue(ctx context.Context, item WorkItem) error
	List(ctx context.Context, limit int) ([]WorkItem, error)
	Get(ctx context.Context, workspaceID, aboutKey string) (WorkItem, error)
	Complete(ctx context.Context, workspaceID, aboutKey string) error
}

// SourceDoc needs JSON tags so WorkItem round-trips through the filesystem.
// (Defined in strategy.go without tags; encoding/json uses field names, which
// is fine — this comment documents the dependency.)

// FileSummaryQueue stores one JSON file per pending WorkItem under dir. Writes
// are atomic (temp file + rename). Single-consumer (one CronJob per
// memory-service) — no lease/visibility-timeout.
type FileSummaryQueue struct {
	dir string
}

// NewFileSummaryQueue creates the queue directory if needed.
func NewFileSummaryQueue(dir string) (*FileSummaryQueue, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create summary queue dir %q: %w", dir, err)
	}
	return &FileSummaryQueue{dir: dir}, nil
}

// filename derives a filesystem-safe, collision-free name from (workspace,
// aboutKey). Same key -> same file, so re-enqueue overwrites.
func (q *FileSummaryQueue) filename(workspaceID, aboutKey string) string {
	sum := sha256.Sum256([]byte(workspaceID + "\x00" + aboutKey))
	return filepath.Join(q.dir, hex.EncodeToString(sum[:])+".json")
}

// Enqueue atomically writes the work item.
func (q *FileSummaryQueue) Enqueue(_ context.Context, item WorkItem) error {
	data, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal work item: %w", err)
	}
	final := q.filename(item.WorkspaceID, item.AboutKey)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write work item: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		return fmt.Errorf("commit work item: %w", err)
	}
	return nil
}

// List returns up to limit pending items. Corrupt/unreadable files are skipped.
func (q *FileSummaryQueue) List(_ context.Context, limit int) ([]WorkItem, error) {
	matches, err := filepath.Glob(filepath.Join(q.dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("scan queue dir: %w", err)
	}
	out := make([]WorkItem, 0, len(matches))
	for _, path := range matches {
		if limit > 0 && len(out) >= limit {
			break
		}
		item, ok := readWorkItem(path)
		if !ok {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// Get returns the pending item for (workspace, aboutKey) or ErrWorkItemNotFound.
func (q *FileSummaryQueue) Get(_ context.Context, workspaceID, aboutKey string) (WorkItem, error) {
	item, ok := readWorkItem(q.filename(workspaceID, aboutKey))
	if !ok {
		return WorkItem{}, ErrWorkItemNotFound
	}
	return item, nil
}

// Complete deletes the item. Missing file is a no-op (idempotent).
func (q *FileSummaryQueue) Complete(_ context.Context, workspaceID, aboutKey string) error {
	err := os.Remove(q.filename(workspaceID, aboutKey))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("complete work item: %w", err)
	}
	return nil
}

// readWorkItem reads + decodes one file. ok=false on any read/parse error.
func readWorkItem(path string) (WorkItem, bool) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from our own Glob of the queue dir
	if err != nil {
		return WorkItem{}, false
	}
	var item WorkItem
	if err := json.Unmarshal(data, &item); err != nil {
		return WorkItem{}, false
	}
	return item, true
}

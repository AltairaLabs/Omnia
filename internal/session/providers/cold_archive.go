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

package providers

import (
	"context"
	"time"

	"github.com/altairalabs/omnia/internal/session"
)

// ColdArchiveProvider defines the interface for long-term, cost-efficient
// session storage (e.g. S3/GCS/Azure Blob with Parquet format). It is
// optimized for batch writes and infrequent reads.
type ColdArchiveProvider interface {
	// WriteParquet serializes sessions into Parquet format and writes them
	// to the configured object store using the given options.
	WriteParquet(ctx context.Context, sessions []*session.Session, opts WriteOpts) error

	// GetSession retrieves a single archived session by ID.
	// Returns session.ErrSessionNotFound if the session is not in the archive.
	GetSession(ctx context.Context, sessionID string) (*session.Session, error)

	// ListAvailableDates returns the dates for which archived data exists,
	// sorted in ascending order.
	ListAvailableDates(ctx context.Context) ([]time.Time, error)

	// QuerySessions searches archived sessions using the given query string.
	// The query format is implementation-specific (e.g. SQL predicate pushdown).
	QuerySessions(ctx context.Context, query string) ([]*session.Session, error)

	// DeleteOlderThan removes all archived data older than the cutoff date.
	DeleteOlderThan(ctx context.Context, cutoff time.Time) error

	// Ping checks connectivity to the underlying store.
	Ping(ctx context.Context) error

	// Close releases resources held by the provider.
	Close() error
}

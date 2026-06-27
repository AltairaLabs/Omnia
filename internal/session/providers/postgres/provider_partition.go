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

package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/altairalabs/omnia/internal/session/providers"
)

func (p *Provider) CreatePartition(ctx context.Context, date time.Time) error {
	// Align to ISO week boundary (Monday).
	isoYear, isoWeek := date.ISOWeek()
	weekStart := isoWeekStart(isoYear, isoWeek)
	weekEnd := weekStart.AddDate(0, 0, 7)

	var totalCreated int
	for _, table := range partitionTables {
		var created int
		err := p.pool.QueryRow(ctx,
			"SELECT create_weekly_partitions($1, $2::DATE, $3::DATE)",
			table, weekStart, weekEnd,
		).Scan(&created)
		if err != nil {
			return fmt.Errorf("postgres: create partition for %s: %w", table, err)
		}
		totalCreated += created
	}

	if totalCreated == 0 {
		return providers.ErrPartitionExists
	}
	return nil
}

func (p *Provider) DropPartition(ctx context.Context, date time.Time) error {
	isoYear, isoWeek := date.ISOWeek()
	suffix := fmt.Sprintf("w%04d_%02d", isoYear, isoWeek)

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Check that the sessions partition exists.
	var exists bool
	err = tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = $1 AND n.nspname = current_schema()
	)`, "sessions_"+suffix).Scan(&exists)
	if err != nil {
		return fmt.Errorf("postgres: check partition: %w", err)
	}
	if !exists {
		return providers.ErrPartitionNotFound
	}

	// Drop all table partitions in reverse dependency order.
	for _, table := range []string{"audit_log", "message_artifacts", "runtime_events", "provider_calls", "tool_calls", "messages", "sessions"} {
		name := pgx.Identifier{table + "_" + suffix}.Sanitize()
		_, err := tx.Exec(ctx, "DROP TABLE IF EXISTS "+name)
		if err != nil {
			return fmt.Errorf("postgres: drop partition %s: %w", name, err)
		}
	}

	return tx.Commit(ctx)
}

// parsePartitionDate tries each known layout to parse a partition bound timestamp.
func parsePartitionDate(s string) (time.Time, bool) {
	for _, layout := range partitionDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func (p *Provider) ListPartitions(ctx context.Context) ([]providers.PartitionInfo, error) {
	query := `SELECT c.relname,
		pg_get_expr(c.relpartbound, c.oid),
		pg_table_size(c.oid),
		pg_stat_get_live_tuples(c.oid)
	FROM pg_class c
	JOIN pg_inherits i ON i.inhrelid = c.oid
	JOIN pg_class parent ON parent.oid = i.inhparent
	JOIN pg_namespace n ON n.oid = parent.relnamespace
	WHERE parent.relname = 'sessions'
	AND n.nspname = current_schema()
	AND c.relispartition
	ORDER BY c.relname`

	rows, err := p.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("postgres: list partitions: %w", err)
	}
	defer rows.Close()

	var infos []providers.PartitionInfo
	for rows.Next() {
		var name, boundExpr string
		var sizeBytes, rowCount int64

		if err := rows.Scan(&name, &boundExpr, &sizeBytes, &rowCount); err != nil {
			return nil, fmt.Errorf("postgres: scan partition: %w", err)
		}

		matches := partBoundRe.FindStringSubmatch(boundExpr)
		if len(matches) != 3 {
			continue
		}

		startDate, ok := parsePartitionDate(matches[1])
		if !ok {
			continue
		}
		endDate, ok := parsePartitionDate(matches[2])
		if !ok {
			continue
		}

		infos = append(infos, providers.PartitionInfo{
			Name:      name,
			StartDate: startDate,
			EndDate:   endDate,
			RowCount:  rowCount,
			SizeBytes: sizeBytes,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate partitions: %w", err)
	}
	if infos == nil {
		infos = []providers.PartitionInfo{}
	}
	return infos, nil
}

// isoWeekStart returns the Monday 00:00 UTC of the given ISO year/week.
func isoWeekStart(isoYear, isoWeek int) time.Time {
	// Jan 4 is always in ISO week 1.
	jan4 := time.Date(isoYear, time.January, 4, 0, 0, 0, 0, time.UTC)
	// Go back to Monday of that week.
	offset := int(time.Monday - jan4.Weekday())
	if jan4.Weekday() == time.Sunday {
		offset = -6
	}
	week1Monday := jan4.AddDate(0, 0, offset)
	return week1Monday.AddDate(0, 0, (isoWeek-1)*7)
}

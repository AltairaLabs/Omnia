/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

package snowflake

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Watermark SQL statements.
const (
	queryGetWatermark = `SELECT last_sync_at FROM _omnia_sync_watermarks WHERE table_name = ?`
	querySetWatermark = `MERGE INTO _omnia_sync_watermarks t
		USING (SELECT ? AS table_name, ? AS last_sync_at, ? AS last_sync_rows) s
		ON t.table_name = s.table_name
		WHEN MATCHED THEN UPDATE SET
			last_sync_at = s.last_sync_at,
			last_sync_rows = s.last_sync_rows,
			updated_at = CURRENT_TIMESTAMP()
		WHEN NOT MATCHED THEN INSERT (table_name, last_sync_at, last_sync_rows)
			VALUES (s.table_name, s.last_sync_at, s.last_sync_rows)`
)

// getWatermark reads the last sync timestamp for a table. Returns zero time if no watermark exists.
func getWatermark(ctx context.Context, db DB, table string) (time.Time, error) {
	var lastSync time.Time
	err := db.QueryRowContext(ctx, queryGetWatermark, table).Scan(&lastSync)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return lastSync, nil
}

// setWatermark writes or updates the watermark for a table.
func setWatermark(ctx context.Context, db DB, table string, syncAt time.Time, rowCount int64) error {
	_, err := db.ExecContext(ctx, querySetWatermark, table, syncAt, rowCount)
	return err
}

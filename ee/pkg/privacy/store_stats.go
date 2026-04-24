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
)

// ConsentStats summarises workspace-wide consent posture for the
// dashboard. TotalUsers counts rows in user_privacy_preferences;
// users without an entry are NOT counted.
type ConsentStats struct {
	TotalUsers       int64            `json:"totalUsers"`
	OptedOutAll      int64            `json:"optedOutAll"`
	GrantsByCategory map[string]int64 `json:"grantsByCategory"`
}

// Stats returns workspace-wide consent posture aggregates. One round-trip.
func (s *PreferencesPostgresStore) Stats(ctx context.Context) (ConsentStats, error) {
	const query = `
		WITH grant_counts AS (
		    SELECT g AS grant, COUNT(*)::bigint AS n
		    FROM user_privacy_preferences,
		         UNNEST(consent_grants) AS g
		    GROUP BY g
		)
		SELECT
		    (SELECT COUNT(*)::bigint FROM user_privacy_preferences)                              AS total_users,
		    (SELECT COUNT(*)::bigint FROM user_privacy_preferences WHERE opt_out_all = TRUE)     AS opted_out_all,
		    COALESCE(
		        (SELECT JSONB_OBJECT_AGG(grant, n) FROM grant_counts),
		        '{}'::jsonb
		    ) AS grants_by_category`

	var grantsJSON []byte
	stats := ConsentStats{GrantsByCategory: map[string]int64{}}
	if err := s.pool.QueryRow(ctx, query).Scan(&stats.TotalUsers, &stats.OptedOutAll, &grantsJSON); err != nil {
		return ConsentStats{}, fmt.Errorf("privacy: consent stats query: %w", err)
	}
	if len(grantsJSON) > 0 {
		if err := json.Unmarshal(grantsJSON, &stats.GrantsByCategory); err != nil {
			return ConsentStats{}, fmt.Errorf("privacy: consent stats decode: %w", err)
		}
	}
	return stats, nil
}

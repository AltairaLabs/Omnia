/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package memory

// AnalyticsAggregateCategory is the canonical consent category string
// enforced by AggregateConsentJoin. The memory-api wire contract
// depends on this exact literal.
const AnalyticsAggregateCategory = "analytics:aggregate"

// AggregateConsentJoin returns SQL fragments that restrict a cross-user
// aggregate over memory_entities to users who have granted the
// analytics:aggregate consent category.
//
// entityAlias is the table alias used for memory_entities in the
// caller's query (e.g. "e" in "FROM memory_entities e"). The returned
// JOIN clause adds a LEFT JOIN against user_privacy_preferences; the
// WHERE clause restricts to rows where either:
//   - virtual_user_id IS NULL (institutional or agent-tier; not user data)
//   - the user has 'analytics:aggregate' in their consent_grants array
//
// LEFT JOIN (not inner JOIN) is critical: institutional and agent-tier
// rows have no matching user_privacy_preferences row, and an inner JOIN
// would silently drop them.
//
// Usage:
//
//	join, where := AggregateConsentJoin("e")
//	sql := `SELECT COUNT(*) FROM memory_entities e ` + join +
//	       ` WHERE e.workspace_id = $1 AND ` + where
//
// The helper does NOT add workspace / forgotten filters — callers
// compose those themselves.
func AggregateConsentJoin(entityAlias string) (joinClause, whereClause string) {
	joinClause = "LEFT JOIN user_privacy_preferences p ON p.user_id = " +
		entityAlias + ".virtual_user_id"
	whereClause = "(" + entityAlias + ".virtual_user_id IS NULL OR " +
		"'" + AnalyticsAggregateCategory + "' = ANY(p.consent_grants))"
	return
}

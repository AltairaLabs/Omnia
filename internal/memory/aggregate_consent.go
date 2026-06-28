/*
Copyright 2026 Altaira Labs.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0
*/

package memory

import (
	"context"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// AnalyticsAggregateCategory is the canonical consent category string
// used by the aggregate consent filter. The memory-api wire contract
// depends on this exact literal.
const AnalyticsAggregateCategory = "analytics:aggregate"

// consentGrantorSource is the minimal interface Aggregate needs to
// resolve the set of user IDs that have granted a given consent
// category. *httpclient.Client (ee/pkg/privacy/httpclient) satisfies
// this interface. A nil source is treated as "empty grantor set" — only
// institutional rows (virtual_user_id IS NULL) are counted (conservative
// safe default for deployments without a privacy-api).
type consentGrantorSource interface {
	ListConsentUsers(ctx context.Context, category privacy.ConsentCategory, granted bool) ([]string, error)
}

// AggregateConsentFilter returns a SQL WHERE fragment that restricts a
// cross-user aggregate over memory_entities to institutional rows
// (virtual_user_id IS NULL) and rows belonging to users in grantorParam.
//
// grantorParam is the SQL placeholder for a text[] bound argument — e.g.
// "$5". The caller must pass the grantor-id slice at that position.
//
// Logic:
//   - virtual_user_id IS NULL → institutional/agent-tier row; always included
//   - virtual_user_id = ANY($N::text[]) → user row whose owner granted
//     the consent category
//
// Empty grantor array semantics: = ANY('{}'::text[]) is false for every
// non-null user_id, so only institutional rows count — the correct
// conservative behaviour when no consent-source data is available.
//
// No JOIN is emitted: grantors are resolved from privacy-api before the
// query and passed as a bound argument, keeping the query plan stable
// regardless of the grantor-set cardinality.
//
// NOTE: the grantor id-set can be large; it is cached by the httpclient
// (30 s TTL) and the Aggregate result itself is cached in cache.go. The
// join-via-privacy-api-export model is a known scalability limit and is
// the target of a future optimisation (out of scope here).
func AggregateConsentFilter(entityAlias, grantorParam string) string {
	return "(" + entityAlias + ".virtual_user_id IS NULL OR " +
		entityAlias + ".virtual_user_id = ANY(" + grantorParam + "::text[]))"
}

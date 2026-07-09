/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"net/textproto"
	"regexp"
)

// headerIndexRefPattern matches a headers[...] index expression with a string
// literal key: headers["X-Foo"] or headers['X-Foo']. Only one of the two
// capture groups (double- or single-quoted) is populated per match. Dynamic
// keys (headers[someVar]) do not match and are intentionally ignored — no
// static check can canonicalize a key it cannot see.
var headerIndexRefPattern = regexp.MustCompile(`headers\s*\[\s*(?:"([^"]*)"|'([^']*)')\s*\]`)

// HeaderRef is a header-key reference found in a CEL expression, paired with the
// canonical form it should have used.
type HeaderRef struct {
	// Raw is the header key exactly as written in the CEL expression.
	Raw string
	// Canonical is the MIME-canonicalized form the key resolves to on the wire.
	Canonical string
}

// NonCanonicalHeaderRefs scans a CEL expression for headers["..."] literal-key
// references whose key is not already in canonical HTTP form. Every key in the
// decision-request headers map is canonicalized on the wire (buildDecisionHeaders
// sets them through an http.Request), so a non-canonical literal — e.g.
// headers["x-omnia-claim-team"] instead of headers["X-Omnia-Claim-Team"] —
// silently misses and reads as absent. The returned refs (deduped by raw key,
// first-seen order) let callers warn the policy author. An empty result means
// every header reference is canonical (or a dynamic key that cannot be checked).
func NonCanonicalHeaderRefs(expr string) []HeaderRef {
	matches := headerIndexRefPattern.FindAllStringSubmatch(expr, -1)
	var refs []HeaderRef
	seen := make(map[string]struct{})
	for _, m := range matches {
		key := m[1]
		if key == "" {
			key = m[2]
		}
		if key == "" {
			continue
		}
		canonical := textproto.CanonicalMIMEHeaderKey(key)
		if canonical == key {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		refs = append(refs, HeaderRef{Raw: key, Canonical: canonical})
	}
	return refs
}

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNonCanonicalHeaderRefs(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want []HeaderRef
	}{
		{
			name: "canonical claim reference is clean",
			expr: `headers["X-Omnia-Claim-Team"] == "platform"`,
			want: nil,
		},
		{
			name: "lowercase claim reference is flagged",
			expr: `has(headers["x-omnia-claim-team"])`,
			want: []HeaderRef{{Raw: "x-omnia-claim-team", Canonical: "X-Omnia-Claim-Team"}},
		},
		{
			name: "single-quoted lowercase is flagged",
			expr: `headers['x-omnia-claim-team'] != "finance"`,
			want: []HeaderRef{{Raw: "x-omnia-claim-team", Canonical: "X-Omnia-Claim-Team"}},
		},
		{
			name: "underscore segment is not a separator",
			expr: `headers["x-omnia-claim-customer_id"]`,
			want: []HeaderRef{{Raw: "x-omnia-claim-customer_id", Canonical: "X-Omnia-Claim-Customer_id"}},
		},
		{
			name: "non-claim header is also flagged (whole map is canonical)",
			expr: `headers["x-omnia-tool-name"] == "refund"`,
			want: []HeaderRef{{Raw: "x-omnia-tool-name", Canonical: "X-Omnia-Tool-Name"}},
		},
		{
			name: "dynamic key is ignored",
			expr: `headers[someVar]`,
			want: nil,
		},
		{
			name: "duplicate bad key reported once",
			expr: `headers["x-omnia-claim-team"] == "a" || headers["x-omnia-claim-team"] == "b"`,
			want: []HeaderRef{{Raw: "x-omnia-claim-team", Canonical: "X-Omnia-Claim-Team"}},
		},
		{
			name: "mixed canonical and non-canonical",
			expr: `headers["X-Omnia-Claim-Team"] == headers["x-omnia-claim-region"]`,
			want: []HeaderRef{{Raw: "x-omnia-claim-region", Canonical: "X-Omnia-Claim-Region"}},
		},
		{
			name: "whitespace inside index still matches",
			expr: `headers[ "x-omnia-claim-team" ]`,
			want: []HeaderRef{{Raw: "x-omnia-claim-team", Canonical: "X-Omnia-Claim-Team"}},
		},
		{
			name: "no header references at all",
			expr: `body.amount > 100`,
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NonCanonicalHeaderRefs(tc.expr))
		})
	}
}

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseKeyValueLabels(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want map[string]string
	}{
		{name: "empty", in: "", want: nil},
		{name: "whitespace only", in: "   ", want: nil},
		{
			name: "single",
			in:   "azure.workload.identity/use=true",
			want: map[string]string{"azure.workload.identity/use": "true"},
		},
		{
			name: "multiple with spaces",
			in:   "a=1, b=2 , c=3",
			want: map[string]string{"a": "1", "b": "2", "c": "3"},
		},
		{
			name: "empty value retained",
			in:   "k=",
			want: map[string]string{"k": ""},
		},
		{
			name: "entries without = are skipped",
			in:   "good=1,bogus,=novalue",
			want: map[string]string{"good": "1"},
		},
		{
			name: "value containing = keeps remainder",
			in:   "url=a=b",
			want: map[string]string{"url": "a=b"},
		},
		{name: "only invalid entries", in: "bogus,,", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseKeyValueLabels(tt.in))
		})
	}
}

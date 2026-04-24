/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package classify

import (
	"context"
	"testing"

	"github.com/altairalabs/omnia/ee/pkg/privacy"
)

// stubRules returns a fixed category for every input.
type stubRules struct{ out privacy.ConsentCategory }

func (s stubRules) Classify(_ string) privacy.ConsentCategory { return s.out }

// stubEmbedding returns a fixed category and an optional error.
type stubEmbedding struct {
	out privacy.ConsentCategory
	err error
}

func (s *stubEmbedding) Classify(_ context.Context, _ string) (privacy.ConsentCategory, error) {
	return s.out, s.err
}

func TestValidator_Apply(t *testing.T) {
	type args struct {
		caller   privacy.ConsentCategory
		ruleOut  privacy.ConsentCategory
		embedOut privacy.ConsentCategory
	}
	type want struct {
		category   privacy.ConsentCategory
		overridden bool
		from       privacy.ConsentCategory
		source     string
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "empty caller, no signals → empty",
			args: args{caller: "", ruleOut: "", embedOut: ""},
			want: want{category: "", overridden: false, from: "", source: ""},
		},
		{
			name: "empty caller, regex finds identity → fill from regex",
			args: args{caller: "", ruleOut: privacy.ConsentMemoryIdentity, embedOut: ""},
			want: want{category: privacy.ConsentMemoryIdentity, overridden: false, from: "", source: SourceRegex},
		},
		{
			name: "empty caller, embedding finds preferences → fill from embedding",
			args: args{caller: "", ruleOut: "", embedOut: privacy.ConsentMemoryPreferences},
			want: want{category: privacy.ConsentMemoryPreferences, overridden: false, from: "", source: SourceEmbedding},
		},
		{
			name: "regex beats embedding when both fire",
			args: args{caller: "", ruleOut: privacy.ConsentMemoryHealth, embedOut: privacy.ConsentMemoryPreferences},
			want: want{category: privacy.ConsentMemoryHealth, overridden: false, from: "", source: SourceRegex},
		},
		{
			name: "caller preferences, regex empty, embedding context → no upgrade (rank tie)",
			args: args{caller: privacy.ConsentMemoryPreferences, ruleOut: "", embedOut: privacy.ConsentMemoryContext},
			want: want{category: privacy.ConsentMemoryPreferences, overridden: false, from: "", source: ""},
		},
		{
			name: "caller preferences, embedding health → upgrade",
			args: args{
				caller:   privacy.ConsentMemoryPreferences,
				ruleOut:  "",
				embedOut: privacy.ConsentMemoryHealth,
			},
			want: want{
				category:   privacy.ConsentMemoryHealth,
				overridden: true,
				from:       privacy.ConsentMemoryPreferences,
				source:     SourceEmbedding,
			},
		},
		{
			name: "caller context, regex location → upgrade",
			args: args{
				caller:   privacy.ConsentMemoryContext,
				ruleOut:  privacy.ConsentMemoryLocation,
				embedOut: "",
			},
			want: want{
				category:   privacy.ConsentMemoryLocation,
				overridden: true,
				from:       privacy.ConsentMemoryContext,
				source:     SourceRegex,
			},
		},
		{
			name: "caller health, regex preferences → keep caller (no downgrade)",
			args: args{caller: privacy.ConsentMemoryHealth, ruleOut: privacy.ConsentMemoryPreferences, embedOut: ""},
			want: want{category: privacy.ConsentMemoryHealth, overridden: false, from: "", source: ""},
		},
		{
			name: "analytics:aggregate caller is left alone",
			args: args{caller: privacy.ConsentAnalyticsAggregate, ruleOut: privacy.ConsentMemoryHealth, embedOut: ""},
			want: want{category: privacy.ConsentAnalyticsAggregate, overridden: false, from: "", source: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(stubRules{out: tt.args.ruleOut}, &stubEmbedding{out: tt.args.embedOut})
			got := v.Apply(context.Background(), tt.args.caller, "ignored")
			if got.Category != tt.want.category {
				t.Errorf("Category = %q, want %q", got.Category, tt.want.category)
			}
			if got.Overridden != tt.want.overridden {
				t.Errorf("Overridden = %v, want %v", got.Overridden, tt.want.overridden)
			}
			if got.From != tt.want.from {
				t.Errorf("From = %q, want %q", got.From, tt.want.from)
			}
			if got.Source != tt.want.source {
				t.Errorf("Source = %q, want %q", got.Source, tt.want.source)
			}
		})
	}
}

func TestValidator_NoEmbedding(t *testing.T) {
	v := NewValidator(stubRules{out: privacy.ConsentMemoryIdentity}, nil)
	got := v.Apply(context.Background(), "", "x")
	if got.Category != privacy.ConsentMemoryIdentity {
		t.Errorf("got %q, want %q", got.Category, privacy.ConsentMemoryIdentity)
	}
	if got.Source != SourceRegex {
		t.Errorf("Source = %q, want %q", got.Source, SourceRegex)
	}
}

func TestValidator_EmbeddingError_FallsThrough(t *testing.T) {
	embed := &stubEmbedding{err: context.DeadlineExceeded}
	v := NewValidator(stubRules{out: privacy.ConsentMemoryIdentity}, embed)
	got := v.Apply(context.Background(), "", "x")
	if got.Category != privacy.ConsentMemoryIdentity {
		t.Errorf("got %q, want %q", got.Category, privacy.ConsentMemoryIdentity)
	}
}

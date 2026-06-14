/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import "context"

// subjectCtxKey is the unexported context key under which the verified
// ServiceAccount subject is stored.
type subjectCtxKey struct{}

// WithSubject returns a copy of ctx carrying the verified ServiceAccount
// subject (e.g. "system:serviceaccount:omnia-system:omnia-session-api").
func WithSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, subjectCtxKey{}, subject)
}

// SubjectFromContext returns the verified ServiceAccount subject previously
// stored by WithSubject, or "" if none is present. Downstream handlers can use
// it for identity-derived authorization (e.g. namespace-from-identity).
func SubjectFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(subjectCtxKey{}).(string); ok {
		return v
	}
	return ""
}

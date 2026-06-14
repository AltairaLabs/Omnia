/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package serviceauth

import "strings"

// serviceAccountPrefix is the scheme prefix of a Kubernetes ServiceAccount
// TokenReview subject ("system:serviceaccount:<namespace>:<name>").
const serviceAccountPrefix = "system:serviceaccount:"

// ParseServiceAccount splits a Kubernetes ServiceAccount subject of the exact
// form "system:serviceaccount:<namespace>:<name>" into its namespace and name.
// It is deliberately strict: ok is false unless the subject matches that 4-part
// shape exactly with non-empty namespace and name. Malformed or non-SA subjects
// (users, groups, extra/missing colons) return ("", "", false) so callers never
// trust an unparseable identity.
func ParseServiceAccount(subject string) (namespace, name string, ok bool) {
	rest, found := strings.CutPrefix(subject, serviceAccountPrefix)
	if !found {
		return "", "", false
	}
	ns, nm, found := strings.Cut(rest, ":")
	// The name segment must not itself contain a colon — that would be a
	// malformed subject, not a valid "<ns>:<name>".
	if !found || ns == "" || nm == "" || strings.Contains(nm, ":") {
		return "", "", false
	}
	return ns, nm, true
}

// authorizer decides whether a verified TokenReview subject is allowed. A
// subject is authorized iff it is an exact match in allowedSubjects OR it parses
// as a ServiceAccount whose namespace is in allowedNamespaces.
type authorizer struct {
	subjects   map[string]struct{}
	namespaces map[string]struct{}
}

// newAuthorizer builds an authorizer from the exact-subject allowlist and the
// trusted-namespace allowlist, dropping empties.
func newAuthorizer(allowedSubjects, allowedNamespaces []string) authorizer {
	return authorizer{
		subjects:   subjectSet(allowedSubjects),
		namespaces: subjectSet(allowedNamespaces),
	}
}

// allowed reports whether subject is authorized: exact-subject match OR its
// ServiceAccount namespace is trusted.
func (a authorizer) allowed(subject string) bool {
	if _, ok := a.subjects[subject]; ok {
		return true
	}
	if len(a.namespaces) == 0 {
		return false
	}
	ns, _, ok := ParseServiceAccount(subject)
	if !ok {
		return false
	}
	_, ok = a.namespaces[ns]
	return ok
}

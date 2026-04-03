/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import "context"

// StaticConsentSource implements ConsentSource with a fixed list of grants.
// Used when consent grants are provided per-request (e.g. via HTTP header)
// instead of read from the database.
type StaticConsentSource struct {
	grants []ConsentCategory
}

// NewStaticConsentSource creates a StaticConsentSource from a list of category strings.
// Invalid categories are silently filtered out.
func NewStaticConsentSource(grants []string) *StaticConsentSource {
	cats := make([]ConsentCategory, 0, len(grants))
	for _, g := range grants {
		if _, valid := CategoryInfo(ConsentCategory(g)); valid {
			cats = append(cats, ConsentCategory(g))
		}
	}
	return &StaticConsentSource{grants: cats}
}

var _ ConsentSource = (*StaticConsentSource)(nil)

func (s *StaticConsentSource) GetConsentGrants(_ context.Context, _ string) ([]ConsentCategory, error) {
	return s.grants, nil
}

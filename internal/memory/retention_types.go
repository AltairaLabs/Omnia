/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package memory

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// retentionTiers is the iteration order the composite worker uses.
// UserForAgent rows collapse into the user tier for retention — the
// user context makes them the more sensitive policy target.
func retentionTiers() []Tier {
	return []Tier{TierInstitutional, TierAgent, TierUser}
}

// RetentionBranch names the pruning strategy a single pass applies.
type RetentionBranch string

const (
	BranchTTL              RetentionBranch = "ttl"
	BranchLRU              RetentionBranch = "lru"
	BranchDecay            RetentionBranch = "decay"
	BranchHardClean        RetentionBranch = "hard_clean"
	BranchConsentRevoke    RetentionBranch = "consent_revoke"
	BranchConsentHardClean RetentionBranch = "consent_hard_clean"
	BranchSupersession     RetentionBranch = "supersession"
)

// sqlPredicate returns the SQL where-clause fragment that isolates rows
// belonging to the tier. Uses the same virtual_user_id / agent_id
// nullability convention as retrieve_multi_tier.classifyTier.
func (t Tier) sqlPredicate() string {
	switch t {
	case TierInstitutional:
		return "virtual_user_id IS NULL AND agent_id IS NULL"
	case TierAgent:
		return "virtual_user_id IS NULL AND agent_id IS NOT NULL"
	case TierUser:
		// Includes both user-only and user-for-agent rows. The user
		// context makes these the more sensitive retention target.
		return "virtual_user_id IS NOT NULL"
	}
	return "FALSE"
}

// tierConfig returns the per-tier config from the cluster default,
// picking the matching field on the tier set. Nil means the tier is
// not configured and should be skipped (implicit Manual).
func tierConfig(policy *omniav1alpha1.MemoryRetentionPolicy, tier Tier) *omniav1alpha1.MemoryTierConfig {
	if policy == nil {
		return nil
	}
	tiers := policy.Spec.Default.Tiers
	switch tier {
	case TierInstitutional:
		return tiers.Institutional
	case TierAgent:
		return tiers.Agent
	case TierUser:
		return tiers.User
	}
	return nil
}

// branchesForMode expands a tier mode to the branches that should run.
// Composite runs TTL + LRU; Decay is recognised but not yet implemented
// in this phase — callers log a not-yet-supported warning.
func branchesForMode(mode omniav1alpha1.MemoryRetentionMode) []RetentionBranch {
	switch mode {
	case omniav1alpha1.MemoryRetentionModeManual, "":
		return nil
	case omniav1alpha1.MemoryRetentionModeTTL:
		return []RetentionBranch{BranchTTL}
	case omniav1alpha1.MemoryRetentionModeLRU:
		return []RetentionBranch{BranchLRU}
	case omniav1alpha1.MemoryRetentionModeDecay:
		return []RetentionBranch{BranchDecay}
	case omniav1alpha1.MemoryRetentionModeComposite:
		return []RetentionBranch{BranchTTL, BranchLRU, BranchDecay}
	}
	return nil
}

// parseRetentionDuration parses the same "d"-suffix syntax the CRD
// controller accepts. Mirrors parseExtendedDuration in the controller
// package so the memory-api doesn't need to import it.
func parseRetentionDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	expanded, err := expandDaySuffix(s)
	if err != nil {
		return 0, err
	}
	return time.ParseDuration(expanded)
}

// expandDaySuffix rewrites each "<N>d" segment as "<N*24>h" so
// time.ParseDuration can consume it. See MemoryTTLConfig docs.
func expandDaySuffix(s string) (string, error) {
	if !strings.Contains(s, "d") {
		return s, nil
	}
	var out strings.Builder
	var digits strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= '0' && c <= '9' {
			digits.WriteByte(c)
			continue
		}
		if c == 'd' {
			if digits.Len() == 0 {
				return "", fmt.Errorf("dangling 'd' suffix in %q", s)
			}
			days, err := strconv.Atoi(digits.String())
			if err != nil {
				return "", fmt.Errorf("invalid day count in %q: %w", s, err)
			}
			fmt.Fprintf(&out, "%dh", days*24)
			digits.Reset()
			continue
		}
		if digits.Len() > 0 {
			out.WriteString(digits.String())
			digits.Reset()
		}
		out.WriteByte(c)
	}
	if digits.Len() > 0 {
		out.WriteString(digits.String())
	}
	return out.String(), nil
}

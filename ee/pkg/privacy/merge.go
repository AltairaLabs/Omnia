/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// ComputeEffectivePolicy merges the inheritance chain, applying stricter rules
// at each level. The chain should be ordered from most general (global) to most
// specific (agent).
func ComputeEffectivePolicy(chain []*omniav1alpha1.SessionPrivacyPolicy) *omniav1alpha1.SessionPrivacyPolicySpec {
	if len(chain) == 0 {
		return &omniav1alpha1.SessionPrivacyPolicySpec{}
	}

	effective := chain[0].Spec.DeepCopy()
	for i := 1; i < len(chain); i++ {
		effective = MergeStricter(effective, &chain[i].Spec)
	}
	return effective
}

// MergeStricter merges an override policy into a base, applying the stricter
// of each setting. The result is a new spec; neither input is mutated.
func MergeStricter(
	base, override *omniav1alpha1.SessionPrivacyPolicySpec,
) *omniav1alpha1.SessionPrivacyPolicySpec {
	result := base.DeepCopy()

	// recording.enabled: false wins (can't enable if parent disables)
	result.Recording.Enabled = base.Recording.Enabled && override.Recording.Enabled
	// recording.facadeData: false wins
	result.Recording.FacadeData = base.Recording.FacadeData && override.Recording.FacadeData
	// recording.richData: false wins
	result.Recording.RichData = base.Recording.RichData && override.Recording.RichData

	result.Recording.PII = MergePII(base.Recording.PII, override.Recording.PII)
	result.UserOptOut = MergeUserOptOut(base.UserOptOut, override.UserOptOut)
	result.Retention = MergeRetention(base.Retention, override.Retention)
	result.Encryption = MergeEncryption(base.Encryption, override.Encryption)
	result.AuditLog = MergeAuditLog(base.AuditLog, override.AuditLog)

	return result
}

// MergePII merges PII configs with the stricter rule (true wins for redact/encrypt).
func MergePII(base, override *omniav1alpha1.PIIConfig) *omniav1alpha1.PIIConfig {
	if base == nil && override == nil {
		return nil
	}

	result := &omniav1alpha1.PIIConfig{
		Redact:  BoolFromEither(base, override, func(c *omniav1alpha1.PIIConfig) bool { return c.Redact }),
		Encrypt: BoolFromEither(base, override, func(c *omniav1alpha1.PIIConfig) bool { return c.Encrypt }),
	}
	result.Patterns = MergePatterns(base, override)

	// Strategy: child overrides parent; default to "replace"
	switch {
	case override != nil && override.Strategy != "":
		result.Strategy = override.Strategy
	case base != nil && base.Strategy != "":
		result.Strategy = base.Strategy
	default:
		result.Strategy = omniav1alpha1.RedactionStrategyReplace
	}

	return result
}

// BoolFromEither returns true if either config has the field set to true (true wins).
func BoolFromEither[T any](a, b *T, getter func(*T) bool) bool {
	return (a != nil && getter(a)) || (b != nil && getter(b))
}

// MergePatterns returns the union of PII patterns from both configs.
func MergePatterns(base, override *omniav1alpha1.PIIConfig) []string {
	seen := map[string]bool{}
	var result []string
	for _, cfg := range []*omniav1alpha1.PIIConfig{base, override} {
		if cfg == nil {
			continue
		}
		for _, p := range cfg.Patterns {
			if !seen[p] {
				result = append(result, p)
				seen[p] = true
			}
		}
	}
	return result
}

// MergeUserOptOut merges user opt-out configs with the stricter rule.
func MergeUserOptOut(base, override *omniav1alpha1.UserOptOutConfig) *omniav1alpha1.UserOptOutConfig {
	if base == nil && override == nil {
		return nil
	}

	enabledGetter := func(c *omniav1alpha1.UserOptOutConfig) bool { return c.Enabled }
	deleteGetter := func(c *omniav1alpha1.UserOptOutConfig) bool { return c.HonorDeleteRequests }
	return &omniav1alpha1.UserOptOutConfig{
		Enabled:             BoolFromEither(base, override, enabledGetter),
		HonorDeleteRequests: BoolFromEither(base, override, deleteGetter),
		DeleteWithinDays: MinInt32Ptr(
			GetOptionalInt32(base, func(c *omniav1alpha1.UserOptOutConfig) *int32 { return c.DeleteWithinDays }),
			GetOptionalInt32(override, func(c *omniav1alpha1.UserOptOutConfig) *int32 { return c.DeleteWithinDays }),
		),
	}
}

// MergeRetention merges retention configs taking the minimum of each field.
func MergeRetention(
	base, override *omniav1alpha1.PrivacyRetentionConfig,
) *omniav1alpha1.PrivacyRetentionConfig {
	if base == nil && override == nil {
		return nil
	}
	if base == nil {
		return override.DeepCopy()
	}
	if override == nil {
		return base.DeepCopy()
	}

	return &omniav1alpha1.PrivacyRetentionConfig{
		Facade:   MergeRetentionTier(base.Facade, override.Facade),
		RichData: MergeRetentionTier(base.RichData, override.RichData),
	}
}

// MergeRetentionTier merges a retention tier taking the minimum.
func MergeRetentionTier(
	base, override *omniav1alpha1.PrivacyRetentionTierConfig,
) *omniav1alpha1.PrivacyRetentionTierConfig {
	if base == nil && override == nil {
		return nil
	}
	if base == nil {
		return override.DeepCopy()
	}
	if override == nil {
		return base.DeepCopy()
	}

	return &omniav1alpha1.PrivacyRetentionTierConfig{
		WarmDays: MinInt32Ptr(base.WarmDays, override.WarmDays),
		ColdDays: MinInt32Ptr(base.ColdDays, override.ColdDays),
	}
}

// MergeEncryption merges encryption configs; true wins.
func MergeEncryption(base, override *omniav1alpha1.EncryptionConfig) *omniav1alpha1.EncryptionConfig {
	if base == nil && override == nil {
		return nil
	}

	result := &omniav1alpha1.EncryptionConfig{
		Enabled: BoolFromEither(base, override, func(c *omniav1alpha1.EncryptionConfig) bool { return c.Enabled }),
	}

	// Use the most specific non-empty KMS provider (override takes precedence if set)
	if override != nil && override.KMSProvider != "" {
		result.KMSProvider = override.KMSProvider
		result.SecretRef = override.SecretRef
		result.KeyID = override.KeyID
	} else if base != nil {
		result.KMSProvider = base.KMSProvider
		result.SecretRef = base.SecretRef
		result.KeyID = base.KeyID
	}

	// Allow KeyID to be overridden independently (same provider, different key per workspace/agent)
	if override != nil && override.KeyID != "" && override.KMSProvider == "" {
		result.KeyID = override.KeyID
	}

	rotGetter := func(c *omniav1alpha1.EncryptionConfig) *omniav1alpha1.KeyRotationConfig {
		return c.KeyRotation
	}
	result.KeyRotation = MergeKeyRotation(
		GetOptionalField(base, rotGetter),
		GetOptionalField(override, rotGetter),
	)

	return result
}

// GetOptionalField safely extracts a field from a possibly-nil struct.
func GetOptionalField[T any, F any](obj *T, getter func(*T) *F) *F {
	if obj == nil {
		return nil
	}
	return getter(obj)
}

// MergeKeyRotation merges key rotation configs; child overrides parent for non-bool fields.
func MergeKeyRotation(base, override *omniav1alpha1.KeyRotationConfig) *omniav1alpha1.KeyRotationConfig {
	if base == nil && override == nil {
		return nil
	}

	result := &omniav1alpha1.KeyRotationConfig{
		Enabled: BoolFromEither(base, override, func(c *omniav1alpha1.KeyRotationConfig) bool { return c.Enabled }),
		ReEncryptExisting: BoolFromEither(base, override,
			func(c *omniav1alpha1.KeyRotationConfig) bool { return c.ReEncryptExisting }),
	}

	// Schedule: child overrides parent.
	switch {
	case override != nil && override.Schedule != "":
		result.Schedule = override.Schedule
	case base != nil:
		result.Schedule = base.Schedule
	}

	// BatchSize: use the smaller value.
	result.BatchSize = MinInt32Ptr(
		GetOptionalInt32(base, func(c *omniav1alpha1.KeyRotationConfig) *int32 { return c.BatchSize }),
		GetOptionalInt32(override, func(c *omniav1alpha1.KeyRotationConfig) *int32 { return c.BatchSize }),
	)

	return result
}

// MergeAuditLog merges audit log configs; true wins.
func MergeAuditLog(base, override *omniav1alpha1.AuditLogConfig) *omniav1alpha1.AuditLogConfig {
	if base == nil && override == nil {
		return nil
	}

	return &omniav1alpha1.AuditLogConfig{
		Enabled: BoolFromEither(base, override, func(c *omniav1alpha1.AuditLogConfig) bool { return c.Enabled }),
		RetentionDays: MinInt32Ptr(
			GetOptionalInt32(base, func(c *omniav1alpha1.AuditLogConfig) *int32 { return c.RetentionDays }),
			GetOptionalInt32(override, func(c *omniav1alpha1.AuditLogConfig) *int32 { return c.RetentionDays }),
		),
	}
}

// GetOptionalInt32 safely extracts an *int32 field from a possibly-nil struct.
func GetOptionalInt32[T any](obj *T, getter func(*T) *int32) *int32 {
	if obj == nil {
		return nil
	}
	return getter(obj)
}

// MinInt32Ptr returns the minimum of two *int32 values (nil means unset).
func MinInt32Ptr(a, b *int32) *int32 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if *a < *b {
		return a
	}
	return b
}

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func TestComputeEffectivePolicy_EmptyChain(t *testing.T) {
	result := ComputeEffectivePolicy(nil)
	require.NotNil(t, result)
}

func TestComputeEffectivePolicy_SinglePolicy(t *testing.T) {
	p := &omniav1alpha1.SessionPrivacyPolicy{
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Recording: omniav1alpha1.RecordingConfig{
				Enabled: true,
				PII: &omniav1alpha1.PIIConfig{
					Redact:   true,
					Patterns: []string{"ssn"},
				},
			},
		},
	}
	result := ComputeEffectivePolicy([]*omniav1alpha1.SessionPrivacyPolicy{p})
	assert.True(t, result.Recording.Enabled)
	assert.True(t, result.Recording.PII.Redact)
	assert.Equal(t, []string{"ssn"}, result.Recording.PII.Patterns)
}

func TestComputeEffectivePolicy_RecordingFalseWins(t *testing.T) {
	global := &omniav1alpha1.SessionPrivacyPolicy{
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Recording: omniav1alpha1.RecordingConfig{Enabled: false},
		},
	}
	ws := &omniav1alpha1.SessionPrivacyPolicy{
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Recording: omniav1alpha1.RecordingConfig{Enabled: true},
		},
	}
	result := ComputeEffectivePolicy([]*omniav1alpha1.SessionPrivacyPolicy{global, ws})
	assert.False(t, result.Recording.Enabled)
}

func TestMergeStricter_PIITrueWins(t *testing.T) {
	base := &omniav1alpha1.SessionPrivacyPolicySpec{
		Recording: omniav1alpha1.RecordingConfig{
			PII: &omniav1alpha1.PIIConfig{Redact: false},
		},
	}
	override := &omniav1alpha1.SessionPrivacyPolicySpec{
		Recording: omniav1alpha1.RecordingConfig{
			PII: &omniav1alpha1.PIIConfig{Redact: true},
		},
	}
	result := MergeStricter(base, override)
	assert.True(t, result.Recording.PII.Redact)
}

func TestMergePII_NilBoth(t *testing.T) {
	assert.Nil(t, MergePII(nil, nil))
}

func TestMergePII_UnionPatterns(t *testing.T) {
	base := &omniav1alpha1.PIIConfig{Patterns: []string{"ssn", "email"}}
	override := &omniav1alpha1.PIIConfig{Patterns: []string{"email", "phone"}}
	result := MergePII(base, override)
	assert.ElementsMatch(t, []string{"ssn", "email", "phone"}, result.Patterns)
}

func TestMergePII_StrategyOverride(t *testing.T) {
	base := &omniav1alpha1.PIIConfig{Strategy: omniav1alpha1.RedactionStrategyReplace}
	override := &omniav1alpha1.PIIConfig{Strategy: omniav1alpha1.RedactionStrategyHash}
	result := MergePII(base, override)
	assert.Equal(t, omniav1alpha1.RedactionStrategyHash, result.Strategy)
}

func TestMergePII_StrategyDefault(t *testing.T) {
	result := MergePII(&omniav1alpha1.PIIConfig{}, &omniav1alpha1.PIIConfig{})
	assert.Equal(t, omniav1alpha1.RedactionStrategyReplace, result.Strategy)
}

func TestMergeUserOptOut_NilBoth(t *testing.T) {
	assert.Nil(t, MergeUserOptOut(nil, nil))
}

func TestMergeUserOptOut_EnabledTrueWins(t *testing.T) {
	base := &omniav1alpha1.UserOptOutConfig{Enabled: false}
	override := &omniav1alpha1.UserOptOutConfig{Enabled: true}
	result := MergeUserOptOut(base, override)
	assert.True(t, result.Enabled)
}

func TestMergeUserOptOut_MinDeleteWithinDays(t *testing.T) {
	base := &omniav1alpha1.UserOptOutConfig{DeleteWithinDays: ptr.To(int32(30))}
	override := &omniav1alpha1.UserOptOutConfig{DeleteWithinDays: ptr.To(int32(14))}
	result := MergeUserOptOut(base, override)
	require.NotNil(t, result.DeleteWithinDays)
	assert.Equal(t, int32(14), *result.DeleteWithinDays)
}

func TestMergeRetention_NilBoth(t *testing.T) {
	assert.Nil(t, MergeRetention(nil, nil))
}

func TestMergeRetention_NilBase(t *testing.T) {
	override := &omniav1alpha1.PrivacyRetentionConfig{
		Facade: &omniav1alpha1.PrivacyRetentionTierConfig{WarmDays: ptr.To(int32(30))},
	}
	result := MergeRetention(nil, override)
	require.NotNil(t, result)
	assert.Equal(t, int32(30), *result.Facade.WarmDays)
}

func TestMergeRetention_MinWins(t *testing.T) {
	base := &omniav1alpha1.PrivacyRetentionConfig{
		Facade: &omniav1alpha1.PrivacyRetentionTierConfig{WarmDays: ptr.To(int32(90))},
	}
	override := &omniav1alpha1.PrivacyRetentionConfig{
		Facade: &omniav1alpha1.PrivacyRetentionTierConfig{WarmDays: ptr.To(int32(30))},
	}
	result := MergeRetention(base, override)
	assert.Equal(t, int32(30), *result.Facade.WarmDays)
}

func TestMergeEncryption_NilBoth(t *testing.T) {
	assert.Nil(t, MergeEncryption(nil, nil))
}

func TestMergeEncryption_TrueWins(t *testing.T) {
	base := &omniav1alpha1.EncryptionConfig{Enabled: true, KMSProvider: omniav1alpha1.KMSProviderAWSKMS}
	override := &omniav1alpha1.EncryptionConfig{Enabled: false}
	result := MergeEncryption(base, override)
	assert.True(t, result.Enabled)
	assert.Equal(t, omniav1alpha1.KMSProviderAWSKMS, result.KMSProvider)
}

func TestMergeEncryption_OverrideKMS(t *testing.T) {
	base := &omniav1alpha1.EncryptionConfig{Enabled: true, KMSProvider: omniav1alpha1.KMSProviderAWSKMS}
	override := &omniav1alpha1.EncryptionConfig{KMSProvider: omniav1alpha1.KMSProviderVault}
	result := MergeEncryption(base, override)
	assert.Equal(t, omniav1alpha1.KMSProviderVault, result.KMSProvider)
}

func TestMergeEncryption_IndependentKeyIDOverride(t *testing.T) {
	base := &omniav1alpha1.EncryptionConfig{
		Enabled:     true,
		KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
		KeyID:       "key-1",
	}
	override := &omniav1alpha1.EncryptionConfig{
		KeyID: "key-2", // Override key without overriding provider
	}
	result := MergeEncryption(base, override)
	assert.Equal(t, omniav1alpha1.KMSProviderAWSKMS, result.KMSProvider)
	assert.Equal(t, "key-2", result.KeyID)
}

func TestMergeAuditLog_NilBoth(t *testing.T) {
	assert.Nil(t, MergeAuditLog(nil, nil))
}

func TestMergeAuditLog_TrueWins(t *testing.T) {
	base := &omniav1alpha1.AuditLogConfig{Enabled: true, RetentionDays: ptr.To(int32(365))}
	override := &omniav1alpha1.AuditLogConfig{Enabled: false, RetentionDays: ptr.To(int32(90))}
	result := MergeAuditLog(base, override)
	assert.True(t, result.Enabled)
	assert.Equal(t, int32(90), *result.RetentionDays)
}

func TestMinInt32Ptr(t *testing.T) {
	assert.Nil(t, MinInt32Ptr(nil, nil))
	assert.Equal(t, int32(5), *MinInt32Ptr(ptr.To(int32(5)), nil))
	assert.Equal(t, int32(5), *MinInt32Ptr(nil, ptr.To(int32(5))))
	assert.Equal(t, int32(3), *MinInt32Ptr(ptr.To(int32(5)), ptr.To(int32(3))))
	assert.Equal(t, int32(3), *MinInt32Ptr(ptr.To(int32(3)), ptr.To(int32(5))))
}

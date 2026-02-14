/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package compliance

import (
	"fmt"

	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

// PresetName represents a supported compliance preset identifier.
type PresetName string

const (
	// PresetGDPR is the EU General Data Protection Regulation preset.
	PresetGDPR PresetName = "gdpr"
	// PresetHIPAA is the US Health Insurance Portability and Accountability Act preset.
	PresetHIPAA PresetName = "hipaa"
	// PresetCCPA is the California Consumer Privacy Act preset.
	PresetCCPA PresetName = "ccpa"
)

// validPresets is the set of all supported preset names.
var validPresets = map[PresetName]bool{
	PresetGDPR:  true,
	PresetHIPAA: true,
	PresetCCPA:  true,
}

// int32Ptr returns a pointer to the given int32 value.
func int32Ptr(v int32) *int32 {
	return &v
}

// GetPreset returns the SessionPrivacyPolicySpec for a named compliance preset.
// Returns an error if the preset name is not recognized.
func GetPreset(name string) (*eev1alpha1.SessionPrivacyPolicySpec, error) {
	preset := PresetName(name)
	if !validPresets[preset] {
		return nil, fmt.Errorf("unknown compliance preset: %q", name)
	}

	switch preset {
	case PresetGDPR:
		return gdprPreset(), nil
	case PresetHIPAA:
		return hipaaPreset(), nil
	case PresetCCPA:
		return ccpaPreset(), nil
	default:
		return nil, fmt.Errorf("unknown compliance preset: %q", name)
	}
}

// ListPresets returns all supported preset names.
func ListPresets() []PresetName {
	return []PresetName{PresetGDPR, PresetHIPAA, PresetCCPA}
}

func gdprPreset() *eev1alpha1.SessionPrivacyPolicySpec {
	return &eev1alpha1.SessionPrivacyPolicySpec{
		Level: eev1alpha1.PolicyLevelWorkspace,
		Recording: eev1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   true,
			PII: &eev1alpha1.PIIConfig{
				Redact:   true,
				Encrypt:  true,
				Patterns: gdprPIIPatterns(),
				Strategy: eev1alpha1.RedactionStrategyReplace,
			},
		},
		Retention: &eev1alpha1.PrivacyRetentionConfig{
			Facade: &eev1alpha1.PrivacyRetentionTierConfig{
				WarmDays: int32Ptr(30),
				ColdDays: int32Ptr(90),
			},
		},
		UserOptOut: &eev1alpha1.UserOptOutConfig{
			Enabled:             true,
			HonorDeleteRequests: true,
			DeleteWithinDays:    int32Ptr(30),
		},
		AuditLog: &eev1alpha1.AuditLogConfig{
			Enabled:       true,
			RetentionDays: int32Ptr(365),
		},
	}
}

func hipaaPreset() *eev1alpha1.SessionPrivacyPolicySpec {
	return &eev1alpha1.SessionPrivacyPolicySpec{
		Level: eev1alpha1.PolicyLevelWorkspace,
		Recording: eev1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   true,
			PII: &eev1alpha1.PIIConfig{
				Redact:   true,
				Encrypt:  true,
				Patterns: hipaaPIIPatterns(),
				Strategy: eev1alpha1.RedactionStrategyReplace,
			},
		},
		Retention: &eev1alpha1.PrivacyRetentionConfig{
			Facade: &eev1alpha1.PrivacyRetentionTierConfig{
				WarmDays: int32Ptr(30),
				ColdDays: int32Ptr(2555),
			},
		},
		Encryption: &eev1alpha1.EncryptionConfig{
			Enabled: true,
		},
		UserOptOut: &eev1alpha1.UserOptOutConfig{
			Enabled:             true,
			HonorDeleteRequests: true,
			DeleteWithinDays:    int32Ptr(30),
		},
		AuditLog: &eev1alpha1.AuditLogConfig{
			Enabled:       true,
			RetentionDays: int32Ptr(2555),
		},
	}
}

func ccpaPreset() *eev1alpha1.SessionPrivacyPolicySpec {
	return &eev1alpha1.SessionPrivacyPolicySpec{
		Level: eev1alpha1.PolicyLevelWorkspace,
		Recording: eev1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   false,
			PII: &eev1alpha1.PIIConfig{
				Redact:   true,
				Encrypt:  false,
				Patterns: ccpaPIIPatterns(),
				Strategy: eev1alpha1.RedactionStrategyMask,
			},
		},
		Retention: &eev1alpha1.PrivacyRetentionConfig{
			Facade: &eev1alpha1.PrivacyRetentionTierConfig{
				WarmDays: int32Ptr(30),
				ColdDays: int32Ptr(30),
			},
		},
		UserOptOut: &eev1alpha1.UserOptOutConfig{
			Enabled:             true,
			HonorDeleteRequests: true,
			DeleteWithinDays:    int32Ptr(45),
		},
		AuditLog: &eev1alpha1.AuditLogConfig{
			Enabled:       true,
			RetentionDays: int32Ptr(365),
		},
	}
}

func gdprPIIPatterns() []string {
	return []string{
		"ssn",
		"credit_card",
		"phone_number",
		"email",
		"ip_address",
	}
}

func hipaaPIIPatterns() []string {
	return []string{
		"ssn",
		"credit_card",
		"phone_number",
		"email",
		"ip_address",
		"custom:^[A-Z]{2,4}-?\\d{6,10}$",
	}
}

func ccpaPIIPatterns() []string {
	return []string{
		"ssn",
		"credit_card",
		"email",
	}
}

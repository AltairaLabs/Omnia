/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package compliance

import (
	"testing"

	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func TestGetPreset_GDPR(t *testing.T) {
	spec, err := GetPreset("gdpr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertRecordingEnabled(t, spec)
	assertPIIRedactionEnabled(t, spec)
	assertPIIEncryptionEnabled(t, spec)
	assertPIIStrategy(t, spec, eev1alpha1.RedactionStrategyReplace)
	assertUserOptOutEnabled(t, spec)
	assertHonorDeleteRequests(t, spec)
	assertDeleteWithinDays(t, spec, 30)
	assertAuditLogEnabled(t, spec)
	assertAuditLogRetentionDays(t, spec, 365)
	assertRetentionWarmDays(t, spec, 30)
	assertRetentionColdDays(t, spec, 90)

	assertPIIPatternsContain(t, spec, "ssn", "credit_card", "phone_number", "email", "ip_address")
}

func TestGetPreset_HIPAA(t *testing.T) {
	spec, err := GetPreset("hipaa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertRecordingEnabled(t, spec)
	assertPIIRedactionEnabled(t, spec)
	assertPIIEncryptionEnabled(t, spec)
	assertPIIStrategy(t, spec, eev1alpha1.RedactionStrategyReplace)
	assertUserOptOutEnabled(t, spec)
	assertHonorDeleteRequests(t, spec)
	assertDeleteWithinDays(t, spec, 30)
	assertAuditLogEnabled(t, spec)
	assertAuditLogRetentionDays(t, spec, 2555)
	assertRetentionColdDays(t, spec, 2555)

	if spec.Encryption == nil || !spec.Encryption.Enabled {
		t.Error("HIPAA preset must have encryption enabled")
	}

	assertPIIPatternsContain(t, spec, "ssn", "credit_card", "phone_number", "email", "ip_address")

	// HIPAA must include medical record number pattern
	found := false
	for _, p := range spec.Recording.PII.Patterns {
		if p == "custom:^[A-Z]{2,4}-?\\d{6,10}$" {
			found = true
			break
		}
	}
	if !found {
		t.Error("HIPAA preset must include medical record number custom pattern")
	}
}

func TestGetPreset_CCPA(t *testing.T) {
	spec, err := GetPreset("ccpa")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertRecordingEnabled(t, spec)
	assertPIIRedactionEnabled(t, spec)
	assertPIIStrategy(t, spec, eev1alpha1.RedactionStrategyMask)
	assertUserOptOutEnabled(t, spec)
	assertHonorDeleteRequests(t, spec)
	assertDeleteWithinDays(t, spec, 45)
	assertAuditLogEnabled(t, spec)
	assertRetentionWarmDays(t, spec, 30)
	assertRetentionColdDays(t, spec, 30)

	if spec.Recording.RichData {
		t.Error("CCPA preset should not enable rich data recording")
	}

	if spec.Recording.PII.Encrypt {
		t.Error("CCPA preset should not require PII encryption")
	}
}

func TestGetPreset_Unknown(t *testing.T) {
	_, err := GetPreset("unknown")
	if err == nil {
		t.Fatal("expected error for unknown preset")
	}
}

func TestGetPreset_EmptyName(t *testing.T) {
	_, err := GetPreset("")
	if err == nil {
		t.Fatal("expected error for empty preset name")
	}
}

func TestListPresets(t *testing.T) {
	presets := ListPresets()
	if len(presets) != 3 {
		t.Fatalf("expected 3 presets, got %d", len(presets))
	}

	expected := map[PresetName]bool{
		PresetGDPR:  true,
		PresetHIPAA: true,
		PresetCCPA:  true,
	}
	for _, p := range presets {
		if !expected[p] {
			t.Errorf("unexpected preset: %s", p)
		}
	}
}

func TestAllPresetsHaveWorkspaceLevel(t *testing.T) {
	for _, name := range ListPresets() {
		spec, err := GetPreset(string(name))
		if err != nil {
			t.Fatalf("unexpected error for preset %s: %v", name, err)
		}
		if spec.Level != eev1alpha1.PolicyLevelWorkspace {
			t.Errorf("preset %s: expected workspace level, got %s", name, spec.Level)
		}
	}
}

func TestAllPresetsHaveAuditLogging(t *testing.T) {
	for _, name := range ListPresets() {
		spec, err := GetPreset(string(name))
		if err != nil {
			t.Fatalf("unexpected error for preset %s: %v", name, err)
		}
		assertAuditLogEnabled(t, spec)
	}
}

func TestAllPresetsHaveUserOptOut(t *testing.T) {
	for _, name := range ListPresets() {
		spec, err := GetPreset(string(name))
		if err != nil {
			t.Fatalf("unexpected error for preset %s: %v", name, err)
		}
		assertUserOptOutEnabled(t, spec)
		assertHonorDeleteRequests(t, spec)
	}
}

func TestAllPresetsHavePIIRedaction(t *testing.T) {
	for _, name := range ListPresets() {
		spec, err := GetPreset(string(name))
		if err != nil {
			t.Fatalf("unexpected error for preset %s: %v", name, err)
		}
		assertPIIRedactionEnabled(t, spec)
	}
}

func TestAllPresetsHaveRetention(t *testing.T) {
	for _, name := range ListPresets() {
		spec, err := GetPreset(string(name))
		if err != nil {
			t.Fatalf("unexpected error for preset %s: %v", name, err)
		}
		if spec.Retention == nil {
			t.Errorf("preset %s: retention must not be nil", name)
		}
		if spec.Retention.Facade == nil {
			t.Errorf("preset %s: facade retention must not be nil", name)
		}
	}
}

// --- assertion helpers ---

func assertRecordingEnabled(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec) {
	t.Helper()
	if !spec.Recording.Enabled {
		t.Error("recording must be enabled")
	}
}

func assertPIIRedactionEnabled(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec) {
	t.Helper()
	if spec.Recording.PII == nil {
		t.Fatal("PII config must not be nil")
	}
	if !spec.Recording.PII.Redact {
		t.Error("PII redaction must be enabled")
	}
}

func assertPIIEncryptionEnabled(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec) {
	t.Helper()
	if spec.Recording.PII == nil {
		t.Fatal("PII config must not be nil")
	}
	if !spec.Recording.PII.Encrypt {
		t.Error("PII encryption must be enabled")
	}
}

func assertPIIStrategy(
	t *testing.T,
	spec *eev1alpha1.SessionPrivacyPolicySpec,
	expected eev1alpha1.RedactionStrategy,
) {
	t.Helper()
	if spec.Recording.PII.Strategy != expected {
		t.Errorf("expected PII strategy %s, got %s", expected, spec.Recording.PII.Strategy)
	}
}

func assertUserOptOutEnabled(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec) {
	t.Helper()
	if spec.UserOptOut == nil {
		t.Fatal("userOptOut must not be nil")
	}
	if !spec.UserOptOut.Enabled {
		t.Error("userOptOut must be enabled")
	}
}

func assertHonorDeleteRequests(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec) {
	t.Helper()
	if spec.UserOptOut == nil {
		t.Fatal("userOptOut must not be nil")
	}
	if !spec.UserOptOut.HonorDeleteRequests {
		t.Error("honorDeleteRequests must be enabled")
	}
}

func assertDeleteWithinDays(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec, expected int32) {
	t.Helper()
	if spec.UserOptOut == nil || spec.UserOptOut.DeleteWithinDays == nil {
		t.Fatal("deleteWithinDays must not be nil")
	}
	if *spec.UserOptOut.DeleteWithinDays != expected {
		t.Errorf("expected deleteWithinDays %d, got %d", expected, *spec.UserOptOut.DeleteWithinDays)
	}
}

func assertAuditLogEnabled(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec) {
	t.Helper()
	if spec.AuditLog == nil {
		t.Fatal("auditLog must not be nil")
	}
	if !spec.AuditLog.Enabled {
		t.Error("auditLog must be enabled")
	}
}

func assertAuditLogRetentionDays(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec, expected int32) {
	t.Helper()
	if spec.AuditLog == nil || spec.AuditLog.RetentionDays == nil {
		t.Fatal("auditLog retentionDays must not be nil")
	}
	if *spec.AuditLog.RetentionDays != expected {
		t.Errorf("expected auditLog retentionDays %d, got %d", expected, *spec.AuditLog.RetentionDays)
	}
}

func assertRetentionWarmDays(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec, expected int32) {
	t.Helper()
	if spec.Retention == nil || spec.Retention.Facade == nil || spec.Retention.Facade.WarmDays == nil {
		t.Fatal("retention facade warmDays must not be nil")
	}
	if *spec.Retention.Facade.WarmDays != expected {
		t.Errorf("expected warmDays %d, got %d", expected, *spec.Retention.Facade.WarmDays)
	}
}

func assertRetentionColdDays(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec, expected int32) {
	t.Helper()
	if spec.Retention == nil || spec.Retention.Facade == nil || spec.Retention.Facade.ColdDays == nil {
		t.Fatal("retention facade coldDays must not be nil")
	}
	if *spec.Retention.Facade.ColdDays != expected {
		t.Errorf("expected coldDays %d, got %d", expected, *spec.Retention.Facade.ColdDays)
	}
}

func assertPIIPatternsContain(t *testing.T, spec *eev1alpha1.SessionPrivacyPolicySpec, patterns ...string) {
	t.Helper()
	if spec.Recording.PII == nil {
		t.Fatal("PII config must not be nil")
	}
	patternSet := make(map[string]bool, len(spec.Recording.PII.Patterns))
	for _, p := range spec.Recording.PII.Patterns {
		patternSet[p] = true
	}
	for _, expected := range patterns {
		if !patternSet[expected] {
			t.Errorf("expected PII pattern %q not found", expected)
		}
	}
}

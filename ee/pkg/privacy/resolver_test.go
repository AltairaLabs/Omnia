/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func TestFacadePolicyJSON_OnlyIncludesFacadeFields(t *testing.T) {
	eff := &EffectivePolicy{
		Recording: omniav1alpha1.RecordingConfig{
			Enabled:    true,
			FacadeData: true,
			RichData:   false,
		},
		Encryption: omniav1alpha1.EncryptionConfig{
			Enabled: true,
			KeyID:   "secret-key-id", // must NOT leak to facade
		},
	}

	raw, err := facadePolicyJSON(eff)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	recording, ok := decoded["recording"].(map[string]any)
	require.True(t, ok, "recording field missing")
	assert.Equal(t, true, recording["enabled"])
	// richData is false/omitempty — it must not appear in the JSON
	_, hasRichData := recording["richData"]
	assert.False(t, hasRichData, "richData=false should be omitted (omitempty)")

	_, hasEncryption := decoded["encryption"]
	assert.False(t, hasEncryption, "encryption config must not leak to facade")
}

func TestFacadePolicyJSON_NilPolicy(t *testing.T) {
	raw, err := facadePolicyJSON(nil)
	require.NoError(t, err)
	assert.Nil(t, raw)
}

func TestResolveEffectivePolicy_NilWhenNoPolicy(t *testing.T) {
	w := &PolicyWatcher{}
	raw, ok := w.ResolveEffectivePolicy("default", "my-agent")
	assert.False(t, ok)
	assert.Nil(t, raw)
}

func TestResolveEffectivePolicy_ReturnsJSON(t *testing.T) {
	w := &PolicyWatcher{log: logr.Discard()}
	policy := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "omnia-system"},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:    true,
				FacadeData: true,
			},
			Encryption: &omniav1alpha1.EncryptionConfig{
				Enabled:     true,
				KMSProvider: omniav1alpha1.KMSProviderAWSKMS,
				KeyID:       "arn:aws:kms:us-east-1:123:key/test",
			},
		},
	}
	w.policies.Store("omnia-system/default", policy)

	raw, ok := w.ResolveEffectivePolicy("default", "my-agent")
	require.True(t, ok)
	require.NotNil(t, raw)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	_, hasRecording := decoded["recording"]
	assert.True(t, hasRecording, "recording field must be present")

	_, hasEncryption := decoded["encryption"]
	assert.False(t, hasEncryption, "encryption config must not leak to facade")

	rawStr := string(raw)
	assert.NotContains(t, rawStr, "keyID", "keyID must not appear in facade response")
}

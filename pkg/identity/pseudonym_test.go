package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPseudonymizeID_Deterministic(t *testing.T) {
	a := PseudonymizeID("user@example.com")
	b := PseudonymizeID("user@example.com")
	assert.Equal(t, a, b, "same input should produce same output")
}

func TestPseudonymizeID_DifferentInputs(t *testing.T) {
	a := PseudonymizeID("alice")
	b := PseudonymizeID("bob")
	assert.NotEqual(t, a, b, "different inputs should produce different outputs")
}

func TestPseudonymizeID_Length(t *testing.T) {
	result := PseudonymizeID("test-user")
	assert.Len(t, result, pseudonymLength, "should be %d hex chars", pseudonymLength)
}

func TestPseudonymizeID_HexOnly(t *testing.T) {
	result := PseudonymizeID("test-user")
	for _, c := range result {
		assert.True(t, (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'),
			"should only contain hex chars, got %c", c)
	}
}

func TestPseudonymizeID_EmptyInput(t *testing.T) {
	assert.Equal(t, "", PseudonymizeID(""), "empty input should return empty string")
}

func TestPseudonymizeID_NoPIILeakage(t *testing.T) {
	result := PseudonymizeID("user@example.com")
	assert.NotContains(t, result, "user")
	assert.NotContains(t, result, "example")
	assert.NotContains(t, result, "@")
}

func TestPseudonymizeID_UsesHMACWhenKeyConfigured(t *testing.T) {
	t.Setenv(pseudonymHMACKeyEnv, "test-secret-key")

	result := PseudonymizeID("test-user")

	h := hmac.New(sha256.New, []byte("test-secret-key"))
	_, _ = h.Write([]byte("test-user"))
	want := hex.EncodeToString(h.Sum(nil))[:pseudonymLength]

	assert.Equal(t, want, result)
	assert.NotEqual(t, "f85ac825d102b9f2", result, "HMAC output should differ from legacy plain SHA-256")
}

func TestPseudonymizeID_FallbacksToLegacySHA256WhenKeyUnset(t *testing.T) {
	t.Setenv(pseudonymHMACKeyEnv, "")
	assert.Equal(t, "f85ac825d102b9f2", PseudonymizeID("test-user"))
}

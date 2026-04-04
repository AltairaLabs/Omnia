package identity

import (
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

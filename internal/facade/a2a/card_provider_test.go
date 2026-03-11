/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestCRDCardProvider_BasicCard(t *testing.T) {
	spec := &omniav1alpha1.AgentCardSpec{
		Name:        "Test Agent",
		Description: "A test agent",
		Version:     "1.0.0",
	}

	provider := NewCRDCardProvider(spec, "http://test-agent.default.svc.cluster.local:9999")
	req := httptest.NewRequest("GET", "/.well-known/agent.json", nil)

	card, err := provider.AgentCard(req)
	require.NoError(t, err)

	assert.Equal(t, "Test Agent", card.Name)
	assert.Equal(t, "A test agent", card.Description)
	assert.Equal(t, "1.0.0", card.Version)
	assert.Equal(t, []string{"text"}, card.DefaultInputModes)
	assert.Equal(t, []string{"text"}, card.DefaultOutputModes)
	require.Len(t, card.SupportedInterfaces, 1)
	assert.Equal(t, "http://test-agent.default.svc.cluster.local:9999/a2a", card.SupportedInterfaces[0].URL)
	assert.Equal(t, "jsonrpc+http", card.SupportedInterfaces[0].ProtocolBinding)
}

func TestCRDCardProvider_WithSkills(t *testing.T) {
	spec := &omniav1alpha1.AgentCardSpec{
		Name: "Skilled Agent",
		Skills: []omniav1alpha1.AgentSkillSpec{
			{
				ID:          "qa",
				Name:        "Question Answering",
				Description: "Answers questions",
				Tags:        []string{"support", "qa"},
				Examples:    []string{"What is your return policy?"},
			},
		},
	}

	provider := NewCRDCardProvider(spec, "")
	req := httptest.NewRequest("GET", "/.well-known/agent.json", nil)

	card, err := provider.AgentCard(req)
	require.NoError(t, err)

	require.Len(t, card.Skills, 1)
	assert.Equal(t, "qa", card.Skills[0].ID)
	assert.Equal(t, "Question Answering", card.Skills[0].Name)
	assert.Equal(t, []string{"support", "qa"}, card.Skills[0].Tags)
}

func TestCRDCardProvider_WithCapabilities(t *testing.T) {
	spec := &omniav1alpha1.AgentCardSpec{
		Name: "Streaming Agent",
		Capabilities: &omniav1alpha1.AgentCapabilitiesSpec{
			Streaming:         true,
			PushNotifications: false,
		},
	}

	provider := NewCRDCardProvider(spec, "")
	req := httptest.NewRequest("GET", "/.well-known/agent.json", nil)

	card, err := provider.AgentCard(req)
	require.NoError(t, err)

	assert.True(t, card.Capabilities.Streaming)
	assert.False(t, card.Capabilities.PushNotifications)
}

func TestCRDCardProvider_WithOrganization(t *testing.T) {
	spec := &omniav1alpha1.AgentCardSpec{
		Name:         "Org Agent",
		Organization: "Altaira Labs",
	}

	provider := NewCRDCardProvider(spec, "")
	req := httptest.NewRequest("GET", "/.well-known/agent.json", nil)

	card, err := provider.AgentCard(req)
	require.NoError(t, err)

	require.NotNil(t, card.Provider)
	assert.Equal(t, "Altaira Labs", card.Provider.Organization)
}

func TestCRDCardProvider_CustomModes(t *testing.T) {
	spec := &omniav1alpha1.AgentCardSpec{
		Name:               "Multi-modal Agent",
		DefaultInputModes:  []string{"text", "audio"},
		DefaultOutputModes: []string{"text", "image"},
	}

	provider := NewCRDCardProvider(spec, "")
	req := httptest.NewRequest("GET", "/.well-known/agent.json", nil)

	card, err := provider.AgentCard(req)
	require.NoError(t, err)

	assert.Equal(t, []string{"text", "audio"}, card.DefaultInputModes)
	assert.Equal(t, []string{"text", "image"}, card.DefaultOutputModes)
}

func TestCRDCardProvider_NoServiceEndpoint(t *testing.T) {
	spec := &omniav1alpha1.AgentCardSpec{
		Name: "No Endpoint Agent",
	}

	provider := NewCRDCardProvider(spec, "")
	req := httptest.NewRequest("GET", "/.well-known/agent.json", nil)

	card, err := provider.AgentCard(req)
	require.NoError(t, err)

	assert.Empty(t, card.SupportedInterfaces)
}

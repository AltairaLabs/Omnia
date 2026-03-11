/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseResolvedClients_Empty(t *testing.T) {
	clients, err := ParseResolvedClients("")
	require.NoError(t, err)
	assert.Nil(t, clients)
}

func TestParseResolvedClients_Valid(t *testing.T) {
	input := `[{"name":"agent-a","url":"http://agent-a:8080","exposeAsTools":true},{"name":"agent-b","url":"http://external.com"}]`

	clients, err := ParseResolvedClients(input)
	require.NoError(t, err)
	require.Len(t, clients, 2)

	assert.Equal(t, "agent-a", clients[0].Name)
	assert.Equal(t, "http://agent-a:8080", clients[0].URL)
	assert.True(t, clients[0].ExposeAsTools)

	assert.Equal(t, "agent-b", clients[1].Name)
	assert.False(t, clients[1].ExposeAsTools)
}

func TestParseResolvedClients_InvalidJSON(t *testing.T) {
	_, err := ParseResolvedClients("not-json")
	assert.Error(t, err)
}

func TestBuildA2AAgentOptions_NoToolClients(t *testing.T) {
	clients := []ResolvedClient{
		{Name: "agent-a", URL: "http://agent-a:8080", ExposeAsTools: false},
	}

	opts := BuildA2AAgentOptions(context.Background(), clients, logr.Discard())
	assert.Len(t, opts, 0)
}

func TestBuildA2AAgentOptions_WithToolClients(t *testing.T) {
	clients := []ResolvedClient{
		{Name: "agent-a", URL: "http://agent-a:8080", ExposeAsTools: true},
		{Name: "agent-b", URL: "http://agent-b:8080", ExposeAsTools: false},
		{Name: "agent-c", URL: "http://agent-c:8080", ExposeAsTools: true},
	}

	opts := BuildA2AAgentOptions(context.Background(), clients, logr.Discard())
	// Should get 2 options (agent-a and agent-c have exposeAsTools)
	assert.Len(t, opts, 2)
}

func TestBuildA2AAgentOptions_EmptyClients(t *testing.T) {
	opts := BuildA2AAgentOptions(context.Background(), nil, logr.Discard())
	assert.Nil(t, opts)
}

func TestBuildA2AAgentOptions_WithAuthTokenEnv(t *testing.T) {
	// Set a test env var.
	t.Setenv("OMNIA_A2A_CLIENT_TOKEN_AGENT_A", "secret-token")

	clients := []ResolvedClient{
		{
			Name:          "agent-a",
			URL:           "http://agent-a:8080",
			ExposeAsTools: true,
			AuthTokenEnv:  "OMNIA_A2A_CLIENT_TOKEN_AGENT_A",
		},
	}

	opts := BuildA2AAgentOptions(context.Background(), clients, logr.Discard())
	// Should get 1 option with auth configured.
	assert.Len(t, opts, 1)
}

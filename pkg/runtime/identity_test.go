package runtime

import (
	"context"
	"testing"

	"github.com/altairalabs/omnia/pkg/policy"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/metadata"
)

func TestParseIdentity_FullMetadata(t *testing.T) {
	md := metadata.New(map[string]string{
		policy.HeaderAgentName:            "assistant",
		policy.HeaderNamespace:            "team-a",
		policy.HeaderSessionID:            "sess-1",
		policy.HeaderRequestID:            "req-1",
		policy.HeaderUserID:               "user-1",
		policy.HeaderUserEmail:            "u@example.com",
		policy.HeaderProvider:             "claude",
		policy.HeaderModel:                "claude-opus-4-8",
		policy.HeaderOrigin:               policy.OriginClientKey,
		policy.HeaderWorkspace:            "ws-1",
		policy.HeaderConsentGrants:        "location,filesystem",
		policy.HeaderConsentLayer:         "session",
		policy.HeaderClaimPrefix + "role": "editor",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	id := parseIdentity(ctx)

	assert.Equal(t, "assistant", id.AgentName)
	assert.Equal(t, "team-a", id.Namespace)
	assert.Equal(t, "sess-1", id.SessionID)
	assert.Equal(t, "req-1", id.RequestID)
	assert.Equal(t, "user-1", id.UserID)
	assert.Equal(t, "u@example.com", id.UserEmail)
	assert.Equal(t, "claude", id.Provider)
	assert.Equal(t, "claude-opus-4-8", id.Model)
	assert.Equal(t, policy.OriginClientKey, id.Origin)
	assert.Equal(t, "ws-1", id.Workspace)
	assert.Equal(t, []string{"location", "filesystem"}, id.ConsentGrants)
	assert.Equal(t, "session", id.ConsentLayer)
	assert.Equal(t, map[string]string{"role": "editor"}, id.Claims)
}

func TestParseIdentity_NoMetadata(t *testing.T) {
	id := parseIdentity(context.Background())
	assert.Empty(t, id.AgentName)
	assert.Nil(t, id.ConsentGrants)
	assert.Empty(t, id.Claims)
}

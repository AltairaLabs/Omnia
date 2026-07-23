package runtime

import (
	"context"
	"strings"

	"github.com/altairalabs/omnia/pkg/policy"
	"google.golang.org/grpc/metadata"
)

// parseIdentity reads x-omnia-* headers from the incoming gRPC metadata into a
// typed Identity. Header names come from the shared policy constants so the SDK
// tracks any rename at compile time. Returns a zero Identity when no metadata.
func parseIdentity(ctx context.Context) Identity {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return Identity{}
	}
	id := Identity{
		AgentName: firstMD(md, policy.HeaderAgentName),
		Namespace: firstMD(md, policy.HeaderNamespace),
		SessionID: firstMD(md, policy.HeaderSessionID),
		RequestID: firstMD(md, policy.HeaderRequestID),
		UserID:    firstMD(md, policy.HeaderUserID),
		UserEmail: firstMD(md, policy.HeaderUserEmail),
		Provider:  firstMD(md, policy.HeaderProvider),
		Model:     firstMD(md, policy.HeaderModel),
		Origin:    firstMD(md, policy.HeaderOrigin),
		Workspace: firstMD(md, policy.HeaderWorkspace),
		Claims:    extractClaims(md),
	}
	if grants := firstMD(md, policy.HeaderConsentGrants); grants != "" {
		id.ConsentGrants = strings.Split(grants, ",")
	}
	id.ConsentLayer = firstMD(md, policy.HeaderConsentLayer)
	return id
}

func firstMD(md metadata.MD, key string) string {
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// extractClaims collects x-omnia-claim-* headers into a map keyed by the name
// after the prefix. Returns an empty (non-nil) map when there are none.
func extractClaims(md metadata.MD) map[string]string {
	claims := map[string]string{}
	for key, vals := range md {
		if len(vals) == 0 {
			continue
		}
		lk := strings.ToLower(key)
		if strings.HasPrefix(lk, policy.HeaderClaimPrefix) {
			claims[strings.TrimPrefix(lk, policy.HeaderClaimPrefix)] = vals[0]
		}
	}
	return claims
}

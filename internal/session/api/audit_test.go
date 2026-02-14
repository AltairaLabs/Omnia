/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestContextFromCtx_Missing(t *testing.T) {
	got, ok := requestContextFromCtx(context.Background())
	assert.False(t, ok)
	assert.Equal(t, RequestContext{}, got)
}

func TestAuditEntry_Fields(t *testing.T) {
	entry := &AuditEntry{
		EventType:   "session_accessed",
		SessionID:   "s1",
		Workspace:   "ws1",
		AgentName:   "agent1",
		Namespace:   "ns1",
		Query:       "search term",
		ResultCount: 5,
		IPAddress:   "192.168.1.1",
		UserAgent:   "curl/7.0",
		Metadata:    map[string]string{"key": "value"},
	}

	assert.Equal(t, "session_accessed", entry.EventType)
	assert.Equal(t, "s1", entry.SessionID)
	assert.Equal(t, "value", entry.Metadata["key"])
}

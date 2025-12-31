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

package runtime

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runtimev1 "github.com/altairalabs/omnia/pkg/runtime/v1"
)

func TestNewServer(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
		WithPackPath("/test/pack.json"),
		WithPromptName("test"),
	)

	require.NotNil(t, server)
	assert.Equal(t, "/test/pack.json", server.packPath)
	assert.Equal(t, "test", server.promptName)
	assert.True(t, server.healthy)
}

func TestServer_Health(t *testing.T) {
	server := NewServer()

	// Initially healthy
	resp, err := server.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
	assert.Equal(t, "ready", resp.Status)

	// Set unhealthy
	server.SetHealthy(false)
	resp, err = server.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.False(t, resp.Healthy)
	assert.Equal(t, "not ready", resp.Status)

	// Set healthy again
	server.SetHealthy(true)
	resp, err = server.Health(context.Background(), &runtimev1.HealthRequest{})
	require.NoError(t, err)
	assert.True(t, resp.Healthy)
}

func TestServer_Close(t *testing.T) {
	server := NewServer(
		WithLogger(logr.Discard()),
	)

	// Close should work even with no conversations
	err := server.Close()
	assert.NoError(t, err)
}

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func newTestServer(t *testing.T, objs ...client.Object) *Server {
	scheme := runtime.NewScheme()
	require.NoError(t, omniav1alpha1.AddToScheme(scheme))

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()

	// Pass nil for clientset since we're not testing logs functionality
	return NewServer(fakeClient, nil, zap.New(zap.UseDevMode(true)))
}

func TestListAgents(t *testing.T) {
	port := int32(8080)
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Facade: omniav1alpha1.FacadeConfig{
				Type: "websocket",
				Port: &port,
			},
		},
		Status: omniav1alpha1.AgentRuntimeStatus{
			Phase: "Running",
		},
	}

	server := newTestServer(t, agent)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var agents []omniav1alpha1.AgentRuntime
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &agents))
	assert.Len(t, agents, 1)
	assert.Equal(t, "test-agent", agents[0].Name)
}

func TestListAgentsWithNamespace(t *testing.T) {
	agent1 := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-1",
			Namespace: "ns1",
		},
	}
	agent2 := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent-2",
			Namespace: "ns2",
		},
	}

	server := newTestServer(t, agent1, agent2)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/agents?namespace=ns1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var agents []omniav1alpha1.AgentRuntime
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &agents))
	assert.Len(t, agents, 1)
	assert.Equal(t, "agent-1", agents[0].Name)
}

func TestGetAgent(t *testing.T) {
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
	}

	server := newTestServer(t, agent)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/agents/default/test-agent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var result omniav1alpha1.AgentRuntime
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
	assert.Equal(t, "test-agent", result.Name)
}

func TestGetAgentNotFound(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/agents/default/nonexistent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetAgentInvalidPath(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/agents/invalid", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListPromptPacks(t *testing.T) {
	pack := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pack",
			Namespace: "default",
		},
		Spec: omniav1alpha1.PromptPackSpec{
			Version: "1.0.0",
		},
	}

	server := newTestServer(t, pack)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/promptpacks", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var packs []omniav1alpha1.PromptPack
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &packs))
	assert.Len(t, packs, 1)
}

func TestGetPromptPack(t *testing.T) {
	pack := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pack",
			Namespace: "default",
		},
	}

	server := newTestServer(t, pack)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/promptpacks/default/test-pack", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestListToolRegistries(t *testing.T) {
	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
	}

	server := newTestServer(t, registry)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/toolregistries", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var registries []omniav1alpha1.ToolRegistry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &registries))
	assert.Len(t, registries, 1)
}

func TestGetToolRegistry(t *testing.T) {
	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
	}

	server := newTestServer(t, registry)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/toolregistries/default/test-registry", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestListProviders(t *testing.T) {
	provider := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-provider",
			Namespace: "default",
		},
	}

	server := newTestServer(t, provider)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/providers", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var providers []omniav1alpha1.Provider
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &providers))
	assert.Len(t, providers, 1)
}

func TestStats(t *testing.T) {
	agent := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-agent",
			Namespace: "default",
		},
		Status: omniav1alpha1.AgentRuntimeStatus{
			Phase: "Running",
		},
	}
	pack := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pack",
			Namespace: "default",
		},
		Status: omniav1alpha1.PromptPackStatus{
			Phase: "Active",
		},
	}
	registry := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-registry",
			Namespace: "default",
		},
		Status: omniav1alpha1.ToolRegistryStatus{
			DiscoveredToolsCount: 2,
			DiscoveredTools: []omniav1alpha1.DiscoveredTool{
				{Name: "tool1", Status: "Available"},
				{Name: "tool2", Status: "Unavailable"},
			},
		},
	}

	server := newTestServer(t, agent, pack, registry)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var stats Stats
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats))
	assert.Equal(t, 1, stats.Agents.Total)
	assert.Equal(t, 1, stats.Agents.Running)
	assert.Equal(t, 1, stats.PromptPacks.Total)
	assert.Equal(t, 1, stats.PromptPacks.Active)
	assert.Equal(t, 2, stats.Tools.Total)
	assert.Equal(t, 1, stats.Tools.Available)
	assert.Equal(t, 1, stats.Tools.Degraded)
}

func TestCORSHeaders(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("OPTIONS", "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
}

func TestMethodNotAllowed(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("DELETE", "/api/v1/agents", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestServerRun(t *testing.T) {
	server := newTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, ":0") // Use port 0 for random available port
	}()

	// Cancel context to trigger shutdown
	cancel()

	// Server should shut down cleanly
	err := <-errCh
	assert.NoError(t, err)
}

func TestGetPromptPackNotFound(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/promptpacks/default/nonexistent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetPromptPackInvalidPath(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/promptpacks/invalid", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetToolRegistryNotFound(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/toolregistries/default/nonexistent", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetToolRegistryInvalidPath(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/toolregistries/invalid", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListPromptPacksWithNamespace(t *testing.T) {
	pack1 := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pack-1",
			Namespace: "ns1",
		},
	}
	pack2 := &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pack-2",
			Namespace: "ns2",
		},
	}

	server := newTestServer(t, pack1, pack2)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/promptpacks?namespace=ns1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var packs []omniav1alpha1.PromptPack
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &packs))
	assert.Len(t, packs, 1)
	assert.Equal(t, "pack-1", packs[0].Name)
}

func TestListToolRegistriesWithNamespace(t *testing.T) {
	reg1 := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "reg-1",
			Namespace: "ns1",
		},
	}
	reg2 := &omniav1alpha1.ToolRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "reg-2",
			Namespace: "ns2",
		},
	}

	server := newTestServer(t, reg1, reg2)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/toolregistries?namespace=ns1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var regs []omniav1alpha1.ToolRegistry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &regs))
	assert.Len(t, regs, 1)
	assert.Equal(t, "reg-1", regs[0].Name)
}

func TestListProvidersWithNamespace(t *testing.T) {
	prov1 := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prov-1",
			Namespace: "ns1",
		},
	}
	prov2 := &omniav1alpha1.Provider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prov-2",
			Namespace: "ns2",
		},
	}

	server := newTestServer(t, prov1, prov2)
	handler := server.Handler()

	req := httptest.NewRequest("GET", "/api/v1/providers?namespace=ns1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var provs []omniav1alpha1.Provider
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &provs))
	assert.Len(t, provs, 1)
	assert.Equal(t, "prov-1", provs[0].Name)
}

func TestPromptPacksMethodNotAllowed(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("POST", "/api/v1/promptpacks", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestToolRegistriesMethodNotAllowed(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("POST", "/api/v1/toolregistries", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestProvidersMethodNotAllowed(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("POST", "/api/v1/providers", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestStatsMethodNotAllowed(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("POST", "/api/v1/stats", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestGetPromptPackMethodNotAllowed(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("POST", "/api/v1/promptpacks/default/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestGetToolRegistryMethodNotAllowed(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("POST", "/api/v1/toolregistries/default/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestParseLogLine(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		containerName string
		wantLevel     string
		wantMessage   string
	}{
		{
			name:          "plain text log",
			line:          "2026-01-05T12:00:00.000000000Z simple message",
			containerName: "facade",
			wantLevel:     "info",
			wantMessage:   "simple message",
		},
		{
			name:          "json zap log",
			line:          `2026-01-05T12:00:00.000000000Z {"level":"info","ts":1767616635.642715,"caller":"main.go:195","msg":"health server starting"}`,
			containerName: "runtime",
			wantLevel:     "info",
			wantMessage:   "[main.go:195] health server starting",
		},
		{
			name:          "json error log",
			line:          `2026-01-05T12:00:00.000000000Z {"level":"error","ts":1767616635.0,"caller":"server.go:50","msg":"connection failed","error":"timeout"}`,
			containerName: "facade",
			wantLevel:     "error",
			wantMessage:   "[server.go:50] connection failed error: timeout",
		},
		{
			name:          "plain error log",
			line:          "2026-01-05T12:00:00.000000000Z error: something went wrong",
			containerName: "facade",
			wantLevel:     "error",
			wantMessage:   "error: something went wrong",
		},
		{
			name:          "plain warning log",
			line:          "2026-01-05T12:00:00.000000000Z warning: low memory",
			containerName: "runtime",
			wantLevel:     "warn",
			wantMessage:   "warning: low memory",
		},
		{
			name:          "plain debug log",
			line:          "2026-01-05T12:00:00.000000000Z debug: processing request",
			containerName: "runtime",
			wantLevel:     "debug",
			wantMessage:   "debug: processing request",
		},
		{
			name:          "json log with logger name",
			line:          `2026-01-05T12:00:00.000000000Z {"level":"info","ts":1767616635.0,"logger":"websocket-server","caller":"server.go:188","msg":"new connection","agent":"test-agent","namespace":"default"}`,
			containerName: "facade",
			wantLevel:     "info",
			wantMessage:   "[websocket-server] [server.go:188] new connection (agent=test-agent, namespace=default)",
		},
		{
			name:          "json log with shorter timestamp",
			line:          `2026-01-05T12:00:00.123Z {"level":"info","ts":1767616635.0,"msg":"short timestamp test"}`,
			containerName: "facade",
			wantLevel:     "info",
			wantMessage:   "short timestamp test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := parseLogLine(tt.line, tt.containerName)
			assert.Equal(t, tt.wantLevel, entry.Level)
			assert.Equal(t, tt.wantMessage, entry.Message)
			assert.Equal(t, tt.containerName, entry.Container)
		})
	}
}

func TestSortLogsByTimestamp(t *testing.T) {
	now := time.Now()
	logs := []LogEntry{
		{Timestamp: now.Add(-1 * time.Hour), Message: "oldest"},
		{Timestamp: now, Message: "newest"},
		{Timestamp: now.Add(-30 * time.Minute), Message: "middle"},
	}

	sortLogsByTimestamp(logs)

	assert.Equal(t, "newest", logs[0].Message)
	assert.Equal(t, "middle", logs[1].Message)
	assert.Equal(t, "oldest", logs[2].Message)
}

func TestGetAgentLogsInvalidPath(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	// Test with only one path component (invalid)
	req := httptest.NewRequest("GET", "/api/v1/agents/invalid", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestGetAgentLogsNoClientset(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	// Request logs endpoint - should fail because clientset is nil
	req := httptest.NewRequest("GET", "/api/v1/agents/default/test-agent/logs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should return 500 error since clientset is nil
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestParseLogStream(t *testing.T) {
	server := newTestServer(t)
	input := `2026-01-05T12:00:00.000000000Z {"level":"info","ts":1767616635.0,"caller":"main.go:100","msg":"starting"}
2026-01-05T12:00:01.000000000Z {"level":"error","ts":1767616636.0,"caller":"main.go:200","msg":"failed","error":"timeout"}
2026-01-05T12:00:02.000000000Z plain log message`

	reader := strings.NewReader(input)
	logs := server.parseLogStream(reader, "test-container")

	require.Len(t, logs, 3)

	// First log
	assert.Equal(t, "info", logs[0].Level)
	assert.Equal(t, "[main.go:100] starting", logs[0].Message)
	assert.Equal(t, "test-container", logs[0].Container)

	// Second log (error with error field)
	assert.Equal(t, "error", logs[1].Level)
	assert.Equal(t, "[main.go:200] failed error: timeout", logs[1].Message)

	// Third log (plain text)
	assert.Equal(t, "info", logs[2].Level)
	assert.Equal(t, "plain log message", logs[2].Message)
}

func TestParseLogLineWithoutTimestamp(t *testing.T) {
	// Test log line without timestamp prefix
	entry := parseLogLine("plain message without timestamp", "facade")
	assert.Equal(t, "info", entry.Level)
	assert.Equal(t, "plain message without timestamp", entry.Message)
	assert.Equal(t, "facade", entry.Container)
}

func TestParseLogLineJSONWithoutCaller(t *testing.T) {
	// Test JSON log without caller field
	line := `2026-01-05T12:00:00.000000000Z {"level":"warn","ts":1767616635.0,"msg":"warning message"}`
	entry := parseLogLine(line, "runtime")
	assert.Equal(t, "warn", entry.Level)
	assert.Equal(t, "warning message", entry.Message)
}

func TestGetAgentLogsMethodNotAllowed(t *testing.T) {
	server := newTestServer(t)
	handler := server.Handler()

	req := httptest.NewRequest("POST", "/api/v1/agents/default/test-agent/logs", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

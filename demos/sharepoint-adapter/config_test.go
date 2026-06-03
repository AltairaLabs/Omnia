package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetenvDefault(t *testing.T) {
	t.Setenv("FOO_X", "set")
	assert.Equal(t, "set", getenvDefault("FOO_X", "fallback"))
	assert.Equal(t, "fallback", getenvDefault("FOO_UNSET", "fallback"))
}

func TestLoadConfig(t *testing.T) {
	t.Setenv("AZURE_TENANT_ID", "tenant")
	t.Setenv("AZURE_CLIENT_ID", "client")
	t.Setenv("AZURE_CLIENT_SECRET", "secret")
	t.Setenv("SHAREPOINT_SITE_ID", "site")
	t.Setenv("MEMORY_API_URL", "http://memory:8080")
	t.Setenv("WORKSPACE_ID", "demo")

	cfg := loadConfig()

	assert.Equal(t, "tenant", cfg.TenantID)
	assert.Equal(t, "client", cfg.ClientID)
	assert.Equal(t, "secret", cfg.ClientSecret)
	assert.Equal(t, "site", cfg.SiteID)
	assert.Equal(t, "http://memory:8080", cfg.MemoryURL)
	assert.Equal(t, "demo", cfg.WorkspaceID)
	assert.Equal(t, defaultGraphBaseURL, cfg.GraphBaseURL)
	assert.Equal(t, "8080", cfg.Port)
}

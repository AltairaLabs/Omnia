package main

import "os"

const defaultGraphBaseURL = "https://graph.microsoft.com/v1.0"

// Config holds adapter settings read from the environment.
type Config struct {
	TenantID     string
	ClientID     string
	ClientSecret string
	SiteID       string
	GraphBaseURL string
	MemoryURL    string
	WorkspaceID  string
	Port         string
}

func loadConfig() Config {
	return Config{
		TenantID:     os.Getenv("AZURE_TENANT_ID"),
		ClientID:     os.Getenv("AZURE_CLIENT_ID"),
		ClientSecret: os.Getenv("AZURE_CLIENT_SECRET"),
		SiteID:       os.Getenv("SHAREPOINT_SITE_ID"),
		GraphBaseURL: getenvDefault("GRAPH_BASE_URL", defaultGraphBaseURL),
		MemoryURL:    os.Getenv("MEMORY_API_URL"),
		WorkspaceID:  os.Getenv("WORKSPACE_ID"),
		Port:         getenvDefault("PORT", "8080"),
	}
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

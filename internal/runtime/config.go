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

// Package runtime implements the PromptKit runtime container.
package runtime

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	// Agent identification
	AgentName string
	Namespace string

	// PromptPack configuration
	PromptPackPath string // Path to the compiled .pack.json file
	PromptName     string // Name of the prompt to use from the pack

	// Session configuration
	SessionType string        // "memory" or "redis"
	SessionURL  string        // Redis URL for session store
	SessionTTL  time.Duration // Session TTL

	// Server ports
	GRPCPort   int
	HealthPort int
}

// Environment variable names.
const (
	envAgentName      = "OMNIA_AGENT_NAME"
	envNamespace      = "OMNIA_NAMESPACE"
	envPromptPackPath = "OMNIA_PROMPTPACK_PATH"
	envPromptName     = "OMNIA_PROMPT_NAME"
	envSessionType    = "OMNIA_SESSION_TYPE"
	envSessionURL     = "OMNIA_SESSION_URL"
	envSessionTTL     = "OMNIA_SESSION_TTL"
	envGRPCPort       = "OMNIA_GRPC_PORT"
	envHealthPort     = "OMNIA_HEALTH_PORT"
)

// Default values.
const (
	defaultPromptPackPath = "/etc/omnia/pack/pack.json"
	defaultPromptName     = "default"
	defaultSessionType    = "memory"
	defaultSessionTTL     = 24 * time.Hour
	defaultGRPCPort       = 9000
	defaultHealthPort     = 9001
)

// Session type constants.
const (
	SessionTypeMemory = "memory"
	SessionTypeRedis  = "redis"
)

// LoadConfig loads configuration from environment variables.
func LoadConfig() (*Config, error) {
	cfg := &Config{
		AgentName:      os.Getenv(envAgentName),
		Namespace:      os.Getenv(envNamespace),
		PromptPackPath: getEnvOrDefault(envPromptPackPath, defaultPromptPackPath),
		PromptName:     getEnvOrDefault(envPromptName, defaultPromptName),
		SessionType:    getEnvOrDefault(envSessionType, defaultSessionType),
		SessionURL:     os.Getenv(envSessionURL),
		GRPCPort:       defaultGRPCPort,
		HealthPort:     defaultHealthPort,
		SessionTTL:     defaultSessionTTL,
	}

	// Parse ports
	if port := os.Getenv(envGRPCPort); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", envGRPCPort, err)
		}
		cfg.GRPCPort = p
	}

	if port := os.Getenv(envHealthPort); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", envHealthPort, err)
		}
		cfg.HealthPort = p
	}

	// Parse session TTL
	if ttl := os.Getenv(envSessionTTL); ttl != "" {
		d, err := time.ParseDuration(ttl)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", envSessionTTL, err)
		}
		cfg.SessionTTL = d
	}

	// Validate required fields
	if cfg.AgentName == "" {
		return nil, fmt.Errorf("%s is required", envAgentName)
	}
	if cfg.Namespace == "" {
		return nil, fmt.Errorf("%s is required", envNamespace)
	}

	// Validate session type
	switch cfg.SessionType {
	case SessionTypeMemory, SessionTypeRedis:
		// Valid
	default:
		return nil, fmt.Errorf("invalid %s: must be 'memory' or 'redis'", envSessionType)
	}

	// Validate Redis URL if using Redis
	if cfg.SessionType == SessionTypeRedis && cfg.SessionURL == "" {
		return nil, fmt.Errorf("%s is required when using Redis sessions", envSessionURL)
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

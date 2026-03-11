/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"context"
	"encoding/json"
	"os"

	"github.com/go-logr/logr"

	"github.com/AltairaLabs/PromptKit/sdk"
)

// ResolvedClient is the JSON structure injected by the controller via OMNIA_A2A_CLIENTS.
type ResolvedClient struct {
	Name          string `json:"name"`
	URL           string `json:"url"`
	ExposeAsTools bool   `json:"exposeAsTools,omitempty"`
	AuthTokenEnv  string `json:"authTokenEnv,omitempty"`
}

// ParseResolvedClients parses the JSON-encoded client list from the env var.
func ParseResolvedClients(jsonData string) ([]ResolvedClient, error) {
	if jsonData == "" {
		return nil, nil
	}
	var clients []ResolvedClient
	if err := json.Unmarshal([]byte(jsonData), &clients); err != nil {
		return nil, err
	}
	return clients, nil
}

// BuildA2AAgentOptions creates SDK options for resolved A2A clients that have
// exposeAsTools enabled. Each client becomes an sdk.WithA2AAgent option that
// registers the remote agent's skills as local tools via the PromptKit Tool Bridge.
func BuildA2AAgentOptions(ctx context.Context, clients []ResolvedClient, log logr.Logger) []sdk.Option {
	var opts []sdk.Option

	for _, c := range clients {
		if !c.ExposeAsTools {
			log.V(1).Info("skipping non-tool client", "client", c.Name, "url", c.URL)
			continue
		}

		builder := sdk.NewA2AAgent(c.URL)

		// Load auth token from the designated env var if configured.
		if c.AuthTokenEnv != "" {
			token := os.Getenv(c.AuthTokenEnv)
			if token != "" {
				builder = builder.WithAuth("Bearer", token)
				log.V(1).Info("client auth configured", "client", c.Name)
			}
		}

		opts = append(opts, sdk.WithA2AAgent(builder))
		log.Info("registered A2A agent as tool source", "client", c.Name, "url", c.URL)
	}

	return opts
}

/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package a2a

import (
	"net/http"

	"github.com/AltairaLabs/PromptKit/runtime/a2a"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// CRDCardProvider builds an A2A Agent Card from CRD fields.
// It implements a2aserver.AgentCardProvider.
type CRDCardProvider struct {
	card a2a.AgentCard
}

// NewCRDCardProvider creates a card provider from the AgentRuntime's A2A config.
func NewCRDCardProvider(spec *omniav1alpha1.AgentCardSpec, serviceEndpoint string) *CRDCardProvider {
	card := a2a.AgentCard{
		Name:               spec.Name,
		Description:        spec.Description,
		Version:            spec.Version,
		DefaultInputModes:  spec.DefaultInputModes,
		DefaultOutputModes: spec.DefaultOutputModes,
	}

	// Default input/output modes if not specified.
	if len(card.DefaultInputModes) == 0 {
		card.DefaultInputModes = []string{"text"}
	}
	if len(card.DefaultOutputModes) == 0 {
		card.DefaultOutputModes = []string{"text"}
	}

	if spec.Organization != "" {
		card.Provider = &a2a.AgentProvider{
			Organization: spec.Organization,
		}
	}

	if spec.Capabilities != nil {
		card.Capabilities = a2a.AgentCapabilities{
			Streaming:         spec.Capabilities.Streaming,
			PushNotifications: spec.Capabilities.PushNotifications,
		}
	}

	for _, s := range spec.Skills {
		card.Skills = append(card.Skills, a2a.AgentSkill{
			ID:          s.ID,
			Name:        s.Name,
			Description: s.Description,
			Tags:        s.Tags,
			Examples:    s.Examples,
		})
	}

	if serviceEndpoint != "" {
		card.SupportedInterfaces = []a2a.AgentInterface{
			{
				URL:             serviceEndpoint + "/a2a",
				ProtocolBinding: "jsonrpc+http",
			},
		}
	}

	return &CRDCardProvider{card: card}
}

// AgentCard returns the built agent card.
func (p *CRDCardProvider) AgentCard(*http.Request) (*a2a.AgentCard, error) {
	return &p.card, nil
}

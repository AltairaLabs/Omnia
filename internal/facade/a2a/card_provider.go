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
	// interfaceURLFn, when set, is consulted at card-serve time for the agent's
	// externally-reachable interface URL (derived from observed HTTPRoutes). A
	// non-empty return overrides the in-cluster SupportedInterfaces URL baked in
	// at construction, so the card reflects exposure without a pod restart (#1576).
	interfaceURLFn func() string
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

// WithInterfaceURLFn sets a serve-time resolver for the agent's external
// interface URL. The function returns the full interface URL (scheme + host +
// path) or "" when no external route is observed, in which case the in-cluster
// URL baked in at construction is kept.
func (p *CRDCardProvider) WithInterfaceURLFn(fn func() string) *CRDCardProvider {
	p.interfaceURLFn = fn
	return p
}

// AgentCard returns the agent card, overriding the interface URL from the
// serve-time resolver when one is configured and returns a non-empty URL.
func (p *CRDCardProvider) AgentCard(*http.Request) (*a2a.AgentCard, error) {
	if p.interfaceURLFn == nil {
		return &p.card, nil
	}
	url := p.interfaceURLFn()
	if url == "" {
		return &p.card, nil
	}
	card := p.card
	card.SupportedInterfaces = []a2a.AgentInterface{
		{URL: url, ProtocolBinding: "jsonrpc+http"},
	}
	return &card, nil
}

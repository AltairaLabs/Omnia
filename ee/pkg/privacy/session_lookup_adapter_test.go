/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package privacy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/altairalabs/omnia/internal/session"
)

func TestSessionToMetadata(t *testing.T) {
	sess := &session.Session{
		Namespace: "my-ns",
		AgentName: "my-agent",
		ID:        "session-1",
	}
	meta := sessionToMetadata(sess)
	assert.Equal(t, "my-ns", meta.Namespace)
	assert.Equal(t, "my-agent", meta.AgentName)
}

func TestNewWarmStoreSessionLookup(t *testing.T) {
	lookup := NewWarmStoreSessionLookup(nil)
	assert.NotNil(t, lookup)
}

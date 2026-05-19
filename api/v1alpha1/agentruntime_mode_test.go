/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentRuntime_EffectiveMode(t *testing.T) {
	t.Run("nil receiver defaults to agent", func(t *testing.T) {
		var ar *AgentRuntime
		assert.Equal(t, AgentRuntimeModeAgent, ar.EffectiveMode())
	})

	t.Run("empty mode defaults to agent", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{}}
		assert.Equal(t, AgentRuntimeModeAgent, ar.EffectiveMode())
	})

	t.Run("explicit agent", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{Mode: AgentRuntimeModeAgent}}
		assert.Equal(t, AgentRuntimeModeAgent, ar.EffectiveMode())
	})

	t.Run("explicit function", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{Mode: AgentRuntimeModeFunction}}
		assert.Equal(t, AgentRuntimeModeFunction, ar.EffectiveMode())
	})
}

func TestAgentRuntime_IsFunctionMode(t *testing.T) {
	t.Run("agent-mode runtime is not function", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{Mode: AgentRuntimeModeAgent}}
		assert.False(t, ar.IsFunctionMode())
	})

	t.Run("function-mode runtime is function", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{Mode: AgentRuntimeModeFunction}}
		assert.True(t, ar.IsFunctionMode())
	})

	t.Run("pre-mode runtime treated as agent", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{}}
		assert.False(t, ar.IsFunctionMode())
	})
}

func TestAgentRuntime_InvocationRecordingEnabled(t *testing.T) {
	t.Run("nil receiver returns false", func(t *testing.T) {
		var ar *AgentRuntime
		assert.False(t, ar.InvocationRecordingEnabled())
	})

	t.Run("agent mode always false even with block set", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{
			Mode: AgentRuntimeModeAgent,
			InvocationRecording: &InvocationRecordingConfig{
				State: InvocationRecordingEnabled,
			},
		}}
		assert.False(t, ar.InvocationRecordingEnabled())
	})

	t.Run("function mode with no block defaults disabled", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{Mode: AgentRuntimeModeFunction}}
		assert.False(t, ar.InvocationRecordingEnabled())
	})

	t.Run("function mode with state disabled", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{
			Mode: AgentRuntimeModeFunction,
			InvocationRecording: &InvocationRecordingConfig{
				State: InvocationRecordingDisabled,
			},
		}}
		assert.False(t, ar.InvocationRecordingEnabled())
	})

	t.Run("function mode with state enabled", func(t *testing.T) {
		ar := &AgentRuntime{Spec: AgentRuntimeSpec{
			Mode: AgentRuntimeModeFunction,
			InvocationRecording: &InvocationRecordingConfig{
				State: InvocationRecordingEnabled,
			},
		}}
		assert.True(t, ar.InvocationRecordingEnabled())
	})
}

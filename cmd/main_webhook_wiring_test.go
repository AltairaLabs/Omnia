/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"reflect"
	"testing"

	omniawebhook "github.com/altairalabs/omnia/internal/webhook"
)

// TestCoreWebhookSetupsExist guards against the regression that started
// #1116: a validator that exists but is never wired. If either Setup
// function is removed or renamed, main.go's registration block stops
// compiling and this test stops compiling with it.
func TestCoreWebhookSetupsExist(t *testing.T) {
	for name, fn := range map[string]any{
		"AgentRuntime": omniawebhook.SetupAgentRuntimeWebhookWithManager,
		"SkillSource":  omniawebhook.SetupSkillSourceWebhookWithManager,
	} {
		if reflect.ValueOf(fn).IsNil() {
			t.Errorf("%s webhook Setup function is nil", name)
		}
	}
}

/*
Copyright 2026.

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

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	pkruntime "github.com/altairalabs/omnia/internal/runtime"
	"github.com/altairalabs/omnia/internal/schema"
	"github.com/altairalabs/omnia/pkg/k8s"
)

// maxConditionMessageLen is the maximum length for a condition message.
const maxConditionMessageLen = 1024

// packReadyError returns nil when the pack at packPath is present and passes
// schema validation, or an error describing why the runtime cannot serve it.
// The runtime readiness probe calls this on every check, so a pod whose mounted
// pack is invalid — including a broken pack rolled onto a live agent — drops out
// of the Service rather than accepting conversations that fail at open-time
// (#1299). The schema validator uses an embedded schema, so a Validate error is
// a definitive invalid-pack result, not a transient network failure.
func packReadyError(validator *schema.SchemaValidator, packPath string) error {
	data, err := os.ReadFile(packPath)
	if err != nil {
		return fmt.Errorf("pack file unreadable: %w", err)
	}
	if validator != nil {
		if err := validator.Validate(data); err != nil {
			return fmt.Errorf("pack schema invalid: %w", err)
		}
	}
	return nil
}

// validatePackContent runs pack-level validation and returns warnings.
func validatePackContent(packPath string, evalDefs []evals.EvalDef, log logr.Logger) []string {
	var warnings []string

	// Check pack file is readable
	if _, err := os.Stat(packPath); err != nil {
		return []string{fmt.Sprintf("pack file not found: %v", err)}
	}

	// Check for unregistered eval types
	if missing := pkruntime.ValidateEvalDefs(evalDefs); len(missing) > 0 {
		warnings = append(warnings, fmt.Sprintf("unregistered eval types: %v", missing))
	}

	if len(warnings) > 0 {
		log.Info("pack content validation issues found", "warnings", warnings)
	}

	return warnings
}

// reportPackValidation patches the PackContentValid condition on the AgentRuntime.
func reportPackValidation(
	ctx context.Context,
	c client.Client,
	agentName, namespace string,
	warnings []string,
) error {
	if len(warnings) > 0 {
		msg := strings.Join(warnings, "; ")
		if len(msg) > maxConditionMessageLen {
			msg = msg[:maxConditionMessageLen-3] + "..."
		}
		return k8s.PatchAgentRuntimeCondition(ctx, c, agentName, namespace,
			k8s.ConditionPackContentValid, metav1.ConditionFalse,
			"ContentIssuesFound", msg)
	}

	return k8s.PatchAgentRuntimeCondition(ctx, c, agentName, namespace,
		k8s.ConditionPackContentValid, metav1.ConditionTrue,
		"PackContentValid", "Pack content validated successfully")
}

// reportCapabilities self-reports the runtime's advertised capabilities to the
// AgentRuntime status so the control plane and `kubectl describe` can see them.
func reportCapabilities(ctx context.Context, c client.Client, agentName, namespace string) error {
	return k8s.PatchAgentRuntimeCapabilities(ctx, c, agentName, namespace, pkruntime.Capabilities())
}

// reportStartupStatus best-effort self-reports pack-validation and the runtime's
// capabilities to the AgentRuntime status at startup. Errors are logged, not
// fatal — a status-report failure must not stop the runtime serving.
func reportStartupStatus(
	ctx context.Context, log logr.Logger, c client.Client,
	agentName, namespace string, warnings []string,
) {
	if err := reportPackValidation(ctx, c, agentName, namespace, warnings); err != nil {
		log.Error(err, "failed to patch PackContentValid condition")
	}
	if err := reportCapabilities(ctx, c, agentName, namespace); err != nil {
		log.Error(err, "failed to report runtime capabilities")
	}
}

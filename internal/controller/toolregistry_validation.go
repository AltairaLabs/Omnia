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

package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// reasonToolRegistryCrossNamespace is the condition reason + Event reason used
// when spec.toolRegistryRef.namespace names a namespace other than the
// AgentRuntime's own. Cross-namespace references are not supported: the agent's
// Role is namespace-scoped, so the pod cannot read the registry, and
// registry-scoped ToolPolicies would silently fail to match (#1874).
const reasonToolRegistryCrossNamespace = "ToolRegistryCrossNamespace"

// rejectCrossNamespaceToolRegistry fails an AgentRuntime whose toolRegistryRef
// names a foreign namespace. The operator would happily resolve such a registry
// and project the tools ConfigMap, but the agent pod's Role is namespace-scoped
// so it can never read that registry — and registry-scoped ToolPolicies then
// silently fail to match (#1874). CEL cannot express this (CRD validation cannot
// see metadata.namespace), so this mirrors the framework-image loud-failure
// path: condition + Warning event + no Deployment.
func (r *AgentRuntimeReconciler) rejectCrossNamespaceToolRegistry(
	ctx context.Context,
	log logr.Logger,
	agentRuntime *omniav1alpha1.AgentRuntime,
) error {
	ref := agentRuntime.Spec.ToolRegistryRef
	if ref == nil || ref.Namespace == nil || *ref.Namespace == agentRuntime.Namespace {
		return nil
	}

	msg := fmt.Sprintf(
		"spec.toolRegistryRef.namespace %q differs from the AgentRuntime namespace %q; "+
			"cross-namespace ToolRegistry references are not supported",
		*ref.Namespace, agentRuntime.Namespace)
	SetCondition(&agentRuntime.Status.Conditions, agentRuntime.Generation,
		ConditionTypeToolRegistryReady, metav1.ConditionFalse, reasonToolRegistryCrossNamespace, msg)
	agentRuntime.Status.Phase = omniav1alpha1.AgentRuntimePhasePending
	if r.Recorder != nil {
		r.Recorder.Event(agentRuntime, corev1.EventTypeWarning, reasonToolRegistryCrossNamespace, msg)
	}
	if statusErr := r.Status().Update(ctx, agentRuntime); statusErr != nil {
		log.Error(statusErr, logMsgFailedToUpdateStatus)
	}
	return errors.New(msg)
}

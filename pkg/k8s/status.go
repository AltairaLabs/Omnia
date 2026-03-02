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

package k8s

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ConditionPackContentValid is the condition type for pack content validation.
// Defined here so the runtime binary can reference it without importing internal/controller.
const ConditionPackContentValid = "PackContentValid"

// PatchAgentRuntimeCondition sets a condition on an AgentRuntime's status subresource.
func PatchAgentRuntimeCondition(
	ctx context.Context, c client.Client,
	name, namespace string,
	condType string, status metav1.ConditionStatus,
	reason, message string,
) error {
	ar, err := GetAgentRuntime(ctx, c, name, namespace)
	if err != nil {
		return fmt.Errorf("get AgentRuntime for status patch: %w", err)
	}

	base := ar.DeepCopy()
	meta.SetStatusCondition(&ar.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: ar.Generation,
		Reason:             reason,
		Message:            message,
	})

	return c.Status().Patch(ctx, ar, client.MergeFrom(base))
}

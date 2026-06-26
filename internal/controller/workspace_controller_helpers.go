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

package controller

import (
	"context"
	"fmt"
	"strings"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	eev1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *WorkspaceReconciler) getClusterRoleForRole(role omniav1alpha1.WorkspaceRole) string {
	switch role {
	case omniav1alpha1.WorkspaceRoleOwner:
		return clusterRoleOwner
	case omniav1alpha1.WorkspaceRoleEditor:
		return clusterRoleEditor
	case omniav1alpha1.WorkspaceRoleViewer:
		return clusterRoleViewer
	default:
		return clusterRoleViewer
	}
}

func (r *WorkspaceReconciler) updateMemberCount(workspace *omniav1alpha1.Workspace) {
	count := &omniav1alpha1.MemberCount{}

	for _, binding := range workspace.Spec.RoleBindings {
		groupCount := int32(len(binding.Groups))
		saCount := int32(len(binding.ServiceAccounts))
		total := groupCount + saCount

		switch binding.Role {
		case omniav1alpha1.WorkspaceRoleOwner:
			count.Owners += total
		case omniav1alpha1.WorkspaceRoleEditor:
			count.Editors += total
		case omniav1alpha1.WorkspaceRoleViewer:
			count.Viewers += total
		}
	}

	// Count direct grants
	for _, grant := range workspace.Spec.DirectGrants {
		switch grant.Role {
		case omniav1alpha1.WorkspaceRoleOwner:
			count.Owners++
		case omniav1alpha1.WorkspaceRoleEditor:
			count.Editors++
		case omniav1alpha1.WorkspaceRoleViewer:
			count.Viewers++
		}
	}

	workspace.Status.Members = count
}

// sanitizeName converts a name to a valid Kubernetes name component
func sanitizeName(name string) string {
	// Simple sanitization - replace non-alphanumeric with dash
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c-'A'+'a') // lowercase
		} else {
			result = append(result, '-')
		}
	}
	// Trim leading/trailing dashes
	s := string(result)
	for len(s) > 0 && s[0] == '-' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '-' {
		s = s[:len(s)-1]
	}
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// validatePrivacyPolicyRefs returns one Condition summarising privacyPolicyRef
// resolution across all service groups. Status is False if ANY referenced policy
// is missing, with a Message listing every unresolved (groupName, policyName) pair.
// Missing refs do not block reconciliation — they are informational only.
func (r *WorkspaceReconciler) validatePrivacyPolicyRefs(ctx context.Context, ws *omniav1alpha1.Workspace) metav1.Condition {
	missing := make([]string, 0, len(ws.Spec.Services))
	resolved := make([]string, 0, len(ws.Spec.Services))
	for i := range ws.Spec.Services {
		sg := &ws.Spec.Services[i]
		if sg.PrivacyPolicyRef == nil {
			continue
		}
		p := &eev1alpha1.SessionPrivacyPolicy{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      sg.PrivacyPolicyRef.Name,
			Namespace: ws.Spec.Namespace.Name,
		}, p)
		if err != nil {
			missing = append(missing, fmt.Sprintf("services[%s] -> %s (%v)", sg.Name, sg.PrivacyPolicyRef.Name, err))
			continue
		}
		resolved = append(resolved, fmt.Sprintf("services[%s] -> %s", sg.Name, sg.PrivacyPolicyRef.Name))
	}
	if len(missing) > 0 {
		return metav1.Condition{
			Type:    ConditionTypePrivacyPolicyResolved,
			Status:  metav1.ConditionFalse,
			Reason:  "PolicyNotFound",
			Message: "unresolved privacyPolicyRef(s): " + strings.Join(missing, "; "),
		}
	}
	if len(resolved) == 0 {
		return metav1.Condition{
			Type:    ConditionTypePrivacyPolicyResolved,
			Status:  metav1.ConditionTrue,
			Reason:  "DefaultPolicy",
			Message: "no service group sets privacyPolicyRef; sessions use global default",
		}
	}
	return metav1.Condition{
		Type:    ConditionTypePrivacyPolicyResolved,
		Status:  metav1.ConditionTrue,
		Reason:  "PolicyResolved",
		Message: strings.Join(resolved, "; "),
	}
}

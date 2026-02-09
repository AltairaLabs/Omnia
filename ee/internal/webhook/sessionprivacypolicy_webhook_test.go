/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package webhook

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = omniav1alpha1.AddToScheme(s)
	return s
}

func int32Ptr(i int32) *int32 { return &i }

func globalPolicy(name string, opts ...func(*omniav1alpha1.SessionPrivacyPolicy)) *omniav1alpha1.SessionPrivacyPolicy {
	p := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level: omniav1alpha1.PolicyLevelGlobal,
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:    true,
				FacadeData: true,
				RichData:   false,
				PII:        &omniav1alpha1.PIIConfig{Redact: true, Patterns: []string{"ssn", "email"}},
			},
			UserOptOut: &omniav1alpha1.UserOptOutConfig{Enabled: true, HonorDeleteRequests: true},
		},
	}
	for _, fn := range opts {
		fn(p)
	}
	return p
}

func workspacePolicy(name, wsName string, opts ...func(*omniav1alpha1.SessionPrivacyPolicy)) *omniav1alpha1.SessionPrivacyPolicy {
	p := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level:        omniav1alpha1.PolicyLevelWorkspace,
			WorkspaceRef: &corev1alpha1.LocalObjectReference{Name: wsName},
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:    true,
				FacadeData: true,
				RichData:   false,
				PII:        &omniav1alpha1.PIIConfig{Redact: true, Patterns: []string{"ssn", "email"}},
			},
			UserOptOut: &omniav1alpha1.UserOptOutConfig{Enabled: true, HonorDeleteRequests: true},
		},
	}
	for _, fn := range opts {
		fn(p)
	}
	return p
}

func agentPolicy(opts ...func(*omniav1alpha1.SessionPrivacyPolicy)) *omniav1alpha1.SessionPrivacyPolicy {
	p := &omniav1alpha1.SessionPrivacyPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "agent-policy"},
		Spec: omniav1alpha1.SessionPrivacyPolicySpec{
			Level:    omniav1alpha1.PolicyLevelAgent,
			AgentRef: &corev1alpha1.NamespacedObjectReference{Name: "my-agent", Namespace: "my-workspace"},
			Recording: omniav1alpha1.RecordingConfig{
				Enabled:    true,
				FacadeData: true,
				RichData:   false,
				PII:        &omniav1alpha1.PIIConfig{Redact: true, Patterns: []string{"ssn", "email"}},
			},
			UserOptOut: &omniav1alpha1.UserOptOutConfig{Enabled: true, HonorDeleteRequests: true},
		},
	}
	for _, fn := range opts {
		fn(p)
	}
	return p
}

func TestSessionPrivacyPolicyValidateCreate(t *testing.T) {
	tests := []struct {
		name           string
		existing       []omniav1alpha1.SessionPrivacyPolicy
		policy         *omniav1alpha1.SessionPrivacyPolicy
		expectError    bool
		expectWarnings bool
	}{
		{
			name:        "valid global policy create",
			existing:    nil,
			policy:      globalPolicy("default"),
			expectError: false,
		},
		{
			name:     "valid workspace policy with compliant parent",
			existing: []omniav1alpha1.SessionPrivacyPolicy{*globalPolicy("default")},
			policy:   workspacePolicy("ws-policy", "my-workspace"),
		},
		{
			name: "valid agent policy with compliant parent chain",
			existing: []omniav1alpha1.SessionPrivacyPolicy{
				*globalPolicy("default"),
				*workspacePolicy("ws-policy", "my-workspace"),
			},
			policy: agentPolicy(),
		},
		{
			name:     "reject workspace policy less restrictive than global — recording enabled",
			existing: []omniav1alpha1.SessionPrivacyPolicy{*globalPolicy("default", func(p *omniav1alpha1.SessionPrivacyPolicy) { p.Spec.Recording.Enabled = false })},
			policy: workspacePolicy("ws-policy", "my-workspace", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Recording.Enabled = true
			}),
			expectError: true,
		},
		{
			name:     "reject workspace policy less restrictive — richData enabled when parent disables",
			existing: []omniav1alpha1.SessionPrivacyPolicy{*globalPolicy("default")},
			policy: workspacePolicy("ws-policy", "my-workspace", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Recording.RichData = true
			}),
			expectError: true,
		},
		{
			name:     "reject workspace policy less restrictive — pii.redact disabled when parent enables",
			existing: []omniav1alpha1.SessionPrivacyPolicy{*globalPolicy("default")},
			policy: workspacePolicy("ws-policy", "my-workspace", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Recording.PII = &omniav1alpha1.PIIConfig{Redact: false}
			}),
			expectError: true,
		},
		{
			name:     "reject workspace policy less restrictive — userOptOut disabled when parent enables",
			existing: []omniav1alpha1.SessionPrivacyPolicy{*globalPolicy("default")},
			policy: workspacePolicy("ws-policy", "my-workspace", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.UserOptOut = &omniav1alpha1.UserOptOutConfig{Enabled: false}
			}),
			expectError: true,
		},
		{
			name: "reject agent policy less restrictive than workspace parent",
			existing: []omniav1alpha1.SessionPrivacyPolicy{
				*globalPolicy("default"),
				*workspacePolicy("ws-policy", "my-workspace"),
			},
			policy: agentPolicy(func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Recording.RichData = true
			}),
			expectError: true,
		},
		{
			name: "reject workspace policy — retention exceeds parent",
			existing: []omniav1alpha1.SessionPrivacyPolicy{*globalPolicy("default", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Retention = &omniav1alpha1.PrivacyRetentionConfig{
					Facade: &omniav1alpha1.PrivacyRetentionTierConfig{WarmDays: int32Ptr(7)},
				}
			})},
			policy: workspacePolicy("ws-policy", "my-workspace", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Retention = &omniav1alpha1.PrivacyRetentionConfig{
					Facade: &omniav1alpha1.PrivacyRetentionTierConfig{WarmDays: int32Ptr(30)},
				}
			}),
			expectError: true,
		},
		{
			name:           "no parent found — allowed with warning",
			existing:       nil,
			policy:         workspacePolicy("ws-orphan", "my-workspace"),
			expectError:    false,
			expectWarnings: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]runtime.Object, 0, len(tt.existing))
			for i := range tt.existing {
				objs = append(objs, &tt.existing[i])
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(newTestScheme()).
				WithRuntimeObjects(objs...).
				Build()

			v := &SessionPrivacyPolicyValidator{Client: fakeClient}
			warnings, err := v.ValidateCreate(context.Background(), tt.policy)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if tt.expectWarnings && len(warnings) == 0 {
				t.Error("expected warnings but got none")
			}
			if !tt.expectWarnings && len(warnings) > 0 {
				t.Errorf("unexpected warnings: %v", warnings)
			}
		})
	}
}

func TestSessionPrivacyPolicyValidateUpdate(t *testing.T) {
	existing := []omniav1alpha1.SessionPrivacyPolicy{*globalPolicy("default")}
	objs := make([]runtime.Object, 0, len(existing))
	for i := range existing {
		objs = append(objs, &existing[i])
	}
	fakeClient := fake.NewClientBuilder().
		WithScheme(newTestScheme()).
		WithRuntimeObjects(objs...).
		Build()

	v := &SessionPrivacyPolicyValidator{Client: fakeClient}

	oldPolicy := workspacePolicy("ws-policy", "my-workspace")
	newPolicy := workspacePolicy("ws-policy", "my-workspace")

	_, err := v.ValidateUpdate(context.Background(), oldPolicy, newPolicy)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSessionPrivacyPolicyValidateDelete(t *testing.T) {
	tests := []struct {
		name        string
		existing    []omniav1alpha1.SessionPrivacyPolicy
		toDelete    *omniav1alpha1.SessionPrivacyPolicy
		expectError bool
	}{
		{
			name: "reject delete of sole global policy",
			existing: []omniav1alpha1.SessionPrivacyPolicy{
				*globalPolicy("default"),
			},
			toDelete:    globalPolicy("default"),
			expectError: true,
		},
		{
			name: "allow delete of global policy when another global exists",
			existing: []omniav1alpha1.SessionPrivacyPolicy{
				*globalPolicy("default"),
				*globalPolicy("backup"),
			},
			toDelete:    globalPolicy("default"),
			expectError: false,
		},
		{
			name: "allow delete of workspace policy",
			existing: []omniav1alpha1.SessionPrivacyPolicy{
				*globalPolicy("default"),
				*workspacePolicy("ws-policy", "my-workspace"),
			},
			toDelete:    workspacePolicy("ws-policy", "my-workspace"),
			expectError: false,
		},
		{
			name: "allow delete of agent policy",
			existing: []omniav1alpha1.SessionPrivacyPolicy{
				*globalPolicy("default"),
				*agentPolicy(),
			},
			toDelete:    agentPolicy(),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := make([]runtime.Object, 0, len(tt.existing))
			for i := range tt.existing {
				objs = append(objs, &tt.existing[i])
			}
			fakeClient := fake.NewClientBuilder().
				WithScheme(newTestScheme()).
				WithRuntimeObjects(objs...).
				Build()

			v := &SessionPrivacyPolicyValidator{Client: fakeClient}
			_, err := v.ValidateDelete(context.Background(), tt.toDelete)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateStricterThanParent(t *testing.T) {
	tests := []struct {
		name     string
		parent   *omniav1alpha1.SessionPrivacyPolicy
		child    *omniav1alpha1.SessionPrivacyPolicy
		wantErrs int
	}{
		{
			name:     "identical policies — no violations",
			parent:   globalPolicy("parent"),
			child:    workspacePolicy("child", "ws"),
			wantErrs: 0,
		},
		{
			name:   "child enables recording when parent disables — 1 violation",
			parent: globalPolicy("parent", func(p *omniav1alpha1.SessionPrivacyPolicy) { p.Spec.Recording.Enabled = false }),
			child: workspacePolicy("child", "ws", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Recording.Enabled = true
			}),
			wantErrs: 1,
		},
		{
			name:   "child has multiple violations",
			parent: globalPolicy("parent"),
			child: workspacePolicy("child", "ws", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Recording.RichData = true
				p.Spec.Recording.PII = nil // drops redact
				p.Spec.UserOptOut = nil    // drops opt-out
			}),
			wantErrs: 3,
		},
		{
			name: "retention warmDays exceeds parent",
			parent: globalPolicy("parent", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Retention = &omniav1alpha1.PrivacyRetentionConfig{
					Facade:   &omniav1alpha1.PrivacyRetentionTierConfig{WarmDays: int32Ptr(7), ColdDays: int32Ptr(30)},
					RichData: &omniav1alpha1.PrivacyRetentionTierConfig{WarmDays: int32Ptr(3)},
				}
			}),
			child: workspacePolicy("child", "ws", func(p *omniav1alpha1.SessionPrivacyPolicy) {
				p.Spec.Retention = &omniav1alpha1.PrivacyRetentionConfig{
					Facade:   &omniav1alpha1.PrivacyRetentionTierConfig{WarmDays: int32Ptr(14), ColdDays: int32Ptr(60)},
					RichData: &omniav1alpha1.PrivacyRetentionTierConfig{WarmDays: int32Ptr(7)},
				}
			}),
			wantErrs: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateStricterThanParent(tt.child, tt.parent)
			if len(errs) != tt.wantErrs {
				t.Errorf("got %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

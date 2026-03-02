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
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/AltairaLabs/PromptKit/runtime/evals"
	_ "github.com/AltairaLabs/PromptKit/runtime/evals/handlers"
	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/pkg/k8s"
)

func TestValidatePackContent_PackFileNotFound(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	warnings := validatePackContent("/nonexistent/pack.json", nil, log)

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}
	if !strings.Contains(warnings[0], "pack file not found") {
		t.Errorf("expected 'pack file not found' warning, got: %s", warnings[0])
	}
}

func TestValidatePackContent_NoEvalDefs(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	// Create a temp pack file
	dir := t.TempDir()
	packPath := filepath.Join(dir, "pack.json")
	if err := os.WriteFile(packPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	warnings := validatePackContent(packPath, nil, log)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}

func TestValidatePackContent_UnregisteredEvalTypes(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	dir := t.TempDir()
	packPath := filepath.Join(dir, "pack.json")
	if err := os.WriteFile(packPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	defs := []evals.EvalDef{
		{Type: "nonexistent_handler_xyz"},
	}

	warnings := validatePackContent(packPath, defs, log)
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "unregistered eval types") {
		t.Errorf("expected unregistered eval types warning, got: %s", warnings[0])
	}
	if !strings.Contains(warnings[0], "nonexistent_handler_xyz") {
		t.Errorf("expected type name in warning, got: %s", warnings[0])
	}
}

func TestValidatePackContent_ValidPackAndEvals(t *testing.T) {
	log := zap.New(zap.UseDevMode(true))
	dir := t.TempDir()
	packPath := filepath.Join(dir, "pack.json")
	if err := os.WriteFile(packPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Use a type that is registered by the handlers import
	defs := []evals.EvalDef{
		{Type: "regex"},
	}

	warnings := validatePackContent(packPath, defs, log)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for valid eval types, got %v", warnings)
	}
}

func TestReportPackValidation_WithWarnings(t *testing.T) {
	s := k8s.Scheme()
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-agent",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(ar).
		WithStatusSubresource(ar).
		Build()

	err := reportPackValidation(
		context.Background(), c,
		"test-agent", "default",
		[]string{"unregistered eval types: [bogus]"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := k8s.GetAgentRuntime(context.Background(), c, "test-agent", "default")
	if err != nil {
		t.Fatalf("failed to get AgentRuntime: %v", err)
	}

	if len(got.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(got.Status.Conditions))
	}
	cond := got.Status.Conditions[0]
	if cond.Type != k8s.ConditionPackContentValid {
		t.Errorf("expected type %s, got %s", k8s.ConditionPackContentValid, cond.Type)
	}
	if cond.Status != metav1.ConditionFalse {
		t.Errorf("expected status False, got %s", cond.Status)
	}
	if cond.Reason != "ContentIssuesFound" {
		t.Errorf("expected reason ContentIssuesFound, got %s", cond.Reason)
	}
}

func TestReportPackValidation_NoWarnings(t *testing.T) {
	s := k8s.Scheme()
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-agent",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(ar).
		WithStatusSubresource(ar).
		Build()

	err := reportPackValidation(
		context.Background(), c,
		"test-agent", "default",
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := k8s.GetAgentRuntime(context.Background(), c, "test-agent", "default")
	if err != nil {
		t.Fatalf("failed to get AgentRuntime: %v", err)
	}

	if len(got.Status.Conditions) != 1 {
		t.Fatalf("expected 1 condition, got %d", len(got.Status.Conditions))
	}
	cond := got.Status.Conditions[0]
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("expected status True, got %s", cond.Status)
	}
	if cond.Reason != "PackContentValid" {
		t.Errorf("expected reason PackContentValid, got %s", cond.Reason)
	}
}

func TestReportPackValidation_AgentNotFound(t *testing.T) {
	s := k8s.Scheme()
	c := fake.NewClientBuilder().WithScheme(s).Build()

	err := reportPackValidation(
		context.Background(), c,
		"nonexistent", "default",
		nil,
	)
	if err == nil {
		t.Fatal("expected error for nonexistent AgentRuntime")
	}
}

func TestReportPackValidation_TruncatesLongMessage(t *testing.T) {
	s := k8s.Scheme()
	ar := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-agent",
			Namespace:  "default",
			Generation: 1,
		},
		Spec: omniav1alpha1.AgentRuntimeSpec{
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "test-pack"},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(s).
		WithRuntimeObjects(ar).
		WithStatusSubresource(ar).
		Build()

	// Create a warning message longer than maxConditionMessageLen
	longWarning := strings.Repeat("x", 2000)
	err := reportPackValidation(
		context.Background(), c,
		"test-agent", "default",
		[]string{longWarning},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := k8s.GetAgentRuntime(context.Background(), c, "test-agent", "default")
	if err != nil {
		t.Fatalf("failed to get AgentRuntime: %v", err)
	}

	cond := got.Status.Conditions[0]
	if len(cond.Message) > maxConditionMessageLen {
		t.Errorf("message should be truncated to %d, got %d", maxConditionMessageLen, len(cond.Message))
	}
	if !strings.HasSuffix(cond.Message, "...") {
		t.Error("truncated message should end with '...'")
	}
}

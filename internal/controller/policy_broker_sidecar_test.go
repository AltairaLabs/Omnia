/*
Copyright 2026 Altaira Labs.

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
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestBuildPolicyBrokerContainer_DefaultImage(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	container := buildPolicyBrokerContainer(agentRuntime, "", "")
	if container.Name != PolicyBrokerContainerName {
		t.Errorf("Name = %q, want %q", container.Name, PolicyBrokerContainerName)
	}
	if container.Image != DefaultPolicyBrokerImage {
		t.Errorf("Image = %q, want %q", container.Image, DefaultPolicyBrokerImage)
	}
	if len(container.Ports) != 2 {
		t.Fatalf("Ports count = %d, want 2", len(container.Ports))
	}
	if container.Ports[0].ContainerPort != DefaultPolicyBrokerPort {
		t.Errorf("broker port = %d, want %d", container.Ports[0].ContainerPort, DefaultPolicyBrokerPort)
	}
	if container.Ports[1].ContainerPort != DefaultPolicyBrokerHealthPort {
		t.Errorf("health port = %d, want %d", container.Ports[1].ContainerPort, DefaultPolicyBrokerHealthPort)
	}
}

// TestBuildPolicyBrokerContainer_MetricsPortName is a regression guard: the
// broker's health port MUST be named "metrics" (not "broker-health") so the
// omnia-agents scrape job / PodMonitor — which both select every container
// port named "metrics" — pick up the broker with no scrape-config changes,
// exactly like the facade (8081) and runtime (9001) health ports.
func TestBuildPolicyBrokerContainer_MetricsPortName(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	container := buildPolicyBrokerContainer(agentRuntime, "", "")

	port := containerPortByName(&container, metricsPortName)
	if port == nil {
		t.Fatalf("container must declare a %q port", metricsPortName)
	}
	if port.ContainerPort != DefaultPolicyBrokerHealthPort {
		t.Errorf("metrics port number = %d, want %d", port.ContainerPort, DefaultPolicyBrokerHealthPort)
	}
}

// TestBuildPolicyBrokerEnvVars_AgentNameDownwardAPI asserts OMNIA_AGENT_NAME
// is sourced from the downward API (metadata.labels['app.kubernetes.io/instance'])
// rather than a literal value, so the broker's Prometheus "agent" ConstLabel
// matches the facade/runtime containers (which use the same field path).
func TestBuildPolicyBrokerEnvVars_AgentNameDownwardAPI(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "test-ns"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	envVars := buildPolicyBrokerEnvVars(agentRuntime, "")

	var found *corev1.EnvVar
	for i := range envVars {
		if envVars[i].Name == "OMNIA_AGENT_NAME" {
			found = &envVars[i]
			break
		}
	}
	if found == nil {
		t.Fatal("OMNIA_AGENT_NAME env var not found")
	}
	if found.ValueFrom == nil || found.ValueFrom.FieldRef == nil {
		t.Fatal("OMNIA_AGENT_NAME must be sourced via downward API FieldRef")
	}
	if found.ValueFrom.FieldRef.FieldPath != fieldPathInstanceLabel {
		t.Errorf("OMNIA_AGENT_NAME field path = %q, want %q",
			found.ValueFrom.FieldRef.FieldPath, fieldPathInstanceLabel)
	}
}

func TestBuildPolicyBrokerContainer_CustomImage(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	customImage := "my-registry/policy-broker:v1.0"
	container := buildPolicyBrokerContainer(agentRuntime, customImage, "")
	if container.Image != customImage {
		t.Errorf("Image = %q, want %q", container.Image, customImage)
	}
}

func TestBuildPolicyBrokerEnvVars_StampsOperatorAPIURL(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
	}
	const url = "http://omnia-arena-controller.omnia-system:8082"

	hasEnv := func(env []corev1.EnvVar, name string) (string, bool) {
		for _, e := range env {
			if e.Name == name {
				return e.Value, true
			}
		}
		return "", false
	}

	withURL := buildPolicyBrokerEnvVars(agentRuntime, url)
	if v, ok := hasEnv(withURL, "OPERATOR_API_URL"); !ok || v != url {
		t.Errorf("OPERATOR_API_URL = %q, present=%v; want %q", v, ok, url)
	}

	withoutURL := buildPolicyBrokerEnvVars(agentRuntime, "")
	if _, ok := hasEnv(withoutURL, "OPERATOR_API_URL"); ok {
		t.Error("OPERATOR_API_URL must not be stamped when no license URL is set")
	}
}

func TestBuildPolicyBrokerEnvVars(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "test-ns"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	envVars := buildPolicyBrokerEnvVars(agentRuntime, "")

	expectedEnvs := map[string]string{
		"OMNIA_NAMESPACE":           "test-ns",
		"POLICY_BROKER_LISTEN_ADDR": fmt.Sprintf(":%d", DefaultPolicyBrokerPort),
		"POLICY_BROKER_HEALTH_ADDR": fmt.Sprintf(":%d", DefaultPolicyBrokerHealthPort),
	}

	envMap := make(map[string]string)
	for _, env := range envVars {
		envMap[env.Name] = env.Value
	}

	for name, expected := range expectedEnvs {
		if envMap[name] != expected {
			t.Errorf("env %q = %q, want %q", name, envMap[name], expected)
		}
	}

	// The broker must NOT receive any upstream URL — unlike policy-proxy it has
	// no inline proxy path, it only serves decisions to the runtime.
	for _, env := range envVars {
		if env.Name == "POLICY_PROXY_UPSTREAM_URL" || env.Name == "POLICY_BROKER_UPSTREAM_URL" {
			t.Errorf("policy-broker must not have an upstream URL env, found %q", env.Name)
		}
	}
}

func TestBuildPolicyBrokerContainer_Probes(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	container := buildPolicyBrokerContainer(agentRuntime, "", "")

	if container.ReadinessProbe == nil {
		t.Fatal("ReadinessProbe is nil")
	}
	if container.ReadinessProbe.HTTPGet.Path != "/readyz" {
		t.Errorf("readiness path = %q, want %q", container.ReadinessProbe.HTTPGet.Path, "/readyz")
	}

	if container.LivenessProbe == nil {
		t.Fatal("LivenessProbe is nil")
	}
	if container.LivenessProbe.HTTPGet.Path != healthzPath {
		t.Errorf("liveness path = %q, want %q", container.LivenessProbe.HTTPGet.Path, healthzPath)
	}
}

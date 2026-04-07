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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func TestBuildPolicyProxyContainer_DefaultImage(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	container := buildPolicyProxyContainer(agentRuntime, "")
	if container.Name != PolicyProxyContainerName {
		t.Errorf("Name = %q, want %q", container.Name, PolicyProxyContainerName)
	}
	if container.Image != DefaultPolicyProxyImage {
		t.Errorf("Image = %q, want %q", container.Image, DefaultPolicyProxyImage)
	}
	if len(container.Ports) != 2 {
		t.Fatalf("Ports count = %d, want 2", len(container.Ports))
	}
	if container.Ports[0].ContainerPort != DefaultPolicyProxyPort {
		t.Errorf("proxy port = %d, want %d", container.Ports[0].ContainerPort, DefaultPolicyProxyPort)
	}
	if container.Ports[1].ContainerPort != DefaultPolicyProxyHealthPort {
		t.Errorf("health port = %d, want %d", container.Ports[1].ContainerPort, DefaultPolicyProxyHealthPort)
	}
}

func TestBuildPolicyProxyContainer_CustomImage(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	customImage := "my-registry/policy-proxy:v1.0"
	container := buildPolicyProxyContainer(agentRuntime, customImage)
	if container.Image != customImage {
		t.Errorf("Image = %q, want %q", container.Image, customImage)
	}
}

func TestBuildPolicyProxyEnvVars(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	envVars := buildPolicyProxyEnvVars(agentRuntime)

	expectedEnvs := map[string]string{
		"POLICY_PROXY_LISTEN_ADDR":  fmt.Sprintf(":%d", DefaultPolicyProxyPort),
		"POLICY_PROXY_HEALTH_ADDR":  fmt.Sprintf(":%d", DefaultPolicyProxyHealthPort),
		"POLICY_PROXY_UPSTREAM_URL": fmt.Sprintf("http://localhost:%d", DefaultRuntimeGRPCPort),
	}

	envMap := make(map[string]string)
	for _, env := range envVars {
		if env.Value != "" {
			envMap[env.Name] = env.Value
		}
	}

	for name, expected := range expectedEnvs {
		if envMap[name] != expected {
			t.Errorf("env %q = %q, want %q", name, envMap[name], expected)
		}
	}

	foundAgentName := false
	foundNamespace := false
	for _, env := range envVars {
		if env.Name == "OMNIA_AGENT_NAME" && env.ValueFrom != nil {
			foundAgentName = true
		}
		if env.Name == "OMNIA_NAMESPACE" && env.ValueFrom != nil {
			foundNamespace = true
		}
	}
	if !foundAgentName {
		t.Error("missing OMNIA_AGENT_NAME downward API env")
	}
	if !foundNamespace {
		t.Error("missing OMNIA_NAMESPACE downward API env")
	}
}

func TestBuildPolicyProxyContainer_Probes(t *testing.T) {
	agentRuntime := &omniav1alpha1.AgentRuntime{
		ObjectMeta: metav1.ObjectMeta{Name: "test-agent", Namespace: "default"},
		Spec:       omniav1alpha1.AgentRuntimeSpec{},
	}

	container := buildPolicyProxyContainer(agentRuntime, "")

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

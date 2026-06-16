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
	"testing"

	"k8s.io/utils/ptr"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func trafficAR(mode string, istio *omniav1alpha1.IstioTrafficRouting) *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			Rollout: &omniav1alpha1.RolloutConfig{
				TrafficRouting: &omniav1alpha1.TrafficRoutingConfig{Mode: mode, Istio: istio},
			},
		},
	}
}

func TestResolveTrafficMode(t *testing.T) {
	legacyIstio := &omniav1alpha1.IstioTrafficRouting{}
	cases := []struct {
		name          string
		mode          string
		istio         *omniav1alpha1.IstioTrafficRouting
		meshAvailable bool
		want          string
		wantDegraded  bool
	}{
		{"unset+mesh", "", nil, true, TrafficModeMesh, false},
		{"unset+nomesh", "", nil, false, TrafficModeReplicaWeighted, false},
		{"legacy-istio-unset-mode", "", legacyIstio, false, TrafficModeExternal, false},
		{"mesh+available", TrafficModeMesh, nil, true, TrafficModeMesh, false},
		{"mesh+unavailable-degrades", TrafficModeMesh, nil, false, TrafficModeReplicaWeighted, true},
		{"explicit-replica", TrafficModeReplicaWeighted, nil, false, TrafficModeReplicaWeighted, false},
		{"explicit-external", TrafficModeExternal, nil, false, TrafficModeExternal, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, degraded := resolveTrafficModeFor(trafficAR(tc.mode, tc.istio).Spec.Rollout.TrafficRouting, tc.meshAvailable)
			if got != tc.want || degraded != tc.wantDegraded {
				t.Fatalf("got (%q,%v) want (%q,%v)", got, degraded, tc.want, tc.wantDegraded)
			}
		})
	}
}

func TestMeshWaypointForResolved(t *testing.T) {
	meshAR := func(mode, waypoint string, hasMesh bool) *omniav1alpha1.AgentRuntime {
		tr := &omniav1alpha1.TrafficRoutingConfig{Mode: mode}
		if hasMesh {
			tr.Mesh = &omniav1alpha1.MeshTrafficRouting{Waypoint: waypoint}
		}
		return &omniav1alpha1.AgentRuntime{
			Spec: omniav1alpha1.AgentRuntimeSpec{
				Rollout: &omniav1alpha1.RolloutConfig{TrafficRouting: tr},
			},
		}
	}
	cases := []struct {
		name          string
		ar            *omniav1alpha1.AgentRuntime
		meshAvailable bool
		want          string
	}{
		{"mesh+waypoint+available", meshAR(TrafficModeMesh, "wp", true), true, "wp"},
		{"mesh+waypoint+unavailable-degrades", meshAR(TrafficModeMesh, "wp", true), false, ""},
		{"mesh+no-waypoint", meshAR(TrafficModeMesh, "", true), true, ""},
		{"replica+waypoint", meshAR(TrafficModeReplicaWeighted, "wp", true), true, ""},
		{"mesh-mode-no-mesh-block", meshAR(TrafficModeMesh, "", false), true, ""},
		{"no-rollout", &omniav1alpha1.AgentRuntime{}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := meshWaypointForResolved(tc.ar, tc.meshAvailable); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// activeTrafficAR builds an AgentRuntime with an active rollout (a candidate
// that differs from the stable spec) and the given traffic-routing config.
// tr=nil means no trafficRouting block.
func activeTrafficAR(tr *omniav1alpha1.TrafficRoutingConfig) *omniav1alpha1.AgentRuntime {
	return &omniav1alpha1.AgentRuntime{
		Spec: omniav1alpha1.AgentRuntimeSpec{
			// stable version empty → candidate "v2" differs → rollout active.
			PromptPackRef: omniav1alpha1.PromptPackRef{Name: "p"},
			Rollout: &omniav1alpha1.RolloutConfig{
				Candidate:      &omniav1alpha1.CandidateOverrides{PromptPackRef: &omniav1alpha1.PromptPackRef{Name: "p", Version: ptr.To("v2")}},
				Steps:          []omniav1alpha1.RolloutStep{{SetWeight: ptr.To[int32](50)}},
				TrafficRouting: tr,
			},
		},
	}
}

func tr(mode string) *omniav1alpha1.TrafficRoutingConfig {
	return &omniav1alpha1.TrafficRoutingConfig{Mode: mode}
}

func TestIsReplicaWeightedActive(t *testing.T) {
	// Inactive rollout: candidate matches stable (no diff) → not active.
	inactive := activeTrafficAR(tr(TrafficModeReplicaWeighted))
	inactive.Spec.Rollout.Candidate.PromptPackRef = &omniav1alpha1.PromptPackRef{Name: "p"}

	// No rollout at all.
	noRollout := &omniav1alpha1.AgentRuntime{}

	cases := []struct {
		name          string
		ar            *omniav1alpha1.AgentRuntime
		meshAvailable bool
		want          bool
	}{
		{"no-rollout", noRollout, false, false},
		{"inactive-candidate", inactive, false, false},
		{"active-no-trafficRouting", activeTrafficAR(nil), false, false},
		{"active-explicit-replica-nomesh", activeTrafficAR(tr(TrafficModeReplicaWeighted)), false, true},
		{"active-explicit-replica-meshavail", activeTrafficAR(tr(TrafficModeReplicaWeighted)), true, true},
		{"active-mesh-available", activeTrafficAR(tr(TrafficModeMesh)), true, false},
		{"active-mesh-unavailable-degrades", activeTrafficAR(tr(TrafficModeMesh)), false, true},
		{"active-external", activeTrafficAR(tr(TrafficModeExternal)), false, false},
		{"active-unset-mode-nomesh", activeTrafficAR(tr("")), false, true},
		{"active-unset-mode-mesh", activeTrafficAR(tr("")), true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isReplicaWeightedActive(tc.ar, tc.meshAvailable); got != tc.want {
				t.Fatalf("isReplicaWeightedActive(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

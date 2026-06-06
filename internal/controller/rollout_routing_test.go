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

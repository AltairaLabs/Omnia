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

package v1alpha1

import "testing"

func TestMeshTrafficRouting_DeepCopy(t *testing.T) {
	in := &TrafficRoutingConfig{
		Mode: "mesh",
		Mesh: &MeshTrafficRouting{
			Hosts:           []string{"agent.ns.svc.cluster.local"},
			StableSubset:    "stable",
			CandidateSubset: "canary",
		},
	}
	out := in.DeepCopy()
	if out.Mesh == in.Mesh {
		t.Fatal("DeepCopy must allocate a new Mesh pointer")
	}
	if out.Mesh.Hosts[0] != "agent.ns.svc.cluster.local" {
		t.Fatalf("hosts not copied: %v", out.Mesh.Hosts)
	}
}

func TestRolloutStatus_EnforcedRoundTrips(t *testing.T) {
	enforced := false
	s := &RolloutStatus{TrafficWeightEnforced: &enforced, TrafficRoutingMode: "replicaWeighted"}
	out := s.DeepCopy()
	if out.TrafficWeightEnforced == s.TrafficWeightEnforced {
		t.Fatal("DeepCopy must allocate a new bool pointer")
	}
	if *out.TrafficWeightEnforced != false || out.TrafficRoutingMode != "replicaWeighted" {
		t.Fatal("fields not copied")
	}
}

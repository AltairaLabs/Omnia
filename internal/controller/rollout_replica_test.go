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

import "testing"

func TestReplicaSplit(t *testing.T) {
	cases := []struct {
		total, weight             int32
		wantCand, wantStable, del int32
	}{
		{10, 10, 1, 9, 10}, // exact at 10 replicas
		{10, 50, 5, 5, 50},
		{10, 100, 10, 0, 100},
		{10, 0, 0, 10, 0},
		{4, 50, 2, 2, 50},
		{4, 10, 0, 4, 0},   // rounds down to 0 → delivered 0, approximate
		{3, 50, 2, 1, 67},  // round(1.5)=2 → delivered 67, approximate
		{1, 50, 1, 0, 100}, // total<2 → binary, delivered 100
	}
	for _, tc := range cases {
		cand, stable, delivered := replicaSplit(tc.total, tc.weight)
		if cand != tc.wantCand || stable != tc.wantStable || delivered != tc.del {
			t.Errorf("replicaSplit(%d,%d) = (%d,%d,%d) want (%d,%d,%d)",
				tc.total, tc.weight, cand, stable, delivered, tc.wantCand, tc.wantStable, tc.del)
		}
	}
}

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

package facade

import "github.com/altairalabs/omnia/pkg/policy"

// identityScope resolves the origin and workspace to propagate for policy
// evaluation (#1769). Origin comes from the validator that admitted the
// request. Workspace prefers the token's own workspace scope (set by
// workspace-scoped validators such as the management plane) and falls back to
// the agent's deployed workspace, so identity.workspace ToolPolicy rules see a
// non-empty value for every validator style.
func identityScope(id *policy.AuthenticatedIdentity, agentWorkspace string) (origin, workspace string) {
	if id != nil {
		origin = id.Origin
		workspace = id.Workspace
	}
	if workspace == "" {
		workspace = agentWorkspace
	}
	return origin, workspace
}

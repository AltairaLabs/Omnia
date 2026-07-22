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

package runtime

import "github.com/altairalabs/omnia/pkg/runtime/contract"

// Capabilities returns the contract capabilities this (OOTB) runtime implements.
// A custom runtime built on pkg/runtime overrides this to advertise its own
// subset; a legacy runtime advertises nothing and is flagged as pre-negotiation.
func Capabilities() []string {
	return contract.KnownCapabilities()
}

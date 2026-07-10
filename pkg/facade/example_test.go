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

package facade_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"

	"github.com/altairalabs/omnia/pkg/facade"
	"github.com/altairalabs/omnia/pkg/facade/auth"
)

// Example shows the minimal boilerplate to stand up a conformant custom
// facade with the pkg/facade SDK: build the data-plane auth chain, wrap the
// protocol handler with Authenticate, translate the admitted identity into
// propagation fields, and emit them as outbound metadata to the runtime. A
// SessionRecorder (elided here) would record the exchange to session-api.
func Example() {
	log := logr.Discard()

	// 1. Data-plane auth chain. Real facades assemble validators from the
	//    AgentRuntime's spec.externalAuth; here a single shared-token
	//    validator stands in.
	shared, _ := auth.NewSharedTokenValidator("s3cr3t-shared-token")
	chain := facade.Chain{shared}

	// 2. Session recorder (one per process, shared across connections).
	_ = facade.NewSessionRecorder("http://session-api:8080", log)

	// 3. The agent's coordinates, propagated on every request.
	scope := facade.IdentityScope{AgentName: "my-agent", Namespace: "team-a", Workspace: "team-a"}

	// 4. Wrap the protocol handler with the auth middleware. On admit the
	//    handler sees the identity and forwards X-Omnia-* metadata to the
	//    runtime.
	handler := facade.Authenticate(chain, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := facade.IdentityFromContext(r.Context())
		ctx := facade.PropagateIdentity(r.Context(), id, scope)
		md := facade.OutboundMetadata(ctx)
		fmt.Printf("admitted origin=%s workspace=%s\n", id.Origin, md["x-omnia-workspace"])
		w.WriteHeader(http.StatusOK)
	}), facade.WithLogger(log))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t-shared-token")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	// Output: admitted origin=shared-token workspace=team-a
}

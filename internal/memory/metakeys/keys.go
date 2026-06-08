/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

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

// Package metakeys defines the well-known Memory.Metadata keys that form the
// contract between memory writers (the runtime's memory__remember overrides,
// document ingestion) and the memory-api write path that reads them. It
// imports nothing so any package can depend on it without an import cycle —
// internal/memory re-exports these as MetaKey* and internal/runtime consumes
// them directly when extending the memory tool schema.
package metakeys

const (
	// Purpose tags why a memory is stored (e.g. "support_continuity",
	// "personalisation"); written to memory_entities.purpose.
	Purpose = "purpose"

	// ConsentCategory tags the consent category (e.g. "memory:health");
	// written to memory_entities.consent_category for the revocation cascade.
	ConsentCategory = "consent_category"

	// AboutKind / AboutKey carry the structured-dedup hint. When both are
	// set, Save treats (workspace, user, agent, about_kind, about_key) as a
	// soft-unique key so a second write supersedes the first in place.
	AboutKind = "about_kind"
	AboutKey  = "about_key"

	// Title / Summary carry display fields for large memories so recall can
	// return a synopsis instead of the full body.
	Title   = "title"
	Summary = "summary"

	// BodySize is the server-stamped octet length of the active observation's
	// content, surfaced so the API DTO can decide inline-vs-preview.
	BodySize = "body_size_bytes"
)

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

package ingestion

import "context"

// SourceDoc is one extracted source document handed to a strategy.
type SourceDoc struct {
	Title string
	URL   string
	Site  string
	Text  string
}

// Item is one embeddable memory item a strategy emits for a document.
// Metadata carries citation + idempotency keys: title, url, site, kind
// ("chunk"|"summary"), index.
type Item struct {
	Content  string
	Metadata map[string]any
}

// IngestionStrategy maps one source document to the embeddable items that
// represent it in the index. It is the seam that supports the chunk↔summary
// spectrum; callers select a strategy per workspace/agent.
type IngestionStrategy interface {
	Ingest(ctx context.Context, doc SourceDoc) ([]Item, error)
}

// baseMetadata builds the citation/idempotency metadata common to every item.
func baseMetadata(doc SourceDoc, kind string, index int) map[string]any {
	return map[string]any{
		"title": doc.Title,
		"url":   doc.URL,
		"site":  doc.Site,
		"kind":  kind,
		"index": index,
	}
}

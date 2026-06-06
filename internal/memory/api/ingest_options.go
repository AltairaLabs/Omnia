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

package api

import "github.com/altairalabs/omnia/internal/memory/ingestion"

// IngestOptions bundles the ingestion wiring passed into buildAPIMux: the
// flag-derived fallback config and the optional async summary queue (nil
// disables the agent path).
type IngestOptions struct {
	Fallback ingestion.Config
	Queue    ingestion.SummaryQueue
}

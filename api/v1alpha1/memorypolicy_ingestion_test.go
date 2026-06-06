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

const testSummarizerAgent = "agent"

func TestIngestionAccessors_NilSafe(t *testing.T) {
	var p *MemoryPolicy
	if p.IngestionStrategy() != "" || p.IngestionSummarizer() != "" {
		t.Fatal("nil policy should return empty ingestion fields")
	}
	if _, _, ok := p.IngestionChunk(); ok {
		t.Fatal("nil policy should report chunk unset")
	}
}

func TestIngestionAccessors_Unset(t *testing.T) {
	p := &MemoryPolicy{}
	if p.IngestionStrategy() != "" || p.IngestionSummarizer() != "" {
		t.Fatal("unset ingestion should return empty strings")
	}
	if _, _, ok := p.IngestionChunk(); ok {
		t.Fatal("unset chunk should report unset")
	}
}

func TestIngestionAccessors_Set(t *testing.T) {
	p := &MemoryPolicy{Spec: MemoryPolicySpec{Ingestion: &MemoryIngestionConfig{
		Strategy:   "summaryThenChunk",
		Summarizer: testSummarizerAgent,
		Chunk:      &MemoryChunkConfig{Size: 120, Overlap: 20},
	}}}
	if p.IngestionStrategy() != "summaryThenChunk" {
		t.Fatalf("strategy: got %q", p.IngestionStrategy())
	}
	if p.IngestionSummarizer() != testSummarizerAgent {
		t.Fatalf("summarizer: got %q", p.IngestionSummarizer())
	}
	size, overlap, ok := p.IngestionChunk()
	if !ok || size != 120 || overlap != 20 {
		t.Fatalf("chunk: got (%d,%d,%v)", size, overlap, ok)
	}
}

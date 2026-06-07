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

package main

import "testing"

func TestFrameworkImagesFlag_Set(t *testing.T) {
	var f frameworkImagesFlag
	// typed entry
	if err := f.Set("langchain=ghcr.io/altairalabs/omnia-langchain-runtime:v1"); err != nil {
		t.Fatalf("typed set: %v", err)
	}
	// bare entry (legacy) -> promptkit, and colon in repo:tag preserved
	if err := f.Set("ghcr.io/altairalabs/omnia-runtime:v1"); err != nil {
		t.Fatalf("bare set: %v", err)
	}
	m := f.images()
	if m["langchain"] != "ghcr.io/altairalabs/omnia-langchain-runtime:v1" {
		t.Fatalf("langchain: %q", m["langchain"])
	}
	if m["promptkit"] != "ghcr.io/altairalabs/omnia-runtime:v1" {
		t.Fatalf("bare must map to promptkit: %q", m["promptkit"])
	}
}

func TestFrameworkImagesFlag_EmptyValueRejected(t *testing.T) {
	var f frameworkImagesFlag
	if err := f.Set(""); err == nil {
		t.Fatal("empty value must error")
	}
	if err := f.Set("langchain="); err == nil {
		t.Fatal("empty image must error")
	}
}

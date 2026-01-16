/*
Copyright 2025.

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

import "strings"

// Mock scenario metadata key.
const (
	MetadataKeyMockScenario = "mock_scenario"
	MetadataKeyContentType  = "content_type"
)

// Default scenario identifiers.
const (
	ScenarioDefault       = "default"
	ScenarioImageAnalysis = "image-analysis"
	ScenarioAudioAnalysis = "audio-analysis"
	ScenarioDocumentQA    = "document-qa"
)

// extractMockScenario determines the mock scenario to use based on message metadata
// and content analysis. Priority:
// 1. Explicit mock_scenario in metadata
// 2. Auto-detection based on content_type metadata
// 3. Auto-detection based on content patterns
// 4. Default scenario
func extractMockScenario(metadata map[string]string, content string) string {
	// Check for explicit scenario in metadata
	if scenario, ok := metadata[MetadataKeyMockScenario]; ok && scenario != "" {
		return scenario
	}

	// Auto-detect based on content_type metadata
	if contentType, ok := metadata[MetadataKeyContentType]; ok {
		if scenario := detectScenarioFromContentType(contentType); scenario != "" {
			return scenario
		}
	}

	// Auto-detect based on content patterns
	if scenario := detectScenarioFromContent(content); scenario != "" {
		return scenario
	}

	return ScenarioDefault
}

// detectScenarioFromContentType maps content types to scenarios.
func detectScenarioFromContentType(contentType string) string {
	switch {
	case isImageContentType(contentType):
		return ScenarioImageAnalysis
	case isAudioContentType(contentType):
		return ScenarioAudioAnalysis
	case isDocumentContentType(contentType):
		return ScenarioDocumentQA
	default:
		return ""
	}
}

// detectScenarioFromContent analyzes content to detect scenarios.
// This is a fallback when metadata doesn't specify the content type.
func detectScenarioFromContent(content string) string {
	// Check for common patterns indicating multi-modal content
	// These patterns might appear in content when referencing uploaded media
	patterns := map[string]string{
		"[image:":    ScenarioImageAnalysis,
		"[audio:":    ScenarioAudioAnalysis,
		"[document:": ScenarioDocumentQA,
		"[pdf:":      ScenarioDocumentQA,
	}

	for pattern, scenario := range patterns {
		if containsPattern(content, pattern) {
			return scenario
		}
	}

	return ""
}

// isImageContentType checks if a content type represents an image.
func isImageContentType(contentType string) bool {
	imageTypes := []string{"image/", "png", "jpg", "jpeg", "gif", "webp", "svg"}
	for _, t := range imageTypes {
		if containsPattern(contentType, t) {
			return true
		}
	}
	return false
}

// isAudioContentType checks if a content type represents audio.
func isAudioContentType(contentType string) bool {
	audioTypes := []string{"audio/", "mp3", "wav", "ogg", "m4a", "flac"}
	for _, t := range audioTypes {
		if containsPattern(contentType, t) {
			return true
		}
	}
	return false
}

// isDocumentContentType checks if a content type represents a document.
func isDocumentContentType(contentType string) bool {
	docTypes := []string{"application/pdf", "pdf", "document", "text/"}
	for _, t := range docTypes {
		if containsPattern(contentType, t) {
			return true
		}
	}
	return false
}

// containsPattern performs case-insensitive substring check.
func containsPattern(s, pattern string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(pattern))
}

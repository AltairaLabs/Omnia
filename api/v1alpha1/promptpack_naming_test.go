package v1alpha1

import "testing"

func TestPromptPackObjectName(t *testing.T) {
	// Deterministic: same coordinate -> same name.
	a := PromptPackObjectName("mypack", "1.2.3")
	b := PromptPackObjectName("mypack", "1.2.3")
	if a != b {
		t.Fatalf("expected deterministic name, got %q and %q", a, b)
	}
	// Distinct coordinates -> distinct names.
	if PromptPackObjectName("mypack", "1.2.3") == PromptPackObjectName("mypack", "1.2.4") {
		t.Fatal("version must affect the name")
	}
	if PromptPackObjectName("mypack", "1.2.3") == PromptPackObjectName("other", "1.2.3") {
		t.Fatal("packName must affect the name")
	}
	// Shape: pp- prefix, 12 hex chars, DNS-1123 safe.
	if got := PromptPackObjectName("mypack", "1.2.3"); len(got) != 15 || got[:3] != "pp-" {
		t.Fatalf("unexpected name shape: %q", got)
	}
	// Prerelease is a distinct coordinate from its release.
	if PromptPackObjectName("mypack", "1.2.4-beta.1") == PromptPackObjectName("mypack", "1.2.4") {
		t.Fatal("prerelease must be a distinct name")
	}
}

package controller

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

func pack(name, version string) omniav1alpha1.PromptPack {
	return omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       omniav1alpha1.PromptPackSpec{PackName: "mypack", Version: version},
	}
}
func strptr(s string) *string { return &s }

func TestSelectPromptPack(t *testing.T) {
	set := []omniav1alpha1.PromptPack{
		pack("a", "1.2.0"), pack("b", "1.3.0-beta.1"), pack("c", "1.2.5"), pack("d", "1.3.0"),
	}
	cases := []struct {
		name           string
		version, track *string
		wantVersion    string
		wantErr        bool
	}{
		{"stable channel picks highest non-prerelease", nil, strptr("stable"), "1.3.0", false},
		{"prerelease channel picks highest overall", nil, strptr("prerelease"), "1.3.0", false}, // 1.3.0 > 1.3.0-beta.1
		{"exact pin", strptr("1.2.5"), nil, "1.2.5", false},
		{"exact pin miss -> error", strptr("9.9.9"), nil, "", true},
		{"both set -> error", strptr("1.2.5"), strptr("stable"), "", true},
		{"neither set -> defaults to stable channel", nil, nil, "1.3.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectPromptPack(set, tc.version, tc.track)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Spec.Version != tc.wantVersion {
				t.Fatalf("got %s want %s", got.Spec.Version, tc.wantVersion)
			}
		})
	}
}

// TestSelectPromptPack_ExactPin_VPrefix proves the exact-version match is
// semver-aware (not raw string ==): the CRD's spec.version pattern allows an
// optional leading "v", so a pack stored as "v1.5.0" must match a pin of
// "1.5.0" (#1837 fix pass).
func TestSelectPromptPack_ExactPin_VPrefix(t *testing.T) {
	set := []omniav1alpha1.PromptPack{pack("a", "v1.5.0")}
	got, err := selectPromptPack(set, strptr("1.5.0"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Spec.Version != "v1.5.0" {
		t.Fatalf("got %s want v1.5.0", got.Spec.Version)
	}
}

func TestSelectPromptPack_prereleaseOnlySet(t *testing.T) {
	// stable channel with only prereleases available -> no match (error), not a prerelease.
	set := []omniav1alpha1.PromptPack{pack("a", "1.0.0-rc.1"), pack("b", "1.0.0-rc.2")}
	if _, err := selectPromptPack(set, nil, strptr("stable")); err == nil {
		t.Fatal("stable channel must not select a prerelease")
	}
	got, err := selectPromptPack(set, nil, strptr("prerelease"))
	if err != nil || got.Spec.Version != "1.0.0-rc.2" {
		t.Fatalf("got %v err %v", got, err)
	}
}

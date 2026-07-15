package packselect

import (
	"errors"
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

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"1.2.3", false},
		{"v1.2.3", false}, // leading v tolerated
		{"1.2.3-beta.1", false},
		{"1.2.3+build.7", false},
		{"v1", true}, // strict: incomplete rejected
		{"1", true},
		{"1.2", true},
		{"not-a-version", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			_, err := ParseVersion(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseVersion(%q) err=%v wantErr=%v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestVersionsEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"identical strings", "1.2.3", "1.2.3", true},
		{"v-prefix tolerated", "v1.5.0", "1.5.0", true},
		{"build metadata ignored", "1.2.3+build.7", "1.2.3+other", true},
		{"different versions", "1.2.3", "1.2.4", false},
		{"prerelease differs from release", "1.0.0-rc.1", "1.0.0", false},
		{"unparseable but string-equal", "draft", "draft", true},
		{"unparseable and unequal", "draft", "final", false},
		{"one parseable one not", "1.0.0", "latest", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := VersionsEqual(tc.a, tc.b); got != tc.want {
				t.Fatalf("VersionsEqual(%q,%q)=%v want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestChannelMax(t *testing.T) {
	set := []omniav1alpha1.PromptPack{
		pack("a", "1.0.0"), pack("b", "1.1.0"), pack("c", "2.0.0-beta.1"),
	}

	t.Run("stable excludes prerelease", func(t *testing.T) {
		got, err := ChannelMax(set, TrackStable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Spec.Version != "1.1.0" {
			t.Fatalf("got %s want 1.1.0", got.Spec.Version)
		}
	})

	t.Run("prerelease picks highest overall", func(t *testing.T) {
		got, err := ChannelMax(set, TrackPrerelease)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Spec.Version != "2.0.0-beta.1" {
			t.Fatalf("got %s want 2.0.0-beta.1", got.Spec.Version)
		}
	})

	t.Run("stable with only prereleases errors", func(t *testing.T) {
		only := []omniav1alpha1.PromptPack{pack("a", "1.0.0-rc.1"), pack("b", "1.0.0-rc.2")}
		_, err := ChannelMax(only, TrackStable)
		if err == nil {
			t.Fatal("expected error for stable channel with only prereleases")
		}
		if !errors.Is(err, ErrNoMatchingPromptPack) {
			t.Fatalf("error should wrap ErrNoMatchingPromptPack: %v", err)
		}
	})

	t.Run("empty set errors", func(t *testing.T) {
		_, err := ChannelMax(nil, TrackStable)
		if !errors.Is(err, ErrNoMatchingPromptPack) {
			t.Fatalf("empty set should wrap ErrNoMatchingPromptPack: %v", err)
		}
	})

	t.Run("unparseable versions skipped", func(t *testing.T) {
		mixed := []omniav1alpha1.PromptPack{pack("a", "garbage"), pack("b", "1.4.0")}
		got, err := ChannelMax(mixed, TrackStable)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Spec.Version != "1.4.0" {
			t.Fatalf("got %s want 1.4.0", got.Spec.Version)
		}
	})
}

func TestSelect(t *testing.T) {
	set := []omniav1alpha1.PromptPack{
		pack("a", "1.2.0"), pack("b", "1.3.0-beta.1"), pack("c", "1.2.5"), pack("d", "1.3.0"),
	}
	cases := []struct {
		name           string
		version, track *string
		wantVersion    string
		wantErr        bool
	}{
		{"exact pin", strptr("1.2.5"), nil, "1.2.5", false},
		{"exact pin v-prefix match", strptr("1.3.0"), nil, "1.3.0", false},
		{"exact pin miss errors", strptr("9.9.9"), nil, "", true},
		{"stable channel", nil, strptr("stable"), "1.3.0", false},
		{"prerelease channel picks highest overall", nil, strptr("prerelease"), "1.3.0", false},
		{"neither -> defaults to stable", nil, nil, "1.3.0", false},
		{"empty version string -> stable channel", strptr(""), nil, "1.3.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Select(set, tc.version, tc.track)
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

func TestSelect_ExactPinVPrefix(t *testing.T) {
	set := []omniav1alpha1.PromptPack{pack("a", "v1.5.0")}
	got, err := Select(set, strptr("1.5.0"), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Spec.Version != "v1.5.0" {
		t.Fatalf("got %s want v1.5.0", got.Spec.Version)
	}
}

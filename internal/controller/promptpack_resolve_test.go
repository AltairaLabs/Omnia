package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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

// labeledPack builds a PromptPack version-object labeled LabelPromptPackName
// so it is discoverable by latestPackForChannel's List+label-selector, as real
// PromptPack version-objects are (promptpack_controller.go's label reconcile).
func labeledPack(objName, packName, version string, namespace string) *omniav1alpha1.PromptPack {
	return &omniav1alpha1.PromptPack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objName,
			Namespace: namespace,
			Labels:    map[string]string{LabelPromptPackName: packName},
		},
		Spec: omniav1alpha1.PromptPackSpec{PackName: packName, Version: version},
	}
}

// TestLatestPackForChannel_StableSkipsPrerelease seeds two stable versions of
// the same pack (labeled, distinct object names+namespace-scoped like real
// version-objects) and asserts the stable channel resolves the highest
// version, not the first- or last-created object.
func TestLatestPackForChannel_StableSkipsPrerelease(t *testing.T) {
	scheme := newTestScheme(t)
	ns := "default"
	p1 := labeledPack("mypack-100", "mypack", "1.0.0", ns)
	p2 := labeledPack("mypack-110", "mypack", "1.1.0", ns)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(p1, p2).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	got, err := r.latestPackForChannel(context.Background(), ns, "mypack", "stable")
	require.NoError(t, err)
	assert.Equal(t, "1.1.0", got.Spec.Version)
}

// TestLatestPackForChannel_MixedTracks seeds a stable and a newer prerelease
// version of the same pack: the stable channel must skip the prerelease and
// resolve the stable, while the prerelease channel resolves the overall
// newest (matching channelMax semantics, #1837).
func TestLatestPackForChannel_MixedTracks(t *testing.T) {
	scheme := newTestScheme(t)
	ns := "default"
	stable := labeledPack("mypack-100", "mypack", "1.0.0", ns)
	pre := labeledPack("mypack-110-beta", "mypack", "1.1.0-beta.1", ns)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(stable, pre).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	gotStable, err := r.latestPackForChannel(context.Background(), ns, "mypack", "stable")
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", gotStable.Spec.Version)

	gotPre, err := r.latestPackForChannel(context.Background(), ns, "mypack", "prerelease")
	require.NoError(t, err)
	assert.Equal(t, "1.1.0-beta.1", gotPre.Spec.Version)
}

// TestLatestPackForChannel_NoMatch wraps errNoMatchingPromptPack when the
// channel is empty (no candidates for the packName at all).
func TestLatestPackForChannel_NoMatch(t *testing.T) {
	scheme := newTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &AgentRuntimeReconciler{Client: c, Scheme: scheme}

	_, err := r.latestPackForChannel(context.Background(), "default", "mypack", "stable")
	require.Error(t, err)
	assert.ErrorIs(t, err, errNoMatchingPromptPack)
}

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	promptPackTrackStable     = "stable"
	promptPackTrackPrerelease = "prerelease"
)

var errNoMatchingPromptPack = errors.New("no PromptPack matches the ref")

// selectPromptPack picks one PromptPack from candidates (all sharing a packName)
// by exact version pin or by channel-max. At most one of version/track may be set;
// if neither is set, defaults to the stable channel.
func selectPromptPack(candidates []omniav1alpha1.PromptPack, version, track *string) (*omniav1alpha1.PromptPack, error) {
	hasVersion := version != nil && *version != ""
	hasTrack := track != nil && *track != ""
	if hasVersion && hasTrack { // both set is an error
		return nil, fmt.Errorf("exactly one of promptPackRef.version or promptPackRef.track must be set")
	}
	if hasVersion {
		for i := range candidates {
			if versionsEqual(candidates[i].Spec.Version, *version) {
				return &candidates[i], nil
			}
		}
		return nil, fmt.Errorf("%w: version %q", errNoMatchingPromptPack, *version)
	}
	// track is set, or neither is set (default to stable)
	if hasTrack {
		return channelMax(candidates, *track)
	}
	return channelMax(candidates, promptPackTrackStable)
}

// parsePackVersion parses a PromptPack spec.version (CRD pattern allows a leading
// "v"). Stripping the "v" before StrictNewVersion accepts full v-prefixed semver
// (v1.5.0 == 1.5.0) while still rejecting incomplete values like "v1"/"1" (which
// then fall back to string equality at call sites) — avoiding lenient coercion.
func parsePackVersion(s string) (*semver.Version, error) {
	return semver.StrictNewVersion(strings.TrimPrefix(s, "v"))
}

// latestPackForChannel resolves the highest version of packName published on
// channel (stable/prerelease), in namespace. PromptPacks are label-keyed
// (LabelPromptPackName=packName, one object per version, per
// resolvePromptPack) rather than name-keyed, so this lists the candidates
// sharing that label and delegates to channelMax for the version comparison.
// Used by the version-triggered rollout (maybeTriggerVersionRollout) to watch
// a channel independently of the agent's pinned stable ref.
func (r *AgentRuntimeReconciler) latestPackForChannel(ctx context.Context, namespace, packName, channel string) (*omniav1alpha1.PromptPack, error) {
	var list omniav1alpha1.PromptPackList
	if err := r.List(ctx, &list,
		client.InNamespace(namespace),
		client.MatchingLabels{LabelPromptPackName: packName},
	); err != nil {
		return nil, fmt.Errorf("failed to list PromptPacks for %q: %w", packName, err)
	}
	return channelMax(list.Items, channel)
}

func channelMax(candidates []omniav1alpha1.PromptPack, track string) (*omniav1alpha1.PromptPack, error) {
	var best *omniav1alpha1.PromptPack
	var bestV *semver.Version
	for i := range candidates {
		v, err := parsePackVersion(candidates[i].Spec.Version)
		if err != nil {
			continue // skip unparseable; spec.version is semver-validated at the CRD, defensive here
		}
		if track == promptPackTrackStable && v.Prerelease() != "" {
			continue
		}
		if bestV == nil || v.GreaterThan(bestV) {
			best, bestV = &candidates[i], v
		}
	}
	if best == nil {
		return nil, fmt.Errorf("%w: no version in channel %q", errNoMatchingPromptPack, track)
	}
	return best, nil
}

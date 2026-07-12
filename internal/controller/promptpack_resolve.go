package controller

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

const (
	promptPackTrackStable     = "stable"
	promptPackTrackPrerelease = "prerelease"
)

var errNoMatchingPromptPack = errors.New("no PromptPack matches the ref")

// selectPromptPack picks one PromptPack from candidates (all sharing a packName)
// by exact version pin or by channel-max. Exactly one of version/track must be set.
func selectPromptPack(candidates []omniav1alpha1.PromptPack, version, track *string) (*omniav1alpha1.PromptPack, error) {
	hasVersion := version != nil && *version != ""
	hasTrack := track != nil && *track != ""
	if hasVersion == hasTrack { // both or neither
		return nil, fmt.Errorf("exactly one of promptPackRef.version or promptPackRef.track must be set")
	}
	if hasVersion {
		for i := range candidates {
			if candidates[i].Spec.Version == *version {
				return &candidates[i], nil
			}
		}
		return nil, fmt.Errorf("%w: version %q", errNoMatchingPromptPack, *version)
	}
	return channelMax(candidates, *track)
}

func channelMax(candidates []omniav1alpha1.PromptPack, track string) (*omniav1alpha1.PromptPack, error) {
	var best *omniav1alpha1.PromptPack
	var bestV *semver.Version
	for i := range candidates {
		v, err := semver.NewVersion(candidates[i].Spec.Version)
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

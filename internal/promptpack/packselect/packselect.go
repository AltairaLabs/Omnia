// Package packselect is the single source of truth for selecting a concrete
// PromptPack version-object from a set of candidates sharing a logical packName.
//
// PromptPacks are label-keyed: every version-object carries the Label
// (omnia.altairalabs.ai/promptpack=<packName>) and metadata.name is a
// deterministic pp-<hash> that never equals the packName (Phase 1, #1836).
// Callers list the candidates by that label and then use this package to pick
// one by exact version pin or by channel-max (stable/prerelease).
//
// This package is a leaf: it imports only semver and api/v1alpha1 so both the
// core controller (internal/controller), the content resolver
// (internal/promptpack), and the EE GC controller (ee/internal/controller) can
// share it without an import cycle.
package packselect

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
)

// ErrNoMatchingPromptPack is the sentinel returned (wrapped) when no candidate
// matches an exact version pin or a channel. Callers detect it with errors.Is
// to distinguish "no such PromptPack" from transient list/parse failures.
var ErrNoMatchingPromptPack = errors.New("no PromptPack matches the ref")

// Label is the label key the operator stamps on every PromptPack version-object
// with its logical spec.packName. It is the single source of truth for the
// resolution label — its value must never change, only its home.
const Label = "omnia.altairalabs.ai/promptpack"

const (
	// TrackStable is the stable release channel; the stable track excludes
	// prereleases from channel-max selection.
	TrackStable = "stable"
	// TrackPrerelease is the prerelease release channel; it selects the highest
	// version overall, including prereleases.
	TrackPrerelease = "prerelease"
)

// ParseVersion parses a PromptPack spec.version (the CRD pattern allows a
// leading "v"). Stripping the "v" before StrictNewVersion accepts full
// v-prefixed semver (v1.5.0 == 1.5.0) while still rejecting incomplete values
// like "v1"/"1" (which then fall back to string equality at call sites) —
// avoiding lenient coercion.
func ParseVersion(s string) (*semver.Version, error) {
	return semver.StrictNewVersion(strings.TrimPrefix(s, "v"))
}

// VersionsEqual reports whether two spec.version strings are semantically equal,
// tolerating a leading "v" on either side via strict semver parsing and ignoring
// build metadata per semver semantics. When either side fails to parse as strict
// semver it falls back to raw string equality — defensive, since these values
// aren't guaranteed to be strict semver at every layer.
func VersionsEqual(a, b string) bool {
	if a == b {
		return true
	}
	av, aErr := ParseVersion(a)
	bv, bErr := ParseVersion(b)
	if aErr != nil || bErr != nil {
		return a == b
	}
	return av.Equal(bv)
}

// ChannelMax returns the highest-version candidate published on track: the
// highest version overall for the prerelease track, or the highest version with
// no prerelease identifier for the stable track. Unparseable versions are
// skipped (spec.version is semver-validated at the CRD; defensive here). Returns
// an error when no candidate qualifies.
func ChannelMax(packs []omniav1alpha1.PromptPack, track string) (*omniav1alpha1.PromptPack, error) {
	var best *omniav1alpha1.PromptPack
	var bestV *semver.Version
	for i := range packs {
		v, err := ParseVersion(packs[i].Spec.Version)
		if err != nil {
			continue // skip unparseable
		}
		if track == TrackStable && v.Prerelease() != "" {
			continue // stable channel excludes prereleases
		}
		if bestV == nil || v.GreaterThan(bestV) {
			best, bestV = &packs[i], v
		}
	}
	if best == nil {
		return nil, fmt.Errorf("%w: no version in channel %q", ErrNoMatchingPromptPack, track)
	}
	return best, nil
}

// Select picks one PromptPack from candidates (all sharing a packName) by exact
// version pin or by channel-max. When version is non-empty it must match a
// candidate exactly (error if none); otherwise it selects the channel-max for
// the given track, defaulting to the stable channel when track is nil/empty.
func Select(packs []omniav1alpha1.PromptPack, version, track *string) (*omniav1alpha1.PromptPack, error) {
	if version != nil && *version != "" {
		for i := range packs {
			if VersionsEqual(packs[i].Spec.Version, *version) {
				return &packs[i], nil
			}
		}
		return nil, fmt.Errorf("%w: version %q", ErrNoMatchingPromptPack, *version)
	}
	channel := TrackStable
	if track != nil && *track != "" {
		channel = *track
	}
	return ChannelMax(packs, channel)
}

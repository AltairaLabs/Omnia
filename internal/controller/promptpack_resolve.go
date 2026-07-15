package controller

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	omniav1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	"github.com/altairalabs/omnia/internal/promptpack/packselect"
)

const (
	promptPackTrackStable     = packselect.TrackStable
	promptPackTrackPrerelease = packselect.TrackPrerelease
)

// errNoMatchingPromptPack aliases packselect's sentinel so existing errors.Is
// call sites in this package (and its tests) continue to detect the "no such
// PromptPack" outcome that now originates in the shared packselect helpers.
var errNoMatchingPromptPack = packselect.ErrNoMatchingPromptPack

// selectPromptPack picks one PromptPack from candidates (all sharing a packName)
// by exact version pin or by channel-max. At most one of version/track may be set
// (both set is an error); if neither is set, defaults to the stable channel. The
// selection itself is delegated to the shared packselect package.
func selectPromptPack(candidates []omniav1alpha1.PromptPack, version, track *string) (*omniav1alpha1.PromptPack, error) {
	hasVersion := version != nil && *version != ""
	hasTrack := track != nil && *track != ""
	if hasVersion && hasTrack { // both set is an error
		return nil, fmt.Errorf("exactly one of promptPackRef.version or promptPackRef.track must be set")
	}
	return packselect.Select(candidates, version, track)
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
	return packselect.ChannelMax(list.Items, channel)
}

// resolvePromptPack resolves an AgentRuntime's PromptPack reference to a concrete
// PromptPack object: it lists the version-objects labelled with the ref's logical
// packName and selects by exact version or channel (via selectPromptPack).
func (r *AgentRuntimeReconciler) resolvePromptPack(ctx context.Context, namespace string, ref omniav1alpha1.PromptPackRef) (*omniav1alpha1.PromptPack, error) {
	var list omniav1alpha1.PromptPackList
	if err := r.List(ctx, &list,
		client.InNamespace(namespace),
		client.MatchingLabels{LabelPromptPackName: ref.Name},
	); err != nil {
		return nil, fmt.Errorf("failed to list PromptPacks for %q: %w", ref.Name, err)
	}
	promptPack, err := selectPromptPack(list.Items, ref.Version, ref.Track)
	if err != nil {
		return nil, fmt.Errorf("resolve PromptPack %q: %w", ref.Name, err)
	}
	return promptPack, nil
}

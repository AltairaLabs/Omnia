/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1alpha1 "github.com/altairalabs/omnia/api/v1alpha1"
	omniav1alpha1 "github.com/altairalabs/omnia/ee/api/v1alpha1"
)

var _ = Describe("PromptPackSourceGC", func() {
	ctx := context.Background()
	const ns = "default"

	// gcSource builds an in-memory PromptPackSource. It never touches the
	// cluster — gcOldVersions only reads Namespace, PackName, and HistoryLimit.
	gcSource := func(packName string) *omniav1alpha1.PromptPackSource {
		return &omniav1alpha1.PromptPackSource{
			ObjectMeta: metav1.ObjectMeta{Name: "src-" + packName, Namespace: ns},
			Spec: omniav1alpha1.PromptPackSourceSpec{
				Type:     omniav1alpha1.PromptPackSourceTypeGit,
				PackName: packName,
				Interval: "5m",
			},
		}
	}

	gcReconciler := func(maxVersions int, minAge time.Duration) *PromptPackSourceReconciler {
		return &PromptPackSourceReconciler{
			Client:               k8sClient,
			Scheme:               k8sClient.Scheme(),
			MaxVersionsPerSource: maxVersions,
			MinRetentionAge:      minAge,
		}
	}

	objName := func(packName, version string) string {
		return corev1alpha1.PromptPackObjectName(packName, version)
	}

	It("clamps a negative historyLimit to zero (no panic)", func() {
		r := gcReconciler(5, 0)
		src := gcSource("clamp-pack")
		src.Spec.HistoryLimit = ptr.To(int32(-1))
		Expect(r.historyLimit(src)).To(Equal(0))
		// gcOldVersions must not panic on a negative limit even with candidates present.
		Expect(r.gcOldVersions(ctx, src)).To(Succeed())
	})

	// seedPack creates a PromptPack version-object with the resolution label and
	// the given phase (set via a status update, since spec is immutable after
	// create), plus its backing -content ConfigMap.
	seedPack := func(packName, version string, phase corev1alpha1.PromptPackPhase) {
		name := objName(packName, version)
		pp := &corev1alpha1.PromptPack{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    map[string]string{labelPromptPackName: packName},
			},
			Spec: corev1alpha1.PromptPackSpec{
				PackName: packName,
				Version:  version,
				Source: corev1alpha1.PromptPackContentSource{
					Type:         corev1alpha1.PromptPackSourceTypeConfigMap,
					ConfigMapRef: &corev1.LocalObjectReference{Name: name + contentSuffix},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pp)).To(Succeed())
		pp.Status.Phase = phase
		Expect(k8sClient.Status().Update(ctx, pp)).To(Succeed())

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name + contentSuffix,
				Namespace: ns,
				Labels: map[string]string{
					labelPromptPackManagedBy: managedByPromptPack,
					labelPromptPackName:      packName,
				},
			},
			Data: map[string]string{"pack.json": "{}"},
		}
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
	}

	// setSupersededCondition stamps a Superseded status condition with a specific
	// transition time, so the min-age guard (which measures from supersession, not
	// creation) can be exercised deterministically — LastTransitionTime is
	// settable via a status update, unlike CreationTimestamp.
	setSupersededCondition := func(packName, version string, at time.Time) {
		name := objName(packName, version)
		pp := &corev1alpha1.PromptPack{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, pp)).To(Succeed())
		pp.Status.Conditions = []metav1.Condition{{
			Type:               supersededConditionType,
			Status:             metav1.ConditionTrue,
			Reason:             "NewerVersionPublished",
			Message:            "superseded",
			LastTransitionTime: metav1.NewTime(at),
		}}
		Expect(k8sClient.Status().Update(ctx, pp)).To(Succeed())
	}

	exists := func(kind client.Object, name string) bool {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, kind)
		return err == nil
	}
	gone := func(kind client.Object, name string) bool {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: ns}, kind)
		return apierrors.IsNotFound(err)
	}

	cleanup := func(packName string) {
		Expect(k8sClient.DeleteAllOf(ctx, &corev1alpha1.AgentRuntime{}, client.InNamespace(ns))).To(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &corev1alpha1.PromptPack{},
			client.InNamespace(ns), client.MatchingLabels{labelPromptPackName: packName})).To(Succeed())
		Expect(k8sClient.DeleteAllOf(ctx, &corev1.ConfigMap{},
			client.InNamespace(ns), client.MatchingLabels{labelPromptPackName: packName})).To(Succeed())
	}

	Context("When Superseded versions exceed the history limit", func() {
		const packName = "gcpack"
		AfterEach(func() { cleanup(packName) })

		It("keeps the newest and deletes the oldest beyond the limit, with their content ConfigMaps", func() {
			for _, v := range []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0"} {
				seedPack(packName, v, corev1alpha1.PromptPackPhaseSuperseded)
			}

			r := gcReconciler(2, 0)
			Expect(r.gcOldVersions(ctx, gcSource(packName))).To(Succeed())

			By("keeping the two newest")
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.3.0"))).To(BeTrue())
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.2.0"))).To(BeTrue())

			By("deleting the two oldest and their content ConfigMaps")
			Expect(gone(&corev1alpha1.PromptPack{}, objName(packName, "1.1.0"))).To(BeTrue())
			Expect(gone(&corev1alpha1.PromptPack{}, objName(packName, "1.0.0"))).To(BeTrue())
			Expect(gone(&corev1.ConfigMap{}, objName(packName, "1.1.0")+contentSuffix)).To(BeTrue())
			Expect(gone(&corev1.ConfigMap{}, objName(packName, "1.0.0")+contentSuffix)).To(BeTrue())
		})
	})

	Context("When a version beyond the limit is referenced by an AgentRuntime", func() {
		const packName = "refpack"
		AfterEach(func() { cleanup(packName) })

		It("retains the referenced version and only deletes the unreferenced overflow", func() {
			for _, v := range []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0"} {
				seedPack(packName, v, corev1alpha1.PromptPackPhaseSuperseded)
			}

			port := int32(8080)
			facades := []corev1alpha1.FacadeConfig{{Type: corev1alpha1.FacadeType("websocket"), Port: &port}}

			// AR pinning 1.0.0 via a rollout candidate ref (covers the candidate path).
			pinned := &corev1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "ar-pin", Namespace: ns},
				Spec: corev1alpha1.AgentRuntimeSpec{
					PromptPackRef: corev1alpha1.PromptPackRef{Name: packName},
					Facades:       facades,
					Rollout: &corev1alpha1.RolloutConfig{
						Steps:     []corev1alpha1.RolloutStep{{SetWeight: ptr.To(int32(10))}},
						Candidate: &corev1alpha1.CandidateOverrides{PromptPackRef: &corev1alpha1.PromptPackRef{Name: packName, Version: ptr.To("1.0.0")}},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pinned)).To(Succeed())

			// AR that follows the stable track (covers the track-only path).
			tracked := &corev1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "ar-track", Namespace: ns},
				Spec: corev1alpha1.AgentRuntimeSpec{
					PromptPackRef: corev1alpha1.PromptPackRef{Name: packName, Track: ptr.To("stable")},
					Facades:       facades,
				},
			}
			Expect(k8sClient.Create(ctx, tracked)).To(Succeed())

			r := gcReconciler(2, 0)
			Expect(r.gcOldVersions(ctx, gcSource(packName))).To(Succeed())

			By("retaining the pinned 1.0.0 even though it is beyond the limit")
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.0.0"))).To(BeTrue())
			By("deleting only the unreferenced overflow 1.1.0")
			Expect(gone(&corev1alpha1.PromptPack{}, objName(packName, "1.1.0"))).To(BeTrue())
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.2.0"))).To(BeTrue())
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.3.0"))).To(BeTrue())
		})
	})

	Context("When candidates are younger than the minimum retention age", func() {
		const packName = "agepack"
		AfterEach(func() { cleanup(packName) })

		It("deletes nothing regardless of the history limit", func() {
			for _, v := range []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0"} {
				seedPack(packName, v, corev1alpha1.PromptPackPhaseSuperseded)
			}

			// All seeded objects are fresh, so a 1h min-age protects every one.
			r := gcReconciler(2, time.Hour)
			Expect(r.gcOldVersions(ctx, gcSource(packName))).To(Succeed())

			for _, v := range []string{"1.0.0", "1.1.0", "1.2.0", "1.3.0"} {
				Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, v))).To(BeTrue())
			}
		})
	})

	Context("When an old version is not Superseded", func() {
		const packName = "activepack"
		AfterEach(func() { cleanup(packName) })

		It("never GCs a non-Superseded version", func() {
			seedPack(packName, "1.0.0", corev1alpha1.PromptPackPhaseActive)
			seedPack(packName, "1.1.0", corev1alpha1.PromptPackPhaseSuperseded)
			seedPack(packName, "1.2.0", corev1alpha1.PromptPackPhaseSuperseded)
			seedPack(packName, "1.3.0", corev1alpha1.PromptPackPhaseSuperseded)

			r := gcReconciler(2, 0)
			Expect(r.gcOldVersions(ctx, gcSource(packName))).To(Succeed())

			By("keeping the Active version even though it is the oldest")
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.0.0"))).To(BeTrue())
			By("GCing only the Superseded overflow")
			Expect(gone(&corev1alpha1.PromptPack{}, objName(packName, "1.1.0"))).To(BeTrue())
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.2.0"))).To(BeTrue())
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.3.0"))).To(BeTrue())
		})
	})

	Context("When the min-age is measured from supersession time", func() {
		const packName = "supage"
		AfterEach(func() { cleanup(packName) })

		It("protects a recently-superseded version and GCs a long-ago-superseded one", func() {
			for _, v := range []string{"1.0.0", "1.1.0", "1.2.0"} {
				seedPack(packName, v, corev1alpha1.PromptPackPhaseSuperseded)
			}
			now := time.Now()
			// limit 1 keeps 1.2.0; candidates beyond the window are 1.1.0 and 1.0.0.
			// Both are freshly created, so a CreationTimestamp-based guard would treat
			// them identically — only the supersession condition distinguishes them.
			setSupersededCondition(packName, "1.1.0", now)                   // just superseded -> protected
			setSupersededCondition(packName, "1.0.0", now.Add(-2*time.Hour)) // superseded long ago -> eligible

			r := gcReconciler(1, time.Hour)
			Expect(r.gcOldVersions(ctx, gcSource(packName))).To(Succeed())

			By("keeping the newest, in the keep window")
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.2.0"))).To(BeTrue())
			By("protecting the recently-superseded 1.1.0 despite being beyond the limit")
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.1.0"))).To(BeTrue())
			By("GCing the long-ago-superseded 1.0.0")
			Expect(gone(&corev1alpha1.PromptPack{}, objName(packName, "1.0.0"))).To(BeTrue())
		})
	})

	Context("When a track-only ref must protect the channel-max, not the pure-highest", func() {
		const packName = "chpack"
		AfterEach(func() { cleanup(packName) })

		It("protects the stable-channel-max for a stable track even when a higher prerelease exists", func() {
			// Pure-highest semver is the prerelease 2.0.0-beta.1; the stable-channel-max
			// (what a stable track resolves to) is 1.1.0.
			seedPack(packName, "1.0.0", corev1alpha1.PromptPackPhaseSuperseded)
			seedPack(packName, "1.1.0", corev1alpha1.PromptPackPhaseSuperseded)
			seedPack(packName, "2.0.0-beta.1", corev1alpha1.PromptPackPhaseSuperseded)

			port := int32(8080)
			facades := []corev1alpha1.FacadeConfig{{Type: corev1alpha1.FacadeType("websocket"), Port: &port}}
			tracked := &corev1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "ar-stable", Namespace: ns},
				Spec: corev1alpha1.AgentRuntimeSpec{
					PromptPackRef: corev1alpha1.PromptPackRef{Name: packName, Track: ptr.To("stable")},
					Facades:       facades,
				},
			}
			Expect(k8sClient.Create(ctx, tracked)).To(Succeed())

			// limit 1 keeps the semver-highest 2.0.0-beta.1; candidates are 1.1.0 and 1.0.0.
			r := gcReconciler(1, 0)
			Expect(r.gcOldVersions(ctx, gcSource(packName))).To(Succeed())

			By("keeping the semver-highest prerelease, in the keep window")
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "2.0.0-beta.1"))).To(BeTrue())
			By("protecting the stable-channel-max 1.1.0 the stable track resolves to")
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.1.0"))).To(BeTrue())
			By("GCing the unreferenced older stable 1.0.0")
			Expect(gone(&corev1alpha1.PromptPack{}, objName(packName, "1.0.0"))).To(BeTrue())
		})
	})

	Context("When a bare ref (no version, no track) protects the implicit stable channel-max", func() {
		const packName = "barepack"
		AfterEach(func() { cleanup(packName) })

		It("protects the stable-channel-max for a bare {name}-only ref that defaults to stable", func() {
			seedPack(packName, "1.0.0", corev1alpha1.PromptPackPhaseSuperseded)
			seedPack(packName, "1.1.0", corev1alpha1.PromptPackPhaseSuperseded)
			seedPack(packName, "2.0.0-beta.1", corev1alpha1.PromptPackPhaseSuperseded)

			port := int32(8080)
			facades := []corev1alpha1.FacadeConfig{{Type: corev1alpha1.FacadeType("websocket"), Port: &port}}
			// A bare ref: neither Version nor Track — the resolver treats this as the
			// stable channel, so GC must protect the stable-channel-max (1.1.0).
			bare := &corev1alpha1.AgentRuntime{
				ObjectMeta: metav1.ObjectMeta{Name: "ar-bare", Namespace: ns},
				Spec: corev1alpha1.AgentRuntimeSpec{
					PromptPackRef: corev1alpha1.PromptPackRef{Name: packName},
					Facades:       facades,
				},
			}
			Expect(k8sClient.Create(ctx, bare)).To(Succeed())

			r := gcReconciler(1, 0)
			Expect(r.gcOldVersions(ctx, gcSource(packName))).To(Succeed())

			By("protecting the stable-channel-max 1.1.0 the bare ref resolves to")
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "1.1.0"))).To(BeTrue())
			By("keeping the semver-highest prerelease, in the keep window")
			Expect(exists(&corev1alpha1.PromptPack{}, objName(packName, "2.0.0-beta.1"))).To(BeTrue())
			By("GCing the unreferenced older stable 1.0.0")
			Expect(gone(&corev1alpha1.PromptPack{}, objName(packName, "1.0.0"))).To(BeTrue())
		})
	})
})

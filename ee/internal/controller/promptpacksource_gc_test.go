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
})

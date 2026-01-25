/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.


*/

// Package selector provides utilities for selecting Kubernetes resources
// using label selectors. It wraps the standard k8s label selector APIs
// to provide a simpler interface for common use cases.
package selector

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FromLabelSelector converts a metav1.LabelSelector to a labels.Selector.
// This supports both matchLabels and matchExpressions.
// Returns labels.Everything() if the selector is nil or empty.
func FromLabelSelector(ls *metav1.LabelSelector) (labels.Selector, error) {
	if ls == nil {
		return labels.Everything(), nil
	}
	return metav1.LabelSelectorAsSelector(ls)
}

// MustFromLabelSelector is like FromLabelSelector but panics on error.
// Use this only when you know the selector is valid (e.g., after CRD validation).
func MustFromLabelSelector(ls *metav1.LabelSelector) labels.Selector {
	sel, err := FromLabelSelector(ls)
	if err != nil {
		panic(fmt.Sprintf("invalid label selector: %v", err))
	}
	return sel
}

// Matches returns true if the given labels match the selector.
func Matches(ls *metav1.LabelSelector, resourceLabels map[string]string) (bool, error) {
	sel, err := FromLabelSelector(ls)
	if err != nil {
		return false, err
	}
	return sel.Matches(labels.Set(resourceLabels)), nil
}

// ListOptions returns client.ListOptions configured with the label selector.
// This is useful for filtering List operations.
func ListOptions(ls *metav1.LabelSelector, namespace string) ([]client.ListOption, error) {
	sel, err := FromLabelSelector(ls)
	if err != nil {
		return nil, err
	}

	opts := []client.ListOption{
		client.MatchingLabelsSelector{Selector: sel},
	}

	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}

	return opts, nil
}

// ListMatching lists resources of type T that match the label selector.
// This is a generic helper that handles selector conversion and list options.
func ListMatching[T any, L client.ObjectList](
	ctx context.Context,
	c client.Client,
	list L,
	ls *metav1.LabelSelector,
	namespace string,
) error {
	opts, err := ListOptions(ls, namespace)
	if err != nil {
		return fmt.Errorf("invalid label selector: %w", err)
	}
	return c.List(ctx, list, opts...)
}

// FilterBySelector filters a slice of objects, returning only those whose
// labels match the selector. The labelFunc extracts labels from each object.
func FilterBySelector[T any](
	items []T,
	ls *metav1.LabelSelector,
	labelFunc func(T) map[string]string,
) ([]T, error) {
	sel, err := FromLabelSelector(ls)
	if err != nil {
		return nil, err
	}

	var result []T
	for _, item := range items {
		if sel.Matches(labels.Set(labelFunc(item))) {
			result = append(result, item)
		}
	}
	return result, nil
}

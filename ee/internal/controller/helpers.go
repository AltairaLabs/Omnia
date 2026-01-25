/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

package controller

// ptr is a helper function for creating pointers to values.
func ptr[T any](v T) *T {
	return &v
}

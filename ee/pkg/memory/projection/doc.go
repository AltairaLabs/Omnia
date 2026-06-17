/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: FSL-1.1-Apache-2.0
This file is part of Omnia Enterprise and is subject to the
Functional Source License. See ee/LICENSE for details.
*/

// Package projection turns a set of memory entities into a stable 2D layout
// for the Memory Galaxy view. It is pure: no DB or HTTP. The pipeline is
// basis-selection -> vectorize -> reduce -> t-SNE -> Procrustes-align to the
// previous layout -> normalize.
package projection

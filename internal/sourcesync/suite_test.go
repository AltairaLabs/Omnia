/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0
*/

package sourcesync

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSourcesyncSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Sourcesync Suite")
}

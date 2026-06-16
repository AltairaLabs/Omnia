/*
Copyright 2026 Altaira Labs.

SPDX-License-Identifier: Apache-2.0

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package netutil

import "testing"

func TestIsClusterInternalHost(t *testing.T) {
	internal := []string{
		"localhost",
		"127.0.0.1",
		"::1",
		"10.0.1.5",
		"172.16.4.4",
		"192.168.1.10",
		"omnia-dashboard",                       // same-namespace short name
		"omnia-dashboard.omnia.svc",             // .svc suffix
		"rag-hero.omnia-demo.svc.cluster.local", // full svc DNS
		"omnia-dashboard.omnia.svc.cluster.local.", // trailing dot
		"some-host.cluster.local",                  // cluster domain
		"printer.local",                            // mDNS/local
	}
	for _, h := range internal {
		if !IsClusterInternalHost(h) {
			t.Errorf("expected %q to be cluster-internal", h)
		}
	}

	external := []string{
		"",
		"dashboard.example",
		"dashboard.example.com",
		"rag-hero.omnia-demo.example.com",
		"omnia-dashboard.omnia", // two-label <svc>.<ns> is ambiguous vs a public domain; not auto-trusted
		"anything.svc.evil.com", // ".svc." substring must NOT match — credential-exfil bypass
		"8.8.8.8",
		"203.0.113.10",
	}
	for _, h := range external {
		if IsClusterInternalHost(h) {
			t.Errorf("expected %q to NOT be cluster-internal", h)
		}
	}
}

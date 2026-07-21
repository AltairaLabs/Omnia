/*
Copyright 2026.

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

package contract

import (
	"os"
	"regexp"
	"testing"
)

// protoPath is relative to this package directory.
const protoPath = "../../../api/proto/runtime/v1/runtime.proto"

var (
	semverRE       = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	protoVersionRE = regexp.MustCompile(`(?m)^// Contract-Version: (\S+)\s*$`)
)

func TestVersionIsSemver(t *testing.T) {
	if !semverRE.MatchString(Version) {
		t.Fatalf("Version = %q, want MAJOR.MINOR.PATCH", Version)
	}
}

// TestVersionMatchesProto is the drift guard: the Go constant and the proto
// marker must never disagree. Copy-without-pinning is what let the community
// LangChain runtime fall six months behind the contract undetected.
func TestVersionMatchesProto(t *testing.T) {
	data, err := os.ReadFile(protoPath)
	if err != nil {
		t.Fatalf("read %s: %v", protoPath, err)
	}
	m := protoVersionRE.FindSubmatch(data)
	if m == nil {
		t.Fatalf("no `// Contract-Version: <semver>` marker found in %s", protoPath)
	}
	if got := string(m[1]); got != Version {
		t.Fatalf("proto Contract-Version = %q but contract.Version = %q; update both together", got, Version)
	}
}

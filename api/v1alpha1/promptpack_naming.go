package v1alpha1

import (
	"crypto/sha256"
	"encoding/hex"
)

// PromptPackObjectName returns the deterministic metadata.name for the PromptPack
// object identified by {packName, version}. The same coordinate always yields the
// same name, so a duplicate coordinate is rejected natively by the apiserver with
// AlreadyExists. This algorithm is a cross-language contract: the deploy API mirrors
// it in TypeScript as "pp-" + sha256hex(packName + "@" + version).slice(0,12).
func PromptPackObjectName(packName, version string) string {
	sum := sha256.Sum256([]byte(packName + "@" + version))
	return "pp-" + hex.EncodeToString(sum[:])[:12]
}

package store

import "testing"

func TestGroupOpsPreparationLockKeyIsStableAndSourceScoped(t *testing.T) {
	const source = "sha256:7777777777777777777777777777777777777777777777777777777777777777"
	const scope = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	first := groupOpsPreparationLockKey("image", 7, source, scope, "image")
	if first != groupOpsPreparationLockKey("image", 7, source, scope, "image") {
		t.Fatal("lock key is not stable")
	}
	if first == groupOpsPreparationLockKey("image", 8, source, scope, "image") || first == groupOpsPreparationLockKey("attachment", 7, source, scope, "file") {
		t.Fatal("distinct source identities share a generation lock")
	}
}

func TestGroupOpsDigestRejectsNonCanonicalHex(t *testing.T) {
	if !groupOpsDigest("sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") || groupOpsDigest("sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA") || groupOpsDigest("sha256:gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg") {
		t.Fatal("digest validation accepted a non-canonical value")
	}
}

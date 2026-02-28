package service

import (
	"testing"
)

func TestBuildCacheKey_SameInputs_SameKey(t *testing.T) {
	k1 := buildCacheKey(commitTmplHash, "model", "diff", "feat", "short", "false")
	k2 := buildCacheKey(commitTmplHash, "model", "diff", "feat", "short", "false")
	if k1 != k2 {
		t.Errorf("identical inputs produced different keys:\n  %q\n  %q", k1, k2)
	}
}

func TestBuildCacheKey_DifferentContent_DifferentKey(t *testing.T) {
	k1 := buildCacheKey(commitTmplHash, "model", "diff A", "feat", "short", "false")
	k2 := buildCacheKey(commitTmplHash, "model", "diff B", "feat", "short", "false")
	if k1 == k2 {
		t.Error("different content must produce different keys")
	}
}

func TestBuildCacheKey_DifferentModel_DifferentKey(t *testing.T) {
	k1 := buildCacheKey(commitTmplHash, "llama3.2:3b", "diff", "", "short", "false")
	k2 := buildCacheKey(commitTmplHash, "mistral:latest", "diff", "", "short", "false")
	if k1 == k2 {
		t.Error("different models must produce different keys")
	}
}

func TestBuildCacheKey_DifferentMode_DifferentKey(t *testing.T) {
	k1 := buildCacheKey(commitTmplHash, "model", "diff", "", "short", "false")
	k2 := buildCacheKey(commitTmplHash, "model", "diff", "", "detailed", "false")
	if k1 == k2 {
		t.Error("different modes must produce different keys")
	}
}

func TestBuildCacheKey_DifferentType_DifferentKey(t *testing.T) {
	k1 := buildCacheKey(commitTmplHash, "model", "diff", "feat", "short", "false")
	k2 := buildCacheKey(commitTmplHash, "model", "diff", "fix", "short", "false")
	if k1 == k2 {
		t.Error("different types must produce different keys")
	}
}

func TestBuildCacheKey_DifferentExplain_DifferentKey(t *testing.T) {
	k1 := buildCacheKey(commitTmplHash, "model", "diff", "", "short", "false")
	k2 := buildCacheKey(commitTmplHash, "model", "diff", "", "short", "true")
	if k1 == k2 {
		t.Error("different explain flags must produce different keys")
	}
}

func TestBuildCacheKey_DifferentTemplate_DifferentKey(t *testing.T) {
	// commitTmplHash vs branchTmplHash — edit commit.tmpl → commit keys change,
	// branch keys are unaffected.
	k1 := buildCacheKey(commitTmplHash, "model", "content", "", "short", "false")
	k2 := buildCacheKey(branchTmplHash, "model", "content", "", "short", "false")
	if k1 == k2 {
		t.Error("different template hashes must produce different keys")
	}
}

func TestBuildCacheKey_NoCollision_AdjacentFieldConcat(t *testing.T) {
	// Without length-prefixing, "feat"+"fix" == "fe"+"atfix".
	// Length-prefixed encoding prevents this.
	k1 := buildCacheKey(commitTmplHash, "model", "diff", "feat", "fix", "false")
	k2 := buildCacheKey(commitTmplHash, "model", "diff", "fe", "atfix", "false")
	if k1 == k2 {
		t.Error("adjacent field concatenation must not cause key collision")
	}
}

func TestPrCacheKey_DifferentBinaryHash_DifferentKey(t *testing.T) {
	commits := []string{"feat: add login", "fix: typo"}
	k1 := prCacheKey("model", "hash-v1", commits, "feat", "short", false)
	k2 := prCacheKey("model", "hash-v2", commits, "feat", "short", false)
	if k1 == k2 {
		t.Error("prCacheKey: different binaryHash must produce different keys — stale PR cache bug")
	}
}

func TestPrCacheKey_SameBinaryHash_SameKey(t *testing.T) {
	commits := []string{"feat: add login"}
	k1 := prCacheKey("model", "abc123", commits, "", "short", false)
	k2 := prCacheKey("model", "abc123", commits, "", "short", false)
	if k1 != k2 {
		t.Error("prCacheKey: identical inputs must produce the same key")
	}
}

// Symmetry check: commit, branch, and pr cache-key helpers all incorporate
// binaryHash so a binary upgrade invalidates all three command caches.
func TestCacheKey_AllVariants_IncludeBinaryHash(t *testing.T) {
	commits := []string{"feat: thing"}
	old := "old-binary"
	neu := "new-binary"

	if commitCacheKey("m", old, "diff", "", "short", false) ==
		commitCacheKey("m", neu, "diff", "", "short", false) {
		t.Error("commitCacheKey must differ on binaryHash change")
	}
	if branchCacheKey("m", old, "task", "", "short", false) ==
		branchCacheKey("m", neu, "task", "", "short", false) {
		t.Error("branchCacheKey must differ on binaryHash change")
	}
	if prCacheKey("m", old, commits, "", "short", false) ==
		prCacheKey("m", neu, commits, "", "short", false) {
		t.Error("prCacheKey must differ on binaryHash change")
	}
	key := buildCacheKey(commitTmplHash, "model", "diff", "", "short", "false")
	if len(key) != 64 {
		t.Errorf("expected 64-char hex SHA256, got len=%d: %q", len(key), key)
	}
	for _, ch := range key {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			t.Errorf("non-hex character %q in key", ch)
		}
	}
}

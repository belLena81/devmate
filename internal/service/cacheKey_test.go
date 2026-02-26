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

func TestBuildCacheKey_IsHexSHA256(t *testing.T) {
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

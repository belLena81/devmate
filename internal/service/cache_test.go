package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestCache(t *testing.T) *DiskCache {
	t.Helper()
	return NewDiskCache(t.TempDir())
}

func TestDiskCache_MissReturnsNotFound(t *testing.T) {
	_, ok := newTestCache(t).Get("nonexistent")
	if ok {
		t.Error("expected miss for unknown key")
	}
}

func TestDiskCache_SetThenGet_ReturnsValue(t *testing.T) {
	c := newTestCache(t)
	if err := c.Set("k", "the response"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	val, ok := c.Get("k")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if val != "the response" {
		t.Errorf("expected %q, got %q", "the response", val)
	}
}

func TestDiskCache_SetOverwrites(t *testing.T) {
	c := newTestCache(t)
	c.Set("k", "first")
	c.Set("k", "second")
	val, _ := c.Get("k")
	if val != "second" {
		t.Errorf("expected %q after overwrite, got %q", "second", val)
	}
}

func TestDiskCache_TwoKeys_Independent(t *testing.T) {
	c := newTestCache(t)
	c.Set("A", "alpha")
	c.Set("B", "beta")
	a, _ := c.Get("A")
	b, _ := c.Get("B")
	if a != "alpha" || b != "beta" {
		t.Errorf("cache entries crossed: A=%q B=%q", a, b)
	}
}

func TestDiskCache_Clear_RemovesAllEntries(t *testing.T) {
	c := newTestCache(t)
	c.Set("k1", "v1")
	c.Set("k2", "v2")
	if err := c.Clear(); err != nil {
		t.Fatalf("Clear error: %v", err)
	}
	if _, ok := c.Get("k1"); ok {
		t.Error("k1 should be gone after Clear")
	}
	if _, ok := c.Get("k2"); ok {
		t.Error("k2 should be gone after Clear")
	}
}

func TestDiskCache_Clear_EmptyCacheIsNoop(t *testing.T) {
	if err := newTestCache(t).Clear(); err != nil {
		t.Errorf("Clear on empty cache should not error: %v", err)
	}
}

func TestDiskCache_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	NewDiskCache(dir).Set("key", "hello")

	// Second instance at the same dir — simulates next process run.
	val, ok := NewDiskCache(dir).Get("key")
	if !ok {
		t.Fatal("expected hit from second instance")
	}
	if val != "hello" {
		t.Errorf("expected %q, got %q", "hello", val)
	}
}

func TestDiskCache_CreatesDirectoryIfMissing(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "cache")
	if err := NewDiskCache(dir).Set("k", "v"); err != nil {
		t.Fatalf("Set should create missing dir: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("cache dir should exist after Set: %v", err)
	}
}

// ─── validCacheKey ────────────────────────────────────────────────────────────

func TestDiskCache_EmptyKey_SetReturnsError(t *testing.T) {
	c := newTestCache(t)
	if err := c.Set("", "v"); err == nil {
		t.Error("expected error for empty key")
	}
}

func TestDiskCache_EmptyKey_GetReturnsMiss(t *testing.T) {
	c := newTestCache(t)
	if _, ok := c.Get(""); ok {
		t.Error("expected miss for empty key")
	}
}

func TestDiskCache_PathSeparator_SetReturnsError(t *testing.T) {
	c := newTestCache(t)
	for _, bad := range []string{"a/b", "a\\b", "../etc/passwd", "sub/dir/key"} {
		if err := c.Set(bad, "v"); err == nil {
			t.Errorf("expected error for key %q containing path separator", bad)
		}
	}
}

func TestDiskCache_DotKeys_SetReturnsError(t *testing.T) {
	c := newTestCache(t)
	for _, bad := range []string{".", ".."} {
		if err := c.Set(bad, "v"); err == nil {
			t.Errorf("expected error for reserved key %q", bad)
		}
	}
}

func TestDiskCache_NullByte_SetReturnsError(t *testing.T) {
	c := newTestCache(t)
	if err := c.Set("key\x00suffix", "v"); err == nil {
		t.Error("expected error for key containing null byte")
	}
}

func TestDiskCache_ValidHexKey_RoundTrips(t *testing.T) {
	// SHA-256 hex keys — the real-world format produced by buildCacheKey —
	// must be accepted without error.
	hexKey := "a3f1c9e2b7d084561f2a3c4e5d6b7890abcdef1234567890abcdef1234567890"
	c := newTestCache(t)
	if err := c.Set(hexKey, "value"); err != nil {
		t.Fatalf("unexpected error for valid hex key: %v", err)
	}
	val, ok := c.Get(hexKey)
	if !ok {
		t.Fatal("expected hit for valid hex key")
	}
	if val != "value" {
		t.Errorf("expected %q, got %q", "value", val)
	}
}

// ─── Stat ─────────────────────────────────────────────────────────────────────

func TestDiskCache_Stat_EmptyCache_ReturnsEmptySlice(t *testing.T) {
	entries, err := newTestCache(t).Stat()
	if err != nil {
		t.Fatalf("Stat on empty cache returned error: %v", err)
	}
	if entries == nil {
		t.Fatal("Stat must return a non-nil slice, even when empty")
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestDiskCache_Stat_NonExistentDir_ReturnsEmptySlice(t *testing.T) {
	// Cache dir that was never written to — Stat must not error.
	c := NewDiskCache(filepath.Join(t.TempDir(), "does-not-exist"))
	entries, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat with missing dir returned error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for missing dir, got %d", len(entries))
	}
}

func TestDiskCache_Stat_ReturnsOneEntryPerKey(t *testing.T) {
	c := newTestCache(t)
	c.Set("key1", "value one")
	c.Set("key2", "value two")
	c.Set("key3", "value three")

	entries, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestDiskCache_Stat_EntryHasCorrectKey(t *testing.T) {
	c := newTestCache(t)
	c.Set("mykey", "some content")

	entries, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Key != "mykey" {
		t.Errorf("expected key %q, got %q", "mykey", entries[0].Key)
	}
}

func TestDiskCache_Stat_EntryHasCorrectSize(t *testing.T) {
	c := newTestCache(t)
	content := "hello world"
	c.Set("k", content)

	entries, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if entries[0].SizeBytes != int64(len(content)) {
		t.Errorf("expected SizeBytes %d, got %d", len(content), entries[0].SizeBytes)
	}
}

func TestDiskCache_Stat_EntryHasNonZeroModTime(t *testing.T) {
	c := newTestCache(t)
	before := time.Now().Add(-time.Second)
	c.Set("k", "v")
	after := time.Now().Add(time.Second)

	entries, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	mt := entries[0].ModTime
	if mt.Before(before) || mt.After(after) {
		t.Errorf("ModTime %v not in expected range [%v, %v]", mt, before, after)
	}
}

func TestDiskCache_Stat_SortedNewestFirst(t *testing.T) {
	dir := t.TempDir()
	c := NewDiskCache(dir)

	now := time.Now()
	for i, key := range []string{"oldest", "middle", "newest"} {
		c.Set(key, "v")
		// Touch the file with an explicit mod-time so the sort is deterministic.
		path := filepath.Join(dir, key)
		mt := now.Add(time.Duration(i) * time.Second)
		os.Chtimes(path, mt, mt)
	}

	entries, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	if entries[0].Key != "newest" {
		t.Errorf("expected newest first, got %q", entries[0].Key)
	}
	if entries[2].Key != "oldest" {
		t.Errorf("expected oldest last, got %q", entries[2].Key)
	}
}

func TestDiskCache_Stat_AfterClear_ReturnsEmpty(t *testing.T) {
	c := newTestCache(t)
	c.Set("a", "1")
	c.Set("b", "2")
	c.Clear()

	entries, err := c.Stat()
	if err != nil {
		t.Fatalf("Stat error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries after Clear, got %d", len(entries))
	}
}

func TestNoopCache_Stat_ReturnsEmptySlice(t *testing.T) {
	entries, err := NoopCache{}.Stat()
	if err != nil {
		t.Fatalf("NoopCache.Stat returned error: %v", err)
	}
	if entries == nil {
		t.Fatal("NoopCache.Stat must return non-nil slice")
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries from NoopCache.Stat, got %d", len(entries))
	}
}
